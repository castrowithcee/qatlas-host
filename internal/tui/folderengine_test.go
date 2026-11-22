package tui

import (
	"slices"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-kit/i18n"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

const aliceEngine = "rootless:alice"

// twoEngineFolders is the folders backend of a host with the system engine and the rootless engine of alice, in
// which two containers of every folder run: the inspection for the system engine lists them with their engine, the
// one for the engine of alice does not, since they belong there.
func twoEngineFolders() *fakeBackend {
	b := foldersBackend()
	b.engines = []string{control.EngineSystem, aliceEngine}
	b.host.enginesFor = func(engine string, res control.SourceInspectResult) control.SourceInspectResult {
		if engine == aliceEngine || len(res.Stacks) == 0 {
			return res
		}
		res.Stacks = slices.Clone(res.Stacks)
		res.Stacks[0].Conflicts = append(slices.Clone(res.Stacks[0].Conflicts),
			control.SourceConflict{Container: "demo-web-1", Project: "demo", Engine: aliceEngine},
			control.SourceConflict{Container: "demo-db-1", Project: "demo", Engine: aliceEngine})
		return res
	}
	return b
}

func engineChoiceOf(t *testing.T, m *Model) string {
	t.Helper()
	f := m.set.form
	if f == nil || f.kind != formFolderEngine {
		t.Fatalf("the form of the engine is not open: %+v\n%s", f, m.render())
	}
	return string(f.form.Values()["engine"].Choice)
}

func TestSuggestEngine(t *testing.T) {
	conflicts := func(engines ...string) control.SourceInspectResult {
		var list []control.SourceConflict
		for _, e := range engines {
			list = append(list, control.SourceConflict{Container: "c", Project: "p", Engine: e})
		}
		return control.SourceInspectResult{Stacks: []control.SourceStack{{Conflicts: list}}}
	}
	both := []string{control.EngineSystem, aliceEngine, "rootless:bob"}
	for _, c := range []struct {
		name    string
		res     control.SourceInspectResult
		offered []string
		want    string
	}{
		{"no containers", conflicts(), both, control.EngineSystem},
		{"containers in one rootless engine", conflicts(aliceEngine, aliceEngine), both, aliceEngine},
		{"the engine of the service has no name", conflicts("", ""), both, control.EngineSystem},
		{"the most containers", conflicts("", aliceEngine, "rootless:bob", "rootless:bob"), both, "rootless:bob"},
		{"a tie goes to the system engine", conflicts("", aliceEngine), both, control.EngineSystem},
		{"an engine that is not offered", conflicts("rootless:carol"), both, control.EngineSystem},
		{"no system engine offered", conflicts(), []string{aliceEngine, "rootless:bob"}, aliceEngine},
	} {
		if got := suggestEngine(c.res, c.offered); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
}

func TestOtherEnginesLeavesOutTheEngineOfTheFlow(t *testing.T) {
	flow := &addFlow{engine: aliceEngine, res: control.SourceInspectResult{Stacks: []control.SourceStack{{Conflicts: []control.SourceConflict{
		{Container: "a", Engine: aliceEngine}, {Container: "b"}, {Container: "c", Engine: control.EngineSystem}, {Container: "d", Engine: control.EngineSystem}}}}}}
	if got := otherEngines(flow); !slices.Equal(got, []string{control.EngineSystem}) {
		t.Fatalf("other engines = %v", got)
	}
}

// With only the system engine there is nothing to choose: the flow is the one without an engine.
func TestFolderAddWithOneEngineAsksForNoEngine(t *testing.T) {
	b := foldersBackend()
	m := newSettingsModel(t, b)
	send(m, addKeys(filesDir)...)
	if m.set.form != nil || m.modal == nil {
		t.Fatalf("form %v, question %v", m.set.form, m.modal)
	}
	if strings.Contains(m.render(), "engine:") || strings.Contains(m.render(), "belongs to the engine") {
		t.Fatalf("an engine is shown on a host with one:\n%s", m.render())
	}
	if !slices.Equal(b.inspectEngines, []string{""}) {
		t.Fatalf("inspections for %q", b.inspectEngines)
	}
}

// The containers of the folder run in the engine of alice, so that engine is preselected; the folder is inspected
// again for it, the confirmation names it, and every step of yes is addressed to it.
func TestFolderAddChoosesTheEngineOfTheContainersAndAppliesThereOnly(t *testing.T) {
	b := twoEngineFolders()
	m := newSettingsModel(t, b)
	send(m, addKeys(filesDir)...)
	if got := engineChoiceOf(t, m); got != aliceEngine {
		t.Fatalf("preselected %q", got)
	}
	contains(t, m, "Engine", "rootless:alice (2 containers of the folder run here)", "preselected is the engine where containers")
	noChange(t, b)
	send(m, "f2")
	if m.set.form != nil || m.modal == nil {
		t.Fatalf("form %v, question %v\n%s", m.set.form, m.modal, m.render())
	}
	if !slices.Equal(b.inspectEngines, []string{"", aliceEngine}) {
		t.Fatalf("inspections for %q", b.inspectEngines)
	}
	says(t, m, "engine: rootless:alice", "The folder belongs to the engine rootless:alice.")
	if strings.Contains(m.render(), "run in the engine rootless:alice, not in") {
		t.Fatalf("the engine of the flow is warned about:\n%s", m.render())
	}
	noChange(t, b)
	send(m, "y")
	want := []string{"plan:" + filesDir + "@" + aliceEngine, "grant:" + filesDir + ":false@" + aliceEngine}
	for _, c := range want {
		if !slices.Contains(b.host.calls, c) {
			t.Fatalf("host calls %v lack %s", b.host.calls, c)
		}
	}
	if !slices.Contains(b.engineLog, "sourceadd:"+filesDir+"@"+aliceEngine) {
		t.Fatalf("the folder was not added in the engine of alice: %v", b.engineLog)
	}
	if got := b.host.changes(); !slices.Equal(got, []string{"apply:3@" + aliceEngine, "grant:" + filesDir + ":false@" + aliceEngine}) {
		t.Fatalf("host changes = %v", got)
	}
}

// The system engine is a choice like any other; it is the one the inspection was made for, so nothing is read again.
func TestFolderAddKeepsTheSystemEngineWithoutInspectingAgain(t *testing.T) {
	b := twoEngineFolders()
	b.host.enginesFor = nil
	m := newSettingsModel(t, b)
	send(m, addKeys(repoDir)...)
	if got := engineChoiceOf(t, m); got != control.EngineSystem {
		t.Fatalf("preselected %q without containers elsewhere", got)
	}
	send(m, "f2")
	says(t, m, "engine: system", "The folder belongs to the engine system.")
	if !slices.Equal(b.inspectEngines, []string{""}) || m.modal == nil {
		t.Fatalf("inspections for %q", b.inspectEngines)
	}
	send(m, "y")
	if !slices.Contains(b.engineLog, "sourceadd:"+repoDir+"@"+control.EngineSystem) {
		t.Fatalf("calls %v", b.engineLog)
	}
}

// A choice other than the preselected one is taken as it is, and the warning about the containers elsewhere
// names the engines and says nothing is blocked.
func TestFolderAddWarnsWhenContainersRunInAnotherEngine(t *testing.T) {
	b := twoEngineFolders()
	m := newSettingsModel(t, b)
	send(m, addKeys(filesDir)...)
	send(m, "left")
	if got := engineChoiceOf(t, m); got != control.EngineSystem {
		t.Fatalf("the choice is %q", got)
	}
	send(m, "f2")
	says(t, m, "engine: system", "Containers of the folder run in the engine rootless:alice, not in system")
	send(m, "y")
	if !slices.Contains(b.engineLog, "sourceadd:"+filesDir+"@"+control.EngineSystem) {
		t.Fatalf("the choice was not taken: %v", b.engineLog)
	}
}

func TestFolderAddRejectionAsksForNoEngine(t *testing.T) {
	m := newSettingsModel(t, twoEngineFolders())
	send(m, addKeys(rejectDir)...)
	if m.set.form != nil {
		t.Fatalf("the engine is asked for a folder that cannot be taken over:\n%s", m.render())
	}
}

// Without root the inspection and the command carry the engine: the choice is made after the first inspection,
// which names no engine, and the command that applies it names the engine that was shown.
func TestFolderAddWithoutRootNamesTheEngineInTheCommands(t *testing.T) {
	b := twoEngineFolders()
	b.dirKeys = nil
	m, f := sudoFolderModel(t, i18n.English, b, wideWidth, 50, map[string]string{
		"source inspect": inspectJSON(t, b.inspects[repoDir], false, nil), "source add": localJSON})
	send(m, addKeys(repoDir)...)
	wantCommands(t, m, f, inspectArgs(repoDir))
	if got := engineChoiceOf(t, m); got != control.EngineSystem {
		t.Fatalf("preselected %q", got)
	}
	send(m, "right")
	send(m, "f2")
	inspect := []string{"source", "inspect", "--output", "json", "--engine", aliceEngine, "--", repoDir}
	wantCommands(t, m, f, inspectArgs(repoDir), inspect)
	add := []string{"source", "add", "--expect-effect", effectExample, "--yes", "--output", "json", "--engine", aliceEngine, "--", repoDir}
	says(t, m, "engine: rootless:alice")
	showsCommand(t, m, add)
	untouched(t, b)
	send(m, "y")
	wantCommands(t, m, f, inspectArgs(repoDir), inspect, add)
	untouched(t, b)
}

// A host with one engine runs the commands as before, without --engine.
func TestFolderAddWithoutRootOnOneEngineNamesNone(t *testing.T) {
	b := sudoFolders()
	m, f := sudoFolderModel(t, i18n.English, b, wideWidth, 40, map[string]string{
		"source inspect": inspectJSON(t, b.inspects[repoDir], false, nil), "source add": localJSON})
	send(m, addKeys(repoDir)...)
	send(m, "y")
	for _, c := range commandsOf(f) {
		if slices.Contains(c, "--engine") {
			t.Fatalf("a command names an engine: %v", c)
		}
	}
}

// engineProposals is the proposal backend of two engines: p-0005, the folder of files, was submitted to the deploy
// service of alice, and the containers of the folder run in the system engine.
func engineProposals() *fakeBackend {
	b := approveBackend()
	b.engines = []string{control.EngineSystem, aliceEngine}
	b.proposals[0].Engine = aliceEngine
	b.host.enginesFor = func(engine string, res control.SourceInspectResult) control.SourceInspectResult {
		if len(res.Stacks) == 0 {
			return res
		}
		res.Stacks = slices.Clone(res.Stacks)
		res.Stacks[0].Conflicts = append(slices.Clone(res.Stacks[0].Conflicts), control.SourceConflict{Container: "demo-web-1", Project: "demo",
			Engine: control.EngineSystem})
		return res
	}
	return b
}

// The proposal shows the engine of the deploy service it was submitted to, and reading, the effect and every step of
// approving are addressed to that engine and to no other. Containers of another engine are a warning, not a block.
func TestProposalApproveShowsTheEngineOfTheSubmissionAndWarnsAboutOthers(t *testing.T) {
	b := engineProposals()
	m := proposalOpen(t, b, true, 0)
	contains(t, m, "Engine", aliceEngine)
	for _, call := range b.proposalEngines {
		if !strings.HasSuffix(call, "@"+aliceEngine) {
			t.Fatalf("a read of the proposal went elsewhere: %v", b.proposalEngines)
		}
	}
	send(m, "A")
	says(t, m, "engine: rootless:alice", "The folder belongs to the engine rootless:alice.",
		"Containers of the folder run in the engine system, not in rootless:alice", "demo-web-1")
	noProposalChange(t, b)
	send(m, "y")
	for _, call := range []string{"show:p-0005@" + aliceEngine, "approve:p-0005@" + aliceEngine, "result:p-0005@" + aliceEngine} {
		if !slices.Contains(b.proposalEngines, call) {
			t.Fatalf("calls %v lack %s", b.proposalEngines, call)
		}
	}
	if !slices.Contains(b.engineLog, "sourceadd:"+filesDir+"@"+aliceEngine) {
		t.Fatalf("the folder was not added in the engine of the proposal: %v", b.engineLog)
	}
	if len(b.sourceAdds) != 1 {
		t.Fatalf("approving was blocked: %v", b.sourceAdds)
	}
}

// Another engine in the effect of the proposal is a different effect, so a proposal that moved is refused.
func TestEffectSignatureChangesWithTheEngine(t *testing.T) {
	flow := func(engine string) *addFlow {
		return &addFlow{engine: engine, res: control.SourceInspectResult{Case: control.SourceCaseLocal}}
	}
	if effectSignature(flow(aliceEngine)) == effectSignature(flow(control.EngineSystem)) {
		t.Fatal("another engine did not change the signature")
	}
}

func TestProposalApproveWithoutRootNamesTheEngineInTheCommands(t *testing.T) {
	b := engineProposals()
	m, f := sudoProposalOpen(t, i18n.English, b, 0, map[string]string{
		"source inspect": inspectJSON(t, b.inspects[filesDir], false, permsExample)})
	contains(t, m, aliceEngine)
	send(m, "A")
	inspect := []string{"source", "inspect", "--output", "json", "--engine", aliceEngine, "--", filesDir}
	wantCommands(t, m, f, inspect)
	approve := []string{"proposal", "approve", "p-0005", "--fingerprint", "fp-p-0005", "--confirm", "--expect-effect", effectExample,
		"--output", "json", "--engine", aliceEngine}
	says(t, m, "engine: rootless:alice")
	showsCommand(t, m, approve)

	m, f = sudoProposalOpen(t, i18n.English, engineProposals(), 0, map[string]string{
		"proposal reject": `{"id":"p-0005","kind":"source-add","state":"rejected","submitter":"ana","submitter_uid":1001}`})
	send(m, "N")
	send(m, "f2")
	send(m, "y")
	reject := commandsOf(f)
	if len(reject) != 1 || !slices.Equal(reject[0], []string{"proposal", "reject", "p-0005", "--output", "json", "--engine", aliceEngine}) {
		t.Fatalf("the rejection ran %v", reject)
	}
}

func TestProposalRejectIsAddressedToTheEngineOfTheSubmission(t *testing.T) {
	b := engineProposals()
	m := proposalOpen(t, b, true, 0)
	send(m, "N", "f2", "y")
	if !slices.Contains(b.proposalEngines, "reject:p-0005@"+aliceEngine) || len(b.proposalRejects) != 1 {
		t.Fatalf("calls %v, rejects %v", b.proposalEngines, b.proposalRejects)
	}
}

// goldenEngineFlow renders the screen after steps on the backend b, as root, in each language and width.
func goldenEngineFlow(t *testing.T, prefix string, b func() *fakeBackend, open func(m *Model)) {
	t.Helper()
	for name, lang := range goldenLangs {
		for size, dim := range goldenFormSizes {
			t.Run(name+"."+size, func(t *testing.T) {
				m := proposalModel(t, lang, b(), true, dim[0], dim[1])
				open(m)
				screen := m.render()
				checkScreen(t, screen, dim[0], dim[1])
				checkGolden(t, prefix+"."+name+"."+size+".golden", screen+"\n")
			})
		}
	}
}

func TestGoldenFolderEngineChoice(t *testing.T) {
	goldenEngineFlow(t, "stacks.engine.choice", twoEngineFolders, func(m *Model) { send(m, addKeys(filesDir)...) })
}

func TestGoldenFolderEngineConfirmation(t *testing.T) {
	goldenEngineFlow(t, "stacks.engine.confirm", twoEngineFolders, func(m *Model) { send(m, seq(addKeys(filesDir), keys("f2"))...) })
}

func TestGoldenProposalApproveEngineWarning(t *testing.T) {
	goldenEngineFlow(t, "proposals.approve.engine", engineProposals, func(m *Model) { send(m, "6", "enter", "A") })
}
