package tui

import (
	"slices"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-kit/i18n"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// grantFor is a valid grant of ada (UID 1000) for the action on the stack.
func grantFor(stack, action string) control.GrantInfo {
	return control.GrantInfo{Grant: control.Grant{ID: "g1", User: "ada", UID: 1000, Action: action, Stack: stack}}
}

// grantStackModel is the Stacks area of ada without root; grants are what the fake service lists.
func grantStackModel(t *testing.T, b *fakeBackend) (*Model, *fakeRoot) {
	t.Helper()
	m, f := sudoStackModel(t, i18n.English, b, wideWidth, 60)
	m.human = func() int { return 1000 }
	return m, f
}

// With a valid grant of the person, each covered action asks without a sudo line and calls the service; without
// one, or with a grant that does not fit, it opens the sudo question as before.
func TestCoveredStackActionCallsTheServiceWithoutSudo(t *testing.T) {
	for _, tt := range []struct {
		name, action, keys, call, done string
		taps                           int
	}{
		{"restart", control.GrantActionStackRestart, "Z", "restart:demo-stack", "restarted", 1},
		{"update", control.GrantActionUpdateRun, "U", "update:demo-stack", "APP_VERSION: 1.4.2 -> 1.5.0", 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := demoBackend()
			b.grants = map[string][]control.GrantInfo{"demo-stack": {grantFor("demo-stack", tt.action)}}
			m, f := grantStackModel(t, b)
			send(m, tt.keys)
			screen := m.render()
			if strings.Contains(screen, "sudo") || m.modal == nil {
				t.Fatalf("the covered question names sudo:\n%s", screen)
			}
			if len(b.actions()) != 0 {
				t.Fatalf("the service ran before the answer: %v", b.actions())
			}
			send(m, "y")
			if got := b.actions(); !slices.Equal(got, []string{tt.call}) {
				t.Fatalf("calls %v, want %s", got, tt.call)
			}
			if len(f.reqs) != 0 || len(f.execs) != 0 {
				t.Fatalf("sudo ran: %+v", f.reqs)
			}
			if !strings.Contains(flat(m.render()), tt.done) {
				t.Errorf("no result %q:\n%s", tt.done, m.render())
			}
		})
	}
}

func TestStackActionWithoutCoveringGrantKeepsSudo(t *testing.T) {
	other := grantFor("demo-stack", control.GrantActionStackRestart)
	other.UID = 1001
	expired := grantFor("demo-stack", control.GrantActionStackRestart)
	expired.Invalid = control.GrantExpired
	for name, grants := range map[string][]control.GrantInfo{"none": nil, "other user": {other}, "expired": {expired},
		"other action": {grantFor("demo-stack", control.GrantActionUpdateCheck)}} {
		t.Run(name, func(t *testing.T) {
			b := demoBackend()
			b.grants = map[string][]control.GrantInfo{"demo-stack": grants}
			m, f := grantStackModel(t, b)
			send(m, "Z")
			if !shows(m.render(), "sudo qhost stack restart --yes --output json -- demo-stack") {
				t.Fatalf("no sudo question:\n%s", m.render())
			}
			send(m, "y")
			if len(f.reqs) == 0 || len(b.actions()) != 0 {
				t.Fatalf("requests %v, calls %v", f.reqs, b.actions())
			}
		})
	}
}

// An action over several stacks never uses a grant, and a refusal of the service is the result.
func TestGrantDoesNotCoverTickedStacksAndShowsRefusal(t *testing.T) {
	b := demoBackend()
	b.grants = map[string][]control.GrantInfo{"demo-stack": {grantFor("demo-stack", control.GrantActionStackRestart)},
		"mail-relay": {grantFor("mail-relay", control.GrantActionStackRestart)}}
	m, f := grantStackModel(t, b)
	send(m, "space", "down", "space", "Z")
	if !strings.Contains(flat(m.render()), "sudo qhost stack restart") || len(f.reqs) != 0 || b.called("grantlist") != 0 {
		t.Fatalf("ticked stacks did not use sudo:\n%s", m.render())
	}
	send(m, "n")

	b.errs = map[string]error{"restart:demo-stack": &control.Error{Code: control.CodeStackAction, Message: "refused"}}
	m, f = grantStackModel(t, b)
	send(m, "Z", "y")
	if len(f.reqs) != 0 || !strings.Contains(flat(m.render()), "0 ok, 1 failed") {
		t.Fatalf("the refusal is not the result:\n%s", m.render())
	}
}

// The check of updates names the one stack of a host that has one.
func TestCoveredCheckOfTheOnlyStack(t *testing.T) {
	b := demoBackend()
	b.status.Engine.Stacks = b.status.Engine.Stacks[:1]
	b.stacks.Stacks = b.stacks.Stacks[:1]
	b.updates.Stacks, b.check.Stacks = b.updates.Stacks[:1], nil
	b.grants = map[string][]control.GrantInfo{"demo-stack": {grantFor("demo-stack", control.GrantActionUpdateCheck)}}
	m, f := grantStackModel(t, b)
	send(m, "u")
	if strings.Contains(m.render(), "sudo") {
		t.Fatalf("the covered check names sudo:\n%s", m.render())
	}
	send(m, "y")
	if got := b.actions(); !slices.Equal(got, []string{"refresh:*"}) || len(f.reqs) != 0 {
		t.Fatalf("calls %v, requests %v", got, f.reqs)
	}

	// With several stacks the check is over all of them and stays with sudo.
	b = demoBackend()
	b.grants = map[string][]control.GrantInfo{"demo-stack": {grantFor("demo-stack", control.GrantActionUpdateCheck)}}
	m, _ = grantStackModel(t, b)
	send(m, "u")
	if !strings.Contains(flat(m.render()), "sudo qhost stack updates --refresh") {
		t.Fatalf("no sudo question:\n%s", m.render())
	}
}

// A restart of a container of an own stack is covered by container.restart; a single one is not.
func TestCoveredContainerRestart(t *testing.T) {
	b := containersBackend()
	b.grants = map[string][]control.GrantInfo{"demo-stack": {grantFor("demo-stack", control.GrantActionContainerRestart)}}
	m, f := sudoCtModel(t, b, ctReport("demo-stack-app-1"))
	m.human = func() int { return 1000 }
	selectContainer(t, m, "demo-stack-app-1")
	send(m, "Z")
	if strings.Contains(m.render(), "sudo") || m.modal == nil {
		t.Fatalf("the covered question names sudo:\n%s", m.render())
	}
	send(m, "y")
	if got := b.actions(); !slices.Equal(got, []string{"ctrestart:0123456789ab"}) || len(f.reqs) != 0 {
		t.Fatalf("calls %v, requests %v", got, f.reqs)
	}

	for _, name := range []string{"example-cache"} {
		b.acts = nil
		send(m, "esc")
		selectContainer(t, m, name)
		send(m, "Z")
		if !strings.Contains(flat(m.render()), "sudo qhost container restart") {
			t.Fatalf("%s: no sudo question:\n%s", name, m.render())
		}
		send(m, "n")
	}

	// Without a grant the question names sudo.
	b.grants = nil
	send(m, "esc")
	selectContainer(t, m, "demo-stack-app-1")
	send(m, "Z")
	if !strings.Contains(flat(m.render()), "sudo qhost container restart") {
		t.Fatalf("no sudo question without a grant:\n%s", m.render())
	}
}
