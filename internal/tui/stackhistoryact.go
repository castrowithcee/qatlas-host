package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// historyDiffMsg carries the diff of the commit the view asked for.
type historyDiffMsg struct {
	gen int
	res control.SourceShowResult
	err error
}

func (msg historyDiffMsg) applyTo(m *Model) {
	h := &m.sf.history
	if h.diff == nil || msg.gen != h.diffGen {
		return
	}
	m.fillDiff(h.diff, msg.res.Diff, msg.res.Truncated, msg.err)
}

// askShow shows the diff of the marked commit: as root at once, otherwise after the question of sudo qhost source
// show. The diff is shown only in the text view, as in the changes.
func (m *Model) askShow() tea.Cmd {
	c, ok := m.selectedCommit()
	hb, can := m.backend.(HistoryBackend)
	if !ok || !can {
		return nil
	}
	h, t := &m.sf.history, m.text
	f := h.folder
	begin := func() int {
		h.diffGen++
		h.diff = &changeDiff{file: short(c.ID) + " " + c.Subject, reading: true}
		return h.diffGen
	}
	if m.euid() == 0 {
		gen, ctx := begin(), m.engineCtx(f.Engine)
		return func() tea.Msg {
			res, err := hb.SourceShow(ctx, f.Path, c.ID)
			return historyDiffMsg{gen: gen, res: res, err: err}
		}
	}
	var gen int
	m.askSudo(sudoAsk{title: textTitleShow, question: t.Lookup(textAskShow, short(c.ID)), consequence: t.Lookup(textWhatShow),
		args:  m.engineArgs(f.Engine, []string{"source", "show", "--output", "json", "--", f.Path, c.ID}),
		begin: func() { gen = begin() },
		done: func(r sudoRun) tea.Cmd {
			msg := historyDiffMsg{gen: gen}
			if !r.decode(&msg.res) || msg.res.Commit == "" {
				msg.err = sudoProblem(r.problem)
			}
			msg.applyTo(m)
			return nil
		}})
	return nil
}
