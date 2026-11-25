package deploysetup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/hookservice"
	"github.com/castrowithcee/qatlas-host/packaging"
)

// hookHost is a host with a deploy service set up, a rootless user and a runner that keeps the user database,
// the marker folders and the unit state like the real programs would.
type hookHost struct {
	*fakeHost
	running map[string]bool
	chowns  []string
}

func newHookHost(t *testing.T) *hookHost {
	t.Helper()
	h := &hookHost{fakeHost: newFakeHost(t, instanceGroups+"qhost-deploy:x:996:\nalice:x:1001:\n", instancePasswd+"qhost-deploy:x:996:996::/var/lib/qhost-deploy:/usr/sbin/nologin\n"),
		running: map[string]bool{}}
	h.put("/etc/subuid", instanceSubIDs)
	h.put("/etc/subgid", instanceSubIDs)
	h.put("/home/alice/"+userUnit, "[Service]\n")
	h.host.Run = h.runHook
	h.host.Chown = func(p string, uid, gid int) error {
		rel, _ := filepath.Rel(h.root, p)
		h.chowns = append(h.chowns, "/"+filepath.ToSlash(rel)+" "+itoa(uid)+":"+itoa(gid))
		return nil
	}
	return h
}

func itoa(n int) string { return strconv.Itoa(n) }

func (h *hookHost) runHook(_ context.Context, name string, args ...string) error {
	line := name + " " + strings.Join(args, " ")
	if h.failOn != "" && strings.HasPrefix(line, h.failOn) {
		return errors.New("failed")
	}
	if name == "systemctl" && len(args) == 3 && (args[0] == "is-enabled" || args[0] == "is-active") {
		if !h.running[args[2]] {
			return errors.New("no")
		}
		return nil
	}
	h.calls = append(h.calls, line)
	switch {
	case name == "groupadd":
		h.appendTo("/etc/group", "qhost-hook:x:995:\n")
	case name == "useradd":
		h.appendTo("/etc/passwd", "qhost-hook:x:995:995::/nonexistent:/usr/sbin/nologin\n")
	case name == "loginctl":
		h.put(filepath.Join(lingerDir, args[1]), "")
	case name == "systemd-tmpfiles":
		for _, l := range strings.Split(h.read(args[1]), "\n") {
			if f := strings.Fields(l); len(f) > 1 && f[0] == "d" {
				if err := os.MkdirAll(h.host.path(f[1]), 0o755); err != nil {
					h.t.Fatal(err)
				}
			}
		}
	case name == "systemctl" && args[0] == "enable":
		h.running[args[2]] = true
	}
	return nil
}

func (h *hookHost) setUpDeploy() {
	h.t.Helper()
	if _, err := h.fakeHost.setUp(); err != nil {
		h.t.Fatal(err)
	}
	h.running["qhost-deploy"] = true
	h.calls, h.chowns = nil, nil
}

func (h *hookHost) setUpHook(listen string) (Plan, error) {
	h.t.Helper()
	p, err := h.host.PlanHook(context.Background(), listen)
	if err != nil {
		return p, err
	}
	return p, p.Apply(context.Background())
}

func TestHookSetupCreatesEverythingAndRepeatsWithoutChange(t *testing.T) {
	h := newHookHost(t)
	h.setUpDeploy()
	plan, err := h.setUpHook("")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"groupadd --system qhost-hook",
		"useradd --system --gid qhost-hook --no-create-home --home-dir /nonexistent --shell /usr/sbin/nologin qhost-hook",
		"systemd-tmpfiles --create /etc/tmpfiles.d/qhost-hook.conf", "systemctl try-restart qhost-deploy",
		"systemctl daemon-reload",
		"systemctl enable --now qhost-hook",
	}
	if !slices.Equal(h.calls, want) {
		t.Fatalf("commands run:\n%v\nwant\n%v", h.calls, want)
	}
	if got := len(plan.Commands()); got != len(want)+4 {
		t.Fatalf("the plan lists %d steps, want %d: %v", got, len(want)+4, plan.Commands())
	}
	if plan.Listen != DefaultHookListen {
		t.Fatalf("listen = %q", plan.Listen)
	}
	cfg, err := HookConfig(h.root)
	if err != nil || cfg.Listen != "127.0.0.1:8480" || len(cfg.Hooks) != 0 || cfg.Version != hookservice.ConfigVersion {
		t.Fatalf("config = %+v, %v", cfg, err)
	}
	if info, err := os.Stat(h.host.path(HookConfigPath)); err != nil || info.Mode().Perm() != 0o644 {
		t.Fatalf("config mode: %v %v", info, err)
	}
	if info, err := os.Stat(h.host.path(HookConfigDir)); err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("config dir mode: %v %v", info, err)
	}
	if len(h.chowns) != 2 || h.chowns[0] != "/etc/qhost-hook 0:995" ||
		!strings.HasPrefix(h.chowns[1], "/etc/qhost-hook/.qhost-setup-") || !strings.HasSuffix(h.chowns[1], " 0:995") {
		t.Fatalf("owners = %v, want root and the group qhost-hook for the directory and the file", h.chowns)
	}
	if got := h.read("/etc/tmpfiles.d/qhost-hook.conf"); got !=
		"d /run/qhost-hook 0755 qhost-hook qhost-hook -\nd /run/qhost-hook/system 0770 qhost-hook qhost-deploy -\n" {
		t.Fatalf("tmpfiles rule: %q", got)
	}
	if got := h.read(HookUnitPath()); got != string(packaging.HookUnit) {
		t.Fatal("the installed unit differs from the embedded one")
	}
	if !HookInstalled(h.root) {
		t.Fatal("the unit is not reported as installed")
	}
	for _, c := range h.calls {
		if strings.HasPrefix(c, "usermod") || strings.Contains(c, "docker") {
			t.Errorf("the user was put into a group: %q", c)
		}
	}

	h.calls, h.chowns = nil, nil
	again, err := h.setUpHook("")
	if err != nil {
		t.Fatal(err)
	}
	if !again.Empty() || len(h.calls) != 0 || len(h.chowns) != 0 {
		t.Fatalf("a repeated setup changes something: %v %v", again.Commands(), h.calls)
	}
}

func TestHookSetupContinuesAfterAnAbort(t *testing.T) {
	h := newHookHost(t)
	h.setUpDeploy()
	h.failOn = "systemd-tmpfiles"
	if _, err := h.setUpHook(""); err == nil || !strings.Contains(err.Error(), "systemd-tmpfiles") {
		t.Fatalf("err = %v", err)
	}
	if HookInstalled(h.root) {
		t.Fatal("the unit was installed after a failed step")
	}
	h.failOn, h.calls = "", nil
	plan, err := h.setUpHook("")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"systemd-tmpfiles --create /etc/tmpfiles.d/qhost-hook.conf", "systemctl try-restart qhost-deploy",
		"systemctl daemon-reload", "systemctl enable --now qhost-hook"}
	if !slices.Equal(h.calls, want) {
		t.Fatalf("the continued setup ran %v, want %v", h.calls, want)
	}
	for _, c := range plan.Commands() {
		if strings.Contains(c, "groupadd") || strings.Contains(c, "useradd") || strings.Contains(c, "config.json") {
			t.Errorf("repeated %q", c)
		}
	}
}

func TestHookSetupWithANewAddressRewritesAndRestarts(t *testing.T) {
	h := newHookHost(t)
	h.setUpDeploy()
	if _, err := h.setUpHook(""); err != nil {
		t.Fatal(err)
	}
	// An existing hook must survive the rewrite.
	h.put(HookConfigPath, `{"version":1,"listen":"127.0.0.1:8480","hooks":[{"id":"h-0123abcd","stack":"web","instance":"system","hash":"sha256:`+strings.Repeat("a", 64)+`"}]}`)
	h.calls = nil

	plan, err := h.setUpHook("127.0.0.1:9090")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"systemctl restart qhost-hook"}; !slices.Equal(h.calls, want) {
		t.Fatalf("commands run: %v, want %v", h.calls, want)
	}
	if len(plan.Steps) != 2 || !strings.Contains(plan.Steps[0].Command, "config.json") {
		t.Fatalf("steps: %v", plan.Commands())
	}
	got, err := HookConfig(h.root)
	if err != nil || got.Listen != "127.0.0.1:9090" || len(got.Hooks) != 1 || got.Hooks[0].ID != "h-0123abcd" {
		t.Fatalf("config = %+v, %v", got, err)
	}
	// Without --listen the stored address stays.
	h.calls = nil
	again, err := h.setUpHook("")
	if err != nil || !again.Empty() || again.Listen != "127.0.0.1:9090" {
		t.Fatalf("plan = %v, %v", again.Commands(), err)
	}
}

func TestHookListenIsValidatedAndANonLocalOneNamesHTTPS(t *testing.T) {
	for addr, local := range map[string]bool{"127.0.0.1:8480": true, "[::1]:8480": true, "localhost:2000": true,
		"0.0.0.0:8480": false, ":8480": false, "192.0.2.10:8480": false, "hooks.example.test:8480": false} {
		got, err := ValidateHookListen(addr)
		if err != nil || got != local {
			t.Errorf("%q: local = %v, err = %v", addr, got, err)
		}
	}
	for _, addr := range []string{"", "8480", "127.0.0.1", "127.0.0.1:0", "127.0.0.1:80", "127.0.0.1:1024", "127.0.0.1:65536",
		"127.0.0.1:x", "bad host:8480", "a;b:8480"} {
		if _, err := ValidateHookListen(addr); err == nil {
			t.Errorf("accepted %q", addr)
		}
	}
	h := newHookHost(t)
	h.setUpDeploy()
	if _, err := h.host.PlanHook(context.Background(), "127.0.0.1:80"); err == nil {
		t.Fatal("planned an invalid address")
	}
	if HookInstalled(h.root) || h.host.exists(HookConfigPath) || len(h.calls) != 0 {
		t.Fatal("a refused address changed something")
	}
	p, err := h.host.PlanHook(context.Background(), "0.0.0.0:8480")
	if err != nil || !strings.Contains(p.Sentence(), "reverse proxy should provide HTTPS") {
		t.Fatalf("sentence = %q, err = %v", p.Sentence(), err)
	}
	local, _ := h.host.PlanHook(context.Background(), "")
	if strings.Contains(local.Sentence(), "reverse proxy") {
		t.Fatalf("a local address names a proxy: %q", local.Sentence())
	}
}

func TestHookSetupMakesAFolderPerInstance(t *testing.T) {
	h := newHookHost(t)
	h.setUpDeploy()
	if _, err := h.setUpInstanceOf("alice"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.setUpHook(""); err != nil {
		t.Fatal(err)
	}
	if got := h.read("/etc/tmpfiles.d/qhost-hook.conf"); got != "d /run/qhost-hook 0755 qhost-hook qhost-hook -\n"+
		"d /run/qhost-hook/system 0770 qhost-hook qhost-deploy -\nd /run/qhost-hook/alice 0770 qhost-hook alice -\n" {
		t.Fatalf("tmpfiles rule: %q", got)
	}
	for _, d := range []string{"/run/qhost-hook/system", "/run/qhost-hook/alice"} {
		if !h.host.exists(d) {
			t.Errorf("no folder %s", d)
		}
	}
}

// setUpInstanceOf sets up a rootless instance with the runner of the hook host.
func (h *hookHost) setUpInstanceOf(user string) (Plan, error) {
	p, err := h.host.PlanInstance(context.Background(), user)
	if err != nil {
		return p, err
	}
	return p, p.Apply(context.Background())
}

func TestInstanceSetupAddsItsFolderOnlyWhereTheReceiverIsSetUp(t *testing.T) {
	h := newHookHost(t)
	h.setUpDeploy()
	plan, err := h.setUpInstanceOf("alice")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range plan.Commands() {
		if strings.Contains(c, "qhost-hook") {
			t.Errorf("the plan without a receiver names it: %q", c)
		}
	}
	if h.host.exists(HookTmpfilesConf) || h.host.exists(HookRunDir) {
		t.Fatal("the instance setup wrote the receiver's files without a receiver")
	}

	// With the receiver set up, a new instance adds its line and folder and nothing else of the receiver.
	h = newHookHost(t)
	h.setUpDeploy()
	if _, err := h.setUpHook(""); err != nil {
		t.Fatal(err)
	}
	h.calls = nil
	plan, err = h.setUpInstanceOf("alice")
	if err != nil {
		t.Fatal(err)
	}
	var hookSteps []string
	for _, c := range plan.Commands() {
		if strings.Contains(c, "qhost-hook") {
			hookSteps = append(hookSteps, c)
		}
	}
	if len(hookSteps) != 2 || !strings.HasPrefix(hookSteps[0], "write /etc/tmpfiles.d/qhost-hook.conf") ||
		!strings.Contains(hookSteps[0], "/run/qhost-hook/alice 0770 qhost-hook alice -") ||
		hookSteps[1] != "systemd-tmpfiles --create /etc/tmpfiles.d/qhost-hook.conf" {
		t.Fatalf("receiver steps of the instance plan: %v", hookSteps)
	}
	if !h.host.exists("/run/qhost-hook/alice") {
		t.Fatal("no folder for the new instance")
	}
	// A repeated instance setup changes nothing, also for the receiver.
	h.calls = nil
	again, err := h.setUpInstanceOf("alice")
	if err != nil || !again.Empty() {
		t.Fatalf("a repeated instance setup is not empty: %v %v", again.Commands(), err)
	}
}

func TestSystemDeploySetupAddsItsFolderWhereTheReceiverIsSetUp(t *testing.T) {
	h := newHookHost(t)
	// The receiver was set up before the system deploy service exists.
	if _, err := h.setUpHook(""); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(h.read("/etc/tmpfiles.d/qhost-hook.conf"), "system") {
		t.Fatal("a folder for a deploy service that does not exist")
	}
	h.calls = nil
	plan, err := h.fakeHost.setUp()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(h.read("/etc/tmpfiles.d/qhost-hook.conf"), "d /run/qhost-hook/system 0770 qhost-hook qhost-deploy -\n") ||
		!h.host.exists("/run/qhost-hook/system") {
		t.Fatalf("no system folder after the deploy setup: %v", plan.Commands())
	}
}

func TestHookSetupRefusesAUserInOtherGroups(t *testing.T) {
	h := newHookHost(t)
	h.put("/etc/group", h.read("/etc/group")+"qhost-hook:x:995:\n")
	h.put("/etc/group", strings.Replace(h.read("/etc/group"), "docker:x:998:dave", "docker:x:998:dave,qhost-hook", 1))
	h.appendTo("/etc/passwd", "qhost-hook:x:995:995::/nonexistent:/usr/sbin/nologin\n")
	_, err := h.host.PlanHook(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), "docker") {
		t.Fatalf("err = %v", err)
	}
}

func TestHookSetupKeepsAnInvalidConfiguration(t *testing.T) {
	h := newHookHost(t)
	h.put(HookConfigPath, "not json")
	if _, err := h.host.PlanHook(context.Background(), ""); err == nil || !strings.Contains(err.Error(), "stays as it is") {
		t.Fatalf("err = %v", err)
	}
	if h.read(HookConfigPath) != "not json" {
		t.Fatal("the configuration was changed")
	}
}

func TestHookUnitPassesItsOwnValidationAndRefusesWeakening(t *testing.T) {
	if err := validateHookUnit(packaging.HookUnit); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"SupplementaryGroups=docker", "User=root", "AmbientCapabilities=CAP_NET_BIND_SERVICE"} {
		if err := validateHookUnit(append(append([]byte{}, packaging.HookUnit...), []byte("\n"+bad+"\n")...)); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	if err := validateHookUnit([]byte("[Service]\nUser=qhost-hook\n")); err == nil {
		t.Error("accepted a unit without its sandbox")
	}
}

// after returns the commands of plan that follow the step first; a missing step fails the test.
func after(t *testing.T, plan Plan, first string) []string {
	t.Helper()
	cmds := plan.Commands()
	i := slices.Index(cmds, first)
	if i < 0 {
		t.Fatalf("no step %q in %v", first, cmds)
	}
	return cmds[i+1:]
}

const hookTmpfilesCreate = "systemd-tmpfiles --create /etc/tmpfiles.d/qhost-hook.conf"

func TestHookSetupRestartsTheRunningDeployServicesWhoseFolderItCreates(t *testing.T) {
	h := newHookHost(t)
	h.setUpDeploy()
	if _, err := h.setUpInstanceOf("alice"); err != nil {
		t.Fatal(err)
	}
	h.calls = nil
	plan, err := h.setUpHook("")
	if err != nil {
		t.Fatal(err)
	}
	rest := after(t, plan, hookTmpfilesCreate)
	want := []string{"systemctl try-restart qhost-deploy", "systemctl try-restart " + instanceUnit("alice")}
	if len(rest) < 2 || !slices.Equal(rest[:2], want) {
		t.Fatalf("steps after the folders: %v, want %v first", rest, want)
	}
	for _, w := range want {
		if !slices.Contains(h.calls, w) {
			t.Errorf("%q did not run: %v", w, h.calls)
		}
	}
	h.calls = nil
	again, err := h.setUpHook("")
	if err != nil || !again.Empty() || len(h.calls) != 0 {
		t.Fatalf("a repeated setup restarts something: %v %v %v", again.Commands(), h.calls, err)
	}
}

func TestHookSetupLeavesAStoppedDeployServiceStopped(t *testing.T) {
	h := newHookHost(t)
	h.setUpDeploy()
	h.running["qhost-deploy"] = false
	plan, err := h.setUpHook("")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range plan.Commands() {
		if strings.HasPrefix(c, "systemctl") && strings.Contains(c, "qhost-deploy") {
			t.Errorf("the plan touches the stopped deploy service: %q", c)
		}
	}
	if h.running["qhost-deploy"] {
		t.Fatal("the stopped deploy service was started")
	}
}

func TestDeploySetupMakesItsFolderBeforeItStarts(t *testing.T) {
	for _, tc := range []struct {
		name, unit, folder string
		setUp              func(h *hookHost) (Plan, error)
	}{
		{"system", "qhost-deploy", "/run/qhost-hook/system", func(h *hookHost) (Plan, error) { return h.fakeHost.setUp() }},
		{"rootless", instanceUnit("alice"), "/run/qhost-hook/alice",
			func(h *hookHost) (Plan, error) { return h.setUpInstanceOf("alice") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHookHost(t)
			if _, err := h.setUpHook(""); err != nil {
				t.Fatal(err)
			}
			plan, err := tc.setUp(h)
			if err != nil {
				t.Fatal(err)
			}
			if rest := after(t, plan, hookTmpfilesCreate); !slices.Contains(rest, "systemctl enable --now "+tc.unit) {
				t.Fatalf("the service starts before its folder exists: %v", plan.Commands())
			}
			if slices.Contains(plan.Commands(), "systemctl try-restart "+tc.unit) {
				t.Fatalf("a service that did not run is restarted: %v", plan.Commands())
			}

			// A running service whose folder the setup makes again restarts after it.
			if err := os.Remove(h.host.path(tc.folder)); err != nil {
				t.Fatal(err)
			}
			plan, err = tc.setUp(h)
			if err != nil {
				t.Fatal(err)
			}
			if got := after(t, plan, hookTmpfilesCreate); !slices.Equal(got, []string{"systemctl try-restart " + tc.unit}) {
				t.Fatalf("steps after the folder: %v", got)
			}
		})
	}
}
