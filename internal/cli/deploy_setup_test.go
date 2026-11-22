package cli

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/channeltest"
	"github.com/castrowithcee/qatlas-host/internal/deploysetup"
	"github.com/castrowithcee/qatlas-host/internal/hostservice"
)

// inventedSetupUsers is the user database of the test host.
const inventedSetupUsers = "root:x:0:0::/root:/bin/bash\nqhost-deploy:x:996:996::/var/lib/qhost-deploy:/usr/sbin/nologin\n"

// useHostRoot makes a host below a temporary root that has Docker and qhost but no deploy service, with a
// runner that only records its commands. It returns the root and the recorded commands.
func useHostRoot(t *testing.T, euid int) (string, *[]string) {
	t.Helper()
	root := t.TempDir()
	for p, content := range map[string]string{
		"etc/group":                    "root:x:0:\ndocker:x:998:alice,qhost-deploy\nqhost:x:997:\nqhost-deploy:x:996:\n",
		filepath.Join("etc", "passwd"): inventedSetupUsers,
	} {
		full := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	old := deploySetup
	var mu sync.Mutex
	var ran []string
	deploySetup = deploySetupHost{euid: func() int { return euid }, host: deploysetup.Host{Root: root,
		Run: func(_ context.Context, name string, args ...string) error {
			mu.Lock()
			defer mu.Unlock()
			if name == "systemctl" && len(args) > 0 && strings.HasPrefix(args[0], "is-") {
				return nil
			}
			ran = append(ran, name+" "+strings.Join(args, " "))
			if name == "systemd-tmpfiles" {
				return os.MkdirAll(filepath.Join(root, "run", "qhost-deploy"), 0o755)
			}
			return nil
		}}}
	t.Cleanup(func() { deploySetup = old })
	useWindowPaths(t, root)
	useHostEndpoint(t, channeltest.Endpoint(t))
	return root, &ran
}

// useWindowPaths puts the host window file and the state of the system deploy service under root, so that no
// command of a test touches the real ones.
func useWindowPaths(t *testing.T, root string) (hostFile, state string) {
	t.Helper()
	hostFile, state = filepath.Join(root, "etc", "qhost", "window.json"), filepath.Join(root, "var", "lib", "qhost-deploy")
	oldFile, oldState := windowHostFile, stateDir
	windowHostFile, stateDir = func() string { return hostFile }, func() string { return state }
	t.Cleanup(func() { windowHostFile, stateDir = oldFile, oldState })
	return hostFile, state
}

func TestSetupNeedsRoot(t *testing.T) {
	root, ran := useHostRoot(t, 1000)
	code, _, stderr := run(t, "deploy-service", "setup", "--confirm", "--yes")
	if code == 0 || !strings.Contains(stderr, "needs root") || len(*ran) != 0 || deploysetup.Installed(root) {
		t.Fatalf("code %d, stderr %q, ran %v", code, stderr, *ran)
	}
}

func TestSetupWithoutConfirmChangesNothing(t *testing.T) {
	root, ran := useHostRoot(t, 0)
	for _, args := range [][]string{{"deploy-service", "setup"}, {"deploy-service", "setup", "--yes"}} {
		code, stdout, stderr := run(t, args...)
		if code != 2 || stdout != "" || !strings.Contains(stderr, "--confirm") || len(*ran) != 0 || deploysetup.Installed(root) {
			t.Fatalf("%v: code %d, stdout %q, stderr %q, ran %v", args, code, stdout, stderr, *ran)
		}
		for _, want := range []string{"Set up the deploy service", "systemctl enable --now qhost-deploy"} {
			if strings.Count(stderr, want) != 1 {
				t.Errorf("%v: stderr should show %q once: %q", args, want, stderr)
			}
		}
	}
}

func TestSetupShowsStepsRunsThemAndRepeatsWithoutChange(t *testing.T) {
	root, ran := useHostRoot(t, 0)
	code, stdout, stderr := run(t, "deploy-service", "setup", "--confirm", "--yes", "--output", "json")
	if code != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	for _, want := range []string{"Set up the deploy service", "write /etc/tmpfiles.d/qhost-deploy.conf", "systemctl enable --now qhost-deploy"} {
		if strings.Count(stderr, want) != 1 {
			t.Errorf("stderr should show %q once: %q", want, stderr)
		}
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil || got["changed"] != true {
		t.Fatalf("stdout %q: %v", stdout, err)
	}
	if !deploysetup.Installed(root) || !strings.Contains(strings.Join(*ran, "\n"), "systemctl enable --now qhost-deploy") {
		t.Fatalf("not set up: %v", *ran)
	}
	*ran = nil
	code, stdout, _ = run(t, "deploy-service", "setup", "--confirm", "--yes", "--output", "json")
	if err := json.Unmarshal([]byte(stdout), &got); code != 0 || err != nil || got["changed"] != false || len(*ran) != 0 {
		t.Fatalf("repeat: code %d, stdout %q, ran %v", code, stdout, *ran)
	}
}

func TestSetupRefusesWithoutDockerGroupAndChangesNothing(t *testing.T) {
	root, ran := useHostRoot(t, 0)
	if err := os.WriteFile(filepath.Join(root, "etc/group"), []byte("root:x:0:\nqhost:x:997:\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := run(t, "deploy-service", "setup", "--confirm", "--yes")
	if code == 0 || !strings.Contains(stderr, "group docker") || len(*ran) != 0 || deploysetup.Installed(root) {
		t.Fatalf("code %d, stderr %q, ran %v", code, stderr, *ran)
	}
}

func TestCommandsWithoutSetupSayNotSetUp(t *testing.T) {
	useHostRoot(t, 1000)
	useEndpoint(t, channeltest.Endpoint(t))
	useDeploySetupRoot(t, t.TempDir())
	for _, args := range [][]string{{"stack", "list"}, {"window", "show"}, {"stack", "runs"}} {
		code, _, stderr := run(t, args...)
		if code != ExitServiceNotRunning || !strings.Contains(stderr, "the deploy service is not set up; run sudo qhost deploy-service setup") ||
			strings.Contains(stderr, "cannot talk") {
			t.Errorf("%v: code %d, stderr %q", args, code, stderr)
		}
	}
}

func TestStatusWithoutSetupShowsBothServices(t *testing.T) {
	useEndpoint(t, channeltest.Endpoint(t))
	useDeploySetupRoot(t, t.TempDir())
	code, stdout, stderr := run(t, "status", "--output", "json")
	if code != 0 || stderr != "" {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatal(err)
	}
	if got["running"] != false || got["deploy_service"] != "not set up" || got["host_service"] != "not running" {
		t.Fatalf("status = %v", got)
	}
}

func TestStatusShowsTheHostServiceNextToTheDeployService(t *testing.T) {
	startService(t, func(net.Conn) (uint32, error) { return 0, nil })
	startHost(t, true)
	code, stdout, stderr := run(t, "status", "--output", "json")
	if code != 0 || stderr != "" {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatal(err)
	}
	if got["running"] != true || got["version"] != "9.9.9" || got["deploy_service"] != "running" ||
		got["host_service"] != "running" || got["host_version"] != "8.8.8" {
		t.Fatalf("status = %v", got)
	}
}

func TestStatusShowsAHostServiceReservedForAdministrators(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root is always served")
	}
	startService(t, func(net.Conn) (uint32, error) { return 0, nil })
	startHost(t, false)
	code, stdout, stderr := run(t, "status", "--output", "json")
	if code != 0 || stderr != "" {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatal(err)
	}
	if got["deploy_service"] != "running" || got["host_service"] != "running (reserved for administrators)" || got["host_version"] != nil {
		t.Fatalf("status = %v", got)
	}
}

// startHost runs a host service on a channel of its own and makes the commands talk to it. Its group database
// makes the user of the test an administrator, or a member of the group qhost without sudo. It returns the state
// directory.
func startHost(t *testing.T, admin bool) string {
	t.Helper()
	me, err := user.Current()
	if err != nil {
		t.Skipf("current user unknown: %v", err)
	}
	dir := t.TempDir()
	group, passwd := filepath.Join(dir, "group"), filepath.Join(dir, "passwd")
	groups := "qhost:x:990:" + me.Username + "\nsudo:x:27:example-other\n"
	if admin {
		groups = "qhost:x:990:" + me.Username + "\nsudo:x:27:" + me.Username + "\n"
	}
	if err := os.WriteFile(group, []byte(groups), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(passwd, []byte(me.Username+":x:"+me.Uid+":"+me.Gid+"::/nonexistent:/usr/sbin/nologin\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ep := channeltest.Endpoint(t)
	useHostEndpoint(t, ep)
	state := filepath.Join(dir, "state")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- hostservice.Run(ctx, slog.New(slog.NewTextHandler(io.Discard, nil)),
			hostservice.Config{Version: "8.8.8", Endpoint: ep, StateDir: state, GroupFile: group, PasswdFile: passwd})
	}()
	t.Cleanup(func() { cancel(); <-done })
	channeltest.WaitReady(t, ep)
	return state
}
