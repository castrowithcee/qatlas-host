package tui

import (
	"slices"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-kit/i18n"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// The Grants section of the stack detail. Users, stacks and IDs are made up.

const (
	grantRows     = 48
	grantWidth    = 80
	reportGrantAd = `{"id":"g-0a1b2c3f","user":"ada","action":"stack.restart","stack":"demo-stack","engine":"system","expires":"","replaced":false,"backup":false,"check":false}`
	reportGrantRm = `{"id":"g-0a1b2c3d","user":"ada","action":"stack.restart","stack":"demo-stack","engine":"system","removed":true}`
)

var (
	grantAddSudo = []string{"grant", "add", "--confirm", "--output", "json", "--", "ada", "stack.restart", "demo-stack"}
	grantRmSudo  = []string{"grant", "remove", "--confirm", "--output", "json", "--", "g-0a1b2c3d"}
	toGrantForm  = seq(toDetail, keys("z"))
)

// detailGrantBackend has one valid and one invalid grant for the demo stack.
func detailGrantBackend() *fakeBackend {
	b := settingsBackend()
	b.grants = map[string][]control.GrantInfo{"demo-stack": {
		{Grant: control.Grant{ID: "g-0a1b2c3d", User: "ada", UID: 1000, Action: control.GrantActionStackRestart, Stack: "demo-stack", Expires: "2027-03-31"}},
		{Grant: control.Grant{ID: "g-0a1b2c3e", User: "cleo", UID: 1002, Action: control.GrantActionUpdateRun, Stack: "demo-stack"}, Invalid: control.GrantUserChanged},
	}}
	return b
}

func emptyGrantBackend() *fakeBackend { return settingsBackend() }

// goldenDetailGrant is goldenScreens at 80 columns with the user having no root, so that the question shows the command.
func goldenDetailGrant(t *testing.T, prefix string, rows int, setup func() *fakeBackend, keys ...string) {
	t.Helper()
	for name, lang := range goldenLangs {
		t.Run(name, func(t *testing.T) {
			m, _ := sudoDetailModel(t, lang, setup(), grantWidth, rows)
			send(m, keys...)
			screen := m.render()
			checkScreen(t, screen, grantWidth, rows)
			checkGolden(t, prefix+"."+name+".golden", screen+"\n")
		})
	}
}

// containsFlat is contains for text that a dialog may wrap.
func containsFlat(t *testing.T, m *Model, want ...string) {
	t.Helper()
	screen := flat(m.render())
	for _, w := range want {
		if !strings.Contains(screen, w) {
			t.Fatalf("the screen lacks %q:\n%s", w, m.render())
		}
	}
}

func TestGoldenGrantSection(t *testing.T) {
	goldenDetailGrant(t, "detail.grants", grantRows, detailGrantBackend, toDetail...)
}

func TestGoldenGrantSectionEmpty(t *testing.T) {
	goldenDetailGrant(t, "detail.grants.none", grantRows, emptyGrantBackend, toDetail...)
}

func TestGoldenGrantForm(t *testing.T) {
	goldenDetailGrant(t, "detail.grants.form", 24, detailGrantBackend, toGrantForm...)
}

func TestGoldenGrantConfirm(t *testing.T) {
	goldenDetailGrant(t, "detail.grants.confirm", 24, detailGrantBackend, seq(toGrantForm, keys("f2"))...)
}

func TestGoldenGrantRemoveQuestion(t *testing.T) {
	goldenDetailGrant(t, "detail.grants.remove", 24, detailGrantBackend, seq(toDetail, keys("Y", "enter"))...)
}

func TestGrantSectionShowsStatesAndWordsInBothLanguages(t *testing.T) {
	m := newTestModel(t, i18n.English, detailGrantBackend(), grantWidth, grantRows)
	send(m, toDetail...)
	contains(t, m, "Grants", "every agent", "ada", "restart the stack", "until 2027-03-31", "valid",
		"cleo", "update the stack", "no expiry", "invalid: the account changed")
	m = newTestModel(t, i18n.German, detailGrantBackend(), grantWidth, grantRows)
	send(m, toDetail...)
	contains(t, m, "Freigaben", "jeden Agenten", "Stack neu starten", "bis 2027-03-31", "gültig", "ungültig: Konto geändert")
}

func TestGrantSectionProblemAndReadFailure(t *testing.T) {
	b := detailGrantBackend()
	b.grantProblem = "grants.json is not valid"
	m := newTestModel(t, i18n.English, b, grantWidth, grantRows)
	send(m, toDetail...)
	contains(t, m, "none of them counts", "grants.json is not valid")
	b = detailGrantBackend()
	b.errs = map[string]error{"grantlist:demo-stack": errServiceBusy()}
	m = newTestModel(t, i18n.English, b, grantWidth, grantRows)
	send(m, toDetail...)
	contains(t, m, "Grants", "DB_PASSWORD")
	send(m, "z")
	if m.set.form != nil || len(b.actions()) != 0 {
		t.Fatal("a grant was offered without a read list")
	}
}

func TestGrantAddRunsTheCommandThroughSudoAndReadsTheListAgain(t *testing.T) {
	b := detailGrantBackend()
	m, f := sudoDetailModel(t, i18n.English, b, grantWidth, grantRows)
	f.report = reportGrantAd
	send(m, toGrantForm...)
	send(m, "f2")
	contains(t, m, "Allow ada to restart the stack (demo-stack) without sudo?", "neither a backup nor a check", "does not expire")
	if !shows(m.render(), sudoCommand(grantAddSudo)) {
		t.Fatalf("the question lacks the command:\n%s", m.render())
	}
	if len(f.reqs) != 0 {
		t.Fatalf("sudo ran before the answer: %+v", f.reqs)
	}
	before := b.calls["grantlist"]
	send(m, "y")
	if got := commandsOf(f); len(got) != 1 || !slices.Equal(got[0], grantAddSudo) {
		t.Fatalf("commands %v", got)
	}
	contains(t, m, "ada may now restart the stack (demo-stack) without sudo.")
	if b.calls["grantlist"] != before+1 {
		t.Fatalf("the list was read %d times after the change, want 1", b.calls["grantlist"]-before)
	}
	if len(b.actions()) != 0 {
		t.Fatalf("the service was called directly: %v", b.actions())
	}
}

func TestGrantAddWithExpiryAndNothingRunsOnCancel(t *testing.T) {
	m, f := sudoDetailModel(t, i18n.English, detailGrantBackend(), grantWidth, grantRows)
	f.report = reportGrantAd
	send(m, toGrantForm...)
	send(m, "tab", "tab")
	send(m, typed("2027-03-31")...)
	send(m, "f2")
	want := []string{"grant", "add", "--expires", "2027-03-31", "--confirm", "--output", "json", "--", "ada", "stack.restart", "demo-stack"}
	if !shows(m.render(), sudoCommand(want)) {
		t.Fatalf("the question lacks the command:\n%s", m.render())
	}
	contains(t, m, "holds until the end of 2027-03-31")
	send(m, "n")
	if len(f.reqs) != 0 {
		t.Fatalf("a cancelled question ran %+v", f.reqs)
	}
}

func TestGrantAddRefusesABadDay(t *testing.T) {
	m, f := sudoDetailModel(t, i18n.English, detailGrantBackend(), grantWidth, grantRows)
	send(m, toGrantForm...)
	send(m, "tab", "tab")
	send(m, typed("31.03.2027")...)
	send(m, "f2")
	contains(t, m, "Give the day as YYYY-MM-DD")
	if m.modal != nil || len(f.reqs) != 0 {
		t.Fatal("a bad day reached the question")
	}
}

func TestGrantAddNeedsAUserWithAccess(t *testing.T) {
	m, _ := sudoDetailModel(t, i18n.English, detailGrantBackend(), grantWidth, grantRows)
	m.sys.loaded, m.sys.users = true, nil
	send(m, toGrantForm...)
	if m.set.form != nil || m.notice == "" {
		t.Fatalf("form %v notice %q", m.set.form != nil, m.notice)
	}
}

func TestGrantAddAnswerWithoutAnIDFails(t *testing.T) {
	m, f := sudoDetailModel(t, i18n.English, detailGrantBackend(), grantWidth, grantRows)
	f.report = `{"removed":true}`
	send(m, toGrantForm...)
	send(m, "f2", "y")
	contains(t, m, "The answer of qhost could not be read.")
}

func TestGrantConfirmationSaysWhatTheStackHasSetUp(t *testing.T) {
	m := newTestModel(t, i18n.English, detailGrantBackend(), grantWidth, grantRows)
	for _, tt := range []struct {
		action string
		setup  grantSetup
		want   string
	}{
		{control.GrantActionStackRestart, grantSetup{backup: true, check: true}, "The stack has a backup and a check set up."},
		{control.GrantActionStackRestart, grantSetup{backup: true}, "The stack has a backup but no check set up."},
		{control.GrantActionStackRestart, grantSetup{check: true}, "The stack has a check but no backup set up."},
		{control.GrantActionUpdateCheck, grantSetup{}, "The stack has neither a backup nor a check set up."},
		{control.GrantActionUpdateRun, grantSetup{backup: true, check: true}, "The stack has a backup and a check set up. An update started with this grant commits and pushes to the source of the stack."},
	} {
		if got := m.grantSetupText(tt.action, tt.setup); got != tt.want {
			t.Errorf("%s %+v: %q", tt.action, tt.setup, got)
		}
	}
}

func TestGrantFormReadsTheSetupFromTheStackSettings(t *testing.T) {
	b := detailGrantBackend()
	b.stackSettings = map[string]control.StackSettings{"demo-stack": {Version: 1, Backup: &control.StackBackupSettings{}, Check: &control.StackCheckSettings{}}}
	m, _ := sudoDetailModel(t, i18n.English, b, grantWidth, grantRows)
	send(m, toGrantForm...)
	send(m, "f2")
	contains(t, m, "The stack has a backup and a check set up.")
}

func TestGrantRemoveAsksThenRunsTheCommandAndReadsTheListAgain(t *testing.T) {
	b := detailGrantBackend()
	m, f := sudoDetailModel(t, i18n.English, b, grantWidth, grantRows)
	f.report = reportGrantRm
	send(m, toDetail...)
	send(m, "Y", "enter")
	containsFlat(t, m, "Withdraw the grant of ada to restart the stack (demo-", "under the account need sudo for")
	if !shows(m.render(), sudoCommand(grantRmSudo)) {
		t.Fatalf("the question lacks the command:\n%s", m.render())
	}
	if len(f.reqs) != 0 {
		t.Fatalf("sudo ran before the answer: %+v", f.reqs)
	}
	before := b.calls["grantlist"]
	send(m, "y")
	if got := commandsOf(f); len(got) != 1 || !slices.Equal(got[0], grantRmSudo) {
		t.Fatalf("commands %v", got)
	}
	contains(t, m, "The grant of ada to restart the stack (demo-stack) was withdrawn.")
	if b.calls["grantlist"] != before+1 {
		t.Fatalf("the list was read %d times after the change, want 1", b.calls["grantlist"]-before)
	}
}

func TestGrantRemoveWithoutGrantOnlyWarns(t *testing.T) {
	m, _ := sudoDetailModel(t, i18n.English, emptyGrantBackend(), grantWidth, grantRows)
	send(m, toDetail...)
	send(m, "Y")
	if m.pick != nil || m.notice == "" {
		t.Fatalf("pick %v notice %q", m.pick != nil, m.notice)
	}
}
