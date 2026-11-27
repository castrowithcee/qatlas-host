package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-kit/localipc"

	"github.com/castrowithcee/qatlas-host/internal/channeltest"
	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/deploysetup"
)

// engineHost has the instances alice and bob set up, as listed by the host double.
func engineHost(t *testing.T, d *hostDouble, users ...string) map[string]localipc.ServiceEndpoint {
	t.Helper()
	eps := map[string]localipc.ServiceEndpoint{}
	for _, u := range users {
		inst, err := deploysetup.NewInstance(u)
		if err != nil {
			t.Fatal(err)
		}
		d.instanceList = append(d.instanceList, inst)
		eps[u] = channeltest.Endpoint(t)
	}
	useInstanceEndpoints(t, eps)
	return eps
}

func statusWith(sources []string, stacks ...string) func(json.RawMessage) any {
	return func(json.RawMessage) any {
		var st control.StatusResult
		for _, p := range sources {
			st.Engine.Sources = append(st.Engine.Sources, control.SourceStatus{ID: "x", Path: p})
		}
		for _, n := range stacks {
			st.Engine.Stacks = append(st.Engine.Stacks, control.StackStatus{Name: n})
		}
		return st
	}
}

func TestSourceAddIntoARootlessEngineUsesItsInstance(t *testing.T) {
	d := useFolderHost(t)
	tree := newPermTree(t)
	eps := engineHost(t, d, "alice", "bob")
	system := startFakeService(t, fakeHandlers{control.StatusCommand: statusWith(nil)})
	alice := serveFake(t, eps["alice"], addHandlers(localInspect(true)))
	bob := serveFake(t, eps["bob"], fakeHandlers{control.StatusCommand: statusWith([]string{"/srv/other"}, "other")})
	useStdin(t, strings.NewReader(""))

	code, _, stderr := run(t, "source", "add", tree.dir, "--engine", "rootless:alice", "--yes")
	if code != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if alice.count(control.SourceInspectCommand) == 0 || alice.count(control.SourceAddCommand) != 1 {
		t.Fatalf("instance calls: %v", alice.calls)
	}
	if system.count(control.SourceInspectCommand) != 0 || system.count(control.SourceAddCommand) != 0 || bob.count(control.SourceAddCommand) != 0 {
		t.Fatal("another engine was asked to take the folder over")
	}
	if system.count(control.StatusCommand) != 1 || bob.count(control.StatusCommand) != 1 {
		t.Fatal("the other engines were not asked for their folders and stacks")
	}
	inst := d.instanceList[0]
	data, err := os.ReadFile(filepath.Join(d.root, inst.DropInDir(), "10-stack-folders.conf"))
	if want := "[Service]\nReadWritePaths=-" + tree.dir + "\n"; err != nil || string(data) != want {
		t.Fatalf("instance drop-in = %q, %v; want %q", data, err, want)
	}
	if d.dropIn(t) != "" {
		t.Fatal("the drop-in of the system service was written")
	}
	if got := strings.Join(d.cmds, ";"); got != "daemon-reload;restart "+instanceUnit("alice") {
		t.Fatalf("commands %q", got)
	}
	for _, o := range d.owners {
		if o != "alice" {
			t.Fatalf("owner asked for %q", o)
		}
	}
	if len(d.owners) == 0 {
		t.Fatal("the owner of the instance was not asked")
	}
}

func TestSourceAddRefusesEnginesWithoutAnInstanceAndUnsafeNames(t *testing.T) {
	d := useFolderHost(t)
	tree := newPermTree(t)
	engineHost(t, d, "alice")
	svc := startFakeService(t, addHandlers(localInspect(true)))
	for _, engine := range []string{"rootless:carol", "rootless:../etc", "rootless:", "podman", "rootless:alice/../bob"} {
		code, _, stderr := run(t, "source", "add", tree.dir, "--engine", engine, "--yes")
		if code != 2 {
			t.Errorf("%s: code %d, stderr %q", engine, code, stderr)
		}
	}
	code, _, stderr := run(t, "source", "add", tree.dir, "--engine", "rootless:carol", "--yes")
	if code != 2 || !strings.Contains(stderr, "no deploy service is set up") || !strings.Contains(stderr, "deploy-service setup") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if svc.count(control.SourceInspectCommand) != 0 || d.dropIn(t) != "" || len(d.cmds) != 0 || len(d.owners) != 0 {
		t.Fatal("a refused engine had an effect")
	}
}

func TestSourceAddRefusesAFolderOverlappingAnotherInstance(t *testing.T) {
	for _, theirs := range []string{"", "/sub"} {
		d := useFolderHost(t)
		tree := newPermTree(t)
		eps := engineHost(t, d, "alice")
		startFakeService(t, fakeHandlers{control.StatusCommand: statusWith([]string{tree.dir + theirs})})
		serveFake(t, eps["alice"], addHandlers(localInspect(true)))
		code, _, stderr := run(t, "source", "add", tree.dir, "--engine", "rootless:alice", "--yes")
		if code != 2 || !strings.Contains(stderr, "overlaps the stack folder") || !strings.Contains(stderr, "system engine") {
			t.Fatalf("%q: code %d, stderr %q", theirs, code, stderr)
		}
		if d.dropIn(t) != "" || len(d.cmds) != 0 || len(d.lchowns) != 0 {
			t.Fatal("a refused folder had an effect")
		}
	}
}

func TestSourceAddRefusesAStackNameAnotherInstanceHas(t *testing.T) {
	d := useFolderHost(t)
	tree := newPermTree(t)
	eps := engineHost(t, d, "alice", "bob")
	startFakeService(t, fakeHandlers{control.StatusCommand: statusWith(nil)})
	alice := serveFake(t, eps["alice"], addHandlers(localInspect(true)))
	serveFake(t, eps["bob"], fakeHandlers{control.StatusCommand: statusWith([]string{"/srv/bob"}, "demo")})
	code, _, stderr := run(t, "source", "add", tree.dir, "--engine", "rootless:alice", "--yes")
	if code != 2 || !strings.Contains(stderr, "rootless:bob engine already has a stack named demo") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if alice.count(control.SourceAddCommand) != 0 || len(d.cmds) != 0 {
		t.Fatal("a refused name had an effect")
	}
}

func TestSourceAddNamesAnInstanceThatCannotBeAskedAndNeedsConfirmation(t *testing.T) {
	d := useFolderHost(t)
	tree := newPermTree(t)
	eps := engineHost(t, d, "alice", "bob")
	startFakeService(t, fakeHandlers{control.StatusCommand: statusWith(nil)})
	svc := serveFake(t, eps["alice"], addHandlers(localInspect(true)))
	useStdin(t, strings.NewReader(""))

	code, _, stderr := run(t, "source", "add", tree.dir, "--engine", "rootless:alice")
	if code != 2 || !strings.Contains(stderr, "rootless:bob engine could not be asked") || !strings.Contains(stderr, "confirmation-required") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if svc.count(control.SourceAddCommand) != 0 || len(d.cmds) != 0 {
		t.Fatal("an unconfirmed folder had an effect")
	}
	code, _, stderr = run(t, "source", "add", tree.dir, "--engine", "rootless:alice", "--yes")
	if code != 0 || !strings.Contains(stderr, "could not be asked") || svc.count(control.SourceAddCommand) != 1 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
}

func TestSourceAddWithoutEngineAsksTheInstancesToo(t *testing.T) {
	d := useFolderHost(t)
	tree := newPermTree(t)
	eps := engineHost(t, d, "alice")
	startFakeService(t, addHandlers(localInspect(true)))
	serveFake(t, eps["alice"], fakeHandlers{control.StatusCommand: statusWith([]string{tree.dir})})
	code, _, stderr := run(t, "source", "add", tree.dir, "--yes")
	if code != 2 || !strings.Contains(stderr, "overlaps the stack folder") || !strings.Contains(stderr, "rootless:alice") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if d.dropIn(t) != "" {
		t.Fatal("the folder was granted")
	}
}

func TestSourceInspectSuggestsTheEngineOfTheContainers(t *testing.T) {
	useFolderHost(t)
	res := localInspect(false)
	res.Stacks[0].Conflicts = []control.SourceConflict{{Container: "demo-1", Project: "demo", Engine: "rootless:alice"}}
	startFakeService(t, addHandlers(res))
	dir := t.TempDir()
	code, stdout, stderr := run(t, "source", "inspect", dir)
	if code != 0 || !strings.Contains(stdout, "--engine rootless:alice") {
		t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	code, stdout, _ = run(t, "source", "inspect", dir, "--output", "json")
	if code != 0 || !strings.Contains(stdout, `"suggested_engines"`) || !strings.Contains(stdout, "rootless:alice") {
		t.Fatalf("code %d, stdout %q", code, stdout)
	}
	// Containers of the system engine suggest nothing: it is the default.
	res.Stacks[0].Conflicts = []control.SourceConflict{{Container: "demo-1", Project: "demo", Engine: control.EngineSystem}}
	startFakeService(t, addHandlers(res))
	if _, stdout, _ = run(t, "source", "inspect", dir); strings.Contains(stdout, "--engine") {
		t.Fatalf("a suggestion for the default engine: %q", stdout)
	}
}

func TestRepoSetIntoARootlessEngineGrantsItsInstance(t *testing.T) {
	e := startRepoService(t)
	d := e.host
	engineHost(t, d, "alice")
	missing := filepath.Join(e.dir, "new")
	code, _, stderr := run(t, "repo", "set", e.srv.URL(), "--dir", missing, "--create", "--engine", "rootless:zed",
		"--fingerprint", e.srv.Key().Fingerprint)
	if code != 2 || !strings.Contains(stderr, "no deploy service is set up") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if _, err := os.Lstat(missing); err == nil || len(d.cmds) != 0 || e.git.Clones() != 0 {
		t.Fatal("a refused engine had an effect")
	}
}

func TestRepoSetNeedsConfirmationWhenAnInstanceCannotBeAsked(t *testing.T) {
	e := startRepoService(t)
	d := e.host
	engineHost(t, d, "bob")
	useStdin(t, strings.NewReader(""))
	code, _, stderr := run(t, "repo", "set", e.srv.URL(), "--dir", e.dir, "--fingerprint", e.srv.Key().Fingerprint)
	if code != 2 || !strings.Contains(stderr, "rootless:bob engine could not be asked") || !strings.Contains(stderr, "--yes") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if e.git.Clones() != 0 || len(d.cmds) != 0 {
		t.Fatal("an unconfirmed repo set had an effect")
	}
	code, _, stderr = run(t, "repo", "set", e.srv.URL(), "--dir", e.dir, "--fingerprint", e.srv.Key().Fingerprint, "--yes")
	if code != 0 || e.git.Clones() != 1 {
		t.Fatalf("code %d, stderr %q, clones %d", code, stderr, e.git.Clones())
	}
}

func TestEngineTargetNamesPathsAndUnitsPerInstance(t *testing.T) {
	inst, err := deploysetup.NewInstance("alice")
	if err != nil {
		t.Fatal(err)
	}
	got := engineTarget{inst: &inst}
	if got.id() != "rootless:alice" || got.unit() != instanceUnit("alice") || got.account() != "alice" ||
		got.dropIn("/r") != "/r/etc/systemd/system/"+instanceUnit("alice")+".d/10-stack-folders.conf" {
		t.Fatalf("target = %s %s %s %s", got.id(), got.unit(), got.account(), got.dropIn("/r"))
	}
	sys := engineTarget{}
	if sys.id() != "system" || sys.unit() != "qhost-deploy" || sys.account() != "root" || sys.dropIn("/r") != "/r"+stackFoldersDropIn {
		t.Fatalf("system target = %s %s %s", sys.id(), sys.unit(), sys.dropIn("/r"))
	}
}

func TestGrantCreatesTheFolderForTheUserOfTheInstance(t *testing.T) {
	d := useFolderHost(t)
	engineHost(t, d, "alice")
	target, err := folders.resolveEngine("rootless:alice")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "stacks")
	var notice strings.Builder
	if err := folders.grant(withEngine(t.Context(), target), dir, true, &notice); err != nil {
		t.Fatal(err)
	}
	if len(d.owners) != 1 || d.owners[0] != "alice" || !strings.Contains(notice.String(), "owner alice:alice") {
		t.Fatalf("owners %v, notice %q", d.owners, notice.String())
	}
	data, err := os.ReadFile(filepath.Join(d.root, d.instanceList[0].StackFoldersDropIn()))
	if err != nil || string(data) != "[Service]\nReadWritePaths=-"+dir+"\n" {
		t.Fatalf("drop-in %q, %v", data, err)
	}
	if got := strings.Join(d.cmds, ";"); got != "daemon-reload;restart "+instanceUnit("alice") {
		t.Fatalf("commands %q", got)
	}
}
