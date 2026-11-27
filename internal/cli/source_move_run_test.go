package cli

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

const (
	moveHead   = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	moveCommit = "cccccccccccccccccccccccccccccccccccccccc"
	moveRevert = "dddddddddddddddddddddddddddddddddddddddd"
)

// moveFake is a deploy service that acts as the real one would on the folders of a move: it checks where the
// folder is at each call and records the calls in their order.
type moveFake struct {
	t                     *testing.T
	mu                    sync.Mutex
	events                []string
	path, into, target    string
	head                  string
	runs                  []control.RunInfo
	commitErr             *control.Error
	commitLands           bool
	deployErr             *control.Error
	runResult             string
	revertBusy            int
	occupyOldPlace        bool
	targetGroupWritable   bool
	headReadWithFolderOut []bool
}

func (f *moveFake) record(e string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, e)
}

func (f *moveFake) recorded() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.events)
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// startMove makes a project folder and a source below a temporary folder and starts the fake service. It moves
// in the rootless engine of alice, where the permissions of the moved folder are set, or with inSystemEngine in
// the system engine, whose deploy service is root and gets no permission change.
func startMove(t *testing.T, change func(*moveFake), inSystemEngine ...bool) *moveFake {
	t.Helper()
	if d := useFolderHost(t); len(inSystemEngine) == 0 || !inSystemEngine[0] {
		d.useRootless(t)
	}
	oldWait, oldPoll := moveRunWait, movePoll
	moveRunWait, movePoll = 5*time.Second, time.Millisecond
	t.Cleanup(func() { moveRunWait, movePoll = oldWait, oldPoll })

	f := newMoveFake(t)
	if change != nil {
		change(f)
	}
	preview := movePreview()
	preview.Running = []control.SourceMoveRunning{{Container: "myapp-app-1", Project: "myapp"}}
	preview.BindMounts = []string{"data"}
	startFakeService(t, fakeHandlers{
		control.StatusCommand: statusWith([]string{f.into}),
		control.SourceMoveCommand: func(raw json.RawMessage) any {
			var a control.SourceMoveArgs
			_ = json.Unmarshal(raw, &a)
			res := preview
			res.Path, res.Into, res.Target = a.Path, a.Into, filepath.Join(a.Into, filepath.Base(a.Path))
			return res
		},
		control.SourceListCommand: func(json.RawMessage) any {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.headReadWithFolderOut = append(f.headReadWithFolderOut, exists(f.target) && !exists(f.path))
			return control.SourceListResult{Sources: []control.SourceStatus{{Path: f.into, Head: f.head}}}
		},
		control.RunsCommand: func(json.RawMessage) any {
			f.mu.Lock()
			defer f.mu.Unlock()
			return control.RunsResult{Runs: slices.Clone(f.runs)}
		},
		control.SourceProjectCommand: func(raw json.RawMessage) any {
			var a control.SourceProjectArgs
			_ = json.Unmarshal(raw, &a)
			if a.Path != f.path || a.Project != "myapp" || !exists(filepath.Join(f.path, "compose.yaml")) {
				return control.NewError(control.CodeInvalid, "the folder is not where the project is")
			}
			f.record(a.Action)
			return control.SourceProjectResult{Path: a.Path, Project: a.Project, Action: a.Action}
		},
		control.SourceFolderCommitCommand: func(raw json.RawMessage) any {
			var a control.SourceFolderCommitArgs
			_ = json.Unmarshal(raw, &a)
			f.record("commit")
			info, err := os.Stat(filepath.Join(f.target, "compose.yaml"))
			f.mu.Lock()
			defer f.mu.Unlock()
			f.targetGroupWritable = err == nil && info.Mode().Perm()&0o060 == 0o060 && !exists(f.path)
			if a.Path != f.into || a.Folder != "myapp" || a.Head != f.head || strings.Join(a.Files, ",") != ".env,compose.yaml" {
				return control.NewError(control.CodeInvalid, "unexpected commit arguments")
			}
			if f.commitErr != nil && !f.commitLands {
				return f.commitErr
			}
			// The service writes the lines of the plan into the .gitignore of the folder.
			_ = os.WriteFile(filepath.Join(f.target, ".gitignore"), []byte("/data\n"), 0o644)
			f.head = moveCommit
			if f.commitErr != nil {
				return f.commitErr
			}
			return control.SourceFolderCommitResult{Path: a.Path, Folder: a.Folder, Commit: moveCommit}
		},
		control.DeployCommand: func(json.RawMessage) any {
			f.record("deploy")
			if f.occupyOldPlace {
				_ = os.Mkdir(f.path, 0o755)
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.deployErr != nil && f.deployErr.Code != control.CodeRunActive {
				return f.deployErr
			}
			run := control.RunInfo{ID: "20250101T000001Z-00000001", Stack: "myapp", Commit: moveCommit, Result: f.runResult}
			if f.runResult != "succeeded" {
				run.Error = "check: the container app is unhealthy"
			}
			f.runs = append([]control.RunInfo{run}, f.runs...)
			if f.deployErr != nil {
				return f.deployErr
			}
			return control.DeployResult{ID: run.ID, Stack: "myapp", Commit: moveCommit}
		},
		control.SourceFolderRevertCommand: func(raw json.RawMessage) any {
			var a control.SourceFolderRevertArgs
			_ = json.Unmarshal(raw, &a)
			f.mu.Lock()
			busy := f.revertBusy > 0
			f.revertBusy--
			f.mu.Unlock()
			if busy {
				return control.NewError(control.CodeBusy, "the working copy is busy with an update or a rollback; try again")
			}
			f.record("revert " + a.Commit[:4])
			if exists(f.target) || !exists(filepath.Join(f.path, "compose.yaml")) {
				return control.NewError(control.CodeInvalid, "the folder is still in the source")
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			f.head = moveRevert
			return control.SourceFolderRevertResult{Path: a.Path, Folder: a.Folder, Revert: moveRevert, Head: moveRevert}
		},
	})
	return f
}

// newMoveFake lays out the project folder with its files and the source folder below a temporary folder.
func newMoveFake(t *testing.T) *moveFake {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f := &moveFake{t: t, path: filepath.Join(base, "legacy", "myapp"), into: filepath.Join(base, "stacks"), head: moveHead, runResult: "succeeded",
		runs: []control.RunInfo{{ID: "20250101T000000Z-00000000", Stack: "myapp", Result: "failed"}}}
	f.target = filepath.Join(f.into, "myapp")
	for _, d := range []string{filepath.Join(f.path, "data"), f.into} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for name, text := range map[string]string{"compose.yaml": "services:\n  app:\n    image: registry.example.test/demo/web:1\n", ".env": "WEB_VERSION=1\n", "data/db.bin": "x"} {
		if err := os.WriteFile(filepath.Join(f.path, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

type moveOutput struct {
	Result     string     `json:"result"`
	Location   string     `json:"location"`
	Moved      bool       `json:"moved"`
	FailedStep string     `json:"failed_step"`
	Error      string     `json:"error"`
	Commit     string     `json:"commit"`
	Revert     string     `json:"revert"`
	Run        string     `json:"run"`
	RunResult  string     `json:"run_result"`
	Steps      []moveStep `json:"steps"`
	Rollback   []moveStep `json:"rollback"`
	Remaining  []string   `json:"remaining"`
}

func (f *moveFake) move(t *testing.T, wantCode int) moveOutput {
	t.Helper()
	code, stdout, stderr := run(t, "source", "move", f.path, "--into", f.into, "--yes", "--output", "json")
	if code != wantCode {
		t.Fatalf("code %d, want %d; stdout %q, stderr %q", code, wantCode, stdout, stderr)
	}
	var out moveOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("%v: %s", err, stdout)
	}
	return out
}

func stepNames(list []moveStep, result string) []string {
	var out []string
	for _, s := range list {
		if s.Result == result {
			out = append(out, s.Step)
		}
	}
	return out
}

func permsOf(t *testing.T, path string) fs.FileMode {
	t.Helper()
	return modeOf(t, path) & (fs.ModePerm | fs.ModeSetgid)
}

// wantBackAsBefore checks that the folder is in its old place with its modes and without a .gitignore.
func (f *moveFake) wantBackAsBefore(t *testing.T) {
	t.Helper()
	if exists(f.target) || !exists(filepath.Join(f.path, "data/db.bin")) {
		t.Fatal("the folder is not back in its old place with its data")
	}
	if m := permsOf(t, filepath.Join(f.path, "compose.yaml")); m != 0o600 {
		t.Errorf("compose.yaml mode %v, want 0600", m)
	}
	if m := permsOf(t, f.path); m != 0o700 {
		t.Errorf("folder mode %v, want 0700", m)
	}
	if exists(filepath.Join(f.path, ".gitignore")) {
		t.Error("the .gitignore the move wrote is still there")
	}
}

func TestSourceMoveRunsDownRenamePermissionsCommitAndDeployInThisOrder(t *testing.T) {
	f := startMove(t, nil)
	out := f.move(t, 0)
	if got := f.recorded(); strings.Join(got, ",") != "down,commit,deploy" {
		t.Fatalf("calls = %v", got)
	}
	want := []string{"down", "rename", "permissions", "commit", "deploy"}
	if !slices.Equal(stepNames(out.Steps, "done"), want) || len(out.Rollback) != 0 || len(out.Remaining) != 0 {
		t.Fatalf("steps = %+v, rollback %+v", out.Steps, out.Rollback)
	}
	if out.Result != moveMoved || !out.Moved || out.Location != f.target || out.Commit != moveCommit || out.RunResult != "succeeded" {
		t.Fatalf("output = %+v", out)
	}
	if !f.targetGroupWritable {
		t.Fatal("the commit did not find the folder renamed and writable for the service group")
	}
	// The head the commit builds on is read with the folder already in the source.
	if len(f.headReadWithFolderOut) == 0 || !f.headReadWithFolderOut[0] {
		t.Fatalf("head reads with the folder renamed: %v", f.headReadWithFolderOut)
	}
	if exists(f.path) || permsOf(t, f.target)&fs.ModeSetgid == 0 || permsOf(t, filepath.Join(f.target, "data/db.bin")) != 0o600 {
		t.Fatal("the folder is not in the source with the group bits, or the bind mount was changed")
	}
}

func TestSourceMoveInTheSystemEngineSetsNoPermissions(t *testing.T) {
	f := startMove(t, nil, true)
	out := f.move(t, 0)
	if out.Result != moveMoved || !slices.Equal(stepNames(out.Steps, "done"), []string{"down", "rename", "permissions", "commit", "deploy"}) {
		t.Fatalf("output = %+v", out)
	}
	if permsOf(t, f.target)&(fs.ModeSetgid|0o070) != 0 {
		t.Fatalf("the folder got group bits: %v", permsOf(t, f.target))
	}
}

func TestSourceMoveWithoutYesAndABlockChangesNothing(t *testing.T) {
	f := startMove(t, nil)
	if code, stdout, _ := run(t, "source", "move", f.path, "--into", f.into, "--output", "json"); code != 0 || !strings.Contains(stdout, `"movable":true`) {
		t.Fatalf("code %d, stdout %s", code, stdout)
	}
	if len(f.recorded()) != 0 || exists(f.target) {
		t.Fatalf("the preview acted: %v", f.recorded())
	}
	if err := os.Mkdir(f.target, 0o755); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := run(t, "source", "move", f.path, "--into", f.into, "--yes"); code != 2 || !strings.Contains(stderr, "nothing was changed") {
		t.Fatalf("code %d, stderr %s", code, stderr)
	}
	if len(f.recorded()) != 0 || !exists(filepath.Join(f.path, "compose.yaml")) {
		t.Fatalf("calls %v", f.recorded())
	}
}

func TestSourceMoveFailingBeforeThePushTakesBackRenameAndPermissions(t *testing.T) {
	f := startMove(t, func(f *moveFake) {
		f.commitErr = control.NewError(control.CodePushRejected, "the remote branch moved; nothing was committed")
	})
	out := f.move(t, 1)
	if got := f.recorded(); strings.Join(got, ",") != "down,commit,up" {
		t.Fatalf("calls = %v", got)
	}
	if out.Result != moveRolledBack || out.FailedStep != "commit" || out.Location != f.path || out.Commit != "" {
		t.Fatalf("output = %+v", out)
	}
	if got := stepNames(out.Rollback, "done"); strings.Join(got, ",") != "rename-back,permissions,gitignore,up" {
		t.Fatalf("rollback = %+v", out.Rollback)
	}
	f.wantBackAsBefore(t)
}

func TestSourceMoveFailingAfterThePushRevertsTheCommit(t *testing.T) {
	// The commit lands, but its answer is lost: the head tells.
	f := startMove(t, func(f *moveFake) {
		f.commitErr, f.commitLands = control.NewError(control.CodeRemote, "the connection was lost"), true
	})
	out := f.move(t, 1)
	if got := f.recorded(); strings.Join(got, ",") != "down,commit,revert cccc,up" {
		t.Fatalf("calls = %v", got)
	}
	if out.Result != moveRolledBack || out.Commit != moveCommit || out.Revert != moveRevert {
		t.Fatalf("output = %+v", out)
	}
	f.wantBackAsBefore(t)
}

func TestSourceMoveWithAFailedCheckRemovesTheContainersAndRevertsTheCommit(t *testing.T) {
	// The revert waits while runs of other stacks hold the working copy.
	f := startMove(t, func(f *moveFake) { f.runResult, f.revertBusy = "failed", 2 })
	out := f.move(t, 1)
	if got := f.recorded(); strings.Join(got, ",") != "down,commit,deploy,down,revert cccc,up" {
		t.Fatalf("calls = %v", got)
	}
	if out.Result != moveRolledBack || out.FailedStep != "deploy" || out.RunResult != "failed" || !strings.Contains(out.Error, "unhealthy") {
		t.Fatalf("output = %+v", out)
	}
	want := "rename-back,remove-containers,revert,permissions,gitignore,up"
	if got := stepNames(out.Rollback, "done"); strings.Join(got, ",") != want {
		t.Fatalf("rollback = %+v", out.Rollback)
	}
	f.wantBackAsBefore(t)
}

func TestSourceMoveWaitsForTheRunThePollStarted(t *testing.T) {
	f := startMove(t, func(f *moveFake) {
		f.deployErr = control.NewError(control.CodeRunActive, "a run of this stack is already active")
	})
	out := f.move(t, 0)
	if out.Result != moveMoved || out.Run != "20250101T000001Z-00000001" {
		t.Fatalf("output = %+v", out)
	}
}

func TestSourceMoveSaysWhatIsLeftWhenTheWayBackStops(t *testing.T) {
	f := startMove(t, func(f *moveFake) { f.runResult, f.occupyOldPlace = "failed", true })
	code, stdout, stderr := run(t, "source", "move", f.path, "--into", f.into, "--yes", "--output", "json")
	if code != 1 || !strings.Contains(stderr, "Finish the way back by hand") || !strings.Contains(stderr, "mv "+f.target+" "+f.path) {
		t.Fatalf("code %d, stderr %s", code, stderr)
	}
	var out moveOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatal(err)
	}
	if out.Result != moveIncomplete || out.Location != f.target || len(out.Remaining) != 6 {
		t.Fatalf("output = %+v", out)
	}
	if got := f.recorded(); strings.Join(got, ",") != "down,commit,deploy" {
		t.Fatalf("calls after the stop = %v", got)
	}
	if !exists(filepath.Join(f.target, "compose.yaml")) {
		t.Fatal("the folder left the source")
	}
}
