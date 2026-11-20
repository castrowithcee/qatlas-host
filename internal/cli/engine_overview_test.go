package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-kit/localipc"

	"github.com/castrowithcee/qatlas-host/internal/channeltest"
	"github.com/castrowithcee/qatlas-host/internal/control"
)

func stacksHandler(names ...string) fakeHandlers {
	return fakeHandlers{control.StacksListCommand: func(json.RawMessage) any {
		res := control.StacksResult{Dir: "/srv/x", Stacks: []control.StackInfo{}}
		for _, n := range names {
			res.Stacks = append(res.Stacks, control.StackInfo{Name: n, Folder: n, Source: "x"})
		}
		return res
	}}
}

func sourcesHandler(path string) fakeHandlers {
	return fakeHandlers{control.SourceListCommand: func(json.RawMessage) any {
		return control.SourceListResult{Sources: []control.SourceStatus{{ID: filepath.Base(path), Path: path, Kind: control.SourceKindLocal}}}
	}}
}

func TestStackListShowsEveryEngineInJSONAndTable(t *testing.T) {
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice")
	startFakeService(t, stacksHandler("web"))
	serveFake(t, eps["alice"], stacksHandler("demo"))

	code, stdout, stderr := run(t, "stack", "list", "--output", "json")
	if code != 0 || stderr != "" {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(stdout), &rows); err != nil {
		t.Fatalf("output %q: %v", stdout, err)
	}
	if len(rows) != 2 || rows[0]["stack"] != "web" || rows[0]["engine"] != "system" ||
		rows[1]["stack"] != "demo" || rows[1]["engine"] != "rootless:alice" {
		t.Fatalf("rows = %v", rows)
	}
	_, table, _ := run(t, "stack", "list")
	if !strings.Contains(strings.ToLower(table), "engine") || !strings.Contains(table, "rootless:alice") {
		t.Fatalf("table lacks the engine:\n%s", table)
	}
}

func TestStackListWithOneServiceOnlyAddsTheEngine(t *testing.T) {
	useFolderHost(t)
	system := startFakeService(t, stacksHandler("web"))
	code, stdout, stderr := run(t, "stack", "list", "--output", "json")
	if code != 0 || stderr != "" || !strings.Contains(stdout, `"engine":"system"`) {
		t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if system.count(control.StacksListCommand) != 1 {
		t.Fatal("the service was not asked once")
	}
}

func TestStackListWarnsAboutAnUnreachableInstance(t *testing.T) {
	d := useFolderHost(t)
	engineHost(t, d, "alice")
	startFakeService(t, stacksHandler("web"))

	code, stdout, stderr := run(t, "stack", "list", "--output", "json")
	if code != 0 || !strings.Contains(stdout, "web") || strings.Contains(stdout, "alice") {
		t.Fatalf("code %d, stdout %q", code, stdout)
	}
	if !strings.Contains(stderr, "warning: the deploy service of the rootless:alice engine could not be asked (not running)") {
		t.Fatalf("no warning: %q", stderr)
	}
}

func TestStackListFailsWhenNoServiceAnswers(t *testing.T) {
	d := useFolderHost(t)
	engineHost(t, d, "alice")
	useEndpoint(t, channeltest.Endpoint(t))
	code, _, stderr := run(t, "stack", "list")
	if code != ExitServiceNotRunning || !strings.Contains(stderr, "not running") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
}

func TestSourceListShowsEveryEngine(t *testing.T) {
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice", "bob")
	startFakeService(t, sourcesHandler("/srv/system"))
	serveFake(t, eps["alice"], sourcesHandler("/srv/alice"))

	code, stdout, stderr := run(t, "source", "list", "--output", "json")
	if code != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	var got struct {
		Sources []map[string]any `json:"sources"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil || len(got.Sources) != 2 {
		t.Fatalf("output %q: %v", stdout, err)
	}
	if got.Sources[0]["engine"] != "system" || got.Sources[1]["engine"] != "rootless:alice" || got.Sources[1]["path"] != "/srv/alice" {
		t.Fatalf("sources = %v", got.Sources)
	}
	if !strings.Contains(stderr, "warning: the deploy service of the rootless:bob engine could not be asked (not running)") {
		t.Fatalf("no warning about bob: %q", stderr)
	}
	_, text, _ := run(t, "source", "list", "--output", "compact")
	if !strings.Contains(text, "source.alice.engine") || !strings.Contains(text, "rootless:alice") {
		t.Fatalf("text lacks the engine:\n%s", text)
	}
}

func TestStatusListsStacksAndFoldersOfInstancesWithTheirEngine(t *testing.T) {
	startFakeService(t, withStatus(nil, []string{"/srv/system"}, "web"))
	root := deploySetup.host.Root
	for _, u := range []string{"alice", "bob"} {
		writeTestFile(t, filepath.Join(root, "etc/tmpfiles.d/qhost-deploy-"+u+".conf"), "")
	}
	ep := channeltest.Endpoint(t)
	useInstanceEndpoints(t, map[string]localipc.ServiceEndpoint{"alice": ep})
	serveFake(t, ep, withStatus(nil, []string{"/srv/alice"}, "demo"))

	code, stdout, stderr := run(t, "status", "--output", "json")
	if code != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"stack.web.engine": "system", "source.x.engine": "system",
		"instance.alice.deploy_service": "running", "instance.alice.stack.demo.engine": "rootless:alice",
		"instance.alice.source.x.engine": "rootless:alice", "instance.alice.source.x.path": "/srv/alice",
		"instance.bob.deploy_service": "not running",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %v, want %v", k, got[k], v)
		}
	}
}

func TestStackRunsRoutesByStackAndByInstance(t *testing.T) {
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice")
	runs := fakeHandlers{control.RunsCommand: func(json.RawMessage) any { return control.RunsResult{} }}
	system := startFakeService(t, withStatus(runs, nil, "web"))
	alice := serveFake(t, eps["alice"], withStatus(runs, nil, "demo"))

	if code, _, stderr := run(t, "stack", "runs", "demo"); code != 0 || alice.count(control.RunsCommand) != 1 || system.count(control.RunsCommand) != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if code, _, stderr := run(t, "stack", "runs"); code != 0 || system.count(control.RunsCommand) != 1 || alice.count(control.RunsCommand) != 1 {
		t.Fatalf("without a stack the system engine was not meant: code %d, stderr %q", code, stderr)
	}
	if code, _, stderr := run(t, "stack", "runs", "--engine", "rootless:alice"); code != 0 || alice.count(control.RunsCommand) != 2 {
		t.Fatalf("--engine: code %d, stderr %q", code, stderr)
	}
}

const testRunID = "20260102T030405Z-0123abcd"

func logHandlers(known bool) fakeHandlers {
	return fakeHandlers{control.LogCommand: func(json.RawMessage) any {
		if !known {
			return control.NewError(control.CodeNoSuchRun, "no such run")
		}
		return control.LogResult{Text: "log text\n"}
	}}
}

func TestStackLogAsksEveryServiceAndReadsTheOneThatKnowsTheRun(t *testing.T) {
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice")
	system := startFakeService(t, logHandlers(false))
	alice := serveFake(t, eps["alice"], logHandlers(true))

	code, stdout, stderr := run(t, "stack", "log", testRunID)
	if code != 0 || stdout != "log text\n" || stderr != "" {
		t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if system.count(control.LogCommand) != 1 || alice.count(control.LogCommand) != 1 {
		t.Fatal("both services should be asked once")
	}
	// --engine asks only the named one.
	if code, _, _ = run(t, "stack", "log", testRunID, "--engine", "rootless:alice"); code != 0 ||
		alice.count(control.LogCommand) != 2 || system.count(control.LogCommand) != 1 {
		t.Fatalf("--engine: code %d", code)
	}
}

func TestStackLogWithTwoKnownRunsNamesBothEngines(t *testing.T) {
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice")
	startFakeService(t, logHandlers(true))
	serveFake(t, eps["alice"], logHandlers(true))

	code, stdout, stderr := run(t, "stack", "log", testRunID)
	if code != 2 || stdout != "" || !strings.Contains(stderr, "engines system, rootless:alice") || !strings.Contains(stderr, "--engine") {
		t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestStackLogOfAnUnknownRunNamesTheEngineThatCouldNotBeAsked(t *testing.T) {
	d := useFolderHost(t)
	engineHost(t, d, "alice")
	startFakeService(t, logHandlers(false))

	code, _, stderr := run(t, "stack", "log", testRunID)
	if code == 0 || !strings.Contains(stderr, "rootless:alice (not running)") || !strings.Contains(stderr, "--engine rootless:alice") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
}

func TestStackLogOfAnUnknownRunWithAllServicesAnsweringIsNotFound(t *testing.T) {
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice")
	startFakeService(t, logHandlers(false))
	serveFake(t, eps["alice"], logHandlers(false))

	code, _, stderr := run(t, "stack", "log", testRunID)
	if code == 0 || !strings.Contains(stderr, "no such run") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
}

func TestStackLogWarnsAboutAServiceThatCouldNotBeAskedWhenAnotherKnowsTheRun(t *testing.T) {
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice", "bob")
	startFakeService(t, logHandlers(false))
	serveFake(t, eps["alice"], logHandlers(true))

	code, stdout, stderr := run(t, "stack", "log", testRunID)
	if code != 0 || stdout != "log text\n" || !strings.Contains(stderr, "warning: the deploy service of the rootless:bob engine could not be asked") {
		t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestStackLogOnAHostWithOneServiceAsksOnce(t *testing.T) {
	useFolderHost(t)
	system := startFakeService(t, logHandlers(true))
	if code, stdout, _ := run(t, "stack", "log", testRunID); code != 0 || stdout != "log text\n" || system.count(control.LogCommand) != 1 {
		t.Fatalf("code %d, stdout %q", code, stdout)
	}
}
