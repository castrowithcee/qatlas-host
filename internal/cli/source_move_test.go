package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/rootless"
)

const (
	moveProject = "/srv/legacy/myapp"
	moveInto    = "/srv/stacks"
)

// movePreview is what the service answers for a project that can be moved.
func movePreview() control.SourceMoveResult {
	return control.SourceMoveResult{
		Path: moveProject, Into: moveInto, Target: moveInto + "/myapp", SourceKind: control.SourceKindRemote,
		Project: "myapp", NewProject: "myapp", Blocks: []control.SourceMoveIssue{}, Warnings: []control.SourceMoveIssue{},
		PlannedFiles:      []string{".env", "compose.yaml"},
		Excluded:          []control.SourceExcluded{{Path: "data", Reason: control.SourceExcludedBindMount, Stack: "myapp"}},
		Ignore:            &control.SourceIgnore{Action: control.SourceIgnoreCreate, Lines: []string{"/data"}},
		ContainersChecked: true,
	}
}

func moveWith(res control.SourceMoveResult) func(json.RawMessage) any {
	return func(raw json.RawMessage) any {
		var a control.SourceMoveArgs
		_ = json.Unmarshal(raw, &a)
		res.Path, res.Into = a.Path, a.Into
		return res
	}
}

func moveHandlers(res control.SourceMoveResult) fakeHandlers {
	return fakeHandlers{control.SourceMoveCommand: moveWith(res), control.StatusCommand: statusWith([]string{moveInto})}
}

func issueCodes(list []control.SourceMoveIssue) []string {
	var out []string
	for _, i := range list {
		out = append(out, i.Code)
	}
	return out
}

func moveJSON(t *testing.T, args ...string) control.SourceMoveResult {
	t.Helper()
	code, stdout, stderr := run(t, append([]string{"source", "move", moveProject, "--into", moveInto, "--output", "json"}, args...)...)
	if code != 0 {
		t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	var out struct {
		control.SourceMoveResult
		Movable bool `json:"movable"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("%v: %s", err, stdout)
	}
	if out.Movable != (len(out.Blocks) == 0) {
		t.Fatalf("movable = %v with blocks %+v", out.Movable, out.Blocks)
	}
	return out.SourceMoveResult
}

func TestSourceMovePreviewShowsTargetFilesAndChangesNothing(t *testing.T) {
	useFolderHost(t)
	svc := startFakeService(t, moveHandlers(movePreview()))
	code, stdout, stderr := run(t, "source", "move", moveProject, "--into", moveInto)
	if code != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	// The text format shows the field names in capitals.
	stdout = strings.ToLower(stdout)
	for _, want := range []string{"target", moveInto + "/myapp", "movable", "yes", "planned_file", "compose.yaml", "data: bind-mount", "gitignore_line", "/data"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout lacks %q: %s", want, stdout)
		}
	}
	for _, command := range []string{control.SourceAddCommand, control.SourceRemoveCommand, control.SourceRemoteCommand} {
		if svc.count(command) != 0 {
			t.Fatalf("the preview called %s", command)
		}
	}
	res := moveJSON(t)
	if res.Target != moveInto+"/myapp" || res.Engine != control.EngineSystem || len(res.PlannedFiles) != 2 || res.Ignore == nil {
		t.Fatalf("json = %+v", res)
	}
}

func TestSourceMoveBlocksAFolderOnAnotherFileSystem(t *testing.T) {
	d := useFolderHost(t)
	d.otherFS = true
	startFakeService(t, moveHandlers(movePreview()))
	res := moveJSON(t)
	if got := issueCodes(res.Blocks); len(got) != 1 || got[0] != control.MoveOtherFilesystem || res.Blocks[0].Fix == "" {
		t.Fatalf("blocks = %+v", res.Blocks)
	}
	_, stdout, _ := run(t, "source", "move", moveProject, "--into", moveInto)
	stdout = strings.ToLower(stdout)
	if !strings.Contains(stdout, "block.other-filesystem") || !strings.Contains(stdout, "movable") || !strings.Contains(stdout, "no") {
		t.Fatalf("stdout = %s", stdout)
	}
}

func TestSourceMoveBlocksAProjectThatRunsInAnotherEngine(t *testing.T) {
	useFolderHost(t)
	useEngine(t, "alice", func(int) []rootless.Container {
		return []rootless.Container{{Name: "myapp-app-1", Project: "myapp", WorkingDir: moveProject}}
	})
	startFakeService(t, moveHandlers(movePreview()))
	res := moveJSON(t)
	b := res.Blocks
	if len(b) != 1 || b[0].Code != control.MoveEngineDiffers || len(b[0].Items) != 1 || b[0].Items[0] != "rootless:alice" {
		t.Fatalf("blocks = %+v", b)
	}
	if got := issueCodes(res.Warnings); len(got) != 1 || got[0] != control.MoveDowntime {
		t.Fatalf("warnings = %+v", res.Warnings)
	}
	if len(res.ProjectEngines) != 1 || res.ProjectEngines[0] != "rootless:alice" {
		t.Fatalf("engines = %v", res.ProjectEngines)
	}
}

func TestSourceMoveFromTheEngineOfTheSourceNeedsNoBlock(t *testing.T) {
	useFolderHost(t)
	preview := movePreview()
	preview.Running = []control.SourceMoveRunning{{Container: "myapp-app-1", Project: "myapp"}}
	startFakeService(t, moveHandlers(preview))
	res := moveJSON(t)
	if len(res.Blocks) != 0 || len(issueCodes(res.Warnings)) != 1 || res.Warnings[0].Code != control.MoveDowntime {
		t.Fatalf("result = %+v", res)
	}
}

func TestSourceMoveIntoARootlessSourceAsksTheSystemEngineForTheContainers(t *testing.T) {
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice")
	system := startFakeService(t, fakeHandlers{control.StatusCommand: statusWith(nil),
		control.ContainerListCommand: func(json.RawMessage) any {
			return control.ContainerListResult{Groups: []control.ContainerGroup{
				{Kind: control.ContainerKindForeign, Name: "myapp", WorkingDir: moveProject},
				{Kind: control.ContainerKindForeign, Name: "else", WorkingDir: "/srv/else"}}}
		}})
	alice := serveFake(t, eps["alice"], moveHandlers(movePreview()))
	res := moveJSON(t)
	if res.Engine != "rootless:alice" || alice.count(control.SourceMoveCommand) != 1 || system.count(control.SourceMoveCommand) != 0 {
		t.Fatalf("engine %q, calls %v / %v", res.Engine, alice.calls, system.calls)
	}
	if len(res.Blocks) != 1 || res.Blocks[0].Code != control.MoveEngineDiffers || res.Blocks[0].Items[0] != control.EngineSystem {
		t.Fatalf("blocks = %+v", res.Blocks)
	}
}

func TestSourceMoveChecksTheOtherEnginesForFoldersAndNames(t *testing.T) {
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice", "bob")
	startFakeService(t, moveHandlers(movePreview()))
	serveFake(t, eps["alice"], fakeHandlers{control.StatusCommand: statusWith([]string{moveInto + "/myapp/inner", "/srv/legacy"}, "myapp")})
	// bob is not served: its deploy service cannot be asked.
	res := moveJSON(t)
	got := issueCodes(res.Blocks)
	want := []string{control.MoveFolderRejected, control.MoveTargetOverlaps, control.MoveNameTaken}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("blocks = %v, want %v", got, want)
	}
	w, ok := issue(res.Warnings, control.MoveNotChecked)
	if !ok || len(w.Items) == 0 || !strings.Contains(strings.Join(w.Items, " "), "rootless:bob") {
		t.Fatalf("warnings = %+v", res.Warnings)
	}
}

func issue(list []control.SourceMoveIssue, code string) (control.SourceMoveIssue, bool) {
	for _, i := range list {
		if i.Code == code {
			return i, true
		}
	}
	return control.SourceMoveIssue{}, false
}

func TestSourceMoveWithoutRootSaysTheRootlessEnginesWereNotAsked(t *testing.T) {
	useFolderHost(t)
	folders.euid = func() int { return 1000 }
	startFakeService(t, moveHandlers(movePreview()))
	res := moveJSON(t)
	w, ok := issue(res.Warnings, control.MoveNotChecked)
	if !ok || !strings.Contains(strings.Join(w.Items, " "), "needs root") {
		t.Fatalf("warnings = %+v", res.Warnings)
	}
}

func TestSourceMoveShowsTheIssuesOfTheServiceWithReasonAndFixAndKeysOnly(t *testing.T) {
	useFolderHost(t)
	res := movePreview()
	res.Blocks = []control.SourceMoveIssue{
		{Code: control.MoveSecretsInEnv, Reason: "the source has a remote", Fix: "Move the keys to secrets.env.", Items: []string{"API_PASSWORD"}},
		{Code: control.MoveRootStack, Reason: "root stack", Fix: "Choose another source."},
	}
	res.Warnings = []control.SourceMoveIssue{{Code: control.MoveFixedTags, Reason: "fixed", Fix: "Use a variable.", Items: []string{"app: registry.example.test/demo/web"}}}
	startFakeService(t, moveHandlers(res))
	got := moveJSON(t)
	// Sorted in the fixed order: the root stack comes before the secrets.
	if strings.Join(issueCodes(got.Blocks), ",") != control.MoveRootStack+","+control.MoveSecretsInEnv {
		t.Fatalf("blocks = %+v", got.Blocks)
	}
	_, stdout, _ := run(t, "source", "move", moveProject, "--into", moveInto)
	stdout = strings.ToLower(stdout)
	for _, want := range []string{"block.secrets-in-env", "fix: move the keys to secrets.env. (api_password)", "warning.fixed-tags"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout lacks %q: %s", want, stdout)
		}
	}
}

func TestSourceMoveMasksUnprintablePathsOfOtherEngines(t *testing.T) {
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice")
	startFakeService(t, moveHandlers(movePreview()))
	serveFake(t, eps["alice"], fakeHandlers{control.StatusCommand: statusWith([]string{"/srv/legacy/myapp/in\x01ner"})})
	_, stdout, _ := run(t, "source", "move", moveProject, "--into", moveInto)
	if strings.ContainsRune(stdout, 1) || !strings.Contains(stdout, "in�ner") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestSourceMoveRefusesBadArguments(t *testing.T) {
	useFolderHost(t)
	startFakeService(t, moveHandlers(movePreview()))
	for _, args := range [][]string{
		{"source", "move", "relative", "--into", moveInto},
		{"source", "move", moveProject, "--into", "stacks"},
		{"source", "move", moveProject},
		{"source", "move", "--into", moveInto},
	} {
		if code, _, _ := run(t, args...); code != 2 {
			t.Errorf("%v: code %d, want 2", args, code)
		}
	}
}
