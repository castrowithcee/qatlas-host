package tui

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/client"
	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/userfile"
)

// System > Log shows the action log of one service at a time: the host service, or the deploy service of an
// engine. H chooses the service. A deploy service that cannot be asked is a note on its page, never a failure of
// the view. The entries carry no argument and no secret; the origin is what the client claimed.

// deployLog is what was read from the deploy service of one engine.
type deployLog struct {
	gen     int
	read    bool
	entries []control.DeployLogEntry
	err     error
	rows    [][]string
}

// deployLogMsg carries a read of the action log of the deploy service of engine.
type deployLogMsg struct {
	engine string
	gen    int
	res    control.DeployLogListResult
	err    error
}

func newDeployLogList(keys *kittui.Keymap, text kittui.Translator) (*kittui.List, error) {
	success := text.Lookup(textLogSuccess)
	tone := func(cell string) kittui.TextStyle {
		if cell != success {
			return kittui.StyleWarn
		}
		return kittui.StyleText
	}
	l, err := kittui.NewList(kittui.ListSpec{Columns: []kittui.Column{
		{Title: textColTime}, {Title: textColUser}, {Title: textColFunction}, {Title: textColStack},
		{Title: textColResult, Tone: tone}, {Title: textColOrigin, Flex: true, Optional: true}},
		Keys: listKeys(), Search: kittui.SearchByKey, Empty: textDeployLogEmpty}, keys, text)
	if err != nil {
		return nil, err
	}
	l.SetFocused(true)
	return l, nil
}

// logServices are the services the page can show: the host service (empty) and every deploy engine.
func (m *Model) logServices() []string { return append([]string{""}, m.engines()...) }

// shownList is the list of the service the page shows.
func (p *hostLogPage) shownList() *kittui.List {
	if p.svc != "" {
		return p.dlist
	}
	return p.list
}

// shownRows are the rows of the service the page shows, as its list holds them.
func (p *hostLogPage) shownRows() [][]string {
	if p.svc == "" {
		return p.rows
	}
	if d := p.dep[p.svc]; d != nil {
		return d.rows
	}
	return nil
}

// nextLogService chooses the next service and reads its log if that was not done yet.
func (m *Model) nextLogService() tea.Cmd {
	p := &m.sys.hlog
	svcs := m.logServices()
	if len(svcs) < 2 {
		return nil
	}
	p.svc = svcs[(slices.Index(svcs, p.svc)+1)%len(svcs)]
	p.shownList().Reset()
	if p.svc == "" {
		return m.showSystemPage()
	}
	if d := p.dep[p.svc]; d != nil && d.read {
		m.fillDeployLog(p.svc)
		return nil
	}
	return m.readDeployLog(p.svc)
}

// readDeployLog reads the latest entries of the deploy service of engine off the event loop.
func (m *Model) readDeployLog(engine string) tea.Cmd {
	p := &m.sys.hlog
	if p.dep == nil {
		p.dep = map[string]*deployLog{}
	}
	d := p.dep[engine]
	if d == nil {
		d = &deployLog{}
		p.dep[engine] = d
	}
	d.gen++
	gen, b, ctx := d.gen, m.backend, m.engineCtx(engine)
	return func() tea.Msg {
		res, err := b.DeployLog(ctx, control.HostLogListArgs{Limit: hostLogLimit})
		return deployLogMsg{engine: engine, gen: gen, res: res, err: err}
	}
}

// applyDeployLog takes a read of the action log of a deploy service.
func (m *Model) applyDeployLog(msg deployLogMsg) {
	d := m.sys.hlog.dep[msg.engine]
	if d == nil || msg.gen != d.gen {
		return
	}
	d.read, d.err, d.entries = true, msg.err, nil
	if msg.err == nil {
		d.entries = msg.res.Entries
	}
	m.fillDeployLog(msg.engine)
}

// fillDeployLog builds the rows of the deploy service of engine from what was read, and sets them in the list
// when that service is the one shown.
func (m *Model) fillDeployLog(engine string) {
	p, t := &m.sys.hlog, m.text
	d := p.dep[engine]
	items := make([]kittui.ListItem, 0, len(d.entries))
	d.rows = d.rows[:0]
	dash := t.Lookup(textNoValue)
	for _, e := range d.entries {
		user := dash
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
		cells := []string{e.Time.In(m.loc).Format("2006-01-02 15:04:05"), user, orDash(e.Command, dash), orDash(e.Stack, dash),
			result, orDash(e.Origin, dash)}
		d.rows = append(d.rows, cells)
		items = append(items, kittui.ListItem{ID: kittui.ItemID(strconv.FormatUint(e.Seq, 10) + "@" + e.Time.UTC().Format(time.RFC3339Nano)), Cells: cells})
	}
	if p.svc != engine {
		return
	}
	if err := p.dlist.SetItems(items); err != nil {
		m.failure = t.Lookup(kittui.TextError, err.Error())
	}
}

func orDash(s, dash string) string {
	if s == "" {
		return dash
	}
	return s
}

// deployLogNote is the sentence for a deploy service that cannot be asked.
func (m *Model) deployLogNote(engine string, err error) string {
	t := m.text
	var remote *control.Error
	switch {
	case errors.Is(err, client.ErrNotRunning):
		return t.Lookup(textDeployLogNotRun, engine)
	case errors.Is(err, client.ErrNoAccess),
		errors.As(err, &remote) && (remote.Code == control.CodeNotAdmin || remote.Code == control.CodeNoAccess):
		return t.Lookup(textDeployLogNoAcc, engine)
	case errors.As(err, &remote):
		return t.Lookup(textDeployLogDown, engine, remote.Message)
	}
	return t.Lookup(textDeployLogDown, engine, err.Error())
}

// logServiceLine names the service the page shows; empty while no deploy service exists.
func (m *Model) logServiceLine(width int) []string {
	if len(m.engines()) == 0 {
		return nil
	}
	text := m.text.Lookup(textLogServiceHost)
	if e := m.sys.hlog.svc; e != "" {
		text = m.text.Lookup(textLogServiceDepl, e)
	}
	return []string{m.line(detailMargin, kittui.StyleActive, text, width)}
}

// deployLogView draws the page of a deploy service in width by height cells.
func (m *Model) deployLogView(width, height int) string {
	p, t := &m.sys.hlog, m.text
	head := m.logServiceLine(width)
	d := p.dep[p.svc]
	switch {
	case d == nil || !d.read:
		head = append(head, m.line(detailMargin, kittui.StyleHint, t.Lookup(textLogReading), width))
	case d.err != nil:
		head = append(head, m.wrapped(detailMargin, kittui.StyleWarn, m.deployLogNote(p.svc, d.err), width)...)
	default:
		head = append(head, "")
		head = head[:min(len(head), max(height-minListRows, 0))]
		view := m.listView(p.dlist, width, max(height-len(head), 1), len(head))
		return strings.Join(append(head, view), "\n")
	}
	return strings.Join(head[:min(len(head), height)], "\n")
}

// saveShownLog saves the entries of the service the log page shows as a file; ok is false when it shows none.
func (m *Model) saveShownLog() (ok bool) {
	s := m.sys
	if m.area != areaSystem || !s.loaded || s.page != pageLog || s.changes != nil {
		return false
	}
	rows, subject := s.hlog.shownRows(), "host"
	if s.hlog.svc != "" {
		subject = s.hlog.svc
	}
	if len(rows) == 0 {
		return false
	}
	lines := make([]string, len(rows))
	for i, r := range rows {
		lines[i] = strings.Join(r, "\t")
	}
	m.saveText(userfile.Name("actionlog", subject, m.now().In(m.loc), ".log"), strings.Join(lines, "\n")+"\n")
	return true
}
