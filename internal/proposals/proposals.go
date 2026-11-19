// Package proposals keeps proposals of changes below the state directory of a service: one private file each,
// replaced atomically, with their lifecycle from open to applied. It knows no kind of proposal. The service
// that owns a store hands it the normalization of its kinds and registers the commands at its own channel.
package proposals

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
	"unicode"

	"github.com/castrowithcee/qatlas-kit/approval"
	"github.com/castrowithcee/qatlas-kit/configfile"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

const (
	dirName = "proposals"
	// fingerprintLabel separates the fingerprints of proposals from every other hash of the same bytes.
	fingerprintLabel = "qhost/proposal/v1"
	// maxStored bounds the files of the store, so a stream of submissions and withdrawals cannot fill the
	// disk. Closed proposals are dropped after keep.
	maxStored = 500
	keep      = 2 * control.ProposalLifetime
	// maxTextChars bounds the text of a reason or a failure that is shown to others.
	maxTextChars = 300
)

var idRe = regexp.MustCompile(`^p-[0-9a-f]{16}$`)

// Normalizer checks the content of one kind of proposal and returns it in its canonical form, the one the
// fingerprint covers. It refuses an unknown kind and never runs, evaluates or expands the content.
type Normalizer func(kind string, content json.RawMessage) (any, *control.Error)

// record is the file of one proposal. Content is the normalized content of its kind, the one the fingerprint
// covers.
type record struct {
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

// Store keeps the proposals of one service. It never touches what a proposal names.
type Store struct {
	dir       string
	passwd    string
	now       func() time.Time
	log       *slog.Logger
	normalize Normalizer
	mu        sync.Mutex
}

// New returns the store below stateDir. passwd is the file that names the submitters; a nil now means the
// system clock.
func New(stateDir, passwd string, now func() time.Time, log *slog.Logger, normalize Normalizer) *Store {
	if now == nil {
		now = time.Now
	}
	return &Store{dir: filepath.Join(stateDir, dirName), passwd: passwd, now: now, log: log, normalize: normalize}
}

// Commands returns the commands of the store with the access each needs, by command name.
func (s *Store) Commands() map[string]control.Command {
	return map[string]control.Command{
		control.ProposalSubmitCommand:   {Access: control.AccessPropose, Handle: s.Submit},
		control.ProposalWithdrawCommand: {Access: control.AccessPropose, Handle: s.Withdraw},
		control.ProposalApproveCommand:  {Access: control.AccessWrite, Handle: s.Approve},
		control.ProposalRejectCommand:   {Access: control.AccessWrite, Handle: s.Reject},
		control.ProposalResultCommand:   {Access: control.AccessWrite, Handle: s.Result},
		control.ProposalListCommand:     {Access: control.AccessRead, Handle: s.List},
		control.ProposalShowCommand:     {Access: control.AccessRead, Handle: s.Show},
	}
}

// Register adds the commands of the store to r.
func (s *Store) Register(r *control.Registry) error {
	for name, c := range s.Commands() {
		if err := r.Register(name, c); err != nil {
			return err
		}
	}
	return nil
}

// Invalid is the answer for content that does not pass the check of its kind.
func Invalid(format string, a ...any) *control.Error {
	return control.NewError(control.CodeInvalid, fmt.Sprintf(format, a...))
}

// DecodeStrict decodes one JSON value and refuses a field the target does not name and trailing data.
func DecodeStrict(raw json.RawMessage, into any) error {
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

// Printable shortens s and replaces what a terminal could interpret, so text of others can be shown and
// logged safely.
func Printable(s string) string {
	var b strings.Builder
	n := 0
	for _, r := range s {
		if n == maxTextChars {
			b.WriteString("...")
			break
		}
		if unicode.IsPrint(r) {
			b.WriteRune(r)
		} else {
			b.WriteByte(' ')
		}
		n++
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// decodeArgs reads the arguments strictly and answers with a fixed text, so a malformed request never echoes
// what it carried.
func decodeArgs(raw json.RawMessage, into any) error {
	if len(raw) == 0 {
		raw = json.RawMessage("{}")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		return control.NewError(control.CodeBadRequest, "the arguments are not valid")
	}
	return nil
}

// Submit normalizes and stores a proposal of the peer. The size is checked before anything is decoded, and at
// most control.ProposalMaxOpen proposals of one submitter may be open.
func (s *Store) Submit(ctx context.Context, raw json.RawMessage) (any, error) {
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
	content, cerr := s.normalize(a.Kind, a.Content)
	if cerr != nil {
		return nil, cerr
	}
	data, err := json.Marshal(content)
	if err != nil {
		return nil, err
	}
	fp, err := approval.Fingerprint(fingerprintLabel, struct {
		Kind    string          `json:"kind"`
		Content json.RawMessage `json:"content"`
	}{a.Kind, data})
	if err != nil {
		return nil, err
	}
	rec := record{Kind: a.Kind, Content: data, Fingerprint: fp, SubmitterUID: uid,
		SubmitterName: s.userName(uid), Submitted: s.now().UTC().Truncate(time.Second), State: control.ProposalOpen}

	s.mu.Lock()
	defer s.mu.Unlock()
	all, err := s.load()
	if err != nil {
		return nil, s.internal("read the proposals", err)
	}
	open := 0
	for _, r := range all {
		if r.SubmitterUID == uid && s.stateOf(r) == control.ProposalOpen {
			open++
		}
	}
	if open >= control.ProposalMaxOpen {
		return nil, control.NewError(control.CodeProposalLimit,
			fmt.Sprintf("at most %d proposals may be open per user; withdraw one first", control.ProposalMaxOpen))
	}
	all = s.prune(all)
	if len(all) >= maxStored {
		return nil, control.NewError(control.CodeProposalLimit, "the store of proposals is full; try again later")
	}
	if rec.ID, err = newID(); err != nil {
		return nil, s.internal("create an id", err)
	}
	if err := s.save(rec); err != nil {
		return nil, s.internal("store the proposal", err)
	}
	return s.info(rec), nil
}

// List answers every proposal, the newest first.
func (s *Store) List(_ context.Context, raw json.RawMessage) (any, error) {
	if err := decodeArgs(raw, &struct{}{}); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	all, err := s.load()
	if err != nil {
		return nil, s.internal("read the proposals", err)
	}
	slices.SortFunc(all, func(a, b record) int {
		if c := b.Submitted.Compare(a.Submitted); c != 0 {
			return c
		}
		return strings.Compare(b.ID, a.ID)
	})
	res := control.ProposalListResult{Proposals: make([]control.ProposalInfo, 0, len(all))}
	for _, r := range all {
		res.Proposals = append(res.Proposals, s.info(r))
	}
	return res, nil
}

// Show answers one proposal with its normalized content.
func (s *Store) Show(_ context.Context, raw json.RawMessage) (any, error) {
	var a control.ProposalIDArgs
	if err := decodeArgs(raw, &a); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, err := s.find(a.ID)
	if err != nil {
		return nil, err
	}
	return control.ProposalShowResult{ProposalInfo: s.info(rec), Content: rec.Content}, nil
}

// Withdraw closes an open proposal at the request of its submitter or root.
func (s *Store) Withdraw(ctx context.Context, raw json.RawMessage) (any, error) {
	uid, ok := control.PeerUID(ctx)
	if !ok {
		return nil, control.NewError(control.CodeInternal, "internal error")
	}
	var a control.ProposalIDArgs
	if err := decodeArgs(raw, &a); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, err := s.find(a.ID)
	if err != nil {
		return nil, err
	}
	if uid != 0 && uid != rec.SubmitterUID {
		return nil, control.NewError(control.CodeNotSubmitter, "only the submitter or root may withdraw a proposal")
	}
	if err := s.transition(rec, control.ProposalWithdrawn); err != nil {
		return nil, err
	}
	rec.State = control.ProposalWithdrawn
	return s.info(rec), nil
}

// Approve accepts the approval of an open proposal whose fingerprint is the one the person was shown. It
// runs before anything is applied: from here on the proposal can be neither withdrawn, rejected nor approved
// again, and it waits for its result. After ProposalApprovalWindow without a result it counts as failed.
func (s *Store) Approve(_ context.Context, raw json.RawMessage) (any, error) {
	var a control.ProposalApproveArgs
	if err := decodeArgs(raw, &a); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, err := s.find(a.ID)
	if err != nil {
		return nil, err
	}
	if st := s.stateOf(rec); st != control.ProposalOpen {
		return nil, control.NewError(control.CodeInvalid, fmt.Sprintf("the proposal is %s, not open", st))
	}
	if a.Fingerprint != rec.Fingerprint {
		return nil, control.NewError(control.CodeProposalChanged, "the proposal is not the one that was shown")
	}
	rec.State, rec.Decided = control.ProposalApproving, s.now().UTC().Truncate(time.Second)
	if err := s.save(rec); err != nil {
		return nil, s.internal("store the proposal", err)
	}
	return s.info(rec), nil
}

// Reject closes an open proposal as rejected with an optional reason the submitter can read.
func (s *Store) Reject(_ context.Context, raw json.RawMessage) (any, error) {
	var a control.ProposalRejectArgs
	if err := decodeArgs(raw, &a); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, err := s.find(a.ID)
	if err != nil {
		return nil, err
	}
	if st := s.stateOf(rec); st != control.ProposalOpen {
		return nil, control.NewError(control.CodeInvalid, fmt.Sprintf("the proposal is %s, not open", st))
	}
	rec.State, rec.Decided, rec.Message = control.ProposalRejected, s.now().UTC().Truncate(time.Second), Printable(a.Reason)
	if err := s.save(rec); err != nil {
		return nil, s.internal("store the proposal", err)
	}
	return s.info(rec), nil
}

// Result records the outcome of an approved proposal. It also takes a result that arrives after the window
// ran out, because the outcome that happened is the truth.
func (s *Store) Result(_ context.Context, raw json.RawMessage) (any, error) {
	var a control.ProposalResultArgs
	if err := decodeArgs(raw, &a); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, err := s.find(a.ID)
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
	} else if rec.State, rec.Message = control.ProposalFailed, Printable(a.Message); rec.Message == "" {
		rec.Message = "the change failed"
	}
	if err := s.save(rec); err != nil {
		return nil, s.internal("store the proposal", err)
	}
	return s.info(rec), nil
}

func (s *Store) transition(rec record, to string) error {
	if st := s.stateOf(rec); st != control.ProposalOpen {
		return control.NewError(control.CodeInvalid, fmt.Sprintf("the proposal is %s, not open", st))
	}
	rec.State = to
	if err := s.save(rec); err != nil {
		return s.internal("store the proposal", err)
	}
	return nil
}

// stateOf is the state shown for a record: an open proposal past its lifetime is expired, whatever the clock
// said when it was stored.
func (s *Store) stateOf(r record) string {
	switch {
	case r.State == control.ProposalOpen && !s.now().Before(r.Submitted.Add(control.ProposalLifetime)):
		return control.ProposalExpired
	case r.State == control.ProposalApproving && !s.now().Before(r.Decided.Add(control.ProposalApprovalWindow)):
		return control.ProposalFailed
	}
	return r.State
}

func (s *Store) info(r record) control.ProposalInfo {
	st, msg := s.stateOf(r), Printable(r.Message)
	if st == control.ProposalFailed && r.State == control.ProposalApproving {
		msg = "the approval did not finish"
	}
	return control.ProposalInfo{ID: r.ID, Kind: r.Kind, State: st, Fingerprint: r.Fingerprint,
		SubmitterUID: r.SubmitterUID, SubmitterName: Printable(r.SubmitterName), Submitted: r.Submitted,
		Expires: r.Submitted.Add(control.ProposalLifetime), Message: msg}
}

func (s *Store) internal(what string, err error) error {
	s.log.Error("proposals: "+what, "error", err)
	return control.NewError(control.CodeInternal, "internal error")
}

func (s *Store) path(id string) string { return filepath.Join(s.dir, id+".json") }

func (s *Store) find(id string) (record, error) {
	notFound := control.NewError(control.CodeNotFound, "no such proposal")
	if !idRe.MatchString(id) {
		return record{}, notFound
	}
	rec, err := readRecord(s.path(id))
	if errors.Is(err, fs.ErrNotExist) {
		return record{}, notFound
	}
	if err != nil || rec.ID != id {
		return record{}, s.internal("read the proposal", fmt.Errorf("%s is not readable: %v", id, err))
	}
	return rec, nil
}

func readRecord(path string) (record, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return record{}, err
	}
	var r record
	if err := DecodeStrict(data, &r); err != nil {
		return record{}, err
	}
	switch r.State {
	case control.ProposalOpen, control.ProposalWithdrawn, control.ProposalRejected, control.ProposalApproving,
		control.ProposalApplied, control.ProposalFailed:
		return r, nil
	}
	return record{}, errors.New("unknown state")
}

// load reads every proposal; a file that cannot be read is logged and skipped.
func (s *Store) load() ([]record, error) {
	entries, err := os.ReadDir(s.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var all []record
	for _, e := range entries {
		id, ok := strings.CutSuffix(e.Name(), ".json")
		if !ok || !idRe.MatchString(id) {
			continue
		}
		r, err := readRecord(filepath.Join(s.dir, e.Name()))
		if err != nil || r.ID != id {
			s.log.Warn("proposals: skipped an unreadable file", "file", e.Name())
			continue
		}
		all = append(all, r)
	}
	return all, nil
}

// prune removes the proposals that are no longer open and are old, and leftovers of interrupted writes, from the
// store, and returns the proposals that remain.
func (s *Store) prune(all []record) []record {
	if entries, err := os.ReadDir(s.dir); err == nil {
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".tmp-") {
				_ = os.Remove(filepath.Join(s.dir, e.Name()))
			}
		}
	}
	return slices.DeleteFunc(all, func(r record) bool {
		if s.stateOf(r) == control.ProposalOpen || !s.now().After(r.Submitted.Add(keep)) {
			return false
		}
		return os.Remove(s.path(r.ID)) == nil
	})
}

func (s *Store) save(r record) error {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(s.dir, 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}
	tmp := filepath.Join(s.dir, ".tmp-"+r.ID)
	_ = os.Remove(tmp)
	if err := configfile.WriteNewPrivateFile(tmp, data); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path(r.ID)); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func newID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "p-" + hex.EncodeToString(b[:]), nil
}

// userName resolves uid in the passwd file of the access check, never through NSS; an unknown user is named by
// its number.
func (s *Store) userName(uid uint32) string {
	if data, err := os.ReadFile(s.passwd); err == nil {
		want := strconv.FormatUint(uint64(uid), 10)
		for _, line := range strings.Split(string(data), "\n") {
			if f := strings.Split(line, ":"); len(f) >= 4 && f[2] == want && f[0] != "" {
				return Printable(f[0])
			}
		}
	}
	return "uid-" + strconv.FormatUint(uint64(uid), 10)
}
