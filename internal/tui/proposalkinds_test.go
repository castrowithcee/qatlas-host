package tui

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

const (
	kindSettingsRow = 1
	kindWindowRow   = 0
	kindGhostRow    = 2
)

// backupTail ends a long backup command; the view must show it though it is far beyond any cut.
const backupTail = "end-of-the-command"

func kindBackupCommand() []string {
	return []string{"sh", "-c", "tar czf /backup/demo.tgz /srv/demo && curl -T /backup/demo.tgz https://backup.example.test/upload/" + backupTail}
}

// longBackupCommand is far beyond any length the view cuts values at.
func longBackupCommand() []string {
	c := kindBackupCommand()
	c[2] = strings.Repeat("a-long-path/", 40) + backupTail
	return c
}

func kindStackSettings() control.StackSettings {
	return control.StackSettings{Version: 1,
		Update: &control.StackUpdateSettings{Mode: "automatic", Ceiling: "minor"},
		Backup: &control.StackBackupSettings{Paths: []string{"data"}, Command: kindBackupCommand()}}
}

// kindsBackend holds three open proposals, newest first: a window, the settings of the stack demo and the
// settings of a stack that does not exist.
func kindsBackend() *fakeBackend {
	b := proposalBackend()
	info := func(id, kind string, hour int) control.ProposalInfo {
		s := at(30, hour, 0)
		return control.ProposalInfo{ID: id, Kind: kind, State: control.ProposalOpen, Fingerprint: "fp-" + id, SubmitterUID: 1001,
			SubmitterName: "ana", Submitted: s, Expires: s.Add(control.ProposalLifetime)}
	}
	b.proposals = []control.ProposalInfo{
		info("p-0012", control.ProposalKindWindow, 18),
		info("p-0011", control.ProposalKindStackSettings, 17),
		info("p-0010", control.ProposalKindStackSettings, 16),
	}
	settings := func(stack string) string {
		raw, _ := json.Marshal(control.ProposalStackSettings{Stack: stack, Settings: kindStackSettings()})
		return string(raw)
	}
	b.proposalContents = map[string]string{
		"p-0012": `{"days":["mon","thu"],"start":"02:30"}`,
		"p-0011": settings("demo"),
		"p-0010": settings("ghost"),
	}
	b.stackSettings = map[string]control.StackSettings{"demo": {Version: 1,
		Update: &control.StackUpdateSettings{Mode: "manual", Ceiling: "minor"}}}
	b.errs = map[string]error{"settings:ghost": control.NewError(control.CodeNoSuchStack, "no such stack")}
	return b
}

func TestKindSettingsViewShowsContentAndDifferenceWithTheCommandMarked(t *testing.T) {
	b := kindsBackend()
	m := proposalOpen(t, b, true, kindSettingsRow)
	contains(t, m, "stack settings", "stack: demo", "Proposed", "Effect if applied now", "These settings would change:",
		"update.mode", "before: \"manual\"", "!! backup.command[0]",
		"The backup command runs on the host later", "approve", "reject")
	if b.called("settings") != 1 || len(b.proposalActs) != 0 {
		t.Fatalf("settings reads %d, proposal calls %v", b.called("settings"), b.proposalActs)
	}
}

func TestKindSettingsViewNeverCutsTheBackupCommandOrAnAddress(t *testing.T) {
	b := kindsBackend()
	long := control.ProposalStackSettings{Stack: "demo", Settings: kindStackSettings()}
	long.Settings.Backup.Command = longBackupCommand()
	raw, _ := json.Marshal(long)
	b.proposalContents["p-0011"] = string(raw)
	m := proposalOpen(t, b, true, kindSettingsRow)
	var all strings.Builder
	for _, l := range m.proposalLines(116) {
		all.WriteString(strings.TrimSpace(l))
	}
	// Wrapping may split the text inside a word, so compare without the line breaks it made.
	if got := strings.Count(all.String(), backupTail); got != 2 {
		t.Fatalf("the end of the command is shown %d times, want 2 (content and effect)", got)
	}
	if got := strings.Count(all.String(), strings.Repeat("a-long-path/", 40)); got != 2 {
		t.Fatalf("the long command is shown whole %d times, want 2", got)
	}
}

func TestKindSettingsViewWithoutRootReadsAndOffersTheKeys(t *testing.T) {
	b := kindsBackend()
	m := proposalOpen(t, b, false, kindSettingsRow)
	contains(t, m, "These settings would change:", "!! backup.command[0]", "approve", "reject")
}

func TestKindSettingsApproveReadsAgainApprovesWritesAndKeepsTheResult(t *testing.T) {
	b := kindsBackend()
	m := proposalOpen(t, b, true, kindSettingsRow)
	send(m, "A")
	contains(t, m, "Approve proposal p-0011 and apply these settings to the stack demo?", "These settings would change:",
		"!! backup.command[0]", "The backup command runs on the host later")
	if len(b.proposalActs) != 0 || len(b.settingsSets) != 0 {
		t.Fatal("something changed before the answer")
	}
	send(m, "y")
	want := []string{"proposalshow:p-0011", "proposalshow:p-0011", "proposalapprove:p-0011", "settingsset:demo",
		"proposalresult:p-0011", "proposalshow:p-0011"}
	if got := b.actions(); !slices.Equal(got, want) {
		t.Fatalf("calls = %v, want %v", got, want)
	}
	if b.called("settings") < 2 {
		t.Fatalf("the settings in force were read %d times", b.called("settings"))
	}
	if len(b.settingsSets) != 1 || b.settingsSets[0].Stack != "demo" || !slices.Equal(b.settingsSets[0].Settings.Backup.Command, kindBackupCommand()) {
		t.Fatalf("writes = %+v", b.settingsSets)
	}
	if len(b.proposalApproves) != 1 || b.proposalApproves[0] != (control.ProposalApproveArgs{ID: "p-0011", Fingerprint: "fp-p-0011"}) {
		t.Fatalf("approves = %+v", b.proposalApproves)
	}
	if len(b.proposalResults) != 1 || b.proposalResults[0] != (control.ProposalResultArgs{ID: "p-0011", Fingerprint: "fp-p-0011", Applied: true}) {
		t.Fatalf("results = %+v", b.proposalResults)
	}
	contains(t, m, "The settings of the stack demo are saved")
}

func TestKindSettingsApproveRefusesAChangedProposalOrChangedSettings(t *testing.T) {
	for name, change := range map[string]func(b *fakeBackend){
		"proposal": func(b *fakeBackend) { b.proposals[1].Fingerprint = "fp-other" },
		"settings": func(b *fakeBackend) {
			b.stackSettings["demo"] = control.StackSettings{Version: 1, Update: &control.StackUpdateSettings{Mode: "automatic"}}
		},
		"stack": func(b *fakeBackend) {
			b.errs["settings:demo"] = control.NewError(control.CodeNoSuchStack, "no such stack")
		},
	} {
		t.Run(name, func(t *testing.T) {
			b := kindsBackend()
			m := proposalOpen(t, b, true, kindSettingsRow)
			send(m, "A")
			change(b)
			send(m, "y")
			if len(b.proposalApproves) != 0 || len(b.settingsSets) != 0 || len(b.proposalResults) != 0 {
				t.Fatalf("something was applied: %v %v", b.proposalActs, b.settingsSets)
			}
			if !strings.Contains(m.failure, "changed since it was shown; nothing was applied") {
				t.Fatalf("failure = %q", m.failure)
			}
			if !m.pv.open {
				t.Fatal("the view was left")
			}
		})
	}
}

func TestKindSettingsOfAnUnknownStackCannotBeApproved(t *testing.T) {
	b := kindsBackend()
	m := proposalOpen(t, b, true, kindGhostRow)
	contains(t, m, "The stack ghost does not exist", "reject")
	if strings.Contains(m.render(), "A approve") {
		t.Fatal("approve offered")
	}
	send(m, "A")
	if m.modal != nil || len(b.proposalActs) != 0 {
		t.Fatal("approving went on")
	}
}

func TestKindSettingsFailedWriteIsKeptAsFailedResult(t *testing.T) {
	b := kindsBackend()
	b.errs["settingsset:demo"] = control.NewError(control.CodeInvalid, "update.tags.X: does not fit")
	m := proposalOpen(t, b, true, kindSettingsRow)
	send(m, "A", "y")
	want := []string{"proposalshow:p-0011", "proposalshow:p-0011", "proposalapprove:p-0011", "settingsset:demo",
		"proposalresult:p-0011", "proposalshow:p-0011"}
	if got := b.actions(); !slices.Equal(got, want) {
		t.Fatalf("calls = %v, want %v", got, want)
	}
	if len(b.proposalResults) != 1 || b.proposalResults[0].Applied || b.proposalResults[0].Message != "update.tags.X: does not fit" {
		t.Fatalf("results = %+v", b.proposalResults)
	}
	if !strings.Contains(m.failure, "update.tags.X") {
		t.Fatalf("failure = %q", m.failure)
	}
}

func TestKindWindowViewAndApprove(t *testing.T) {
	b := kindsBackend()
	m := proposalOpen(t, b, true, kindWindowRow)
	contains(t, m, "maintenance window", "The maintenance window of the host would change:", "days[0]", "start", "before: \"03:00\"", "after:  \"02:30\"")
	send(m, "A")
	contains(t, m, "Approve proposal p-0012 and set this maintenance window?", "after:  \"02:30\"")
	if len(b.hostWindows) != 0 {
		t.Fatal("the window was set before the answer")
	}
	send(m, "y")
	want := []string{"proposalshow:p-0012", "proposalshow:p-0012", "proposalapprove:p-0012", "hostwindowset:", "proposalresult:p-0012", "proposalshow:p-0012"}
	if got := b.actions(); !slices.Equal(got, want) {
		t.Fatalf("calls = %v, want %v", got, want)
	}
	if len(b.hostWindows) != 1 || b.hostWindows[0].Start != "02:30" || !slices.Equal(b.hostWindows[0].Days, []string{"mon", "thu"}) {
		t.Fatalf("windows = %+v", b.hostWindows)
	}
	if len(b.proposalResults) != 1 || !b.proposalResults[0].Applied {
		t.Fatalf("results = %+v", b.proposalResults)
	}
	contains(t, m, "The maintenance window is changed")
}

func TestKindWindowApproveRefusesChangedProposalOrWindowAndKeepsFailedResult(t *testing.T) {
	for name, change := range map[string]func(b *fakeBackend){
		"proposal": func(b *fakeBackend) { b.proposals[0].Fingerprint = "fp-other" },
		"window":   func(b *fakeBackend) { b.window.Start = "04:00" },
	} {
		t.Run(name, func(t *testing.T) {
			b := kindsBackend()
			m := proposalOpen(t, b, true, kindWindowRow)
			send(m, "A")
			change(b)
			send(m, "y")
			if len(b.proposalApproves) != 0 || len(b.hostWindows) != 0 {
				t.Fatalf("something was applied: %v", b.proposalActs)
			}
			if !strings.Contains(m.failure, "nothing was applied") {
				t.Fatalf("failure = %q", m.failure)
			}
		})
	}
	b := kindsBackend()
	b.errs["hostwindowset:"] = control.NewError(control.CodeInvalid, "the start is not a time")
	m := proposalOpen(t, b, true, kindWindowRow)
	send(m, "A", "y")
	if len(b.proposalResults) != 1 || b.proposalResults[0].Applied || b.proposalResults[0].Message != "the start is not a time" {
		t.Fatalf("results = %+v", b.proposalResults)
	}
}

func TestKindProposalRejectWorks(t *testing.T) {
	for row, id := range map[int]string{kindSettingsRow: "p-0011", kindWindowRow: "p-0012"} {
		b := kindsBackend()
		m := proposalOpen(t, b, true, row)
		send(m, "N", "f2", "y")
		if len(b.proposalRejects) != 1 || b.proposalRejects[0].ID != id || len(b.proposalApproves) != 0 {
			t.Fatalf("rejects = %+v", b.proposalRejects)
		}
	}
}

func TestKindProposalShowsNoControlCharacters(t *testing.T) {
	b := kindsBackend()
	b.proposalContents["p-0011"] = `{"stack":"demo","settings":{"version":1,"backup":{"command":["echo","\u001b[31mred\u0007"]}}}`
	m := proposalOpen(t, b, true, kindSettingsRow)
	screen := m.render()
	if strings.ContainsAny(screen, "\x1b\x07") && strings.Contains(screen, "red\x07") {
		t.Fatalf("control characters reach the screen:\n%q", screen)
	}
	contains(t, m, "!! backup.command[1]", `\u001b[31mred\u0007`)
}

func TestGoldenProposalsViewStackSettings(t *testing.T) {
	goldenKinds(t, "proposals.view.settings", kindSettingsRow)
}

func TestGoldenProposalsViewWindow(t *testing.T) {
	goldenKinds(t, "proposals.view.window", kindWindowRow)
}

func goldenKinds(t *testing.T, prefix string, row int) {
	t.Helper()
	for name, lang := range goldenLangs {
		for size, dim := range goldenFormSizes {
			t.Run(name+"."+size, func(t *testing.T) {
				m := proposalModel(t, lang, kindsBackend(), true, dim[0], dim[1])
				send(m, "6")
				for range row {
					send(m, "down")
				}
				send(m, "enter")
				screen := m.render()
				checkScreen(t, screen, dim[0], dim[1])
				checkGolden(t, prefix+"."+name+"."+size+".golden", screen+"\n")
			})
		}
	}
}
