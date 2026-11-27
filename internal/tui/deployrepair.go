package tui

import (
	"errors"

	tea "charm.land/bubbletea/v2"

	"github.com/castrowithcee/qatlas-kit/localipc"
	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/client"
)

// deployDown is why a deploy service that is set up does not answer; zero while it answers or does not exist.
type deployDown int

const (
	downNone deployDown = iota
	// downNotRunning means nothing listens on the channel of the service.
	downNotRunning
	// downRefused means the channel refused this connection.
	downRefused
)

// markUnreachable decides whether a failed refresh means that the service of the system engine is set up but does
// not answer. Every other failure stays an error of the message line.
func (m *Model) markUnreachable(err error) bool {
	down := downNone
	if m.deploy != nil && m.deploy.Installed() {
		switch {
		case errors.Is(err, localipc.ErrRefused):
			down = downRefused
		case errors.Is(err, client.ErrNotRunning):
			down = downNotRunning
		}
	}
	if down == downNone {
		m.outage = outage{}
		return false
	}
	m.down = down
	return true
}

// outage is why the areas show a deploy service that is set up and does not answer: down is the cause, repairChecked
// is set once the setup was asked about it and repairEmpty when it has no step for it.
type outage struct {
	down                       deployDown
	repairChecked, repairEmpty bool
}

// clearOutage ends the states of a missing and of an unreachable service; the next failed refresh may set one again.
func (m *Model) clearOutage() {
	m.notSetUp, m.outage = false, outage{}
}

// refreshed takes a refresh and returns what follows it: a read that was asked for meanwhile and the question to
// the setup about a new outage.
func (m *Model) refreshed(msg refreshMsg) tea.Cmd {
	m.apply(msg)
	if m.stale {
		m.stale = false
		return tea.Batch(m.reload(), m.checkRepair())
	}
	return m.checkRepair()
}

// repairPlanMsg is the result of asking the setup what is left to repair.
type repairPlanMsg struct{ empty bool }

func (r repairPlanMsg) applyTo(m *Model) { m.repairEmpty = r.empty && m.down != downNone }

// checkRepair asks the setup once per outage whether it has steps, so the area can say that it has none. A plan that
// fails counts as having steps: G shows the refusal.
func (m *Model) checkRepair() tea.Cmd {
	if m.down == downNone || m.repairChecked || m.deploy == nil {
		return nil
	}
	m.repairChecked = true
	d, ctx := m.deploy, m.ctx
	return func() tea.Msg {
		plan, err := d.Plan(ctx, "")
		return repairPlanMsg{empty: err == nil && len(plan.Commands) == 0}
	}
}

// downBody says why the service does not answer and what to do about it.
func (m *Model) downBody() string {
	cause := textDownNotRunning
	if m.down == downRefused {
		cause = textDownRefused
	}
	next := textDownRepair
	if m.repairEmpty {
		next = textDownNothing
	}
	return m.text.Lookup(cause) + "\n" + m.text.Lookup(next)
}

// setupHints is the footer key of G: it sets the service up, or repairs one that is set up and does not answer.
// An outage the setup has no step for offers nothing.
func (m *Model) setupHints() []kittui.Hint {
	if m.repairEmpty {
		return nil
	}
	label := textKeySetupDeploy
	if m.down != downNone {
		label = textKeyRepairDeploy
	}
	return []kittui.Hint{{Action: actSetupDeploy, Label: label, Priority: 3}}
}
