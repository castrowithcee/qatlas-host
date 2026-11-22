package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/deploysetup"
	"github.com/castrowithcee/qatlas-host/internal/hookservice"
)

func stacksWithTrigger(triggers map[string]string) fakeHandlers {
	return fakeHandlers{control.StacksListCommand: func(json.RawMessage) any {
		res := control.StacksResult{Dir: "/srv/x", Stacks: []control.StackInfo{}}
		for name, trigger := range triggers {
			res.Stacks = append(res.Stacks, control.StackInfo{Name: name, Folder: name, Source: "x", Trigger: trigger})
		}
		return res
	}}
}

// hookFixture sets up the receiver below a temporary root and a deploy service with the given stacks.
func hookFixture(t *testing.T, triggers map[string]string) (root string) {
	t.Helper()
	startFakeService(t, stacksWithTrigger(triggers))
	root, _ = useHookHostRoot(t, 0)
	if code, _, stderr := run(t, "hook-service", "setup", "--confirm", "--yes"); code != 0 {
		t.Fatalf("setup: code %d, stderr %q", code, stderr)
	}
	return root
}

func hookConfigPath(root string) string { return filepath.Join(root, "etc/qhost-hook/config.json") }

func addHook(t *testing.T, args ...string) (id, path, stderr string) {
	t.Helper()
	code, stdout, stderr := run(t, append([]string{"hook", "add"}, append(args, "--output", "json")...)...)
	if code != 0 {
		t.Fatalf("hook add %v: code %d, stderr %q", args, code, stderr)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout %q: %v", stdout, err)
	}
	return got["id"].(string), got["path"].(string), stderr
}

func TestHookAddStoresTheHashAndShowsTheSecretOnce(t *testing.T) {
	root := hookFixture(t, map[string]string{"web": "webhook"})
	id, path, stderr := addHook(t, "web")
	secret, ok := strings.CutPrefix(path, "/hook/")
	if !ok || secret == "" {
		t.Fatalf("path %q", path)
	}
	if strings.Contains(stderr, secret) {
		t.Fatalf("stderr shows the secret: %q", stderr)
	}
	if !strings.Contains(stderr, "shown only once") || strings.Contains(stderr, "warning") {
		t.Fatalf("stderr %q", stderr)
	}
	raw, err := os.ReadFile(hookConfigPath(root))
	if err != nil || strings.Contains(string(raw), secret) {
		t.Fatalf("configuration holds the secret or cannot be read: %v", err)
	}
	cfg, err := deploysetup.HookConfig(root)
	if err != nil || len(cfg.Hooks) != 1 {
		t.Fatalf("config %+v: %v", cfg, err)
	}
	h := cfg.Hooks[0]
	if h.ID != id || h.Stack != "web" || h.Instance != "system" || h.Hash != hookservice.HashSecret(secret) {
		t.Fatalf("stored %+v for id %s", h, id)
	}
	if info, err := os.Stat(hookConfigPath(root)); err != nil || info.Mode().Perm() != 0o644 {
		t.Fatalf("mode: %v %v", info, err)
	}
	entries, _ := os.ReadDir(filepath.Dir(hookConfigPath(root)))
	if len(entries) != 1 {
		t.Fatalf("temporary files left behind: %v", entries)
	}
}

func TestHookAddTwiceGivesTwoIDsAndListShowsNoHashOrSecret(t *testing.T) {
	root := hookFixture(t, map[string]string{"web": "webhook"})
	id1, path1, _ := addHook(t, "web")
	id2, path2, _ := addHook(t, "web")
	if id1 == id2 || path1 == path2 {
		t.Fatalf("ids %s %s, paths equal %v", id1, id2, path1 == path2)
	}
	cfg, err := deploysetup.HookConfig(root)
	if err != nil || len(cfg.Hooks) != 2 {
		t.Fatalf("config %+v: %v", cfg, err)
	}
	for _, args := range [][]string{{"hook", "list", "--output", "json"}, {"hook", "list"}} {
		code, stdout, stderr := run(t, args...)
		if code != 0 || !strings.Contains(stdout, id1) || !strings.Contains(stdout, id2) || !strings.Contains(stdout, "web") {
			t.Fatalf("%v: code %d, stdout %q, stderr %q", args, code, stdout, stderr)
		}
		for _, h := range cfg.Hooks {
			if strings.Contains(stdout, strings.TrimPrefix(h.Hash, "sha256:")) || strings.Contains(stdout, "sha256") {
				t.Fatalf("list shows a hash: %q", stdout)
			}
		}
		for _, p := range []string{path1, path2} {
			if strings.Contains(stdout, strings.TrimPrefix(p, "/hook/")) {
				t.Fatalf("list shows a secret: %q", stdout)
			}
		}
	}
	_, stdout, _ := run(t, "hook", "list", "--output", "json")
	if !strings.Contains(stdout, `"modified"`) {
		t.Fatalf("list lacks the file time: %q", stdout)
	}
}

func TestHookRemoveDeletesOneAndRefusesUnknown(t *testing.T) {
	root := hookFixture(t, map[string]string{"web": "webhook"})
	id1, _, _ := addHook(t, "web")
	id2, _, _ := addHook(t, "web")
	code, _, stderr := run(t, "hook", "remove", "h-00000000")
	if code == 0 || !strings.Contains(stderr, "no such webhook") || !strings.Contains(stderr, "not-found") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if cfg, _ := deploysetup.HookConfig(root); len(cfg.Hooks) != 2 {
		t.Fatalf("unknown id changed the hooks: %+v", cfg)
	}
	if code, _, stderr := run(t, "hook", "remove", id1); code != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	cfg, err := deploysetup.HookConfig(root)
	if err != nil || len(cfg.Hooks) != 1 || cfg.Hooks[0].ID != id2 || cfg.Listen == "" {
		t.Fatalf("config %+v: %v", cfg, err)
	}
}

func TestHookCommandsKeepADamagedFile(t *testing.T) {
	root := hookFixture(t, map[string]string{"web": "webhook"})
	addHook(t, "web")
	damaged := `{"version":1,"listen":"127.0.0.1:8480","hooks":[{"id":"nope"}]}`
	if err := os.WriteFile(hookConfigPath(root), []byte(damaged), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"hook", "add", "web"}, {"hook", "remove", "h-0a1b2c3d"}} {
		code, stdout, stderr := run(t, args...)
		if code == 0 || stdout != "" || !strings.Contains(stderr, "hook configuration") {
			t.Fatalf("%v: code %d, stdout %q, stderr %q", args, code, stdout, stderr)
		}
		raw, err := os.ReadFile(hookConfigPath(root))
		if err != nil || string(raw) != damaged {
			t.Fatalf("%v changed the file: %q %v", args, raw, err)
		}
	}
}

func TestHookAddAndRemoveNeedTheReceiver(t *testing.T) {
	startFakeService(t, stacksWithTrigger(map[string]string{"web": "webhook"}))
	useHookHostRoot(t, 0)
	for _, args := range [][]string{{"hook", "add", "web"}, {"hook", "remove", "h-0a1b2c3d"}} {
		code, stdout, stderr := run(t, args...)
		if code == 0 || stdout != "" || !strings.Contains(stderr, "qhost hook-service setup") {
			t.Fatalf("%v: code %d, stdout %q, stderr %q", args, code, stdout, stderr)
		}
	}
}

func TestHookAddAndRemoveNeedRoot(t *testing.T) {
	root := hookFixture(t, map[string]string{"web": "webhook"})
	deploySetup.euid = func() int { return 1000 }
	for _, args := range [][]string{{"hook", "add", "web"}, {"hook", "remove", "h-0a1b2c3d"}} {
		code, _, stderr := run(t, args...)
		if code == 0 || !strings.Contains(stderr, "needs root") {
			t.Fatalf("%v: code %d, stderr %q", args, code, stderr)
		}
	}
	if cfg, _ := deploysetup.HookConfig(root); len(cfg.Hooks) != 0 {
		t.Fatalf("hooks %+v", cfg)
	}
}

func TestHookAddWarnsForTriggerWindowAndRefusesAnUnknownStack(t *testing.T) {
	root := hookFixture(t, map[string]string{"web": "window"})
	_, _, stderr := addHook(t, "web")
	if !strings.Contains(stderr, "warning") || !strings.Contains(stderr, "window") || !strings.Contains(stderr, "interval or webhook") {
		t.Fatalf("stderr %q", stderr)
	}
	if cfg, _ := deploysetup.HookConfig(root); len(cfg.Hooks) != 1 {
		t.Fatalf("the hook was not stored: %+v", cfg)
	}
	code, _, stderr := run(t, "hook", "add", "other")
	if code == 0 || !strings.Contains(stderr, "no such stack") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if cfg, _ := deploysetup.HookConfig(root); len(cfg.Hooks) != 1 {
		t.Fatalf("an unknown stack got a hook: %+v", cfg)
	}
}

func TestHookAddNamesTheInstanceOfARootlessEngine(t *testing.T) {
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice")
	startFakeService(t, withStatus(nil, []string{"/srv/system"}, "web"))
	serveFake(t, eps["alice"], withStatus(stacksWithTrigger(map[string]string{"demo": "webhook"}), []string{"/srv/alice"}, "demo"))
	root, _ := useHookHostRoot(t, 0)
	if code, _, stderr := run(t, "hook", "add", "demo"); code == 0 || !strings.Contains(stderr, "qhost hook-service setup") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if code, _, stderr := run(t, "hook-service", "setup", "--confirm", "--yes"); code != 0 {
		t.Fatalf("setup: code %d, stderr %q", code, stderr)
	}
	addHook(t, "demo")
	cfg, err := deploysetup.HookConfig(root)
	if err != nil || len(cfg.Hooks) != 1 || cfg.Hooks[0].Instance != "alice" || cfg.Hooks[0].Stack != "demo" {
		t.Fatalf("config %+v: %v", cfg, err)
	}
}

func TestHookListNeedsAnAdministrator(t *testing.T) {
	hookFixture(t, map[string]string{"web": "webhook"})
	old := hostEnv
	t.Cleanup(func() { hostEnv = old })
	hostEnv = hostEnvironment{euid: func() int { return 1001 }, admin: func(int) (bool, error) { return false, nil }}
	code, stdout, stderr := run(t, "hook", "list")
	if code == 0 || stdout != "" || !strings.Contains(stderr, "reserved for administrators") {
		t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	hostEnv = hostEnvironment{euid: func() int { return 1000 }, admin: func(int) (bool, error) { return true, nil }}
	if code, _, stderr := run(t, "hook", "list"); code != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
}
