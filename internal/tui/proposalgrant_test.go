package tui

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-kit/i18n"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

const grantRow = 0

// grantContent is the content of a grant proposal of the user ana for the stack demo.
func grantContent(action, expires, reason string) string {
	raw, _ := json.Marshal(control.ProposalGrant{User: "ana", UID: 1001, Action: action, Stack: "demo", Expires: expires, Reason: reason})
	return string(raw)
}

// grantBackend holds one open grant proposal, newest first, and a stack demo with a backup and no check.
func grantBackend(content string) *fakeBackend {
	b := proposalBackend()
	s := at(30, 18, 0)
	b.proposals = []control.ProposalInfo{{ID: "p-0020", Kind: control.ProposalKindGrant, State: control.ProposalOpen, Fingerprint: "fp-p-0020",
		SubmitterUID: 1001, SubmitterName: "ana", Submitted: s, Expires: s.Add(control.ProposalLifetime)}}
	b.proposalContents = map[string]string{"p-0020": content}
	b.stackSettings = map[string]control.StackSettings{"demo": {Version: 1,
		Backup: &control.StackBackupSettings{Paths: []string{"data"}}}}
	return b
}

// clearField empties the text field of the open form, which starts with the expiry asked for.
func clearField(m *Model) { send(m, slices.Repeat([]string{"backspace"}, 12)...) }

const grantReason = "Our deploy bot restarts the demo app after each release."

func grantOpen(t *testing.T, b *fakeBackend, root bool) *Model {
	t.Helper()
	return proposalOpen(t, b, root, grantRow)
}

func TestGrantViewShowsTheSentenceReasonAndStateOfTheStack(t *testing.T) {
	m := grantOpen(t, grantBackend(grantContent(control.GrantActionUpdateRun, "2030-01-31", grantReason)), false)
	contains(t, m, "standing grant", "ana (uid 1001) may update the stack demo without sudo", "expiry asked for: after the end of 2030-01-31 (UTC)",
		"> "+grantReason, "backup of the stack: set up", "health check of the stack: not set up", "commits and pushes to the source")
	send(m, "esc")
	contains(t, m, "standing grant")
}

func TestGrantViewMasksControlCharactersOfTheReason(t *testing.T) {
	m := grantOpen(t, grantBackend(grantContent(control.GrantActionStackRestart, "", "look\x1b]0;title\x07 here\nsecond")), false)
	screen := m.render()
	if strings.ContainsAny(screen, "\x1b\x07") {
		t.Fatalf("control characters reach the screen:\n%q", screen)
	}
	contains(t, m, `> look\x1b]0;title\a here`, "> second")
	if strings.Contains(screen, "commits and pushes") {
		t.Fatal("a restart grant warns about pushing")
	}
}

func TestGrantOfAStackThatCannotBeReadShowsUnknown(t *testing.T) {
	b := grantBackend(grantContent(control.GrantActionStackRestart, "", grantReason))
	b.errs = map[string]error{"settings:demo": control.NewError(control.CodeInternal, "boom")}
	m := grantOpen(t, b, false)
	contains(t, m, "backup of the stack: unknown", "health check of the stack: unknown")
	if !m.canApprove() {
		t.Fatal("an unknown state stops the approval")
	}
}

func TestGrantOfAnUnknownStackCannotBeApproved(t *testing.T) {
	b := grantBackend(grantContent(control.GrantActionStackRestart, "", grantReason))
	b.errs = map[string]error{"settings:demo": control.NewError(control.CodeNoSuchStack, "no such stack")}
	m := grantOpen(t, b, false)
	contains(t, m, "The stack demo does not exist")
	if m.canApprove() {
		t.Fatal("approving is offered")
	}
}

func TestGrantWithAnUnknownActionCannotBeShown(t *testing.T) {
	m := grantOpen(t, grantBackend(grantContent("stack.delete", "", grantReason)), false)
	if m.canApprove() {
		t.Fatal("approving is offered")
	}
	contains(t, m, "cannot be read")
}

func TestGrantApproveRunsTheCommandWithFingerprintAndExpiry(t *testing.T) {
	b := grantBackend(grantContent(control.GrantActionStackRestart, "2030-01-31", grantReason))
	m, f := sudoProposalOpen(t, i18n.English, b, grantRow, map[string]string{
		"proposal approve": `{"proposal":"p-0020","id":"g-0a1b2c3d","user":"ana","action":"stack.restart","stack":"demo","engine":"system","expires":"2030-01-15","replaced":false,"backup":true,"check":false,"requested_expires":"2030-01-31"}`})
	send(m, "e")
	clearField(m)
	send(m, typed("2030-01-15")...)
	send(m, "f2")
	if m.set.form != nil {
		t.Fatalf("the form stays open:\n%s", m.render())
	}
	contains(t, m, "expiry of the grant: after the end of 2030-01-15 (UTC)", "You set this expiry")
	send(m, "A")
	args := []string{"proposal", "approve", "p-0020", "--fingerprint", "fp-p-0020", "--expires", "2030-01-15", "--confirm", "--output", "json"}
	says(t, m, "Approve proposal p-0020 and add this grant?", "> "+grantReason, "backup of the stack: set up")
	showsCommand(t, m, args)
	if len(f.reqs) != 0 {
		t.Fatalf("sudo ran before the answer: %+v", f.reqs)
	}
	send(m, "y")
	wantCommands(t, m, f, args)
	contains(t, m, "The grant g-0a1b2c3d for ana is added")
	if got := b.actions(); slices.ContainsFunc(got, func(a string) bool { return strings.HasPrefix(a, "proposalapprove") }) {
		t.Fatalf("the service was called directly: %v", got)
	}
}

func TestGrantApproveWithoutDateLeavesOutTheFlag(t *testing.T) {
	m, f := sudoProposalOpen(t, i18n.English, grantBackend(grantContent(control.GrantActionUpdateCheck, "", grantReason)), grantRow, map[string]string{
		"proposal approve": `{"proposal":"p-0020","id":"g-0a1b2c3d","user":"ana","action":"update.check","stack":"demo","expires":"","replaced":false}`})
	send(m, "A", "y")
	wantCommands(t, m, f, []string{"proposal", "approve", "p-0020", "--fingerprint", "fp-p-0020", "--confirm", "--output", "json"})
}

func TestGrantExpiryRefusesWhatTheCommandWouldRefuse(t *testing.T) {
	for requested, bad := range map[string][]string{
		"2030-01-31": {"2030-02-01", "tomorrow", "2030-1-5", "2030-02-30", "2030-01-15; reboot"},
		"":           {"31.01.2030", "x"},
	} {
		m := grantOpen(t, grantBackend(grantContent(control.GrantActionStackRestart, requested, grantReason)), false)
		for _, v := range bad {
			send(m, "e")
			clearField(m)
			send(m, typed(v)...)
			send(m, "f2")
			if m.set.form == nil {
				t.Fatalf("requested %q: %q is taken", requested, v)
			}
			send(m, "esc")
			if m.pv.kind.expires != "" {
				t.Fatalf("requested %q: %q is kept", requested, v)
			}
		}
	}
	for requested, ok := range map[string]string{"2030-01-31": "2030-01-31", "": "2040-12-31"} {
		if !grantExpiryOK(requested, ok) {
			t.Fatalf("requested %q: %q is refused", requested, ok)
		}
	}
}

func TestGrantExpiryStaysWhenTheProposalIsReadAgain(t *testing.T) {
	m := grantOpen(t, grantBackend(grantContent(control.GrantActionStackRestart, "", grantReason)), false)
	send(m, "e")
	clearField(m)
	send(m, typed("2031-05-06")...)
	send(m, "f2")
	send(m, "R")
	if got := m.pv.kind.expires; got != "2031-05-06" {
		t.Fatalf("expiry %q after reading again", got)
	}
}

func TestGrantCanBeRejected(t *testing.T) {
	b := grantBackend(grantContent(control.GrantActionStackRestart, "", grantReason))
	m := grantOpen(t, b, true)
	send(m, "N", "f2", "y")
	if len(b.proposalRejects) != 1 || b.proposalRejects[0].ID != "p-0020" || len(b.proposalApproves) != 0 {
		t.Fatalf("rejects = %+v", b.proposalRejects)
	}
}

func TestGrantCountsInTheStartNotice(t *testing.T) {
	m := proposalModel(t, i18n.English, grantBackend(grantContent(control.GrantActionStackRestart, "", grantReason)), true, 120, 40)
	contains(t, m, "1 open proposal")
}

func TestGoldenProposalsViewGrant(t *testing.T) { goldenGrant(t, "proposals.view.grant", false) }

func TestGoldenProposalsViewGrantExpiry(t *testing.T) {
	goldenGrant(t, "proposals.view.grant.expiry", true, "e")
}

func TestGoldenProposalsGrantList(t *testing.T) { goldenGrant(t, "proposals.list.grant", false, "6") }

func TestGoldenProposalsGrantApprove(t *testing.T) {
	goldenGrant(t, "proposals.approve.grant", true, "A")
}

// goldenGrant draws the grant proposal after keys; the detail is opened first unless keys start with the list
// alone. With setDate the expiry 2030-01-15 is set before keys.
func goldenGrant(t *testing.T, prefix string, setDate bool, keys ...string) {
	t.Helper()
	listOnly := len(keys) == 1 && keys[0] == "6"
	for name, lang := range goldenLangs {
		for size, dim := range goldenFormSizes {
			t.Run(name+"."+size, func(t *testing.T) {
				b := grantBackend(grantContent(control.GrantActionUpdateRun, "2030-01-31", grantReason))
				m := proposalModel(t, lang, b, false, dim[0], dim[1])
				m.euid = func() int { return 1000 }
				m.root = (&fakeRoot{}).hook()
				send(m, "6")
				if !listOnly {
					send(m, "enter")
				}
				if setDate {
					send(m, "e")
					clearField(m)
					send(m, typed("2030-01-15")...)
					send(m, "f2")
					if len(keys) > 0 && keys[0] == "e" {
						keys = nil
					}
				}
				if !listOnly {
					send(m, keys...)
				}
				screen := m.render()
				checkScreen(t, screen, dim[0], dim[1])
				checkGolden(t, prefix+"."+name+"."+size+".golden", screen+"\n")
			})
		}
	}
}
