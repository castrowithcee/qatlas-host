package tui

import (
	tea "charm.land/bubbletea/v2"

	kittui "github.com/castrowithcee/qatlas-kit/tui"
)

// beginPlan asks for the confirmation of the plan: a covered action calls the service, an update shows its preview
// first, and without root and a covering grant the question runs the commands through sudo. stack and engine name
// the stack of a covered check.
func (m *Model) beginPlan(p plan, stack, engine string, covered bool) tea.Cmd {
	if covered && p.kind == kindCheck {
		p.only, p.engines = stack, []string{engine}
	}
	if p.kind == kindUpdate {
		return m.openPreview(p, covered)
	}
	if m.euid() != 0 && !covered {
		m.askSudoPlan(p)
		return nil
	}
	dialog, err := m.confirmDialog(p, nil)
	if err != nil {
		m.failure = m.text.Lookup(kittui.TextError, err.Error())
		return nil
	}
	m.modal = &modal{kind: modalAsk, screen: p.kind.screen(), dialog: dialog, plan: p, yes: func() tea.Cmd { return m.start(p) }}
	return nil
}
