package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/rootless"
)

// engineContainers is the container side of a fake deploy service: it lists groups and holds containers by name.
type engineContainers struct {
	engine string
	groups []control.ContainerGroup
	names  []string
}

func (e engineContainers) handlers() fakeHandlers {
	has := func(name string) bool {
		for _, n := range e.names {
			if n == name {
				return true
			}
		}
		return false
	}
	act := func(raw json.RawMessage) any {
		var a control.ContainerActionArgs
		_ = json.Unmarshal(raw, &a)
		return control.ContainerActionResult{ContainerSummary: control.ContainerSummary{Name: a.Name, State: control.ContainerOK, EngineState: "running"},
			Kind: control.ContainerKindSingle}
	}
	return fakeHandlers{
		control.ContainerListCommand: func(json.RawMessage) any {
			groups := make([]control.ContainerGroup, len(e.groups))
			for i, g := range e.groups {
				g.Engine = e.engine
				groups[i] = g
			}
			return control.ContainerListResult{Groups: groups}
		},
		control.ContainerGetCommand: func(raw json.RawMessage) any {
			var a control.ContainerGetArgs
			_ = json.Unmarshal(raw, &a)
			if !has(a.Name) {
				return control.NewError(control.CodeNotFound, "no such container")
			}
			return control.ContainerDetail{ContainerSummary: control.ContainerSummary{Name: a.Name, State: control.ContainerOK, EngineState: "running"},
				ID: "id-" + a.Name, Engine: e.engine, Kind: control.ContainerKindSingle}
		},
		control.ContainerRestartCommand: act, control.ContainerStartCommand: act, control.ContainerStopCommand: act,
		control.ContainerLogsCommand: func(json.RawMessage) any {
			return control.ContainerLogsResult{Lines: []control.ContainerLogLine{{Source: "x", Stream: "stdout", Text: "from " + e.engine}}}
		},
	}
}

func single(name string) control.ContainerGroup {
	return control.ContainerGroup{Kind: control.ContainerKindSingle, Name: name, State: control.ContainerOK,
		Containers: []control.ContainerSummary{{Name: name, Image: "registry.example.test/demo/app:1", State: control.ContainerOK}}}
}

func foreign(name string) control.ContainerGroup {
	return control.ContainerGroup{Kind: control.ContainerKindForeign, Name: name, State: control.ContainerOK}
}

type listed struct {
	Groups []struct {
		Kind, Name, Engine string
		ReadOnly           bool `json:"read_only"`
	}
	Notes []string
}

func listJSON(t *testing.T) (listed, string) {
	t.Helper()
	code, stdout, stderr := run(t, "container", "list", "--output", "json")
	if code != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	var got listed
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("output %q: %v", stdout, err)
	}
	return got, stderr
}

func asNonRoot(d *hostDouble) { folders.euid = func() int { return 1000 } }

func TestContainerListMergesEnginesSortedAndSaysSudoIsNeeded(t *testing.T) {
	d := useFolderHost(t)
	asNonRoot(d)
	eps := engineHost(t, d, "alice")
	system := engineContainers{engine: "system", groups: []control.ContainerGroup{single("zeta"), single("lonely")}}
	alice := engineContainers{engine: "rootless:alice", groups: []control.ContainerGroup{foreign("app"), single("lonely")}}
	startFakeService(t, system.handlers())
	serveFake(t, eps["alice"], alice.handlers())

	got, stderr := listJSON(t)
	var order []string
	for _, g := range got.Groups {
		order = append(order, g.Kind+" "+g.Name+" "+g.Engine)
	}
	want := "foreign app rootless:alice|single lonely system|single lonely rootless:alice|single zeta system"
	if strings.Join(order, "|") != want {
		t.Fatalf("order = %v", order)
	}
	if !strings.Contains(stderr, "note: "+noSudoEngineNote) || len(got.Notes) != 1 || got.Notes[0] != noSudoEngineNote {
		t.Fatalf("no sudo note: stderr %q, notes %v", stderr, got.Notes)
	}
	_, table, _ := run(t, "container", "list")
	if !strings.Contains(strings.ToLower(table), "engine") || !strings.Contains(table, "rootless:alice") {
		t.Fatalf("table lacks the engine:\n%s", table)
	}
}

func TestContainerListWarnsAboutAnUnreachableInstance(t *testing.T) {
	d := useFolderHost(t)
	engineHost(t, d, "alice")
	startFakeService(t, engineContainers{engine: "system", groups: []control.ContainerGroup{single("web")}}.handlers())

	got, stderr := listJSON(t)
	if len(got.Groups) != 1 || got.Groups[0].Engine != "system" {
		t.Fatalf("groups = %+v", got.Groups)
	}
	if !strings.Contains(stderr, "warning: the deploy service of the rootless:alice engine could not be asked (not running)") {
		t.Fatalf("no warning: %q", stderr)
	}
}

func TestContainerListShowsEnginesWithoutInstanceOnlyForRootAndReadOnly(t *testing.T) {
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice")
	startFakeService(t, engineContainers{engine: "system", groups: []control.ContainerGroup{single("web")}}.handlers())
	serveFake(t, eps["alice"], engineContainers{engine: "rootless:alice", groups: []control.ContainerGroup{single("mine")}}.handlers())
	// bob has an engine and no deploy service.
	useEngine(t, "bob", func(int) []rootless.Container {
		return []rootless.Container{{Name: "ghost", State: "running", Image: "registry.example.test/demo/ghost:1"},
			{Name: "app-web-1", Project: "app", State: "exited", WorkingDir: "/srv/app"}, {Name: "app-db-1", Project: "app", State: "running"}}
	})

	got, stderr := listJSON(t)
	byName := map[string]string{}
	for _, g := range got.Groups {
		byName[g.Name] = g.Engine
		if g.ReadOnly != (g.Engine == "rootless:bob") {
			t.Errorf("group %+v: read-only wrong", g)
		}
	}
	if len(got.Groups) != 4 || byName["ghost"] != "rootless:bob" || byName["app"] != "rootless:bob" || byName["mine"] != "rootless:alice" {
		t.Fatalf("groups = %+v", got.Groups)
	}
	if !strings.Contains(stderr, "the Docker engine rootless:bob has no deploy service: its containers are shown read-only") ||
		strings.Contains(stderr, "only with sudo") {
		t.Fatalf("notes: %q", stderr)
	}
	_, table, _ := run(t, "container", "list")
	if !strings.Contains(table, "read-only") || !strings.Contains(table, "stopped") {
		t.Fatalf("table:\n%s", table)
	}
}

func TestContainerListNeverReadsAnEngineThatHasADeployService(t *testing.T) {
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice")
	startFakeService(t, engineContainers{engine: "system", groups: []control.ContainerGroup{single("web")}}.handlers())
	serveFake(t, eps["alice"], engineContainers{engine: "rootless:alice", groups: []control.ContainerGroup{single("mine")}}.handlers())
	useEngine(t, "alice", func(int) []rootless.Container { return []rootless.Container{{Name: "mine", State: "running"}} })

	got, stderr := listJSON(t)
	if len(got.Groups) != 2 || stderr != "" || len(got.Notes) != 0 {
		t.Fatalf("groups %+v, stderr %q, notes %v", got.Groups, stderr, got.Notes)
	}
	for _, g := range got.Groups {
		if g.ReadOnly {
			t.Fatalf("an engine with a deploy service is read-only: %+v", g)
		}
	}
}

func TestContainerActionGoesToTheEngineOfTheContainer(t *testing.T) {
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice")
	system := startFakeService(t, engineContainers{engine: "system", names: []string{"web"}}.handlers())
	alice := serveFake(t, eps["alice"], engineContainers{engine: "rootless:alice", names: []string{"mine"}}.handlers())

	code, stdout, stderr := run(t, "container", "restart", "mine", "--yes", "--output", "json")
	if code != 0 || !strings.Contains(stdout, `"name":"id-mine"`) {
		t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if alice.count(control.ContainerRestartCommand) != 1 || system.count(control.ContainerRestartCommand) != 0 {
		t.Fatalf("restart: alice %d, system %d", alice.count(control.ContainerRestartCommand), system.count(control.ContainerRestartCommand))
	}
	if code, _, stderr = run(t, "container", "stop", "nope", "--yes"); code == 0 || !strings.Contains(stderr, "no such container") {
		t.Fatalf("unknown: %d %q", code, stderr)
	}
}

func TestContainerNameInTwoEnginesNeedsTheEngine(t *testing.T) {
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice")
	system := startFakeService(t, engineContainers{engine: "system", names: []string{"web"}}.handlers())
	alice := serveFake(t, eps["alice"], engineContainers{engine: "rootless:alice", names: []string{"web"}}.handlers())

	code, _, stderr := run(t, "container", "restart", "web", "--yes")
	if code == 0 || !strings.Contains(stderr, "the container web exists in the engines system, rootless:alice") || !strings.Contains(stderr, "--engine") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if alice.count(control.ContainerRestartCommand)+system.count(control.ContainerRestartCommand) != 0 {
		t.Fatal("an ambiguous action reached an engine")
	}
	code, _, stderr = run(t, "container", "restart", "web", "--yes", "--engine", "rootless:alice")
	if code != 0 || alice.count(control.ContainerRestartCommand) != 1 || system.count(control.ContainerRestartCommand) != 0 {
		t.Fatalf("code %d, stderr %q, alice %d", code, stderr, alice.count(control.ContainerRestartCommand))
	}
	if code, _, _ = run(t, "container", "show", "web"); code == 0 {
		t.Fatal("show of an ambiguous name succeeded")
	}
}

func TestContainerActionNeverFallsToAnotherEngine(t *testing.T) {
	d := useFolderHost(t)
	engineHost(t, d, "alice")
	system := startFakeService(t, engineContainers{engine: "system", names: []string{"web"}}.handlers())

	// alice does not answer and might have a "web" too: the action stops, show only warns.
	code, _, stderr := run(t, "container", "restart", "web", "--yes")
	if code == 0 || !strings.Contains(stderr, "rootless:alice") || !strings.Contains(stderr, "--engine system") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if system.count(control.ContainerRestartCommand) != 0 {
		t.Fatal("the action fell to another engine")
	}
	code, stdout, stderr := run(t, "container", "show", "web", "--output", "json")
	if code != 0 || !strings.Contains(stdout, `"engine":"system"`) || !strings.Contains(stderr, "warning: the deploy service of the rootless:alice engine could not be asked") {
		t.Fatalf("show: code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	// the named engine is never replaced by another one
	if code, _, _ = run(t, "container", "restart", "web", "--yes", "--engine", "rootless:alice"); code == 0 || system.count(control.ContainerRestartCommand) != 0 {
		t.Fatalf("a named, unreachable engine was replaced: code %d", code)
	}
}

func TestContainerOfAnEngineWithoutInstanceIsRefused(t *testing.T) {
	d := useFolderHost(t)
	system := startFakeService(t, engineContainers{engine: "system", names: []string{"web"}}.handlers())
	useEngine(t, "bob", func(int) []rootless.Container {
		return []rootless.Container{{Name: "ghost", State: "running"}, {Name: "web", State: "running"}}
	})

	code, _, stderr := run(t, "container", "restart", "ghost", "--yes")
	if code == 0 || !strings.Contains(stderr, "rootless:bob") || !strings.Contains(stderr, "no deploy service") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if code, _, stderr = run(t, "container", "logs", "ghost"); code == 0 || !strings.Contains(stderr, "no deploy service") {
		t.Fatalf("logs: code %d, stderr %q", code, stderr)
	}
	// "web" is in the system engine and in the engine without instance: choose, and only the system engine can act
	code, _, stderr = run(t, "container", "restart", "web", "--yes")
	if code == 0 || !strings.Contains(stderr, "the engines system, rootless:bob") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if code, _, _ = run(t, "container", "restart", "ghost", "--yes", "--engine", "rootless:bob"); code == 0 {
		t.Fatal("an action on an engine without deploy service succeeded")
	}
	if code, _, _ = run(t, "container", "restart", "web", "--yes", "--engine", "system"); code != 0 || system.count(control.ContainerRestartCommand) != 1 {
		t.Fatalf("system action: code %d", code)
	}

	// without root nothing of the other engines is read: the container is simply not found
	asNonRoot(d)
	if code, _, stderr = run(t, "container", "restart", "ghost", "--yes"); code == 0 || !strings.Contains(stderr, "no such container") {
		t.Fatalf("non-root: code %d, stderr %q", code, stderr)
	}
}

func TestContainerLogsGoToTheEngineOfTheContainer(t *testing.T) {
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice")
	system := startFakeService(t, engineContainers{engine: "system", names: []string{"web"}}.handlers())
	alice := serveFake(t, eps["alice"], engineContainers{engine: "rootless:alice", names: []string{"mine"}}.handlers())

	code, stdout, stderr := run(t, "container", "logs", "mine")
	if code != 0 || !strings.Contains(stdout, "from rootless:alice") {
		t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if alice.count(control.ContainerLogsCommand) != 1 || system.count(control.ContainerLogsCommand) != 0 {
		t.Fatal("the logs were read from the wrong engine")
	}
}

func TestContainerHelpNamesTheEngineAndTheSudoHint(t *testing.T) {
	for _, args := range [][]string{{"container", "list", "--help"}, {"container", "restart", "--help"}, {"container", "show", "--help"}, {"container", "logs", "--help"}} {
		code, out, _ := run(t, args...)
		if code != 0 || !strings.Contains(out, "engine") {
			t.Fatalf("%v: %d %q", args, code, out)
		}
	}
	_, out, _ := run(t, "container", "list", "--help")
	if !strings.Contains(out, "only with sudo") {
		t.Fatalf("list help lacks the sudo hint:\n%s", out)
	}
	_, out, _ = run(t, "help", "containers")
	if !strings.Contains(out, "--engine") || !strings.Contains(out, "no deploy service") {
		t.Fatalf("topic lacks the engines:\n%s", out)
	}
}

func TestEngineGroupsOfAnEngineWithoutInstance(t *testing.T) {
	groups := engineGroups(rootless.Engine{ID: "rootless:bob", Containers: []rootless.Container{
		{Name: "app-web-1", Project: "app", State: "running", Image: "demo/web:1", WorkingDir: "/srv/app"},
		{Name: "app-db-1", Project: "app", State: "restarting", Image: "demo/db:1"},
		{Name: "lone", State: "exited", Image: "demo/lone:1"},
	}})
	if len(groups) != 2 {
		t.Fatalf("groups = %+v", groups)
	}
	app, lone := groups[0], groups[1]
	if app.Kind != control.ContainerKindForeign || app.State != control.ContainerRestarting || len(app.Containers) != 2 ||
		app.Engine != "rootless:bob" || !app.ReadOnly || app.WorkingDir != "/srv/app" {
		t.Fatalf("app = %+v", app)
	}
	if lone.Kind != control.ContainerKindSingle || lone.State != control.ContainerStopped || lone.Containers[0].EngineState != "exited" {
		t.Fatalf("lone = %+v", lone)
	}
}
