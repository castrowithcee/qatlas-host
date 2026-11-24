package tui

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-kit/i18n"
	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

const (
	moveSource    = "/srv/stacks-example"
	moveRootStack = "/srv/rootstack-example"
	moveTarget    = moveSource + "/demo-foreign"
)

// moveBackend is adoptBackend with two sources in the system engine: one that takes a second stack and one that is
// a root stack.
func moveBackend() *fakeBackend {
	b := adoptBackend(localResult(foreignStack()))
	b.status.Engine.Sources = []control.SourceStatus{
		{ID: "srv-stacks-example", Path: moveSource, Kind: control.SourceKindRemote},
		{ID: "srv-rootstack-example", Path: moveRootStack, Kind: control.SourceKindLocal},
	}
	b.stacks.Stacks = append(b.stacks.Stacks, control.StackInfo{Name: "solo", Folder: rootStackFolder, SourceDir: moveRootStack})
	return b
}

func movePreview(mod func(*control.SourceMoveResult)) control.SourceMoveResult {
	res := control.SourceMoveResult{Path: foreignDir, Into: moveSource, Target: moveTarget, Engine: control.EngineSystem,
		Project: "legacy-blog", NewProject: "legacy-blog", PlannedFiles: []string{"compose.yaml", ".env"},
		Excluded: []control.SourceExcluded{{Path: "data", Reason: control.SourceExcludedBindMount}},
		Blocks:   []control.SourceMoveIssue{}, Warnings: []control.SourceMoveIssue{{Code: control.MoveDowntime,
			Reason: "the containers of the project are recreated from the new folder", Fix: "Move in a quiet period."}}}
	if mod != nil {
		mod(&res)
	}
	return res
}

func jsonOf(t *testing.T, v any) string {
	t.Helper()
	out, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// moveModel is the interface on b with a fake sudo (or root) that prints preview for the command without --yes and
// outcome for the one with it.
func moveModel(t *testing.T, lang i18n.Lang, b *fakeBackend, root bool, width, rows int, preview, outcome string) (*Model, *fakeRoot) {
	t.Helper()
	m := newTestModel(t, lang, b, width, rows)
	f := &fakeRoot{root: root}
	m.euid = func() int {
		if root {
			return 0
		}
		return 1000
	}
	m.root = f.hook()
	f.reports = func(argv []string) string {
		if slices.Contains(argv, "--yes") {
			return outcome
		}
		return preview
	}
	return m, f
}

// toMoveChoice marks the foreign project and opens the takeover, which asks where it goes.
func toMoveChoice(m *Model) {
	send(m, "5")
	m.ctList.Select(kittui.ItemID("legacy-blog-db-1"))
	send(m, "a")
}

func TestMoveTargetsAreTheSourcesOfTheEngineThatAreNoRootStack(t *testing.T) {
	b := moveBackend()
	b.status.Engine.Sources = []control.SourceStatus{
		{ID: "srv-stacks-example", Path: moveSource, Engine: control.EngineSystem},
		{ID: "srv-rootstack-example", Path: moveRootStack, Engine: control.EngineSystem},
		{ID: "srv-other-example", Path: "/srv/other-example", Engine: aliceEngine},
		{ID: "srv-more-example", Path: "/srv/more-example", Engine: control.EngineSystem}}
	b.stacks.Stacks[len(b.stacks.Stacks)-1].Engine = control.EngineSystem
	m := newTestModel(t, i18n.English, b, wideWidth, 40)
	paths := func(src foreignSource) []string {
		var out []string
		for _, s := range m.moveTargets(src) {
			out = append(out, s.Path)
		}
		return out
	}
	if got, want := paths(foreignSource{engine: control.EngineSystem}), []string{moveSource, "/srv/more-example"}; !slices.Equal(got, want) {
		t.Fatalf("system engine: %v, want %v", got, want)
	}
	if got, want := paths(foreignSource{engine: aliceEngine}), []string{"/srv/other-example"}; !slices.Equal(got, want) {
		t.Fatalf("rootless engine: %v, want %v", got, want)
	}
}

func TestMoveChoiceOffersTheMatchingSourcesAndOwnIsTheDefault(t *testing.T) {
	b := moveBackend()
	m, f := moveModel(t, i18n.English, b, false, wideWidth, 40, "", "")
	toMoveChoice(m)
	send(m, "enter")
	form := m.set.form
	if form == nil || form.kind != formAdoptTarget || string(form.form.Values()["target"].Choice) != moveOwn {
		t.Fatalf("form %+v\n%s", form, m.render())
	}
	contains(t, m, "a source of its own", "moved into the source "+moveSource)
	if strings.Contains(m.render(), moveRootStack) {
		t.Fatalf("a root stack is offered:\n%s", m.render())
	}
	if b.called("sourceinspect") != 0 || len(f.reqs) != 0 {
		t.Fatalf("something ran before the choice: %d, %v", b.called("sourceinspect"), f.reqs)
	}
}

func TestMoveOwnSourceGoesTheOldWay(t *testing.T) {
	b := moveBackend()
	m, _ := moveModel(t, i18n.English, b, true, wideWidth, 40, "", "")
	b.host = &fakeFolders{}
	toMoveChoice(m)
	send(m, "f2")
	contains(t, m, "Foreign project", "works: deploy")
	if b.called("sourceinspect") != 1 {
		t.Fatalf("inspections %d", b.called("sourceinspect"))
	}
}

func TestMoveBlockedPreviewCannotBeRun(t *testing.T) {
	b := moveBackend()
	preview := jsonOf(t, movePreview(func(r *control.SourceMoveResult) {
		r.Blocks = []control.SourceMoveIssue{{Code: control.MoveOtherFilesystem, Reason: "the folder and the source are on different file systems",
			Fix: "Choose a source on the same file system."}}
	}))
	m, f := moveModel(t, i18n.English, b, false, wideWidth, 50, preview, "")
	toMoveChoice(m)
	send(m, "right", "f2")
	contains(t, m, "cannot be moved into "+moveSource, "other-filesystem: the folder and the source are on different file systems",
		"what to do: Choose a source on the same file system.", "Nothing was changed.")
	send(m, "y")
	wantCommands(t, m, f, []string{"source", "move", "--into", moveSource, "--output", "json", "--", foreignDir})
}

func TestMovePreviewRunsWithoutATerminalAndTheMoveHasTheExactCommand(t *testing.T) {
	outcome := jsonOf(t, map[string]any{"target": moveTarget, "project": "legacy-blog", "result": "moved", "location": moveTarget, "commit": "0123456789abcdef"})
	for _, root := range []bool{false, true} {
		name := "sudo"
		if root {
			name = "root"
		}
		t.Run(name, func(t *testing.T) {
			b := moveBackend()
			m, f := moveModel(t, i18n.English, b, root, wideWidth, 50, jsonOf(t, movePreview(nil)), outcome)
			toMoveChoice(m)
			send(m, "right", "f2")
			contains(t, m, "Move the project legacy-blog into the source "+moveSource+" as the folder "+moveTarget,
				"target folder: "+moveTarget, "Files of the first commit (2)", "compose.yaml", "Warning", "downtime:")
			if got := shows(m.render(), "sudo qhost source move --into "+moveSource+" --yes --output json -- "+foreignDir); got == root {
				t.Fatalf("the command is shown %v, as root %v:\n%s", got, root, m.render())
			}
			// The preview gets a standard input that is no terminal, so the command cannot ask to move.
			if got := f.reqs[len(f.reqs)-1]; len(got.Secret) == 0 && !got.Validate {
				t.Fatalf("the preview has the terminal: %+v", got)
			}
			send(m, "y")
			wantCommands(t, m, f,
				[]string{"source", "move", "--into", moveSource, "--output", "json", "--", foreignDir},
				[]string{"source", "move", "--into", moveSource, "--yes", "--output", "json", "--", foreignDir})
			contains(t, m, "The project legacy-blog is moved to", "and deployed there.", "the folder is now in: "+moveTarget, "commit: 0123456789")
		})
	}
}

func TestMoveCommandHasNoEngineFlagOnAHostWithSeveralEngines(t *testing.T) {
	outcome := jsonOf(t, map[string]any{"target": moveTarget, "project": "legacy-blog", "result": "moved", "location": moveTarget, "commit": "0123456789abcdef"})
	b := moveBackend()
	b.engines = []string{control.EngineSystem, aliceEngine}
	for i := range b.status.Engine.Sources {
		b.status.Engine.Sources[i].Engine = control.EngineSystem
	}
	m, f := moveModel(t, i18n.English, b, false, wideWidth, 50, jsonOf(t, movePreview(nil)), outcome)
	toMoveChoice(m)
	send(m, "right", "f2")
	if !shows(m.render(), "sudo qhost source move --into "+moveSource+" --yes --output json -- "+foreignDir) {
		t.Fatalf("the command without --engine is not shown:\n%s", m.render())
	}
	send(m, "y")
	wantCommands(t, m, f,
		[]string{"source", "move", "--into", moveSource, "--output", "json", "--", foreignDir},
		[]string{"source", "move", "--into", moveSource, "--yes", "--output", "json", "--", foreignDir})
}

func TestMoveShowsTheWayBackAndWhatRemains(t *testing.T) {
	for name, c := range map[string]struct {
		outcome map[string]any
		want    []string
	}{
		"rolled back": {map[string]any{"result": "rolled-back", "failed_step": "deploy", "error": "the run failed", "location": foreignDir,
			"rollback": []map[string]string{{"step": "rename", "result": "done"}}, "remaining": []string{}},
			[]string{"failed at the step deploy and was taken back: the run failed", "the folder is now in: " + foreignDir, "way back, rename: done"}},
		"incomplete": {map[string]any{"result": "rollback-incomplete", "failed_step": "commit", "error": "push rejected", "location": moveTarget,
			"rollback": []map[string]string{{"step": "rename", "result": "failed", "detail": "busy"}}, "remaining": []string{"rename " + moveTarget + " back"}},
			[]string{"the way back did not finish: push rejected", "way back, rename: failed busy", "left to do by hand: rename " + moveTarget + " back"}},
	} {
		t.Run(name, func(t *testing.T) {
			b := moveBackend()
			m, f := moveModel(t, i18n.English, b, false, wideWidth, 50, jsonOf(t, movePreview(nil)), jsonOf(t, c.outcome))
			f.runErr = exitCode(t, "1")
			toMoveChoice(m)
			send(m, "right", "f2", "y")
			says(t, m, c.want...)
		})
	}
}

// The empty line the preview gets as its standard input is no secret: what qhost wrote to standard error stays whole.
func TestScrubKeepsTheLinesForABlankInput(t *testing.T) {
	stderr := "qhost: source-unreadable: the source cannot be read\nhint: run it again\n"
	if got := scrub(stderr, []byte("\n")); got != stderr {
		t.Fatalf("scrub changed the text: %q", got)
	}
	if got := scrub("value secret-e2e here", []byte("secret-e2e")); got != "value <hidden> here" {
		t.Fatalf("scrub kept a secret: %q", got)
	}
}
