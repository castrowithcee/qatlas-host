package tui

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/castrowithcee/qatlas-kit/localipc"
	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/client"
	"github.com/castrowithcee/qatlas-host/internal/control"
)

// System > Log shows the action log of the host service, newest first, under the result of the check of its chain
// of hashes. Administrators in the group qhost read it without sudo. Every request to the host service is an entry
// of this log itself, so it is read when the page opens for the first time, after a change made from here and on r,
// never on a timer.

// HostLog reads the action log of the host service.
type HostLog interface {
	// LogList reads a page of the action log, newest first.
	LogList(ctx context.Context, args control.HostLogListArgs) (control.HostLogListResult, error)
	// LogVerify checks the chain of hashes of the action log.
	LogVerify(ctx context.Context) (control.HostLogVerifyResult, error)
}

// HostChannel is the HostLog over the channel of the host service.
type HostChannel struct{ endpoint localipc.ServiceEndpoint }

// NewHostChannel returns the HostLog over the channel at ep.
func NewHostChannel(ep localipc.ServiceEndpoint) *HostChannel { return &HostChannel{endpoint: ep} }

func (h *HostChannel) LogList(ctx context.Context, args control.HostLogListArgs) (res control.HostLogListResult, err error) {
	err = client.Do(ctx, h.endpoint, client.CallTimeout, control.HostLogListCommand, args, &res)
	return res, err
}

func (h *HostChannel) LogVerify(ctx context.Context) (res control.HostLogVerifyResult, err error) {
	err = client.Do(ctx, h.endpoint, client.CallTimeout, control.HostLogVerifyCommand, nil, &res)
	return res, err
}

// hostLogLimit is how many entries the page shows; qhost audit narrows the log further.
const hostLogLimit = control.HostLogDefaultLimit

// hostLogPage is what the log page holds. gen separates the answers of a read that was overtaken.
type hostLogPage struct {
	list      *kittui.List
	gen       int
	read      bool
	reading   bool
	verifying bool
	stale     bool
	entries   []control.HostLogEntry
	rows      [][]string
	// svc is the engine whose deploy service the page shows, empty for the host service. dlist is the list of
	// the deploy services, dep what was read from each.
	svc      string
	dlist    *kittui.List
	dep      map[string]*deployLog
	err      error
	chain    *control.HostLogVerifyResult
	chainErr error
}

// hostLogMsg carries a read of the action log, hostVerifyMsg a check of its chain.
type hostLogMsg struct {
	gen int
	res control.HostLogListResult
	err error
}

type hostVerifyMsg struct {
	gen int
	res control.HostLogVerifyResult
	err error
}

func newHostLogList(keys *kittui.Keymap, text kittui.Translator) (*kittui.List, error) {
	success := text.Lookup(textLogSuccess)
	tone := func(cell string) kittui.TextStyle {
		if cell != success {
			return kittui.StyleWarn
		}
		return kittui.StyleText
	}
	l, err := kittui.NewList(kittui.ListSpec{Columns: []kittui.Column{
		{Title: textColTime}, {Title: textColUser}, {Title: textColAction}, {Title: textColResult, Tone: tone},
		{Title: textColChange, Flex: true, Optional: true}},
		Keys: listKeys(), Search: kittui.SearchByKey, Empty: textLogEmpty}, keys, text)
	if err != nil {
		return nil, err
	}
	l.SetFocused(true)
	return l, nil
}

// readHostLog reads the latest entries and checks the chain, both off the event loop.
func (m *Model) readHostLog() tea.Cmd {
	p := &m.sys.hlog
	if m.hostLog == nil {
		p.read, p.err = true, errNoHostLog
		return nil
	}
	p.gen++
	p.reading, p.verifying, p.stale = true, true, false
	gen, h, ctx := p.gen, m.hostLog, m.ctx
	return tea.Batch(func() tea.Msg {
		res, err := h.LogList(ctx, control.HostLogListArgs{Limit: hostLogLimit})
		return hostLogMsg{gen: gen, res: res, err: err}
	}, func() tea.Msg {
		res, err := h.LogVerify(ctx)
		return hostVerifyMsg{gen: gen, res: res, err: err}
	})
}

// errNoHostLog is the failure of an interface that was given no way to reach the host service.
var errNoHostLog = errors.New("no host channel")

// applyHostLog takes a read of the action log; a failed read keeps the entries read before.
func (m *Model) applyHostLog(msg hostLogMsg) {
	p := &m.sys.hlog
	if msg.gen != p.gen {
		return
	}
	p.reading, p.read, p.err = false, true, msg.err
	if msg.err != nil {
		return
	}
	p.entries = msg.res.Entries
	m.fillHostLog()
}

// applyHostVerify takes a check of the chain.
func (m *Model) applyHostVerify(msg hostVerifyMsg) {
	p := &m.sys.hlog
	if msg.gen != p.gen {
		return
	}
	p.verifying, p.chain, p.chainErr = false, nil, msg.err
	if msg.err == nil {
		p.chain = &msg.res
	}
}

// fillHostLog sets the rows of the list from what was read last.
func (m *Model) fillHostLog() {
	p, t := &m.sys.hlog, m.text
	items := make([]kittui.ListItem, 0, len(p.entries))
	p.rows = p.rows[:0]
	for _, e := range p.entries {
		user := t.Lookup(textNoValue)
		if e.ClientUID != nil {
			user = m.uidName(int(*e.ClientUID))
		}
		result := t.Lookup(textLogSuccess)
		switch {
		case e.Result == control.HostLogResultSuccess:
		case e.ErrorCode != "":
			result = t.Lookup(textLogFailureCode, e.ErrorCode)
		default:
			result = t.Lookup(textLogFailure)
		}
		change := ""
		if c := e.Change; c != nil {
			change = c.ActionID + " " + c.JournalID
			if c.UndoOf != "" {
				change = t.Lookup(textLogUndoOf, change, c.UndoOf)
			}
		}
		cells := []string{e.Time.In(m.loc).Format("2006-01-02 15:04:05"), user, e.Action, result, change}
		p.rows = append(p.rows, cells)
		items = append(items, kittui.ListItem{ID: kittui.ItemID(strconv.FormatUint(e.Seq, 10) + "@" + e.Time.UTC().Format(time.RFC3339Nano)), Cells: cells})
	}
	if err := p.list.SetItems(items); err != nil {
		m.failure = t.Lookup(kittui.TextError, err.Error())
	}
}

// hostLogError is the sentence of a failed call to the host service.
func (m *Model) hostLogError(err error, failed kittui.TextID) string {
	t := m.text
	var remote *control.Error
	switch {
	case errors.Is(err, errNoHostLog):
		return t.Lookup(textLogNoChannel)
	case errors.Is(err, client.ErrNotRunning):
		return t.Lookup(textLogNotRunning)
	case errors.Is(err, client.ErrNoAccess),
		errors.As(err, &remote) && (remote.Code == control.CodeNotAdmin || remote.Code == control.CodeNoAccess):
		return t.Lookup(textLogNoAccess)
	case errors.As(err, &remote):
		return t.Lookup(failed, remote.Message)
	}
	return t.Lookup(failed, err.Error())
}

// chainLine is the result of the check of the chain in words, with its style.
func (m *Model) chainLine() (string, kittui.TextStyle) {
	p, t := &m.sys.hlog, m.text
	switch {
	case p.verifying:
		return t.Lookup(textChainChecking), kittui.StyleHint
	case p.chainErr != nil:
		return m.hostLogError(p.chainErr, textChainFailed), kittui.StyleFail
	case p.chain == nil:
		return "", kittui.StyleHint
	}
	c := *p.chain
	if !c.Intact {
		text := t.Lookup(textChainBroken, "?", 0, "?")
		if len(c.Problems) > 0 {
			pr := c.Problems[0]
			text = t.Lookup(textChainBroken, pr.Date, pr.Line, m.chainProblem(pr.Code))
		}
		if c.MoreProblems || len(c.Problems) > 1 {
			text += " " + t.Lookup(textChainMore)
		}
		return text, kittui.StyleFail
	}
	if len(c.Days) == 0 {
		return t.Lookup(textChainNone), kittui.StyleOK
	}
	entries, damaged := 0, 0
	for _, d := range c.Days {
		entries += d.Entries
		damaged += d.Damaged
	}
	text := t.Lookup(textChainIntact, len(c.Days), entries)
	if damaged > 0 {
		text += " " + t.Lookup(textChainDamaged, damaged)
	}
	return text, kittui.StyleOK
}

// chainProblem is a problem code of the check in words; an unknown code stays as it is.
func (m *Model) chainProblem(code string) string {
	switch code {
	case "unparsable-line":
		return m.text.Lookup(textChainUnparsable)
	case "bad-genesis":
		return m.text.Lookup(textChainGenesis)
	case "missing-entries":
		return m.text.Lookup(textChainMissing)
	case "chain-broken":
		return m.text.Lookup(textChainLink)
	}
	return code
}

// hostLogView draws the log page in width by height cells: the result of the check of the chain, a note where the
// log is older than the host, and the entries.
func (m *Model) hostLogView(width, height int) string {
	p, t := &m.sys.hlog, m.text
	if p.svc != "" {
		return m.deployLogView(width, height)
	}
	head := m.logServiceLine(width)
	if text, style := m.chainLine(); text != "" {
		head = append(head, m.wrapped(detailMargin, style, text, width)...)
	}
	switch {
	case p.err != nil && len(p.entries) == 0:
		lines := append(head, m.wrapped(detailMargin, kittui.StyleFail, m.hostLogError(p.err, textLogFailed), width)...)
		return strings.Join(lines[:min(len(lines), height)], "\n")
	case !p.read:
		lines := append(head, m.line(detailMargin, kittui.StyleHint, t.Lookup(textLogReading), width))
		return strings.Join(lines[:min(len(lines), height)], "\n")
	case p.err != nil:
		head = append(head, m.wrapped(detailMargin, kittui.StyleFail, m.hostLogError(p.err, textLogFailed), width)...)
	case p.stale:
		head = append(head, m.wrapped(detailMargin, kittui.StyleWarn, t.Lookup(textLogStale), width)...)
	}
	if len(head) > 0 {
		head = append(head, "")
	}
	head = head[:min(len(head), max(height-minListRows, 0))]
	view := m.listView(p.list, width, max(height-len(head), 1), len(head))
	return strings.Join(append(head, view), "\n")
}

// hostLogFooter are the own keys of the log page.
func (m *Model) hostLogFooter(own, common, nav []kittui.Hint) []kittui.Hint {
	l := m.sys.hlog.shownList()
	if l.Searching() {
		return []kittui.Hint{{Action: actEnter, Label: textFilterKeep}, {Action: actBack, Label: textFilterClear}}
	}
	if len(m.logServices()) > 1 {
		own = append(own, kittui.Hint{Action: actInstance, Label: textKeyLogService, Priority: 1})
	}
	if len(m.sys.hlog.shownRows()) > 0 {
		own = append(own, kittui.Hint{Action: actSearch, Label: kittui.TextKeySearch, Priority: 1}, saveLogHint())
		if l.Query() != "" {
			own = append(own, kittui.Hint{Action: actBack, Label: textFilterClear})
		}
	}
	return append(append(own, common...), nav...)
}
