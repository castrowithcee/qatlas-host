package cli

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/castrowithcee/qatlas-host/internal/client"
	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/rootless"
	"github.com/castrowithcee/qatlas-host/internal/tui"
)

func stacksAnswer(source string, names ...string) func(json.RawMessage) any {
	return func(json.RawMessage) any {
		res := control.StacksResult{Dir: "/srv/" + source}
		for _, n := range names {
			res.Stacks = append(res.Stacks, control.StackInfo{Name: n, Source: source, SourceDir: "/srv/" + source})
		}
		return res
	}
}

func runsAnswer(runs ...control.RunInfo) func(json.RawMessage) any {
	return func(json.RawMessage) any { return control.RunsResult{Runs: runs} }
}

func tuiHandlers(source string, stacks ...string) fakeHandlers {
	return withStatus(fakeHandlers{
		control.StacksListCommand: stacksAnswer(source, stacks...),
		control.DeployCommand: func(raw json.RawMessage) any {
			var a control.DeployArgs
			_ = json.Unmarshal(raw, &a)
			return control.DeployResult{ID: "20260102T030405Z-0123abcd", Stack: a.Stack}
		},
		control.CredentialListCommand: func(json.RawMessage) any {
			return control.CredentialListResult{Credentials: []control.CredentialInfo{{Name: "from-" + source}}}
		},
		control.CredentialSetCommand: func(json.RawMessage) any { return struct{}{} },
		control.NotifyListCommand: func(json.RawMessage) any {
			return control.NotifyListResult{Channels: []control.NotifyChannelInfo{{Name: "chan-" + source}}}
		},
	}, []string{"/srv/" + source}, stacks...)
}

func TestEngineBackendMergesTheListsAndMarksTheEngine(t *testing.T) {
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice")
	startFakeService(t, tuiHandlers("system", "web"))
	serveFake(t, eps["alice"], tuiHandlers("alice", "demo"))
	b := newEngineBackend()

	st, err := b.Status(context.Background())
	if err != nil || len(st.NotChecked) != 0 || len(st.Engine.Stacks) != 2 || len(st.Engine.Sources) != 2 {
		t.Fatalf("status %+v, err %v", st, err)
	}
	if st.Engine.Stacks[0].Engine != control.EngineSystem || st.Engine.Stacks[1].Engine != "rootless:alice" ||
		st.Engine.Sources[1].Engine != "rootless:alice" || st.Engine.Sources[1].Path != "/srv/alice" {
		t.Fatalf("engines not marked: %+v", st.Engine)
	}
	stacks, err := b.Stacks(context.Background())
	if err != nil || len(stacks.Stacks) != 2 || stacks.Stacks[0].Name != "web" || stacks.Stacks[0].Engine != control.EngineSystem ||
		stacks.Stacks[1].Name != "demo" || stacks.Stacks[1].Engine != "rootless:alice" {
		t.Fatalf("stacks %+v, err %v", stacks, err)
	}
	ids, err := b.Engines(context.Background())
	if err != nil || strings.Join(ids, ",") != "system,rootless:alice" {
		t.Fatalf("engines %v, err %v", ids, err)
	}
}

func TestEngineBackendSingleEngineIsMarkedSystem(t *testing.T) {
	useFolderHost(t)
	startFakeService(t, tuiHandlers("system", "web"))
	st, err := newEngineBackend().Stacks(context.Background())
	if err != nil || len(st.Stacks) != 1 || st.Stacks[0].Engine != control.EngineSystem || len(st.NotChecked) != 0 {
		t.Fatalf("stacks %+v, err %v", st, err)
	}
}

func TestEngineBackendRunsMergeNewestFirst(t *testing.T) {
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice")
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	startFakeService(t, fakeHandlers{control.RunsCommand: runsAnswer(
		control.RunInfo{ID: "s1", Stack: "web", Started: now.Add(-2 * time.Hour)},
		control.RunInfo{ID: "s2", Stack: "web", Started: now.Add(-4 * time.Hour)})})
	serveFake(t, eps["alice"], fakeHandlers{control.RunsCommand: runsAnswer(control.RunInfo{ID: "a1", Stack: "demo", Started: now.Add(-3 * time.Hour)})})

	res, err := newEngineBackend().Runs(context.Background(), "")
	if err != nil || len(res.Runs) != 3 {
		t.Fatalf("runs %+v, err %v", res, err)
	}
	var got []string
	for _, r := range res.Runs {
		got = append(got, r.ID+"@"+r.Engine)
	}
	if strings.Join(got, " ") != "s1@system a1@rootless:alice s2@system" {
		t.Fatalf("order and engines: %v", got)
	}
}

func TestEngineBackendUnreachableInstanceIsANoteNotAnError(t *testing.T) {
	d := useFolderHost(t)
	engineHost(t, d, "alice") // alice is set up but does not run
	startFakeService(t, tuiHandlers("system", "web"))
	b := newEngineBackend()

	st, err := b.Status(context.Background())
	if err != nil || len(st.Engine.Stacks) != 1 || len(st.NotChecked) != 1 || st.NotChecked[0].Engine != "rootless:alice" ||
		st.NotChecked[0].Reason != "not running" {
		t.Fatalf("status %+v, err %v", st, err)
	}
	stacks, err := b.Stacks(context.Background())
	if err != nil || len(stacks.Stacks) != 1 || len(stacks.NotChecked) != 1 || stacks.NotChecked[0].Engine != "rootless:alice" {
		t.Fatalf("stacks %+v, err %v", stacks, err)
	}
	runs, err := b.Runs(context.Background(), "")
	if err == nil && len(runs.NotChecked) != 1 {
		t.Fatalf("runs %+v", runs)
	}
}

func TestEngineBackendFailsOnlyWhenNoServiceAnswers(t *testing.T) {
	d := useFolderHost(t)
	engineHost(t, d, "alice")
	noSystemService(t) // the system service is not set up, and alice does not run
	_, err := newEngineBackend().Status(context.Background())
	if !errors.Is(err, client.ErrNotRunning) {
		t.Fatalf("err %v", err)
	}
}

func TestEngineBackendActionGoesToTheInstanceThatHasTheStack(t *testing.T) {
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice", "bob")
	system := startFakeService(t, tuiHandlers("system", "web"))
	alice := serveFake(t, eps["alice"], tuiHandlers("alice", "demo"))
	bob := serveFake(t, eps["bob"], tuiHandlers("bob", "other"))

	res, err := newEngineBackend().Deploy(context.Background(), "demo")
	if err != nil || res.Stack != "demo" {
		t.Fatalf("res %+v, err %v", res, err)
	}
	if alice.count(control.DeployCommand) != 1 || system.count(control.DeployCommand) != 0 || bob.count(control.DeployCommand) != 0 {
		t.Fatal("the deploy did not reach only the instance that has the stack")
	}
	if _, err = newEngineBackend().Deploy(context.Background(), "nothing"); err == nil || !strings.Contains(err.Error(), "none of the engines") {
		t.Fatalf("an unknown stack: %v", err)
	}
	if alice.count(control.DeployCommand)+system.count(control.DeployCommand)+bob.count(control.DeployCommand) != 1 {
		t.Fatal("an unknown stack was deployed somewhere")
	}
}

func TestEngineBackendAmbiguousStackNamesBothEnginesAndWithEngineChooses(t *testing.T) {
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice")
	system := startFakeService(t, tuiHandlers("system", "demo"))
	alice := serveFake(t, eps["alice"], tuiHandlers("alice", "demo"))
	b := newEngineBackend()

	_, err := b.Deploy(context.Background(), "demo")
	if err == nil || !strings.Contains(err.Error(), "exists in the engines system, rootless:alice") || !strings.Contains(err.Error(), "--engine") {
		t.Fatalf("err %v", err)
	}
	if system.count(control.DeployCommand)+alice.count(control.DeployCommand) != 0 {
		t.Fatal("an ambiguous stack was deployed")
	}
	if _, err = b.Deploy(tui.WithEngine(context.Background(), "rootless:alice"), "demo"); err != nil {
		t.Fatal(err)
	}
	if alice.count(control.DeployCommand) != 1 || system.count(control.DeployCommand) != 0 {
		t.Fatal("the named engine was not the one asked")
	}
	if _, err = b.Deploy(tui.WithEngine(context.Background(), control.EngineSystem), "demo"); err != nil || system.count(control.DeployCommand) != 1 {
		t.Fatalf("system: %v", err)
	}
}

func TestEngineBackendNamedEngineThatIsDownIsNeverReplaced(t *testing.T) {
	d := useFolderHost(t)
	engineHost(t, d, "alice") // does not run
	system := startFakeService(t, tuiHandlers("system", "demo"))
	b := newEngineBackend()
	ctx := tui.WithEngine(context.Background(), "rootless:alice")

	if _, err := b.Deploy(ctx, "demo"); !errors.Is(err, client.ErrNotRunning) {
		t.Fatalf("deploy: %v", err)
	}
	if err := b.CredentialSet(ctx, "registry", "value"); !errors.Is(err, client.ErrNotRunning) {
		t.Fatalf("credential: %v", err)
	}
	if _, err := b.Credentials(ctx); !errors.Is(err, client.ErrNotRunning) {
		t.Fatalf("credentials: %v", err)
	}
	if _, err := b.Stacks(ctx); !errors.Is(err, client.ErrNotRunning) {
		t.Fatalf("stacks: %v", err)
	}
	for _, c := range []string{control.DeployCommand, control.CredentialSetCommand, control.CredentialListCommand, control.StacksListCommand, control.StatusCommand} {
		if system.count(c) != 0 {
			t.Fatalf("the system engine got %s although another engine was named", c)
		}
	}
	// An engine that is not set up is refused as well.
	if err := b.CredentialSet(tui.WithEngine(context.Background(), "rootless:carol"), "registry", "value"); err == nil {
		t.Fatal("an unknown engine was accepted")
	}
	if system.count(control.CredentialSetCommand) != 0 {
		t.Fatal("an unknown engine fell back to the system engine")
	}
}

func TestEngineBackendCredentialsAndNotificationsStayPerInstance(t *testing.T) {
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice")
	system := startFakeService(t, tuiHandlers("system", "web"))
	alice := serveFake(t, eps["alice"], tuiHandlers("alice", "demo"))
	b := newEngineBackend()

	// Without a choice the whole-engine settings are those of the system engine.
	creds, err := b.Credentials(context.Background())
	if err != nil || len(creds.Credentials) != 1 || creds.Credentials[0].Name != "from-system" {
		t.Fatalf("default %+v, err %v", creds, err)
	}
	if err := b.CredentialSet(context.Background(), "registry", "secret-value"); err != nil {
		t.Fatal(err)
	}
	if system.count(control.CredentialSetCommand) != 1 || alice.count(control.CredentialSetCommand) != 0 {
		t.Fatal("the default credential did not stay in the system engine")
	}
	actx := tui.WithEngine(context.Background(), "rootless:alice")
	creds, err = b.Credentials(actx)
	if err != nil || creds.Credentials[0].Name != "from-alice" {
		t.Fatalf("alice %+v, err %v", creds, err)
	}
	if err := b.CredentialSet(actx, "registry", "secret-value"); err != nil {
		t.Fatal(err)
	}
	notes, err := b.Notifications(actx)
	if err != nil || notes.Channels[0].Name != "chan-alice" {
		t.Fatalf("notifications %+v, err %v", notes, err)
	}
	if system.count(control.CredentialSetCommand) != 1 || alice.count(control.CredentialSetCommand) != 1 ||
		system.count(control.NotifyListCommand) != 0 {
		t.Fatal("a setting of alice reached the system engine")
	}
}

func TestEngineBackendSeveralInstancesWithoutSystemNeedAChoice(t *testing.T) {
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice", "bob")
	noSystemService(t)
	alice := serveFake(t, eps["alice"], tuiHandlers("alice", "demo"))
	bob := serveFake(t, eps["bob"], tuiHandlers("bob", "other"))
	b := newEngineBackend()

	err := b.CredentialSet(context.Background(), "registry", "secret-value")
	if err == nil || !strings.Contains(err.Error(), "name the engine") || !strings.Contains(err.Error(), "rootless:alice, rootless:bob") {
		t.Fatalf("err %v", err)
	}
	if alice.count(control.CredentialSetCommand)+bob.count(control.CredentialSetCommand) != 0 {
		t.Fatal("a credential went to an instance that nobody chose")
	}
	if err := b.CredentialSet(tui.WithEngine(context.Background(), "rootless:bob"), "registry", "secret-value"); err != nil ||
		bob.count(control.CredentialSetCommand) != 1 || alice.count(control.CredentialSetCommand) != 0 {
		t.Fatalf("named bob: %v", err)
	}
	// The lists still merge both.
	st, err := b.Stacks(context.Background())
	if err != nil || len(st.Stacks) != 2 {
		t.Fatalf("stacks %+v, err %v", st, err)
	}
}

func TestEngineBackendOnlyInstanceIsTheDefaultAndNeedsNoSystemService(t *testing.T) {
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice")
	noSystemService(t)
	alice := serveFake(t, eps["alice"], tuiHandlers("alice", "demo"))
	b := newEngineBackend()

	if !deployPresent() {
		t.Fatal("a host with a running instance counts as not set up")
	}
	if !(tuiDeploySetup{}).Installed() {
		t.Fatal("the interface offers the setup instead of the stacks")
	}
	if _, err := b.Status(context.Background()); err != nil {
		t.Fatalf("status: %v", err)
	}
	ids, _ := b.Engines(context.Background())
	if strings.Join(ids, ",") != "rootless:alice" {
		t.Fatalf("engines %v", ids)
	}
	if err := b.CredentialSet(context.Background(), "registry", "secret-value"); err != nil || alice.count(control.CredentialSetCommand) != 1 {
		t.Fatalf("credential: %v", err)
	}
}

func TestDeployPresentWithoutAnyService(t *testing.T) {
	useFolderHost(t)
	noSystemService(t)
	if deployPresent() || (tuiDeploySetup{}).Installed() {
		t.Fatal("a host without a deploy service counts as set up")
	}
	if _, err := newEngineBackend().Status(context.Background()); !errors.Is(err, client.ErrNotRunning) {
		t.Fatalf("err %v", err)
	}
}

func TestEngineBackendRunLogComesFromTheServiceThatKnowsIt(t *testing.T) {
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice")
	logOf := func(text string, known ...string) fakeHandlers {
		return fakeHandlers{control.LogCommand: func(raw json.RawMessage) any {
			var a control.LogArgs
			_ = json.Unmarshal(raw, &a)
			for _, k := range known {
				if a.ID == k {
					return control.LogResult{Text: text}
				}
			}
			return &control.Error{Code: control.CodeNoSuchRun, Message: "no such run"}
		}}
	}
	startFakeService(t, logOf("system log", "sys-run", "both"))
	serveFake(t, eps["alice"], logOf("alice log", "alice-run", "both"))
	b := newEngineBackend()

	if res, err := b.Log(context.Background(), "alice-run"); err != nil || res.Text != "alice log" {
		t.Fatalf("alice-run: %+v %v", res, err)
	}
	if res, err := b.Log(context.Background(), "sys-run"); err != nil || res.Text != "system log" {
		t.Fatalf("sys-run: %+v %v", res, err)
	}
	if _, err := b.Log(context.Background(), "both"); err == nil || !strings.Contains(err.Error(), "exists in the engines system, rootless:alice") {
		t.Fatalf("both: %v", err)
	}
	if res, err := b.Log(tui.WithEngine(context.Background(), "rootless:alice"), "both"); err != nil || res.Text != "alice log" {
		t.Fatalf("named: %+v %v", res, err)
	}
	var remote *control.Error
	if _, err := b.Log(context.Background(), "none"); !errors.As(err, &remote) || remote.Code != control.CodeNoSuchRun {
		t.Fatalf("none: %v", err)
	}
}

func TestEngineBackendContainersMergeInstancesAndReadEnginesWithoutOneAsRoot(t *testing.T) {
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice")
	startFakeService(t, engineContainers{engine: "system", groups: []control.ContainerGroup{single("web"), single("shared")}}.handlers())
	serveFake(t, eps["alice"], engineContainers{engine: "rootless:alice", groups: []control.ContainerGroup{single("shared")}}.handlers())
	useEngine(t, "bob", func(int) []rootless.Container { return []rootless.Container{{Name: "ghost", State: "running"}} })

	res, err := newEngineBackend().Containers(context.Background())
	if err != nil || res.EnginesUnread || len(res.NotChecked) != 0 {
		t.Fatalf("result %+v, err %v", res, err)
	}
	var got []string
	for _, g := range res.Groups {
		got = append(got, g.Name+"@"+g.Engine)
		if g.ReadOnly != (g.Engine == "rootless:bob") {
			t.Errorf("group %+v: read-only wrong", g)
		}
	}
	if strings.Join(got, " ") != "shared@system shared@rootless:alice web@system ghost@rootless:bob" &&
		strings.Join(got, " ") != "ghost@rootless:bob shared@system shared@rootless:alice web@system" {
		t.Fatalf("groups %v", got)
	}
	// A named engine asks only that engine's service: nothing is read behind it.
	res, err = newEngineBackend().Containers(tui.WithEngine(context.Background(), "rootless:alice"))
	if err != nil || len(res.Groups) != 1 || res.Groups[0].Engine != "rootless:alice" {
		t.Fatalf("named engine: %+v, err %v", res, err)
	}
}

func TestEngineBackendContainersWithoutRootSayEnginesAreUnread(t *testing.T) {
	d := useFolderHost(t)
	asNonRoot(d)
	engineHost(t, d, "alice") // set up, not running
	startFakeService(t, engineContainers{engine: "system", groups: []control.ContainerGroup{single("web")}}.handlers())

	res, err := newEngineBackend().Containers(context.Background())
	if err != nil || !res.EnginesUnread || len(res.Groups) != 1 || len(res.NotChecked) != 1 || res.NotChecked[0].Engine != "rootless:alice" {
		t.Fatalf("result %+v, err %v", res, err)
	}
}

func TestEngineBackendContainerCallsGoToTheNamedEngineOnly(t *testing.T) {
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice")
	restarted := map[string]int{}
	handlers := func(engine string) fakeHandlers {
		h := engineContainers{engine: engine, groups: []control.ContainerGroup{single("shared")}, names: []string{"shared"}}.handlers()
		h[control.ContainerRestartCommand] = func(json.RawMessage) any {
			restarted[engine]++
			return control.ContainerActionResult{ContainerSummary: control.ContainerSummary{Name: "shared", State: control.ContainerOK, EngineState: "running"},
				Kind: control.ContainerKindSingle}
		}
		return h
	}
	startFakeService(t, handlers("system"))
	serveFake(t, eps["alice"], handlers("rootless:alice"))
	b := newEngineBackend()
	alice := tui.WithEngine(context.Background(), "rootless:alice")

	got, err := b.Container(alice, "shared")
	if err != nil || got.Engine != "rootless:alice" {
		t.Fatalf("detail %+v, err %v", got, err)
	}
	logs, err := b.ContainerLogs(alice, control.ContainerLogsArgs{Name: "shared"})
	if err != nil || len(logs.Lines) != 1 || logs.Lines[0].Text != "from rootless:alice" {
		t.Fatalf("logs %+v, err %v", logs, err)
	}
	if _, err := b.ContainerRestart(alice, "shared"); err != nil || restarted["rootless:alice"] != 1 || restarted["system"] != 0 {
		t.Fatalf("restarted %v, err %v", restarted, err)
	}
	if _, err := b.ContainerRestart(tui.WithEngine(context.Background(), "system"), "shared"); err != nil || restarted["system"] != 1 {
		t.Fatalf("restarted %v, err %v", restarted, err)
	}
}

func TestEngineBackendContainerActionOnAnEngineThatIsDownFailsWithoutFallback(t *testing.T) {
	d := useFolderHost(t)
	engineHost(t, d, "alice") // set up, not running
	restarted := 0
	h := engineContainers{engine: "system", groups: []control.ContainerGroup{single("shared")}, names: []string{"shared"}}.handlers()
	h[control.ContainerRestartCommand] = func(json.RawMessage) any { restarted++; return control.ContainerActionResult{} }
	startFakeService(t, h)

	if _, err := newEngineBackend().ContainerRestart(tui.WithEngine(context.Background(), "rootless:alice"), "shared"); err == nil || restarted != 0 {
		t.Fatalf("err %v, system restarted %d times", err, restarted)
	}
}
