package cli

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/window"
)

type windowEnv struct {
	mu          sync.Mutex
	order       []string
	showCount   int
	fingerprint func(n int) string
	current     func(n int) control.WindowResult
	showN       int
	content     control.WindowSetArgs
	set         control.WindowSetArgs
	result      control.ProposalResultArgs
}

func (e *windowEnv) note(s string) {
	e.mu.Lock()
	e.order = append(e.order, s)
	e.mu.Unlock()
}

func (e *windowEnv) history() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return strings.Join(e.order, ",")
}

func startWindowEnv(t *testing.T, env *windowEnv) {
	t.Helper()
	if env.fingerprint == nil {
		env.fingerprint = func(int) string { return "fp-one" }
	}
	if env.current == nil {
		env.current = func(int) control.WindowResult {
			return control.WindowResult{Days: []string{"mon", "tue"}, Start: "03:00", Duration: "2h", Zone: "local"}
		}
	}
	startFakeService(t, fakeHandlers{
		control.ProposalShowCommand: func(json.RawMessage) any {
			env.mu.Lock()
			env.showCount++
			n := env.showCount
			env.mu.Unlock()
			env.note("show")
			info := proposalInfo(approveID, control.ProposalOpen)
			info.Kind = control.ProposalKindWindow
			info.Fingerprint = env.fingerprint(n)
			content, _ := json.Marshal(env.content)
			return control.ProposalShowResult{ProposalInfo: info, Content: content}
		},
		control.WindowShowCommand: func(json.RawMessage) any {
			env.mu.Lock()
			env.showN++
			n := env.showN
			env.mu.Unlock()
			env.note("get")
			return env.current(n)
		},
		control.ProposalApproveCommand: func(json.RawMessage) any {
			env.note("approve")
			return proposalInfo(approveID, control.ProposalApproving)
		},
		control.WindowSetCommand: func(raw json.RawMessage) any {
			env.note("set")
			_ = json.Unmarshal(raw, &env.set)
			return control.WindowResult{Days: env.set.Days, Start: env.set.Start, Duration: "2h", Zone: "local"}
		},
		control.ProposalResultCommand: func(raw json.RawMessage) any {
			env.note("result")
			_ = json.Unmarshal(raw, &env.result)
			return proposalInfo(approveID, control.ProposalApplied)
		},
	})
}

func TestWindowApproveWithAnEngineShowsTheDifferenceAndSetsTheOwnWindow(t *testing.T) {
	useFolderHost(t)
	env := &windowEnv{content: control.WindowSetArgs{Days: []string{"sat"}, Start: "01:30"}}
	startWindowEnv(t, env)
	useTerminalStdin(t, "y\n")
	code, stdout, stderr := run(t, "proposal", "approve", approveID, "--engine", "system", "--output", "json")
	if code != 0 {
		t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if got, want := env.history(), "show,get,show,get,approve,set,result"; got != want {
		t.Fatalf("order %q, want %q", got, want)
	}
	if strings.Join(env.set.Days, ",") != "sat" || env.set.Start != "01:30" || !env.result.Applied {
		t.Fatalf("set %+v, result %+v", env.set, env.result)
	}
	for _, want := range []string{"days", `"mon,tue"`, `"sat"`, "start", `"03:00"`, `"01:30"`} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr lacks %q:\n%s", want, stderr)
		}
	}
	if strings.Contains(stderr, "duration") {
		t.Errorf("an unchanged part is shown:\n%s", stderr)
	}
}

func TestWindowApproveShowsNoControlCharacters(t *testing.T) {
	useFolderHost(t)
	env := &windowEnv{content: control.WindowSetArgs{Zone: "UTC\x1b[2J\u202e"}}
	startWindowEnv(t, env)
	useTerminalStdin(t, "n\n")
	_, _, stderr := run(t, "proposal", "approve", approveID, "--engine", "system")
	if strings.ContainsAny(stderr, "\x1b\u202e") {
		t.Fatalf("stderr holds a control character: %q", stderr)
	}
}

func TestWindowApproveRefusesAChangedCurrentWindowAndShowsIt(t *testing.T) {
	useFolderHost(t)
	env := &windowEnv{content: control.WindowSetArgs{Start: "01:30"}, current: func(n int) control.WindowResult {
		w := control.WindowResult{Days: []string{"mon"}, Start: "03:00", Duration: "2h", Zone: "local"}
		if n > 1 {
			w.Start = "04:00"
		}
		return w
	}}
	startWindowEnv(t, env)
	useTerminalStdin(t, "y\n")
	code, _, stderr := run(t, "proposal", "approve", approveID, "--engine", "system")
	if code == 0 || !strings.Contains(stderr, "The effect changed since it was shown") ||
		strings.Contains(env.history(), "approve") || strings.Contains(env.history(), "set") {
		t.Fatalf("code %d, order %q, stderr %q", code, env.history(), stderr)
	}
}

func TestWindowApproveRefusesAChangedProposal(t *testing.T) {
	useFolderHost(t)
	env := &windowEnv{content: control.WindowSetArgs{Start: "01:30"}, fingerprint: func(n int) string {
		if n == 1 {
			return "fp-one"
		}
		return "fp-two"
	}}
	startWindowEnv(t, env)
	useTerminalStdin(t, "y\n")
	code, _, stderr := run(t, "proposal", "approve", approveID, "--engine", "system")
	if code == 0 || !strings.Contains(stderr, "changed since it was shown") || strings.Contains(env.history(), "approve") {
		t.Fatalf("code %d, order %q, stderr %q", code, env.history(), stderr)
	}
}

func TestWindowApproveDeclinedChangesNothing(t *testing.T) {
	useFolderHost(t)
	env := &windowEnv{content: control.WindowSetArgs{Start: "01:30"}}
	startWindowEnv(t, env)
	useTerminalStdin(t, "n\n")
	if code, _, _ := run(t, "proposal", "approve", approveID, "--engine", "system"); code == 0 || env.history() != "show,get" {
		t.Fatalf("code %d, order %q", code, env.history())
	}
}

func TestWindowApproveWithoutAnEngineSetsTheHostWindowFile(t *testing.T) {
	useFolderHost(t)
	file := useHostWindow(t)
	if _, err := window.SetHost(file, "", window.Change{Zone: ptr("UTC"), Start: ptr(180)}); err != nil {
		t.Fatal(err)
	}
	env := &windowEnv{content: control.WindowSetArgs{Days: []string{"sat"}, Start: "01:30"}}
	startWindowEnv(t, env)
	useTerminalStdin(t, "y\n")
	code, stdout, stderr := run(t, "proposal", "approve", approveID, "--output", "json")
	if code != 0 {
		t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	// the host window is read from its file and written by the client: no window command reaches a service
	if got, want := env.history(), "show,show,approve,result"; got != want {
		t.Fatalf("order %q, want %q", got, want)
	}
	w, ok, err := window.ReadHost(file)
	if err != nil || !ok || w.Start != 90 || w.Zone != "UTC" || w.Days.Names()[0] != "sat" || !env.result.Applied {
		t.Fatalf("host window = %+v, %v, %v; result %+v", w, ok, err, env.result)
	}
	for _, want := range []string{"the host window would change", `"03:00"`, `"01:30"`} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr lacks %q:\n%s", want, stderr)
		}
	}
	if got := decodeObject(t, stdout); got["origin"] != "host" || got["start"] != "01:30" {
		t.Errorf("output = %s", stdout)
	}
}

func TestWindowApproveWithAnEngineNamesTheEngineInTheEffect(t *testing.T) {
	useFolderHost(t)
	env := &windowEnv{content: control.WindowSetArgs{Start: "01:30"}}
	startWindowEnv(t, env)
	useTerminalStdin(t, "n\n")
	_, _, stderr := run(t, "proposal", "approve", approveID, "--engine", "system")
	if !strings.Contains(stderr, "the own window of the engine system would change") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestWindowApproveOfTheHostWindowNeedsRootAndLeavesTheFileAlone(t *testing.T) {
	useFolderHost(t)
	file := useHostWindow(t)
	windowEuid = func() int { return 1000 }
	env := &windowEnv{content: control.WindowSetArgs{Start: "01:30"}}
	startWindowEnv(t, env)
	useTerminalStdin(t, "y\n")
	code, _, stderr := run(t, "proposal", "approve", approveID)
	if code == 0 || !strings.Contains(stderr, "needs root") || !env.result.Applied == false && env.result.Message == "" {
		t.Fatalf("code %d, stderr %q, result %+v", code, stderr, env.result)
	}
	if _, ok, _ := window.ReadHost(file); ok {
		t.Fatal("the host window was written without root")
	}
	if env.result.Applied {
		t.Fatalf("the failed change was recorded as applied: %+v", env.result)
	}
}
