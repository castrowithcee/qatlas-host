package cli

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/castrowithcee/qatlas-kit/localipc"

	"github.com/castrowithcee/qatlas-host/internal/channeltest"
	"github.com/castrowithcee/qatlas-host/internal/deploysetup"
	"github.com/castrowithcee/qatlas-host/internal/service"
)

// useRootlessHost makes a host below a temporary root whose users alice, carol (an administrator) and dave (in
// docker) each have a rootless engine and subordinate ids, with a runner that records its commands and keeps the
// files the real programs would make. It returns the root and the recorded commands.
func useRootlessHost(t *testing.T, euid int) (string, *[]string) {
	t.Helper()
	root := t.TempDir()
	ids := "alice:100000:65536\ncarol:165536:65536\ndave:231072:65536\n"
	files := map[string]string{
		"etc/group":  "root:x:0:\nsudo:x:27:carol\ndocker:x:998:dave\nqhost:x:997:\n",
		"etc/passwd": instancePasswd,
		"etc/subuid": ids,
		"etc/subgid": ids,
	}
	for _, u := range []string{"alice", "carol", "dave"} {
		files[path.Join("home", u, ".config/systemd/user/docker.service")] = "[Service]\n"
	}
	for p, content := range files {
		writeTestFile(t, filepath.Join(root, p), content)
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
			switch name {
			case "loginctl":
				writeTestFile(t, filepath.Join(root, "var/lib/systemd/linger", args[1]), "")
			case "systemd-tmpfiles":
				user := strings.TrimSuffix(strings.TrimPrefix(path.Base(args[1]), "qhost-deploy-"), ".conf")
				return os.MkdirAll(filepath.Join(root, "run", "qhost-deploy-"+user), 0o755)
			}
			return nil
		}}}
	t.Cleanup(func() { deploySetup = old })
	useHostEndpoint(t, channeltest.Endpoint(t))
	return root, &ran
}

func writeTestFile(t *testing.T, p, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func instanceUsers(t *testing.T, root string) []string {
	t.Helper()
	list, err := deploysetup.Instances(root)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, i := range list {
		out = append(out, i.User())
	}
	return out
}

func TestRootlessSetupShowsStepsRunsThemAndRepeatsWithoutChange(t *testing.T) {
	root, ran := useRootlessHost(t, 0)
	code, stdout, stderr := run(t, "deploy-service", "setup", "--rootless", "alice", "--confirm", "--yes", "--output", "json")
	if code != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	for _, want := range []string{"rootless Docker engine of the user alice", "loginctl enable-linger alice",
		"write /etc/tmpfiles.d/qhost-deploy-alice.conf", "systemctl enable --now " + instanceUnit("alice")} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr lacks %q: %q", want, stderr)
		}
	}
	if strings.Contains(stderr, "warning") {
		t.Errorf("stderr warns: %q", stderr)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil || got["changed"] != true || got["unit"] != instanceUnit("alice") {
		t.Fatalf("stdout %q: %v", stdout, err)
	}
	if users := instanceUsers(t, root); len(users) != 1 || users[0] != "alice" || deploysetup.Installed(root) {
		t.Fatalf("instances %v, system service installed %v", users, deploysetup.Installed(root))
	}
	*ran = nil
	code, stdout, _ = run(t, "deploy-service", "setup", "--rootless", "alice", "--confirm", "--yes", "--output", "json")
	if err := json.Unmarshal([]byte(stdout), &got); code != 0 || err != nil || got["changed"] != false || len(*ran) != 0 {
		t.Fatalf("repeat: code %d, stdout %q, ran %v", code, stdout, *ran)
	}
}

func TestRootlessSetupRefusesAndChangesNothing(t *testing.T) {
	for _, c := range []struct{ user, want string }{
		{"dave", "group docker"},
		{"Alice", "must match"},
		{"ghost", "does not exist"},
	} {
		t.Run(c.user, func(t *testing.T) {
			root, ran := useRootlessHost(t, 0)
			code, _, stderr := run(t, "deploy-service", "setup", "--rootless", c.user, "--confirm", "--yes")
			if code == 0 || !strings.Contains(stderr, c.want) || len(*ran) != 0 || len(instanceUsers(t, root)) != 0 {
				t.Fatalf("code %d, stderr %q, ran %v", code, stderr, *ran)
			}
		})
	}
}

func TestRootlessSetupNeedsRootAndConfirm(t *testing.T) {
	root, ran := useRootlessHost(t, 1000)
	if code, _, stderr := run(t, "deploy-service", "setup", "--rootless", "alice", "--confirm", "--yes"); code == 0 ||
		!strings.Contains(stderr, "needs root") || len(*ran) != 0 {
		t.Fatalf("code %d, stderr %q, ran %v", code, stderr, *ran)
	}
	deploySetup.euid = func() int { return 0 }
	if code, _, stderr := run(t, "deploy-service", "setup", "--rootless", "alice", "--yes"); code == 0 ||
		!strings.Contains(stderr, "--confirm") || len(*ran) != 0 || len(instanceUsers(t, root)) != 0 {
		t.Fatalf("code %d, stderr %q, ran %v", code, stderr, *ran)
	}
}

func TestRootlessSetupWarnsAboutAnAdministrator(t *testing.T) {
	useRootlessHost(t, 0)
	code, _, stderr := run(t, "deploy-service", "setup", "--rootless", "carol", "--confirm", "--yes")
	if code != 0 || !strings.Contains(stderr, "warning: carol is an administrator") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
}

// useInstanceUser makes the user database below the setup root name the user of the test alice, with the
// test's uid, and root.
func useInstanceUser(t *testing.T) uint32 {
	t.Helper()
	me, err := user.Current()
	if err != nil {
		t.Skipf("current user unknown: %v", err)
	}
	uid, err := strconv.ParseUint(me.Uid, 10, 32)
	if err != nil || uid == 0 {
		t.Skip("needs a user other than root")
	}
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "etc/passwd"), "root:x:0:0::/root:/bin/bash\nalice:x:"+me.Uid+":"+me.Gid+"::/home/alice:/bin/bash\n")
	useDeploySetupRoot(t, root)
	return uint32(uid)
}

func TestInstanceRunDerivesEverythingFromTheUser(t *testing.T) {
	uid := useInstanceUser(t)
	target, err := instanceTarget("alice", int(uid))
	if err != nil {
		t.Fatal(err)
	}
	if target.endpoint.Dir != "/run/qhost-deploy-alice" || target.endpoint.ServiceUser != "alice" || target.endpoint.Group != "qhost" ||
		target.stateDir != "/var/lib/qhost-deploy-alice" || target.socket != "/run/user/"+strconv.FormatUint(uint64(uid), 10)+"/docker.sock" {
		t.Fatalf("target = %+v", target)
	}
	for _, c := range []struct {
		user string
		uid  int
		want string
	}{
		{"alice", int(uid) + 1, "never as uid"},
		{"root", 0, "never as uid 0"},
		{"ghost", int(uid), "does not exist"},
		{"Alice", int(uid), "must match"},
	} {
		if _, err := instanceTarget(c.user, c.uid); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s as %d: err = %v, want %q", c.user, c.uid, err, c.want)
		}
	}
}

func TestInstanceRunRefusesAnotherUserAndASecondSocket(t *testing.T) {
	useInstanceUser(t)
	writeTestFile(t, filepath.Join(deploySetup.host.Root, "etc/passwd"), "alice:x:4242:4242::/home/alice:/bin/bash\n")
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"deploy-service", "run", "--instance", "alice"}, "runs as that user (uid 4242)"},
		{[]string{"deploy-service", "run", "--instance", "alice", "--docker-socket", "/run/example.sock"}, "exclude each other"},
	} {
		if code, _, stderr := run(t, c.args...); code == 0 || !strings.Contains(stderr, c.want) {
			t.Errorf("%v: code %d, stderr %q", c.args, code, stderr)
		}
	}
}

// useInstanceEndpoints makes every instance answer on a channel of its own from the map, or on none.
func useInstanceEndpoints(t *testing.T, eps map[string]localipc.ServiceEndpoint) {
	t.Helper()
	old := instanceEndpoint
	instanceEndpoint = func(i deploysetup.Instance) localipc.ServiceEndpoint {
		if ep, ok := eps[i.User()]; ok {
			return ep
		}
		ep := channeltest.Endpoint(t)
		ep.Dir += "/missing"
		return ep
	}
	t.Cleanup(func() { instanceEndpoint = old })
}

func TestStatusShowsEveryInstance(t *testing.T) {
	useEndpoint(t, channeltest.Endpoint(t))
	root := t.TempDir()
	useDeploySetupRoot(t, root)
	for _, u := range []string{"alice", "bob"} {
		writeTestFile(t, filepath.Join(root, "etc/tmpfiles.d/qhost-deploy-"+u+".conf"), "")
	}
	ep := channeltest.Endpoint(t)
	useInstanceEndpoints(t, map[string]localipc.ServiceEndpoint{"alice": ep})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- service.Run(ctx, slog.New(slog.NewTextHandler(io.Discard, nil)), service.Config{Version: "7.7.7", Endpoint: ep,
			StateDir: filepath.Join(t.TempDir(), "state"), PeerCheck: func(net.Conn) (uint32, error) { return 0, nil }})
	}()
	t.Cleanup(func() { cancel(); <-done })
	channeltest.WaitReady(t, ep)

	code, stdout, stderr := run(t, "status", "--output", "json")
	if code != 0 || stderr != "" {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatal(err)
	}
	if got["deploy_service"] != "not set up" || got["instance.alice.unit"] != instanceUnit("alice") ||
		got["instance.alice.deploy_service"] != "running" || got["instance.alice.version"] != "7.7.7" ||
		got["instance.bob.deploy_service"] != "not running" || got["instance.bob.unit"] != instanceUnit("bob") {
		t.Fatalf("status = %v", got)
	}
}

// instancePasswd is the user database of the instance tests.
const instancePasswd = "root:x:0:0::/root:/bin/bash\nalice:x:1001:1001::/home/alice:/bin/bash\ncarol:x:1002:1002::/home/carol:/bin/bash\ndave:x:1003:1003::/home/dave:/bin/bash\n"

// instanceUnit is the unit name of the instance of user.
func instanceUnit(user string) string { return "qhost-deploy@" + user + ".service" }
