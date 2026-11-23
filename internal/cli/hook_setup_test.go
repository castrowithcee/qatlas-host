package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/deploysetup"
)

// useHookHostRoot extends the host of useHostRoot with a runner that keeps the group database and the unit state,
// so that the receiver setup runs through. The caller of the admin check is euid too, never an administrator
// unless root, so no test depends on the groups of the real user. It returns the root and the commands that ran.
func useHookHostRoot(t *testing.T, euid int) (string, *[]string) {
	t.Helper()
	root, ran := useHostRoot(t, euid)
	old := hostEnv
	t.Cleanup(func() { hostEnv = old })
	hostEnv.euid = func() int { return euid }
	hostEnv.admin = func(int) (bool, error) { return false, nil }
	enabled := false
	deploySetup.host.Chown = func(string, int, int) error { return nil }
	deploySetup.host.Run = func(_ context.Context, name string, args ...string) error {
		line := name + " " + strings.Join(args, " ")
		if name == "systemctl" && len(args) > 0 && strings.HasPrefix(args[0], "is-") {
			if enabled {
				return nil
			}
			return os.ErrNotExist
		}
		*ran = append(*ran, line)
		switch name {
		case "groupadd":
			return appendLine(filepath.Join(root, "etc/group"), "qhost-hook:x:995:\n")
		case "useradd":
			return appendLine(filepath.Join(root, "etc/passwd"), "qhost-hook:x:995:995::/nonexistent:/usr/sbin/nologin\n")
		case "systemd-tmpfiles":
			return os.MkdirAll(filepath.Join(root, "run/qhost-hook/system"), 0o755)
		case "systemctl":
			enabled = enabled || args[0] == "enable"
		}
		return nil
	}
	return root, ran
}

func appendLine(path, line string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(line)
	return err
}

func TestHookSetupNeedsRoot(t *testing.T) {
	root, ran := useHookHostRoot(t, 1000)
	code, _, stderr := run(t, "hook-service", "setup", "--confirm", "--yes")
	if code == 0 || !strings.Contains(stderr, "needs root") || len(*ran) != 0 || deploysetup.HookInstalled(root) {
		t.Fatalf("code %d, stderr %q, ran %v", code, stderr, *ran)
	}
}

func TestHookSetupWithoutConfirmShowsTheCommandsAndChangesNothing(t *testing.T) {
	root, ran := useHookHostRoot(t, 0)
	for _, args := range [][]string{{"hook-service", "setup"}, {"hook-service", "setup", "--yes"}} {
		code, stdout, stderr := run(t, args...)
		if code != 2 || stdout != "" || !strings.Contains(stderr, "--confirm") || len(*ran) != 0 ||
			deploysetup.HookInstalled(root) {
			t.Fatalf("%v: code %d, stdout %q, stderr %q, ran %v", args, code, stdout, stderr, *ran)
		}
		for _, want := range []string{"Set up the webhook receiver", "groupadd --system qhost-hook",
			"write /etc/qhost-hook/config.json", "systemctl enable --now qhost-hook"} {
			if strings.Count(stderr, want) != 1 {
				t.Errorf("%v: stderr should show %q once: %q", args, want, stderr)
			}
		}
	}
	if _, err := os.Stat(filepath.Join(root, "etc/qhost-hook")); err == nil {
		t.Fatal("the plan wrote the configuration")
	}
}

func TestHookSetupRunsTheStepsAndRepeatsWithoutChange(t *testing.T) {
	root, ran := useHookHostRoot(t, 0)
	code, stdout, stderr := run(t, "hook-service", "setup", "--confirm", "--yes", "--output", "json")
	if code != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil || got["changed"] != true || got["listen"] != "127.0.0.1:8480" {
		t.Fatalf("stdout %q: %v", stdout, err)
	}
	if !deploysetup.HookInstalled(root) || !strings.Contains(strings.Join(*ran, "\n"), "systemctl enable --now qhost-hook") {
		t.Fatalf("not set up: %v", *ran)
	}
	*ran = nil
	code, stdout, _ = run(t, "hook-service", "setup", "--confirm", "--yes", "--output", "json")
	if err := json.Unmarshal([]byte(stdout), &got); code != 0 || err != nil || got["changed"] != false || len(*ran) != 0 {
		t.Fatalf("repeat: code %d, stdout %q, ran %v", code, stdout, *ran)
	}
}

func TestHookSetupRefusesABadAddressAndNamesHTTPSForANonLocalOne(t *testing.T) {
	root, ran := useHookHostRoot(t, 0)
	code, _, stderr := run(t, "hook-service", "setup", "--listen", "127.0.0.1:80", "--confirm", "--yes")
	if code != 2 || !strings.Contains(stderr, "port") || len(*ran) != 0 || deploysetup.HookInstalled(root) {
		t.Fatalf("code %d, stderr %q, ran %v", code, stderr, *ran)
	}
	if q := hookQuestion("0.0.0.0:8480"); !strings.Contains(q, "reverse proxy should provide HTTPS") {
		t.Fatalf("question = %q", q)
	}
	if q := hookQuestion("127.0.0.1:8480"); strings.Contains(q, "reverse proxy") {
		t.Fatalf("question = %q", q)
	}
	code, _, stderr = run(t, "hook-service", "setup", "--listen", "0.0.0.0:9000")
	if code != 2 || !strings.Contains(stderr, "reverse proxy should provide HTTPS") || !strings.Contains(stderr, "listen 0.0.0.0:9000") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
}
