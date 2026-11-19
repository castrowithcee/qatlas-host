package cli

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/castrowithcee/qatlas-kit/output"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/rootless"
)

// engineDouble is one rootless engine of the current user behind a real socket; its list is the fake's.
type engineDouble struct {
	mu    sync.Mutex
	list  func(n int) []rootless.Container
	calls int
}

type engineClient struct{ d *engineDouble }

func (c engineClient) List(context.Context) ([]rootless.Container, error) {
	c.d.mu.Lock()
	defer c.d.mu.Unlock()
	c.d.calls++
	return c.d.list(c.d.calls), nil
}
func (engineClient) Close() {}

func (d *engineDouble) count() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.calls
}

// useEngine makes the host have one rootless engine of the current user, named after user.
func useEngine(t *testing.T, user string, list func(n int) []rootless.Container) *engineDouble {
	t.Helper()
	dir, err := os.MkdirTemp("", "eng")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	uid := os.Getuid()
	rundir := filepath.Join(dir, strconv.Itoa(uid))
	if err := os.MkdirAll(rundir, 0o700); err != nil {
		t.Fatal(err)
	}
	l, err := net.Listen("unix", filepath.Join(rundir, "docker.sock"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	d := &engineDouble{list: list}
	folders.detector = rootless.Detector{RunDir: dir, Timeout: time.Second,
		Users: []rootless.User{{Name: user, UID: uint32(uid), Home: dir}},
		Dial:  func(string, time.Duration) (rootless.Client, error) { return engineClient{d}, nil }}
	return d
}

func stackResult(path string) control.SourceInspectResult {
	return control.SourceInspectResult{Path: path, Case: control.SourceCaseLocal,
		Stacks: []control.SourceStack{{Name: "demo", Folder: "demo", Conflicts: []control.SourceConflict{
			{Container: "old-demo-1", Project: "old", Engine: control.EngineSystem}}}, {Name: "other", Folder: "other"}}}
}

func TestAddEnginesReportsConflictsPerStackFolderUnderAnyProjectName(t *testing.T) {
	useFolderHost(t)
	useEngine(t, "alice", func(int) []rootless.Container {
		return []rootless.Container{
			{Name: "demo-web-1", Project: "demo", WorkingDir: "/srv/stacks/demo"},
			{Name: "legacy-1", Project: "legacy", WorkingDir: "/srv/stacks/demo"},
			{Name: "elsewhere-1", Project: "demo", WorkingDir: "/srv/other"},
		}
	})
	res := addEngines(context.Background(), stackResult("/srv/stacks"))
	got := res.Stacks[0].Conflicts
	if len(got) != 3 || got[0].Engine != "system" || got[1] != (control.SourceConflict{Container: "demo-web-1", Project: "demo", Engine: "rootless:alice"}) ||
		got[2].Project != "legacy" {
		t.Fatalf("demo conflicts = %+v", got)
	}
	if len(res.Stacks[1].Conflicts) != 0 || len(res.EnginesNotChecked) != 0 {
		t.Fatalf("result = %+v", res)
	}
}

func TestAddEnginesScansOncePerCall(t *testing.T) {
	useFolderHost(t)
	e := useEngine(t, "alice", func(int) []rootless.Container { return nil })
	addEngines(context.Background(), stackResult("/srv/stacks"))
	if e.count() != 1 {
		t.Fatalf("the engine was asked %d times", e.count())
	}
}

func TestAddEnginesWithoutRootAsksNoEngineAndSaysWhy(t *testing.T) {
	useFolderHost(t)
	e := useEngine(t, "alice", func(int) []rootless.Container {
		return []rootless.Container{{Name: "x", Project: "demo", WorkingDir: "/srv/stacks/demo"}}
	})
	folders.euid = func() int { return 1000 }
	res := addEngines(context.Background(), stackResult("/srv/stacks"))
	if e.count() != 0 || len(res.Stacks[0].Conflicts) != 1 {
		t.Fatalf("asked %d times, conflicts %+v", e.count(), res.Stacks[0].Conflicts)
	}
	if len(res.EnginesNotChecked) != 1 || res.EnginesNotChecked[0].Engine != "rootless" ||
		!strings.Contains(res.EnginesNotChecked[0].Reason, "needs root") {
		t.Fatalf("not checked = %+v", res.EnginesNotChecked)
	}
}

func TestAddEnginesKeepsRejectionAndEmptyAnswers(t *testing.T) {
	useFolderHost(t)
	e := useEngine(t, "alice", func(int) []rootless.Container { return nil })
	rej := control.SourceInspectResult{Path: "/srv/x", Case: control.SourceCaseReject, Stacks: []control.SourceStack{}}
	none := control.SourceInspectResult{Path: "/srv/x", Case: control.SourceCaseClone, Stacks: []control.SourceStack{}}
	addEngines(context.Background(), rej)
	addEngines(context.Background(), none)
	if e.count() != 0 {
		t.Fatalf("engine asked %d times", e.count())
	}
}

func TestAddEnginesListsAnEngineThatIsSetUpButNotRunning(t *testing.T) {
	useFolderHost(t)
	home := t.TempDir()
	unit := filepath.Join(home, ".config", "systemd", "user")
	if err := os.MkdirAll(unit, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(unit, "docker.service"), []byte("[Service]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	folders.detector = rootless.Detector{RunDir: t.TempDir(), Users: []rootless.User{{Name: "bob", UID: 4242, Home: home}}}
	res := addEngines(context.Background(), stackResult("/srv/stacks"))
	if len(res.EnginesNotChecked) != 1 || res.EnginesNotChecked[0].Engine != "rootless:bob" || len(res.Stacks[0].Conflicts) != 1 {
		t.Fatalf("result = %+v", res)
	}
}

func TestSourceResultNamesTheEngineAndTheEnginesNotChecked(t *testing.T) {
	res := stackResult("/srv/stacks")
	res.Stacks[0].Conflicts = append(res.Stacks[0].Conflicts, control.SourceConflict{Container: "web-1", Project: "demo", Engine: "rootless:alice"})
	res.EnginesNotChecked = []control.EngineNotChecked{{Engine: "rootless:bob", Reason: "its engine is not running"}}
	var text strings.Builder
	for _, f := range sourceResult(output.FormatTable, res).Fields {
		text.WriteString(f.Name + "=" + toString(f.Value) + "\n")
	}
	for _, want := range []string{"in the system engine", "in the rootless:alice engine", "duplicate containers", "ports twice",
		"engine_not_checked=engine rootless:bob was not checked: its engine is not running"} {
		if !strings.Contains(text.String(), want) {
			t.Fatalf("text lacks %q:\n%s", want, text.String())
		}
	}
	flat := sourceResult(output.FormatJSON, res)
	var names []string
	for _, f := range flat.Fields {
		names = append(names, f.Name)
	}
	if !strings.Contains(strings.Join(names, ","), "engines_not_checked") {
		t.Fatalf("json fields %v", names)
	}
}

func toString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func TestProposalApproveRefusesANewEngineConflictBetweenShowAndApply(t *testing.T) {
	d := useFolderHost(t)
	dir := t.TempDir()
	useEngine(t, "alice", func(n int) []rootless.Container {
		if n == 1 {
			return nil
		}
		return []rootless.Container{{Name: "demo-web-1", Project: "demo", WorkingDir: dir}}
	})
	env := &approveEnv{dir: dir}
	startApproveEnv(t, env)
	useTerminalStdin(t, "y\n")
	code, _, stderr := run(t, "proposal", "approve", approveID, "--output", "json")
	if code == 0 || !strings.Contains(stderr, "effect of the proposal changed") ||
		strings.Contains(env.history(), "approve") || strings.Contains(env.history(), "add") || d.dropIn(t) != "" {
		t.Fatalf("code %d, order %q, stderr %q", code, env.history(), stderr)
	}
}

func TestProposalApproveRefusesANewEngineNotChecked(t *testing.T) {
	d := useFolderHost(t)
	dir := t.TempDir()
	home := t.TempDir()
	unit := filepath.Join(home, ".config", "systemd", "user")
	if err := os.MkdirAll(unit, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(unit, "docker.service"), []byte("[Service]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The first look finds no engine; from the second inspection on, the engine of bob is set up but not running.
	env := &approveEnv{dir: dir, inspect: func(n int) control.SourceInspectResult {
		if n > 1 {
			folders.detector = rootless.Detector{RunDir: t.TempDir(), Users: []rootless.User{{Name: "bob", UID: 4242, Home: home}}}
		}
		return localInspect(false)
	}}
	startApproveEnv(t, env)
	useTerminalStdin(t, "y\n")
	code, _, stderr := run(t, "proposal", "approve", approveID)
	if code == 0 || !strings.Contains(stderr, "effect of the proposal changed") || strings.Contains(env.history(), "add") || d.dropIn(t) != "" {
		t.Fatalf("code %d, order %q, stderr %q", code, env.history(), stderr)
	}
}
