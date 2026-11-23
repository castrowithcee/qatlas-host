package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// Without root, an action that a standing grant covers calls the deploy service directly, as root does; the service
// decides again on the request, so a grant withdrawn meanwhile ends in its refusal, shown as the result.

// grantActions maps the actions a grant can cover to its name.
var grantActions = map[actionKind]string{kindRestart: control.GrantActionStackRestart,
	kindCheck: control.GrantActionUpdateCheck, kindUpdate: control.GrantActionUpdateRun}

// grantCoverMsg answers whether a grant covers an action; then goes on with it.
type grantCoverMsg struct {
	covered bool
	then    func(covered bool) tea.Cmd
}

// grantCovers reads the grants of the stack now and reports whether one is valid for the person behind the
// interface and the action. The person is told apart by user ID, not by name.
func grantCovers(b Backend, uid int, call func(Backend) (control.GrantListResult, error), action, stack string) bool {
	res, err := call(b)
	if err != nil || res.Problem != "" {
		return false
	}
	for _, g := range res.Grants {
		if g.Invalid == "" && int(g.UID) == uid && g.Action == action && g.Stack == stack {
			return true
		}
	}
	return false
}

// coverThen goes on with then(covered): at once as root or when the person is unknown, else after the grants of the
// stack are read.
func (m *Model) coverThen(engine, stack, action string, then func(covered bool) tea.Cmd) tea.Cmd {
	uid := m.humanUID()
	if m.euid() == 0 || uid == 0 {
		return then(false)
	}
	ctx, b := m.engineCtx(engine), m.backend
	return func() tea.Msg {
		covered := grantCovers(b, uid, func(b Backend) (control.GrantListResult, error) { return b.GrantList(ctx, stack) },
			action, stack)
		return grantCoverMsg{covered: covered, then: then}
	}
}

// applyGrantCover goes on unless the interface moved on meanwhile.
func (m *Model) applyGrantCover(msg grantCoverMsg) tea.Cmd {
	if m.modal != nil || m.busy() {
		return nil
	}
	return msg.then(msg.covered)
}

// onlyStack is the one stack of a host that has one, which the check of updates may then name.
func (m *Model) onlyStack() (name, engine string, ok bool) {
	for _, r := range m.rows {
		if ok {
			return "", "", false
		}
		name, engine, ok = r.name, r.engine, true
	}
	return name, engine, ok
}
