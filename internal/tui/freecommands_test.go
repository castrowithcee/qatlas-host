package tui

import (
	"slices"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-kit/i18n"
	kittui "github.com/castrowithcee/qatlas-kit/tui"
	"github.com/castrowithcee/qatlas-kit/uiprefs"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

const propFree = "h-0005"

// demoFreeProposals are the invented host proposals plus one proposal of free commands, with the switch as given.
func demoFreeProposals(t *testing.T, on bool) *fakeHostProposals {
	t.Helper()
	h := demoHostProposals(t)
	s := at(30, 18, 0)
	h.list = append(h.list, control.ProposalInfo{ID: propFree, Kind: control.ProposalKindCommand, State: control.ProposalOpen,
		Fingerprint: "fp-" + propFree, SubmitterUID: 1001, SubmitterName: "ben", Submitted: s, Expires: s.Add(control.ProposalLifetime)})
	h.free = map[string]control.ProposalCommand{propFree: {
		Commands: []string{"systemctl restart demo-app.service", "echo cleanup done", "rm -f /var/tmp/demo-cache.tmp"},
		Reason:   "Restart the demo app after the config change.",
	}}
	h.freeOn = on
	return h
}

// toFreeProposal opens the proposals page with the switch read and the free proposal under the cursor.
func toFreeProposal(t *testing.T, m *Model, h *fakeHostProposals) {
	t.Helper()
	toProposalsPage(t, m, h)
	markProposal(t, m, propFree)
}

// freeReport is what `proposal run` prints when the second of three commands fails.
const freeReport = `{"proposal_id":"h-0005","journal_id":"20261102T090000-0007","result":"failed","notice":"unchecked, no computed effect, no undo",` +
	`"commands":[{"command":"systemctl restart demo-app.service","ran":true,"exit_code":0,"output":"","truncated":false,"timed_out":false},` +
	`{"command":"echo cleanup done","ran":true,"exit_code":3,"output":"cleanup failed: demo-cache busy","truncated":false,"timed_out":false},` +
	`{"command":"rm -f /var/tmp/demo-cache.tmp","ran":false,"exit_code":-1,"output":"","truncated":false,"timed_out":false}]}`

// readSwitch reads the switch of the fake host service, as the start does.
func readSwitch(m *Model, h *fakeHostProposals) {
	m.hostProps = h
	drain(m, m.readFreeSwitch())
}

func TestGoldenSettingsFreeSwitchOff(t *testing.T) {
	goldenSystem(t, "settings.free.off", func(t *testing.T, m *Model) {
		readSwitch(m, demoFreeProposals(t, false))
		send(m, "4", "down", "down", "down", "down", "down", "down", "down")
	})
}

func TestGoldenSettingsFreeSwitchOn(t *testing.T) {
	goldenSystem(t, "settings.free.on", func(t *testing.T, m *Model) {
		readSwitch(m, demoFreeProposals(t, true))
		send(m, "4", "down", "down", "down", "down", "down", "down", "down")
	})
}

func TestGoldenSystemProposalsFreeList(t *testing.T) {
	goldenSystem(t, "system.proposals.free", func(t *testing.T, m *Model) { toFreeProposal(t, m, demoFreeProposals(t, true)) })
}

func TestGoldenSystemProposalFreeDetail(t *testing.T) {
	goldenSystem(t, "system.proposal.free", func(t *testing.T, m *Model) {
		toFreeProposal(t, m, demoFreeProposals(t, true))
		send(m, "enter")
	})
}

func TestGoldenSystemProposalFreeDetailSwitchOff(t *testing.T) {
	goldenSystem(t, "system.proposal.free.off", func(t *testing.T, m *Model) {
		toFreeProposal(t, m, demoFreeProposals(t, false))
		send(m, "enter")
	})
}

func TestGoldenSystemProposalFreeQuestion(t *testing.T) {
	goldenSystem(t, "system.proposal.free.ask", func(t *testing.T, m *Model) {
		toFreeProposal(t, m, demoFreeProposals(t, true))
		send(m, "A")
	})
}

// goldenFreeResult is goldenSystem for a run whose report comes from the fake root.
func TestGoldenSystemProposalFreeResult(t *testing.T) {
	for name, lang := range goldenLangs {
		for size, dim := range goldenFormSizes {
			t.Run(name+"."+size, func(t *testing.T) {
				width, rows := dim[0], dim[1]
				m, f := systemModel(t, lang, width, rows)
				toFreeProposal(t, m, demoFreeProposals(t, true))
				f.report, f.runErr = freeReport, exitError(t, 1)
				send(m, "A", "y")
				screen := m.render()
				checkScreen(t, screen, width, rows)
				checkGolden(t, "system.proposal.free.result."+name+"."+size+".golden", screen+"\n")
			})
		}
	}
}

func TestFreeRunAsksWithTheCountAndRunsExactlyTheProposedArguments(t *testing.T) {
	m, f := systemModel(t, i18n.English, wideWidth, 40)
	toFreeProposal(t, m, demoFreeProposals(t, true))
	f.report = freeReport
	f.runErr = exitError(t, 1)
	send(m, "A")
	screen := flat(m.render())
	for _, want := range []string{"Run the 3 commands of proposal h-0005 as root?", "nothing checks them",
		"$ sudo qhost proposal run h-0005 --fingerprint fp-h-0005 --confirm --output json"} {
		if !strings.Contains(screen, want) {
			t.Fatalf("the question lacks %q:\n%s", want, screen)
		}
	}
	if len(f.reqs) != 0 {
		t.Fatalf("something ran before the answer: %v", f.reqs)
	}
	send(m, "y")
	want := []string{"sudo-fake", "qhost-fake", "proposal", "run", propFree, "--fingerprint", "fp-" + propFree, "--confirm", "--output", "json"}
	if len(f.execs) != 1 || !slices.Equal(f.execs[0].Args, want) {
		t.Fatalf("commands %v, want %q", f.execs, want)
	}
	for _, a := range f.execs[0].Args {
		if strings.Contains(a, "systemctl") || strings.Contains(a, "rm -f") {
			t.Fatalf("a proposed command reached the argument list: %q", a)
		}
	}
}

// A failed command ends the run: the sheet names each command with its exit code and output and those that did
// not run, although the command itself exits with a failure after printing the report.
func TestFreeRunShowsExitCodeAndOutputPerCommandAndWhatDidNotRun(t *testing.T) {
	m, f := systemModel(t, i18n.English, wideWidth, 40)
	h := demoFreeProposals(t, true)
	toFreeProposal(t, m, h)
	f.report = freeReport
	f.runErr = exitError(t, 1)
	send(m, "A", "y")
	screen := flat(m.render())
	for _, want := range []string{"command 2 of 3 did not succeed, 1 did not run", "1. systemctl restart demo-app.service", "exit code 0",
		"2. echo cleanup done", "exit code 3", "cleanup failed: demo-cache busy", "3. rm -f /var/tmp/demo-cache.tmp", "did not run",
		"20261102T090000-0007"} {
		if !strings.Contains(screen, want) {
			t.Fatalf("the result lacks %q:\n%s", want, m.render())
		}
	}
	if h.lists != 2 || !m.sys.jrnl.stale {
		t.Fatalf("the list was read %d times, journal stale %v", h.lists, m.sys.jrnl.stale)
	}
}

func TestFreeRunWithoutReportExplainsTheFailure(t *testing.T) {
	m, f := systemModel(t, i18n.English, wideWidth, 40)
	toFreeProposal(t, m, demoFreeProposals(t, true))
	f.report, f.runErr = "", exitError(t, 3)
	send(m, "A", "y")
	if screen := flat(m.render()); !strings.Contains(screen, "was not changed as asked") {
		t.Fatalf("a run without a report says nothing:\n%s", screen)
	}
}

func TestFreeProposalOffersNoRunWhileTheSwitchIsOff(t *testing.T) {
	m, f := systemModel(t, i18n.English, wideWidth, 40)
	toFreeProposal(t, m, demoFreeProposals(t, false))
	for _, hint := range m.hostProposalFooter(nil, nil, nil) {
		if hint.Action == actApprove {
			t.Fatalf("the footer offers a run: %+v", hint)
		}
	}
	send(m, "A")
	if m.modal != nil || len(f.reqs) != 0 || !strings.Contains(flat(m.render()), "Running is off") {
		t.Fatalf("a key ran or asked while the switch is off:\n%s", m.render())
	}
	send(m, "enter")
	if m.modal == nil || m.modal.yes != nil {
		t.Fatalf("the view of the proposal offers a yes while the switch is off:\n%s", m.render())
	}
	screen := flat(m.render())
	if !strings.Contains(screen, "unchecked, no computed effect, no undo") || !strings.Contains(screen, "Settings, General") {
		t.Fatalf("the view lacks the notice or the name of the switch:\n%s", screen)
	}
	send(m, "y")
	if len(f.reqs) != 0 {
		t.Fatalf("something ran: %v", f.reqs)
	}
}

func TestFreeProposalOffersRunWhileTheSwitchIsOn(t *testing.T) {
	m, _ := systemModel(t, i18n.English, wideWidth, 40)
	toFreeProposal(t, m, demoFreeProposals(t, true))
	if !slices.ContainsFunc(m.hostProposalFooter(nil, nil, nil), func(h kittui.Hint) bool { return h.Action == actApprove }) {
		t.Fatal("the footer offers no run while the switch is on")
	}
	send(m, "enter")
	if m.modal == nil || m.modal.yes == nil {
		t.Fatalf("the view offers no yes while the switch is on:\n%s", m.render())
	}
}

func TestFreeProposalShowsControlCharactersEscapedInCommandsAndReason(t *testing.T) {
	m, _ := systemModel(t, i18n.English, wideWidth, 40)
	h := demoFreeProposals(t, true)
	h.free[propFree] = control.ProposalCommand{
		Commands: []string{"echo \x1b[31mred\x1b[0m", "printf 'a\x07b'"},
		Reason:   "look\x1b]0;title\x07 here",
	}
	toFreeProposal(t, m, h)
	send(m, "enter")
	raw := m.render()
	for _, bad := range []string{"\x1b[31m", "\x1b]0", "\x07"} {
		if strings.Contains(raw, bad) {
			t.Fatalf("a control character reached the screen: %q", bad)
		}
	}
	screen := flat(raw)
	for _, want := range []string{`> look\x1b]0;title\a here`, `1. echo \x1b[31mred\x1b[0m`, `2. printf 'a\ab'`} {
		if !strings.Contains(screen, want) {
			t.Fatalf("the view lacks %q:\n%s", want, raw)
		}
	}
}

func TestFreeRunResultEscapesTheOutputOfACommand(t *testing.T) {
	m, f := systemModel(t, i18n.English, wideWidth, 40)
	toFreeProposal(t, m, demoFreeProposals(t, true))
	f.report = `{"journal_id":"j-1","result":"applied","commands":[{"command":"echo x","ran":true,"exit_code":0,` +
		`"output":"line\u001b[2Jone","truncated":true,"timed_out":false}]}`
	send(m, "A", "y")
	raw := m.render()
	if strings.Contains(raw, "\x1b[2J") {
		t.Fatalf("a control character of the output reached the screen")
	}
	if screen := flat(raw); !strings.Contains(screen, `line\x1b[2Jone`) || !strings.Contains(screen, "The output is cut.") {
		t.Fatalf("the output is not shown escaped and marked as cut:\n%s", raw)
	}
}

func TestFreeProposalIsMarkedInTheListInTheWarningTone(t *testing.T) {
	m, _ := systemModel(t, i18n.English, wideWidth, 40)
	toFreeProposal(t, m, demoFreeProposals(t, true))
	if !strings.Contains(m.render(), "free commands") || m.sys.prop.tones[m.text.Lookup(textPropKindFree)] != kittui.StyleWarn {
		t.Fatalf("no warning mark for the free proposal:\n%s", m.render())
	}
	if _, marked := m.sys.prop.tones[m.text.Lookup(textPropKindHostChange)]; marked {
		t.Fatal("a change of the host is marked like free commands")
	}
}

func TestFreeSwitchIsAFieldOfGeneralOnlyOnceItWasRead(t *testing.T) {
	m, _ := systemModel(t, i18n.English, wideWidth, 40)
	send(m, "4")
	if strings.Contains(m.render(), "Free proposals") {
		t.Fatalf("the switch shows without having been read:\n%s", m.render())
	}
}

func TestFreeSwitchOnAsksWithTheConsequenceAndRunsEnableOverSudo(t *testing.T) {
	m, f := systemModel(t, i18n.English, wideWidth, 40)
	readSwitch(m, demoFreeProposals(t, false))
	m.savePrefs = func(p uiprefs.Prefs) error { return nil }
	send(m, "4", "down", "down", "down", "down", "down", "down", "down")
	contains(t, m, "Free proposals", "does not check and cannot undo")
	f.report = `{"enabled":true}`
	send(m, "left", "f2")
	screen := flat(m.render())
	for _, want := range []string{"Allow agents to propose free commands?", "$ sudo qhost proposal commands enable --output json", "cannot undo"} {
		if !strings.Contains(screen, want) {
			t.Fatalf("the question lacks %q:\n%s", want, screen)
		}
	}
	if len(f.reqs) != 0 {
		t.Fatalf("something ran before the answer: %v", f.reqs)
	}
	send(m, "y")
	want := []string{"sudo-fake", "qhost-fake", "proposal", "commands", "enable", "--output", "json"}
	if len(f.execs) != 1 || !slices.Equal(f.execs[0].Args, want) {
		t.Fatalf("commands %v, want %q", f.execs, want)
	}
	if !m.free.enabled || !strings.Contains(flat(m.render()), "Proposals of free commands are on.") {
		t.Fatalf("the switch is not on:\n%s", m.render())
	}
}

func TestFreeSwitchOffRunsDisableOverSudo(t *testing.T) {
	m, f := systemModel(t, i18n.English, wideWidth, 40)
	readSwitch(m, demoFreeProposals(t, true))
	m.savePrefs = func(p uiprefs.Prefs) error { return nil }
	send(m, "4", "down", "down", "down", "down", "down", "down", "down")
	f.report = `{"enabled":false}`
	send(m, "right", "f2", "y")
	want := []string{"sudo-fake", "qhost-fake", "proposal", "commands", "disable", "--output", "json"}
	if len(f.execs) != 1 || !slices.Equal(f.execs[0].Args, want) {
		t.Fatalf("commands %v, want %q", f.execs, want)
	}
	if m.free.enabled {
		t.Fatal("the switch is still on")
	}
}

func TestFreeSwitchUnchangedAsksNothing(t *testing.T) {
	m, f := systemModel(t, i18n.English, wideWidth, 40)
	readSwitch(m, demoFreeProposals(t, true))
	m.savePrefs = func(p uiprefs.Prefs) error { return nil }
	send(m, "4", "down", "f2")
	if m.modal != nil || len(f.reqs) != 0 {
		t.Fatalf("saving without a change of the switch asked or ran something: %v", f.reqs)
	}
}

func TestChangesOfFreeCommandsOfferNoUndo(t *testing.T) {
	m, f := systemModel(t, i18n.English, wideWidth, 40)
	toChangesPage(t, m)
	f.report = `[{"id":"20261102T090000.000000Z-aaaaaa","time":"2026-11-02T09:00:00Z","action":"proposal.run",` +
		`"sentence":"Run 3 unchecked commands of a proposal.","result":"failed","reversible":false,"sudo_uid":1000,` +
		`"undo_sentence":"","undo_commands":[]}]`
	send(m, "R")
	if e, ok := m.selectedJournal(); !ok || e.undoable() {
		t.Fatalf("the entry of free commands offers an undo: %+v", e)
	}
	asked := len(f.reqs)
	send(m, "enter")
	if screen := flat(m.render()); !strings.Contains(screen, "Free commands are not checked") || !strings.Contains(screen, "cannot be undone") {
		t.Fatalf("no reason why there is no undo:\n%s", screen)
	}
	send(m, "y")
	if len(f.reqs) != asked {
		t.Fatalf("an undo ran: %v", f.reqs[asked:])
	}
}

func TestFreeTextsExistInBothLanguages(t *testing.T) {
	for _, lang := range []i18n.Lang{i18n.English, i18n.German} {
		m, _ := systemModel(t, lang, wideWidth, 40)
		for id := range freeEnglish() {
			if m.text.Lookup(id) == "" {
				t.Errorf("%s: no text for %s", lang, id)
			}
		}
	}
	if len(freeEnglish()) != len(freeGerman()) {
		t.Errorf("%d English texts, %d German", len(freeEnglish()), len(freeGerman()))
	}
}
