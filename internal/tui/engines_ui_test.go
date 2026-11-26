package tui

import (
	"slices"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-kit/i18n"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

const aliceDir = "/home/alice/stacks-example"

// twoEnginesBackend is the stacks of two deploy services, as the routing backend delivers them: the system engine
// and the rootless engine of alice. Both have a stack "wiki" and a stack folder with the ID "srv-main", so
// nothing tells the entries apart but their engine.
func twoEnginesBackend() *fakeBackend {
	const alice = "rootless:alice"
	b := &fakeBackend{engines: []string{control.EngineSystem, alice}}
	b.status = control.StatusResult{Version: "9.9.9", StartedAt: at(27, 8, 0), Engine: control.EngineStatus{LastPoll: at(30, 14, 30),
		Sources: []control.SourceStatus{
			{ID: "srv-main", Path: stackDir, Kind: control.SourceKindRemote, URL: repoURL, LastPoll: at(30, 14, 30), Engine: control.EngineSystem},
			{ID: "srv-main", Path: aliceDir, Kind: control.SourceKindLocal, LastPoll: at(30, 14, 25), Engine: alice},
		},
		Stacks: []control.StackStatus{
			{Name: "mail-relay", Source: "srv-main", Engine: control.EngineSystem, Deployed: commitA, LastRun: "succeeded", Findings: []control.FindingInfo{}},
			{Name: "wiki", Source: "srv-main", Engine: control.EngineSystem, Deployed: commitA, LastRun: "succeeded", Findings: []control.FindingInfo{}},
			{Name: "photo-album", Source: "srv-main", Engine: alice, Deployed: commitA, Pending: commitB, LastRun: "succeeded", Findings: []control.FindingInfo{}},
			{Name: "wiki", Source: "srv-main", Engine: alice, Deployed: commitB, LastRun: "succeeded", Findings: []control.FindingInfo{}},
		}}}
	b.stacks = control.StacksResult{Stacks: []control.StackInfo{
		{Name: "mail-relay", Source: "srv-main", SourceDir: stackDir, Engine: control.EngineSystem, Update: "manual"},
		{Name: "wiki", Source: "srv-main", SourceDir: stackDir, Engine: control.EngineSystem, Update: "none"},
		{Name: "photo-album", Source: "srv-main", SourceDir: aliceDir, Engine: alice, Update: "automatic"},
		{Name: "wiki", Source: "srv-main", SourceDir: aliceDir, Engine: alice, Update: "none"},
	}}
	b.runs = control.RunsResult{Runs: []control.RunInfo{
		{ID: "20261030T140200Z-0000000c", Stack: "wiki", Engine: control.EngineSystem, Started: at(30, 14, 2), Trigger: "manual", DurationMS: 4000, Result: "succeeded"},
		{ID: "20261030T140200Z-0000000c", Stack: "wiki", Engine: alice, Started: at(30, 14, 2), Trigger: "manual", DurationMS: 5000, Result: "succeeded"},
	}}
	b.logs = map[string]string{"20261030T140200Z-0000000c": "2026-10-30T14:02:00Z run started\n"}
	b.creds = nil
	b.credsBy = map[string][]control.CredentialInfo{
		control.EngineSystem: {{Name: "registry.docker.io", Modified: at(30, 9, 0)}},
		alice:                {{Name: "ghcr.io", Modified: at(29, 9, 0)}},
	}
	b.notifyBy = map[string][]control.NotifyChannelInfo{
		control.EngineSystem: {{Name: "ops-hook", Type: control.NotifyWebhook}},
		alice:                {{Name: "alice-hook", Type: control.NotifyWebhook}},
	}
	b.window = control.WindowResult{Days: []string{"sun"}, Start: "03:00", Duration: "2h", Zone: "UTC", NextOpening: at(31, 3, 0)}
	b.window.Origin = control.WindowOriginHost
	return b
}

// twoEnginesDown is twoEnginesBackend with the service of a third engine that could not be asked.
func twoEnginesDown() *fakeBackend {
	b := twoEnginesBackend()
	b.status.NotChecked = []control.EngineNotChecked{{Engine: "rootless:bob", Reason: "its deploy service is not running"}}
	b.engines = append(b.engines, "rootless:bob")
	return b
}

// rootlessOnlyBackend has two rootless instances and no system service: nothing is the default.
func rootlessOnlyBackend() *fakeBackend {
	b := twoEnginesBackend()
	b.engines = []string{"rootless:alice", "rootless:bob"}
	for i := range b.status.Engine.Sources {
		if b.status.Engine.Sources[i].Engine == control.EngineSystem {
			b.status.Engine.Sources[i].Engine = "rootless:bob"
		}
	}
	for i := range b.status.Engine.Stacks {
		if b.status.Engine.Stacks[i].Engine == control.EngineSystem {
			b.status.Engine.Stacks[i].Engine = "rootless:bob"
		}
	}
	for i := range b.stacks.Stacks {
		if b.stacks.Stacks[i].Engine == control.EngineSystem {
			b.stacks.Stacks[i].Engine = "rootless:bob"
		}
	}
	for i := range b.runs.Runs {
		if b.runs.Runs[i].Engine == control.EngineSystem {
			b.runs.Runs[i].Engine = "rootless:bob"
		}
	}
	b.credsBy["rootless:bob"] = []control.CredentialInfo{{Name: "quay.io", Modified: at(28, 9, 0)}}
	return b
}

var toInstanceCreds = keys("4", "up", "right", "down")

func TestGoldenStacksOfTwoEngines(t *testing.T) {
	goldenScreens(t, "stacks.engines", 26, twoEnginesBackend)
}

func TestGoldenStacksOfTwoEnginesWithOneDown(t *testing.T) {
	goldenScreens(t, "stacks.engines.down", 30, twoEnginesDown)
}

func TestGoldenSettingsCredentialsOfTwoInstances(t *testing.T) {
	goldenScreens(t, "settings.creds.instance", settingsRows, twoEnginesBackend, toInstanceCreds...)
}

func TestGoldenSettingsCredentialsChooseTheInstance(t *testing.T) {
	goldenScreens(t, "settings.creds.instance.choose", settingsRows, rootlessOnlyBackend, toInstanceCreds...)
}

func TestGoldenSettingsNotificationsOfTwoInstances(t *testing.T) {
	goldenScreens(t, "settings.notify.instance", settingsRows, twoEnginesBackend, seq(toInstanceCreds, keys("H"))...)
}

// A host that shows one engine is drawn as before, whatever the engine of its entries is called.
func TestOneEngineShowsNoSections(t *testing.T) {
	plain := newTestModel(t, i18n.English, demoBackend(), wideWidth, 24).render()
	b := demoBackend()
	for i := range b.status.Engine.Stacks {
		b.status.Engine.Stacks[i].Engine = control.EngineSystem
	}
	for i := range b.stacks.Stacks {
		b.stacks.Stacks[i].Engine = control.EngineSystem
	}
	for i := range b.check.Stacks {
		b.check.Stacks[i].Engine = control.EngineSystem
	}
	for i := range b.updates.Stacks {
		b.updates.Stacks[i].Engine = control.EngineSystem
	}
	for i := range b.runs.Runs {
		b.runs.Runs[i].Engine = control.EngineSystem
	}
	if got := newTestModel(t, i18n.English, b, wideWidth, 24).render(); got != plain {
		t.Fatalf("the screen changed:\n%s\nwant:\n%s", got, plain)
	}
}

func TestSectionsHaveAHeadingEach(t *testing.T) {
	for _, lang := range []i18n.Lang{i18n.English, i18n.German} {
		screen := newTestModel(t, lang, twoEnginesBackend(), wideWidth, 26).render()
		system, alice := strings.Index(screen, "Engine system"), strings.Index(screen, "Engine rootless:alice")
		if system < 0 || alice < system {
			t.Fatalf("%v: headings out of place:\n%s", lang, screen)
		}
		if at := strings.Index(screen, aliceDir); at < alice {
			t.Fatalf("%v: the folder of alice is not under her heading:\n%s", lang, screen)
		}
	}
}

func TestUnreachableInstanceIsANoteWithItsEngine(t *testing.T) {
	screen := newTestModel(t, i18n.English, twoEnginesDown(), wideWidth, 30).render()
	if !strings.Contains(screen, "Engine rootless:bob") || !strings.Contains(flat(screen), "engine rootless:bob was not checked: its deploy service is not running") {
		t.Fatalf("no note for the engine that was not asked:\n%s", screen)
	}
}

// markStack moves the mark to the stack of the engine, over the sections.
func markStack(t *testing.T, m *Model, engine, name string) {
	t.Helper()
	send(m, "home")
	for range 30 {
		if id, ok := m.list.Selected(); ok && m.rows[id].name == name && m.rows[id].engine == engine {
			return
		}
		send(m, "down")
	}
	t.Fatalf("stack %s of %s is not reachable in the list", name, engine)
}

func TestCursorPassesFromSectionToSection(t *testing.T) {
	m := newTestModel(t, i18n.English, twoEnginesBackend(), wideWidth, 26)
	send(m, "home")
	if m.listEngine != control.EngineSystem {
		t.Fatalf("starts in %q", m.listEngine)
	}
	for range 20 {
		send(m, "down")
	}
	if m.listEngine != "rootless:alice" {
		t.Fatalf("the cursor stayed in %q", m.listEngine)
	}
	for range 20 {
		send(m, "up")
	}
	if m.listEngine != control.EngineSystem {
		t.Fatalf("the cursor did not come back, it is in %q", m.listEngine)
	}
}

func TestHomeAndEndReachTheOuterSections(t *testing.T) {
	m := newTestModel(t, i18n.English, twoEnginesBackend(), wideWidth, 26)
	send(m, "end")
	if m.listEngine != "rootless:alice" {
		t.Fatalf("end stays in %q", m.listEngine)
	}
	last := m.cursorKey()
	send(m, "down")
	if m.cursorKey() != last {
		t.Fatal("end did not reach the last row of the last section")
	}
	send(m, "home")
	if m.listEngine != control.EngineSystem {
		t.Fatalf("home stays in %q", m.listEngine)
	}
	want := m.secs[0].list
	want.Act(m.firstKey())
	wantKey := m.cursorKey()
	send(m, "end", "home")
	if m.cursorKey() != wantKey {
		t.Fatal("home did not reach the first row of the first section")
	}
	if n := cursorCount(m.render()); n != 1 {
		t.Fatalf("%d cursors on the screen, want 1", n)
	}
}

// cursorCount counts the rows of the sections that carry the cursor mark.
func cursorCount(screen string) int {
	return strings.Count(screen, "│ > ") + strings.Count(screen, "║ > ")
}

func TestOnlyTheMarkedSectionShowsItsCursor(t *testing.T) {
	m := newTestModel(t, i18n.English, twoEnginesBackend(), wideWidth, 26)
	for _, keys := range [][]string{{"home"}, {"end"}} {
		send(m, keys...)
		for range 20 {
			send(m, "down")
		}
		if n := cursorCount(m.render()); n != 1 {
			t.Fatalf("%v: %d cursors on the screen, want 1:\n%s", keys, n, m.render())
		}
		send(m, "home")
		if n := cursorCount(m.render()); n != 1 {
			t.Fatalf("%v then home: %d cursors on the screen, want 1:\n%s", keys, n, m.render())
		}
	}
}

// A stack name two engines have is two rows, and an action goes to the engine of the marked row only.
func TestActionsGoToTheEngineOfTheMarkedStack(t *testing.T) {
	for _, engine := range []string{control.EngineSystem, "rootless:alice"} {
		t.Run(engine, func(t *testing.T) {
			b := twoEnginesBackend()
			m := newTestModel(t, i18n.English, b, wideWidth, 26)
			m.euid = func() int { return 0 }
			markStack(t, m, engine, "wiki")
			send(m, "a", "y")
			want := "deploy:wiki@" + engine
			if !slices.Contains(b.engineLog, want) || len(b.acts) != 1 {
				t.Fatalf("log %v, acts %v, want %q once", b.engineLog, b.acts, want)
			}
		})
	}
}

func TestTickedStacksOfTwoEnginesEachGoToTheirOwn(t *testing.T) {
	b := twoEnginesBackend()
	m := newTestModel(t, i18n.English, b, wideWidth, 26)
	m.euid = func() int { return 0 }
	markStack(t, m, control.EngineSystem, "wiki")
	send(m, "space")
	markStack(t, m, "rootless:alice", "wiki")
	send(m, "space", "a", "y")
	var got []string
	for _, e := range b.engineLog {
		if strings.HasPrefix(e, "deploy:") {
			got = append(got, e)
		}
	}
	if !slices.Equal(got, []string{"deploy:wiki@system", "deploy:wiki@rootless:alice"}) {
		t.Fatalf("deploys %v", got)
	}
}

func TestSudoCommandsNameTheEngineOfEachStep(t *testing.T) {
	b := twoEnginesBackend()
	m, f := sudoStackModel(t, i18n.English, b, wideWidth, 26)
	markStack(t, m, "rootless:alice", "wiki")
	send(m, "a")
	if !shows(m.render(), "sudo qhost stack deploy --output json --engine rootless:alice -- wiki") {
		t.Fatalf("the question lacks the engine:\n%s", m.render())
	}
	send(m, "y")
	if got := commandsOf(f); len(got) != 1 || !slices.Contains(got[0], "rootless:alice") {
		t.Fatalf("commands %v", got)
	}
}

func TestDetailReadsTheEngineOfTheOpenedStack(t *testing.T) {
	b := twoEnginesBackend()
	m := newTestModel(t, i18n.English, b, wideWidth, 40)
	markStack(t, m, "rootless:alice", "wiki")
	send(m, "enter")
	for _, want := range []string{"changelog:wiki@rootless:alice", "backups:wiki@rootless:alice", "secrets:wiki@rootless:alice"} {
		if !slices.Contains(b.engineLog, want) {
			t.Fatalf("log %v lacks %q", b.engineLog, want)
		}
	}
	for _, e := range b.engineLog {
		if strings.HasSuffix(e, "@system") {
			t.Fatalf("a read went to the system engine: %v", b.engineLog)
		}
	}
}

func TestFolderActionsGoToTheEngineOfTheFolder(t *testing.T) {
	b := twoEnginesBackend()
	m := newTestModel(t, i18n.English, b, wideWidth, 26)
	m.euid = func() int { return 0 }
	send(m, "home")
	for range 30 {
		if f, ok := m.selectedFolder(); ok && f.Path == aliceDir {
			break
		}
		send(m, "down")
	}
	if f, ok := m.selectedFolder(); !ok || f.Engine != "rootless:alice" {
		t.Fatalf("the marked folder is %+v", f)
	}
	send(m, "d", "y")
	if !slices.Contains(b.engineLog, "sourceremove:"+aliceDir+"@rootless:alice") {
		t.Fatalf("log %v", b.engineLog)
	}
}

func TestRunsAreToldApartByEngine(t *testing.T) {
	b := twoEnginesBackend()
	m := newTestModel(t, i18n.English, b, wideWidth, 26)
	send(m, "3")
	if got := len(m.runByID); got != 2 {
		t.Fatalf("%d runs, want the two of the same ID", got)
	}
	for _, engine := range []string{"rootless:alice", control.EngineSystem} {
		send(m, "enter")
		if m.log.row.engine != engine {
			t.Fatalf("the log is of %q, want %q", m.log.row.engine, engine)
		}
		if want := "log:20261030T140200Z-0000000c@" + engine; !slices.Contains(b.engineLog, want) {
			t.Fatalf("log %v lacks %q", b.engineLog, want)
		}
		send(m, "esc", "down")
	}
}

// With several instances the credentials page shows one, reads it by name and writes to it only.
func TestCredentialsFollowTheChosenInstance(t *testing.T) {
	b := twoEnginesBackend()
	m := newTestModel(t, i18n.English, b, wideWidth, settingsRows)
	m.euid = func() int { return 0 }
	send(m, toInstanceCreds...)
	if screen := m.render(); !strings.Contains(screen, "Instance: system") || !strings.Contains(screen, "registry.docker.io") ||
		strings.Contains(screen, "ghcr.io") {
		t.Fatalf("not the system instance:\n%s", screen)
	}
	send(m, "H")
	if screen := m.render(); !strings.Contains(screen, "Instance: rootless:alice") || !strings.Contains(screen, "ghcr.io") ||
		strings.Contains(screen, "registry.docker.io") {
		t.Fatalf("not the instance of alice:\n%s", screen)
	}
	send(m, "d", "y")
	if !slices.Contains(b.engineLog, "creddel:ghcr.io@rootless:alice") {
		t.Fatalf("log %v", b.engineLog)
	}
	for _, e := range b.engineLog {
		if strings.HasPrefix(e, "creddel:") && !strings.HasSuffix(e, "@rootless:alice") {
			t.Fatalf("a write went elsewhere: %v", b.engineLog)
		}
	}
}

func TestNotificationsFollowTheChosenInstance(t *testing.T) {
	b := twoEnginesBackend()
	m := newTestModel(t, i18n.English, b, wideWidth, settingsRows)
	m.euid = func() int { return 0 }
	send(m, "4", "up", "right", "right", "right", "down", "H")
	if screen := m.render(); !strings.Contains(screen, "alice-hook") || strings.Contains(screen, "ops-hook") {
		t.Fatalf("not the instance of alice:\n%s", screen)
	}
	send(m, "s", "y")
	if !slices.Contains(b.engineLog, "notifytest:alice-hook@rootless:alice") {
		t.Fatalf("log %v", b.engineLog)
	}
}

func TestSettingsSudoCommandsNameTheInstance(t *testing.T) {
	b := twoEnginesBackend()
	m, f := sudoStackModel(t, i18n.English, b, wideWidth, settingsRows)
	send(m, toInstanceCreds...)
	send(m, "H", "d")
	if !shows(m.render(), "sudo qhost credential delete --yes --output json --engine rootless:alice -- ghcr.io") {
		t.Fatalf("the question lacks the instance:\n%s", m.render())
	}
	send(m, "y")
	if got := commandsOf(f); len(got) != 1 || !slices.Contains(got[0], "rootless:alice") {
		t.Fatalf("commands %v", got)
	}
}

// With no system service and several instances nothing is the default: the page asks, reads nothing and changes
// nothing until one is chosen.
func TestNoInstanceIsGuessed(t *testing.T) {
	b := rootlessOnlyBackend()
	m := newTestModel(t, i18n.English, b, wideWidth, settingsRows)
	m.euid = func() int { return 0 }
	send(m, toInstanceCreds...)
	if slices.ContainsFunc(b.engineLog, func(e string) bool { return strings.HasPrefix(e, "credentials:") }) {
		t.Fatalf("a read before the choice: %v", b.engineLog)
	}
	send(m, "n")
	if m.set.form != nil || !strings.Contains(flat(m.render()), "Choose the instance first.") {
		t.Fatalf("a form opened without an instance:\n%s", m.render())
	}
	send(m, "H")
	if m.settingsEngine() != "rootless:alice" || !slices.Contains(b.engineLog, "credentials:@rootless:alice") {
		t.Fatalf("engine %q, log %v", m.settingsEngine(), b.engineLog)
	}
	send(m, "H")
	if m.settingsEngine() != "rootless:bob" || !strings.Contains(m.render(), "quay.io") {
		t.Fatalf("engine %q:\n%s", m.settingsEngine(), m.render())
	}
}

// A host with one deploy service has no choice, and the calls name no engine.
func TestOneInstanceHasNoChoice(t *testing.T) {
	b := settingsBackend()
	m := newTestModel(t, i18n.English, b, wideWidth, settingsRows)
	send(m, toCreds...)
	if screen := m.render(); strings.Contains(screen, "Instance") || strings.Contains(screen, "instance") {
		t.Fatalf("a choice on a host with one instance:\n%s", screen)
	}
	send(m, "H")
	if !slices.Contains(b.engineLog, "credentials:@") {
		t.Fatalf("log %v", b.engineLog)
	}
}
