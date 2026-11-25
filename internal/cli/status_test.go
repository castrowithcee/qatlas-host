package cli

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/castrowithcee/qatlas-kit/localipc"

	"github.com/castrowithcee/qatlas-host/internal/changelog"
	"github.com/castrowithcee/qatlas-host/internal/channeltest"
	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/deploysetup"
	"github.com/castrowithcee/qatlas-host/internal/engine"
	"github.com/castrowithcee/qatlas-host/internal/registry"
	"github.com/castrowithcee/qatlas-host/internal/service"
)

// testGit is the git layer of the next service a test starts; nil means the system git.
var testGit service.Git

// testRegistry configures the registry clients of the next service a test starts.
var testRegistry registry.Options

// testChangelog configures the GitHub client of the next service a test starts.
var testChangelog changelog.Options

// testDeployer is the deployer of the next service a test starts; nil means none.
var testDeployer engine.Deployer

// testBackups is the backup manager of the next service a test starts; nil means none.
var testBackups service.Backups

// testDocker reads the containers of the next service a test starts; nil means the engine at the default socket.
var testDocker service.Docker

// testGroupFile and testPasswdFile name the user database the service checks; empty means the system files.
var testGroupFile, testPasswdFile string

func useEndpoint(t *testing.T, ep localipc.ServiceEndpoint) {
	t.Helper()
	old := serviceEndpoint
	serviceEndpoint = func() localipc.ServiceEndpoint { return ep }
	t.Cleanup(func() { serviceEndpoint = old })
	useDeploySetupDone(t)
	missing := channeltest.Endpoint(t)
	missing.Dir += "/missing"
	useHostEndpoint(t, missing)
}

// useDeploySetupDone makes the host look as if the setup of the deploy service ran: its unit is installed.
func useDeploySetupDone(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	unit := filepath.Join(root, filepath.FromSlash(deploysetup.UnitPath()))
	if err := os.MkdirAll(filepath.Dir(unit), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unit, []byte("[Service]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	useDeploySetupRoot(t, root)
}

func useDeploySetupRoot(t *testing.T, root string) {
	t.Helper()
	old := deploySetup
	deploySetup.host = deploysetup.Host{Root: root}
	t.Cleanup(func() { deploySetup = old })
}

func useHostEndpoint(t *testing.T, ep localipc.ServiceEndpoint) {
	t.Helper()
	old := hostEndpoint
	hostEndpoint = func() localipc.ServiceEndpoint { return ep }
	t.Cleanup(func() { hostEndpoint = old })
}

func useStateDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "state")
	old := stateDir
	stateDir = func() string { return dir }
	t.Cleanup(func() { stateDir = old })
	return dir
}

func startService(t *testing.T, check service.PeerCheck) {
	t.Helper()
	startServiceLogged(t, check, io.Discard)
}

// startServiceLogged starts the service with its log on logOut and returns once the channel exists.
func startServiceLogged(t *testing.T, check service.PeerCheck, logOut io.Writer) string {
	t.Helper()
	ep := channeltest.Endpoint(t)
	useEndpoint(t, ep)
	dir := useStateDir(t)
	hostWindow := windowHostFile()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- service.Run(ctx, slog.New(slog.NewTextHandler(logOut, &slog.HandlerOptions{Level: slog.LevelDebug})),
			service.Config{Version: "9.9.9", Endpoint: ep, StateDir: dir, WindowHostFile: hostWindow, WindowHostPoll: 20 * time.Millisecond, PeerCheck: check, Git: testGit, Deployer: testDeployer, Backups: testBackups, Registry: testRegistry, Changelog: testChangelog, Docker: testDocker, GroupFile: testGroupFile, PasswdFile: testPasswdFile})
	}()
	t.Cleanup(func() { cancel(); <-done })
	channeltest.WaitReady(t, ep)
	return dir
}

func TestStatusJSON(t *testing.T) {
	startService(t, func(net.Conn) (uint32, error) { return 0, nil })
	code, stdout, stderr := run(t, "status", "--output", "json")
	if code != 0 || stderr != "" {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("invalid JSON %q: %v", stdout, err)
	}
	started, err := time.Parse(time.RFC3339, got["started_at"].(string))
	if got["running"] != true || got["version"] != "9.9.9" || err != nil || time.Since(started) > time.Minute {
		t.Fatalf("status = %v", got)
	}
}

func TestStatusTable(t *testing.T) {
	startService(t, func(net.Conn) (uint32, error) { return 0, nil })
	code, stdout, _ := run(t, "status")
	if code != 0 || !strings.Contains(stdout, "9.9.9") {
		t.Fatalf("code %d, stdout %q", code, stdout)
	}
}

func TestStatusWithoutRunningService(t *testing.T) {
	useEndpoint(t, channeltest.Endpoint(t))
	code, stdout, stderr := run(t, "status")
	if code != ExitServiceNotRunning || stdout != "" ||
		!strings.Contains(stderr, "not running") || !strings.Contains(stderr, "unreachable") {
		t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestStatusWithoutServiceDirectory(t *testing.T) {
	ep := channeltest.Endpoint(t)
	ep.Dir += "/missing"
	useEndpoint(t, ep)
	if code, _, stderr := run(t, "status"); code != ExitServiceNotRunning || !strings.Contains(stderr, "not running") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
}

func TestStatusAsNonRootNamesSudo(t *testing.T) {
	startService(t, func(net.Conn) (uint32, error) { return 0, control.ErrNotRoot() })
	code, _, stderr := run(t, "status")
	if code != 1 || !strings.Contains(stderr, "sudo") || !strings.Contains(stderr, "permission") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
}

func TestStatusWithoutAccessSaysReservedAndShowsNoData(t *testing.T) {
	startService(t, func(net.Conn) (uint32, error) { return 0, control.ErrNoAccess() })
	code, stdout, stderr := run(t, "status")
	if code != 1 || !strings.Contains(stderr, "qhost is reserved for administrators (sudo)") || strings.Contains(stderr, "only root") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Errorf("a refused user got data: %q", stdout)
	}
}

func TestStatusFieldsAreUniqueAcrossSources(t *testing.T) {
	head := strings.Repeat("a", 40)
	polled := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	e := control.EngineStatus{
		Sources: []control.SourceStatus{
			{ID: "srv-first", Path: "/srv/first", Kind: "remote", LastPoll: polled, Head: head, RemoteHead: head},
			{ID: "srv-second", Path: "/srv/second", Kind: "remote", LastPoll: polled, PollError: "fetch: connection timed out"},
		},
		Stacks: []control.StackStatus{
			{Name: "demo-stack", Source: "srv-first", Deployed: head},
			{Name: "demo-stack", Source: "srv-second",
				Findings: []control.FindingInfo{{Kind: control.FindingNameCollision, Detail: "the name is taken"}}},
			{Name: "web-stack", Source: "srv-second"},
		},
	}
	got := map[string]any{}
	for _, f := range engineFields(control.EngineSystem, e) {
		if _, dup := got[f.Name]; dup {
			t.Fatalf("duplicate field %s", f.Name)
		}
		got[f.Name] = f.Value
	}
	for name, want := range map[string]any{
		"source.srv-first.path":                       "/srv/first",
		"source.srv-first.last_poll":                  "2026-03-04T05:06:07Z",
		"source.srv-first.head":                       head,
		"source.srv-second.kind":                      "remote",
		"source.srv-second.poll_error":                "fetch: connection timed out",
		"stack.demo-stack.source":                     "srv-first",
		"stack.demo-stack.deployed":                   head,
		"source.srv-second.stack.demo-stack.source":   "srv-second",
		"source.srv-second.stack.demo-stack.findings": "name-collision: the name is taken",
		"stack.web-stack.source":                      "srv-second",
	} {
		if got[name] != want {
			t.Errorf("%s = %v, want %v", name, got[name], want)
		}
	}
}

func TestRepairText(t *testing.T) {
	at := time.Date(2026, 5, 4, 3, 2, 1, 0, time.UTC)
	for _, tt := range []struct {
		r    control.RepairInfo
		want string
	}{
		{control.RepairInfo{Attempts: 1, Max: 3, Next: at}, "1 of 3 attempts, next 2026-05-04T03:02:01Z"},
		{control.RepairInfo{Attempts: 3, Max: 3}, "3 of 3 attempts"},
		{control.RepairInfo{Attempts: 3, Max: 3, Halted: &control.HoldInfo{Since: at}}, "halted since 2026-05-04T03:02:01Z after 3 of 3 attempts"},
	} {
		if got := repairText(&tt.r); got != tt.want {
			t.Errorf("repairText(%+v) = %q, want %q", tt.r, got, tt.want)
		}
	}
}

func TestObservationInStatusFields(t *testing.T) {
	until := time.Date(2026, 5, 4, 3, 2, 1, 0, time.UTC)
	commit := strings.Repeat("b", 40)
	got := map[string]any{}
	for _, f := range engineFields(control.EngineSystem, control.EngineStatus{Stacks: []control.StackStatus{
		{Name: "web", Source: "srv", Observe: &control.ObserveInfo{Commit: commit, Until: until}},
	}}) {
		got[f.Name] = f.Value
	}
	if want := "until 2026-05-04T03:02:01Z, update " + commit; got["stack.web.observe"] != want {
		t.Errorf("observe = %v, want %q", got["stack.web.observe"], want)
	}
}

func TestStatusShowsTheWebhookReceiverAsNotSetUp(t *testing.T) {
	startService(t, func(net.Conn) (uint32, error) { return 0, nil })
	code, stdout, _ := run(t, "status", "--output", "json")
	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); code != 0 || err != nil || got["hook_service"] != "not set up" {
		t.Fatalf("code %d, stdout %q, err %v", code, stdout, err)
	}
	if _, ok := got["hook_listen"]; ok {
		t.Errorf("an address is shown for a receiver that is not set up: %v", got)
	}
}

func TestStatusShowsTheWebhookReceiverWithStateAddressAndHooks(t *testing.T) {
	startService(t, func(net.Conn) (uint32, error) { return 0, nil })
	root := deploySetup.host.Root
	writeTestFile(t, filepath.Join(root, filepath.FromSlash(deploysetup.HookUnitPath())), "[Service]\n")
	writeTestFile(t, filepath.Join(root, filepath.FromSlash(deploysetup.HookConfigPath)),
		`{"version":1,"listen":"127.0.0.1:8480","hooks":[`+
			`{"id":"h-00000001","stack":"web","instance":"system","hash":"sha256:`+strings.Repeat("a", 64)+`"},`+
			`{"id":"h-00000002","stack":"db","instance":"system","hash":"sha256:`+strings.Repeat("b", 64)+`"}]}`)
	for state, want := range map[bool]string{true: "running", false: "not running"} {
		old := hookActive
		hookActive = func(context.Context) bool { return state }
		code, stdout, stderr := run(t, "status", "--output", "json")
		hookActive = old
		var got map[string]any
		if err := json.Unmarshal([]byte(stdout), &got); code != 0 || err != nil {
			t.Fatalf("code %d, stdout %q, stderr %q, err %v", code, stdout, stderr, err)
		}
		if got["hook_service"] != want || got["hook_listen"] != "127.0.0.1:8480" || got["hook_count"] != float64(2) {
			t.Fatalf("state %v: status = %v", state, got)
		}
		if strings.Contains(stdout, "sha256:") {
			t.Errorf("a hash is in the status: %q", stdout)
		}
	}
}

func TestStatusNamesTheStateFileThatKeepsTheDeployServiceFromStarting(t *testing.T) {
	useEndpoint(t, channeltest.Endpoint(t))
	dir := useStateDir(t)
	path := filepath.Join(dir, "sources.json")
	const doc = `{"version":99,"sources":[]}`
	writeTestFile(t, path, doc)
	code, stdout, stderr := run(t, "status")
	if code != ExitServiceNotRunning || stdout != "" || !strings.Contains(stderr, "not running") ||
		!strings.Contains(stderr, path) || !strings.Contains(stderr, "troubleshooting") {
		t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != doc {
		t.Fatalf("status changed the file: %q, %v", got, err)
	}
}

func TestStatusStaysAsItWasWhenTheStateIsReadable(t *testing.T) {
	useEndpoint(t, channeltest.Endpoint(t))
	useStateDir(t)
	code, _, stderr := run(t, "status")
	if code != ExitServiceNotRunning || strings.Contains(stderr, "state cannot be read") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
}

func TestStatusShowsAnUnreadableHookConfigurationWithItsFile(t *testing.T) {
	startService(t, func(net.Conn) (uint32, error) { return 0, nil })
	root := deploySetup.host.Root
	writeTestFile(t, filepath.Join(root, filepath.FromSlash(deploysetup.HookUnitPath())), "[Service]\n")
	path := filepath.Join(root, filepath.FromSlash(deploysetup.HookConfigPath))
	writeTestFile(t, path, `{"version":99,"listen":"127.0.0.1:8480","hooks":[]}`)
	code, stdout, stderr := run(t, "status", "--output", "json")
	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); code != 0 || err != nil {
		t.Fatalf("code %d, stdout %q, stderr %q, err %v", code, stdout, stderr, err)
	}
	cfg, _ := got["hook_config"].(string)
	if !strings.HasPrefix(cfg, "unreadable: ") || !strings.Contains(cfg, path) {
		t.Fatalf("hook_config = %q", cfg)
	}
}

func TestImageFingerprintsInStatusFieldsAndRunsText(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	images := []control.FingerprintInfo{{Service: "app", Image: "reg.example.test/app:1", RepoDigest: digest, ImageID: "sha256:" + strings.Repeat("b", 64)},
		{Service: "db", Image: "local/db"}}
	got := map[string]any{}
	for _, f := range engineFields(control.EngineSystem, control.EngineStatus{Stacks: []control.StackStatus{{Name: "web", Source: "srv", Images: images}}}) {
		got[f.Name] = f.Value
	}
	for name, want := range map[string]string{"stack.web.image.app.repo_digest": digest, "stack.web.image.db.repo_digest": "missing", "stack.web.image.db.image_id": "missing", "stack.web.image.db.ref": "local/db"} {
		if got[name] != want {
			t.Errorf("%s = %v, want %q", name, got[name], want)
		}
	}
	if want := "app=sha256:aaaaaaaaaaaa, db=missing"; imagesText(images, true) != want {
		t.Errorf("table text = %q, want %q", imagesText(images, true), want)
	}
	if want := "app=" + digest + ", db=missing"; imagesText(images, false) != want {
		t.Errorf("full text = %q, want %q", imagesText(images, false), want)
	}
}
