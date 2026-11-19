package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/castrowithcee/qatlas-kit/approval"
	"github.com/castrowithcee/qatlas-kit/configfile"

	"github.com/castrowithcee/qatlas-host/internal/backup"
	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/gitrepo"
	"github.com/castrowithcee/qatlas-host/internal/notifier"
	"github.com/castrowithcee/qatlas-host/internal/stacks"
	"github.com/castrowithcee/qatlas-host/internal/window"
)

const (
	proposalsDir = "proposals"
	// proposalLabel separates the fingerprints of proposals from every other hash of the same bytes.
	proposalLabel = "qhost/proposal/v1"
	// maxStoredProposals bounds the files of the store, so a stream of submissions and withdrawals cannot fill
	// the disk. Closed proposals are dropped after proposalKeep.
	maxStoredProposals = 500
	proposalKeep       = 2 * control.ProposalLifetime
)

var proposalIDRe = regexp.MustCompile(`^p-[0-9a-f]{16}$`)

// errNotOpen tells that a proposal can no longer change its state: it is not open or it has expired.
var errNotOpen = errors.New("the proposal is not open")

// proposalRecord is the file of one proposal. Content is the normalized content of its kind, the one the
// fingerprint covers.
type proposalRecord struct {
	ID            string          `json:"id"`
	Kind          string          `json:"kind"`
	Content       json.RawMessage `json:"content"`
	Fingerprint   string          `json:"fingerprint"`
	SubmitterUID  uint32          `json:"submitter_uid"`
	SubmitterName string          `json:"submitter_name"`
	Submitted     time.Time       `json:"submitted"`
	State         string          `json:"state"`
	// Decided is when the approval was accepted or the proposal was rejected; Message is the reason or failure.
	Decided time.Time `json:"decided,omitzero"`
	Message string    `json:"message,omitempty"`
}

// proposalService keeps the proposals below the state directory of the service, one private file each,
// replaced atomically. It never touches what a proposal names.
type proposalService struct {
	dir    string
	passwd string
	now    func() time.Time
	log    *slog.Logger
	// stackKnown tells whether a stack of that name is among those the service deploys; kinds that name a
	// stack are refused without it.
	stackKnown func(name string) bool
	// channelKnown tells whether a notification channel of that name exists, like the check of notify.add.
	channelKnown func(name string) (bool, error)
	mu           sync.Mutex
}

func newProposalService(stateDir, passwd string, now func() time.Time, log *slog.Logger) *proposalService {
	if now == nil {
		now = time.Now
	}
	return &proposalService{dir: filepath.Join(stateDir, proposalsDir), passwd: passwd, now: now, log: log}
}

func registerProposalCommands(r *control.Registry, p *proposalService) error {
	commands := map[string]control.Command{
		control.ProposalSubmitCommand:   proposing(p.handleSubmit),
		control.ProposalWithdrawCommand: proposing(p.handleWithdraw),
		control.ProposalApproveCommand:  writing(p.handleApprove),
		control.ProposalRejectCommand:   writing(p.handleReject),
		control.ProposalResultCommand:   writing(p.handleResult),
		control.ProposalListCommand:     reading(p.handleList),
		control.ProposalShowCommand:     reading(p.handleShow),
	}
	for name, c := range commands {
		if err := r.Register(name, c); err != nil {
			return err
		}
	}
	return nil
}

func invalid(format string, a ...any) *control.Error {
	return control.NewError(control.CodeInvalid, fmt.Sprintf(format, a...))
}

// proposalNormalizers has one function per kind of proposal. Each checks the content of its kind with the
// rules of the hand-typed input and returns it in its canonical form, the one the fingerprint covers. It
// decodes strictly: a field the schema does not name is refused. It never runs, evaluates or expands the
// content, and looks at nothing but the content and, where the kind names a stack, whether the stack exists.
var proposalNormalizers = map[string]func(p *proposalService, content json.RawMessage) (any, *control.Error){
	control.ProposalKindSourceAdd:     (*proposalService).normalizeSourceAdd,
	control.ProposalKindStackSettings: (*proposalService).normalizeStackSettings,
	control.ProposalKindWindow:        (*proposalService).normalizeWindow,
	control.ProposalKindNotifyChannel: (*proposalService).normalizeNotifyChannel,
}

func (p *proposalService) normalizeProposal(kind string, content json.RawMessage) (any, *control.Error) {
	if n, ok := proposalNormalizers[kind]; ok {
		return n(p, content)
	}
	return nil, invalid("unknown proposal kind %q", printable(kind))
}

func (p *proposalService) normalizeSourceAdd(content json.RawMessage) (any, *control.Error) {
	var c control.ProposalSourceAdd
	if err := decodeStrict(content, &c); err != nil {
		return nil, invalid("the content of a %s proposal must be a JSON object with the fields path and url only", control.ProposalKindSourceAdd)
	}
	if err := checkDirShape(c.Path); err != nil {
		return nil, invalid("the path is not valid: %s", err)
	}
	if c.URL != "" {
		if err := gitrepo.ValidateURL(c.URL); err != nil {
			return nil, invalid("the url is not valid: %s", err)
		}
	}
	return c, nil
}

// normalizeStackSettings checks the settings with stacks.ParseSettings, the check of stack.settings.set, and
// stores them with every default written out, as stack.settings.get answers them. The schema has no secret
// member, so a secret field is an unknown field and refuses the proposal; nothing is dropped.
func (p *proposalService) normalizeStackSettings(content json.RawMessage) (any, *control.Error) {
	var c control.ProposalStackSettings
	if err := decodeStrict(content, &c); err != nil {
		return nil, invalid("the content of a %s proposal must be a JSON object with the fields stack and settings only, "+
			"and settings in the schema of 'qhost stack settings'", control.ProposalKindStackSettings)
	}
	if !backup.ValidStack(c.Stack) {
		return nil, invalid("the stack name is not valid")
	}
	data, err := json.Marshal(c.Settings)
	if err != nil {
		return nil, invalid("the settings cannot be encoded")
	}
	rules, err := stacks.ParseSettings(data)
	if err != nil {
		return nil, invalid("%s", printable(err.Error()))
	}
	if p.stackKnown == nil || !p.stackKnown(c.Stack) {
		return nil, control.NewError(control.CodeInvalid, "the stack is not known to the service")
	}
	canonical, err := settingsOf(rules)
	if err != nil {
		return nil, invalid("the settings cannot be encoded")
	}
	return control.ProposalStackSettings{Stack: c.Stack, Settings: canonical}, nil
}

// normalizeWindow checks the parts with the parsers of window.set and stores each in its canonical form. It
// writes nothing. The schema has no secret member, so any other field refuses the proposal.
func (p *proposalService) normalizeWindow(content json.RawMessage) (any, *control.Error) {
	var c control.WindowSetArgs
	if err := decodeStrict(content, &c); err != nil {
		return nil, invalid("the content of a %s proposal must be a JSON object with the fields days, start, duration and zone only", control.ProposalKindWindow)
	}
	if c.Days == nil && c.Start == "" && c.Duration == "" && c.Zone == "" {
		return nil, invalid("name at least one part of the window")
	}
	change, err := windowChange(c)
	if err != nil {
		var ie *window.InvalidError
		if errors.As(err, &ie) {
			return nil, invalid("%s", printable(ie.Text))
		}
		return nil, invalid("the window is not valid")
	}
	var out control.WindowSetArgs
	if change.Days != nil {
		out.Days = change.Days.Names()
	}
	if change.Start != nil {
		out.Start = window.FormatStart(*change.Start)
	}
	if change.Duration != nil {
		out.Duration = window.FormatDuration(*change.Duration)
	}
	if change.Zone != nil {
		out.Zone = *change.Zone
	}
	return out, nil
}

// normalizeNotifyChannel checks the channel with notifier.Validate without secrets, the check of notify.add
// for everything but them, and stores it with the defaults of its type written out. A member that carries a
// secret part is refused by name, before any other check, and every other member the schema does not name is
// refused too. A name that is taken is refused as notify.add refuses it. It reaches no network.
func (p *proposalService) normalizeNotifyChannel(content json.RawMessage) (any, *control.Error) {
	var members map[string]json.RawMessage
	if err := json.Unmarshal(content, &members); err != nil {
		return nil, invalid("the content of a %s proposal must be a JSON object", control.ProposalKindNotifyChannel)
	}
	for _, f := range control.ProposalNotifySecretFields {
		if _, named := members[f]; named {
			return nil, invalid("a proposal cannot carry a secret part of a channel; the person who approves enters it")
		}
	}
	var c control.ProposalNotifyChannel
	if err := decodeStrict(content, &c); err != nil {
		return nil, invalid("the content of a %s proposal must be a JSON object with the fields name, type, events, "+
			"host, port, security, user, from, to, chat_id and base_url only", control.ProposalKindNotifyChannel)
	}
	ch, err := notifier.Validate(notifier.Channel{Name: c.Name, Type: c.Type, Events: c.Events, Host: c.Host, Port: c.Port,
		Security: c.Security, User: c.User, From: c.From, To: c.To, ChatID: c.ChatID, BaseURL: c.BaseURL}, nil)
	if err != nil {
		var ie *notifier.InvalidError
		if errors.As(err, &ie) {
			return nil, invalid("%s", printable(ie.Text))
		}
		return nil, invalid("the channel is not valid")
	}
	if p.channelKnown != nil {
		known, err := p.channelKnown(ch.Name)
		if err != nil {
			p.log.Error("proposals: read the channels", "error", err)
			return nil, control.NewError(control.CodeInternal, "internal error")
		}
		if known {
			return nil, invalid("a channel of this name exists; remove it first")
		}
	}
	return control.ProposalNotifyChannel{Name: ch.Name, Type: ch.Type, Events: ch.Events, Host: ch.Host, Port: ch.Port,
		Security: ch.Security, User: ch.User, From: ch.From, To: ch.To, ChatID: ch.ChatID, BaseURL: ch.BaseURL}, nil
}

func decodeStrict(raw json.RawMessage, into any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		return err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return errors.New("trailing data")
	}
	return nil
}

func (p *proposalService) handleSubmit(ctx context.Context, raw json.RawMessage) (any, error) {
	uid, ok := control.PeerUID(ctx)
	if !ok {
		return nil, control.NewError(control.CodeInternal, "internal error")
	}
	// The size is checked before anything is decoded.
	if len(raw) > control.ProposalMaxBytes {
		return nil, control.NewError(control.CodeTooLarge, "a proposal may hold at most 64 KiB")
	}
	var a control.ProposalSubmitArgs
	if err := decodeArgs(raw, &a); err != nil {
		return nil, err
	}
	content, cerr := p.normalizeProposal(a.Kind, a.Content)
	if cerr != nil {
		return nil, cerr
	}
	data, err := json.Marshal(content)
	if err != nil {
		return nil, err
	}
	fp, err := approval.Fingerprint(proposalLabel, struct {
		Kind    string          `json:"kind"`
		Content json.RawMessage `json:"content"`
	}{a.Kind, data})
	if err != nil {
		return nil, err
	}
	rec := proposalRecord{Kind: a.Kind, Content: data, Fingerprint: fp, SubmitterUID: uid,
		SubmitterName: p.userName(uid), Submitted: p.now().UTC().Truncate(time.Second), State: control.ProposalOpen}

	p.mu.Lock()
	defer p.mu.Unlock()
	all, err := p.load()
	if err != nil {
		return nil, p.internal("read the proposals", err)
	}
	open := 0
	for _, r := range all {
		if r.SubmitterUID == uid && p.stateOf(r) == control.ProposalOpen {
			open++
		}
	}
	if open >= control.ProposalMaxOpen {
		return nil, control.NewError(control.CodeProposalLimit,
			fmt.Sprintf("at most %d proposals may be open per user; withdraw one first", control.ProposalMaxOpen))
	}
	all = p.prune(all)
	if len(all) >= maxStoredProposals {
		return nil, control.NewError(control.CodeProposalLimit, "the store of proposals is full; try again later")
	}
	if rec.ID, err = newProposalID(); err != nil {
		return nil, p.internal("create an id", err)
	}
	if err := p.save(rec); err != nil {
		return nil, p.internal("store the proposal", err)
	}
	return p.info(rec), nil
}

func (p *proposalService) handleList(_ context.Context, raw json.RawMessage) (any, error) {
	if err := decodeArgs(raw, &struct{}{}); err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	all, err := p.load()
	if err != nil {
		return nil, p.internal("read the proposals", err)
	}
	slices.SortFunc(all, func(a, b proposalRecord) int {
		if c := b.Submitted.Compare(a.Submitted); c != 0 {
			return c
		}
		return strings.Compare(b.ID, a.ID)
	})
	res := control.ProposalListResult{Proposals: make([]control.ProposalInfo, 0, len(all))}
	for _, r := range all {
		res.Proposals = append(res.Proposals, p.info(r))
	}
	return res, nil
}

func (p *proposalService) handleShow(_ context.Context, raw json.RawMessage) (any, error) {
	var a control.ProposalIDArgs
	if err := decodeArgs(raw, &a); err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	rec, err := p.find(a.ID)
	if err != nil {
		return nil, err
	}
	return control.ProposalShowResult{ProposalInfo: p.info(rec), Content: rec.Content}, nil
}

func (p *proposalService) handleWithdraw(ctx context.Context, raw json.RawMessage) (any, error) {
	uid, ok := control.PeerUID(ctx)
	if !ok {
		return nil, control.NewError(control.CodeInternal, "internal error")
	}
	var a control.ProposalIDArgs
	if err := decodeArgs(raw, &a); err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	rec, err := p.find(a.ID)
	if err != nil {
		return nil, err
	}
	if uid != 0 && uid != rec.SubmitterUID {
		return nil, control.NewError(control.CodeNotSubmitter, "only the submitter or root may withdraw a proposal")
	}
	if err := p.transition(rec, control.ProposalWithdrawn); err != nil {
		return nil, err
	}
	rec.State = control.ProposalWithdrawn
	return p.info(rec), nil
}

// handleApprove accepts the approval of an open proposal whose fingerprint is the one the person was shown. It
// runs before anything is applied: from here on the proposal can be neither withdrawn, rejected nor approved
// again, and it waits for its result. After ProposalApprovalWindow without a result it counts as failed.
func (p *proposalService) handleApprove(_ context.Context, raw json.RawMessage) (any, error) {
	var a control.ProposalApproveArgs
	if err := decodeArgs(raw, &a); err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	rec, err := p.find(a.ID)
	if err != nil {
		return nil, err
	}
	if st := p.stateOf(rec); st != control.ProposalOpen {
		return nil, control.NewError(control.CodeInvalid, fmt.Sprintf("the proposal is %s, not open", st))
	}
	if a.Fingerprint != rec.Fingerprint {
		return nil, control.NewError(control.CodeProposalChanged, "the proposal is not the one that was shown")
	}
	rec.State, rec.Decided = control.ProposalApproving, p.now().UTC().Truncate(time.Second)
	if err := p.save(rec); err != nil {
		return nil, p.internal("store the proposal", err)
	}
	return p.info(rec), nil
}

// handleReject closes an open proposal as rejected with an optional reason the submitter can read.
func (p *proposalService) handleReject(_ context.Context, raw json.RawMessage) (any, error) {
	var a control.ProposalRejectArgs
	if err := decodeArgs(raw, &a); err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	rec, err := p.find(a.ID)
	if err != nil {
		return nil, err
	}
	if st := p.stateOf(rec); st != control.ProposalOpen {
		return nil, control.NewError(control.CodeInvalid, fmt.Sprintf("the proposal is %s, not open", st))
	}
	rec.State, rec.Decided, rec.Message = control.ProposalRejected, p.now().UTC().Truncate(time.Second), printable(a.Reason)
	if err := p.save(rec); err != nil {
		return nil, p.internal("store the proposal", err)
	}
	return p.info(rec), nil
}

// handleResult records the outcome of an approved proposal. It also takes a result that arrives after the window
// ran out, because the outcome that happened is the truth.
func (p *proposalService) handleResult(_ context.Context, raw json.RawMessage) (any, error) {
	var a control.ProposalResultArgs
	if err := decodeArgs(raw, &a); err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	rec, err := p.find(a.ID)
	if err != nil {
		return nil, err
	}
	if rec.State != control.ProposalApproving {
		return nil, control.NewError(control.CodeInvalid, fmt.Sprintf("the proposal is %s, not being approved", rec.State))
	}
	if a.Fingerprint != rec.Fingerprint {
		return nil, control.NewError(control.CodeProposalChanged, "the proposal is not the one that was approved")
	}
	if a.Applied {
		rec.State, rec.Message = control.ProposalApplied, ""
	} else if rec.State, rec.Message = control.ProposalFailed, printable(a.Message); rec.Message == "" {
		rec.Message = "the change failed"
	}
	if err := p.save(rec); err != nil {
		return nil, p.internal("store the proposal", err)
	}
	return p.info(rec), nil
}

func (p *proposalService) transition(rec proposalRecord, to string) error {
	if st := p.stateOf(rec); st != control.ProposalOpen {
		return control.NewError(control.CodeInvalid, fmt.Sprintf("the proposal is %s, not open", st))
	}
	rec.State = to
	if err := p.save(rec); err != nil {
		return p.internal("store the proposal", err)
	}
	return nil
}

// stateOf is the state shown for a record: an open proposal past its lifetime is expired, whatever the clock
// said when it was stored.
func (p *proposalService) stateOf(r proposalRecord) string {
	switch {
	case r.State == control.ProposalOpen && !p.now().Before(r.Submitted.Add(control.ProposalLifetime)):
		return control.ProposalExpired
	case r.State == control.ProposalApproving && !p.now().Before(r.Decided.Add(control.ProposalApprovalWindow)):
		return control.ProposalFailed
	}
	return r.State
}

func (p *proposalService) info(r proposalRecord) control.ProposalInfo {
	st, msg := p.stateOf(r), printable(r.Message)
	if st == control.ProposalFailed && r.State == control.ProposalApproving {
		msg = "the approval did not finish"
	}
	return control.ProposalInfo{ID: r.ID, Kind: r.Kind, State: st, Fingerprint: r.Fingerprint,
		SubmitterUID: r.SubmitterUID, SubmitterName: printable(r.SubmitterName), Submitted: r.Submitted,
		Expires: r.Submitted.Add(control.ProposalLifetime), Message: msg}
}

func (p *proposalService) internal(what string, err error) error {
	p.log.Error("proposals: "+what, "error", err)
	return control.NewError(control.CodeInternal, "internal error")
}

func (p *proposalService) path(id string) string { return filepath.Join(p.dir, id+".json") }

func (p *proposalService) find(id string) (proposalRecord, error) {
	notFound := control.NewError(control.CodeNotFound, "no such proposal")
	if !proposalIDRe.MatchString(id) {
		return proposalRecord{}, notFound
	}
	rec, err := readProposal(p.path(id))
	if errors.Is(err, fs.ErrNotExist) {
		return proposalRecord{}, notFound
	}
	if err != nil || rec.ID != id {
		return proposalRecord{}, p.internal("read the proposal", fmt.Errorf("%s is not readable: %v", id, err))
	}
	return rec, nil
}

func readProposal(path string) (proposalRecord, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return proposalRecord{}, err
	}
	var r proposalRecord
	if err := decodeStrict(data, &r); err != nil {
		return proposalRecord{}, err
	}
	switch r.State {
	case control.ProposalOpen, control.ProposalWithdrawn, control.ProposalRejected, control.ProposalApproving,
		control.ProposalApplied, control.ProposalFailed:
		return r, nil
	}
	return proposalRecord{}, errors.New("unknown state")
}

// load reads every proposal; a file that cannot be read is logged and skipped.
func (p *proposalService) load() ([]proposalRecord, error) {
	entries, err := os.ReadDir(p.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var all []proposalRecord
	for _, e := range entries {
		id, ok := strings.CutSuffix(e.Name(), ".json")
		if !ok || !proposalIDRe.MatchString(id) {
			continue
		}
		r, err := readProposal(filepath.Join(p.dir, e.Name()))
		if err != nil || r.ID != id {
			p.log.Warn("proposals: skipped an unreadable file", "file", e.Name())
			continue
		}
		all = append(all, r)
	}
	return all, nil
}

// prune removes the proposals that are no longer open and are old, and leftovers of interrupted writes, from the
// store, and returns the proposals that remain.
func (p *proposalService) prune(all []proposalRecord) []proposalRecord {
	if entries, err := os.ReadDir(p.dir); err == nil {
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".tmp-") {
				_ = os.Remove(filepath.Join(p.dir, e.Name()))
			}
		}
	}
	return slices.DeleteFunc(all, func(r proposalRecord) bool {
		if p.stateOf(r) == control.ProposalOpen || !p.now().After(r.Submitted.Add(proposalKeep)) {
			return false
		}
		return os.Remove(p.path(r.ID)) == nil
	})
}

func (p *proposalService) save(r proposalRecord) error {
	if err := os.MkdirAll(p.dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(p.dir, 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}
	tmp := filepath.Join(p.dir, ".tmp-"+r.ID)
	_ = os.Remove(tmp)
	if err := configfile.WriteNewPrivateFile(tmp, data); err != nil {
		return err
	}
	if err := os.Rename(tmp, p.path(r.ID)); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func newProposalID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "p-" + hex.EncodeToString(b[:]), nil
}

// userName resolves uid in the passwd file of the access check, never through NSS; an unknown user is named by
// its number.
func (p *proposalService) userName(uid uint32) string {
	if data, err := os.ReadFile(p.passwd); err == nil {
		want := strconv.FormatUint(uint64(uid), 10)
		for _, line := range strings.Split(string(data), "\n") {
			if f := strings.Split(line, ":"); len(f) >= 4 && f[2] == want && f[0] != "" {
				return printable(f[0])
			}
		}
	}
	return "uid-" + strconv.FormatUint(uint64(uid), 10)
}
