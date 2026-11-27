package deploysetup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/castrowithcee/qatlas-host/packaging"
)

// fakeHost is a root directory with the user database of a host with Docker and qhost installed, and a runner
// that keeps the database and the service state like the real programs would.
type fakeHost struct {
	t       *testing.T
	root    string
	calls   []string
	enabled bool
	active  bool
	failOn  string
	host    Host
	channel channelState
}

// channelState is the owner and mode the fake host reports for /run/qhost-deploy, where the test user owns the
// real folder; the command systemd-tmpfiles sets it right.
type channelState struct {
	uid, gid int
	mode     os.FileMode
}

var rightChannel = channelState{uid: 0, gid: 997, mode: os.ModeDir | os.ModeSetgid | 0o750}

type fakeInfo struct {
	os.FileInfo
	st channelState
}

func (i fakeInfo) Mode() os.FileMode { return i.st.mode }
func (i fakeInfo) Sys() any          { return &syscall.Stat_t{Uid: uint32(i.st.uid), Gid: uint32(i.st.gid)} }

func (f *fakeHost) lstat(p string) (os.FileInfo, error) {
	info, err := os.Lstat(p)
	if err != nil || p != filepath.Join(f.root, "run/qhost-deploy") || info.Mode()&os.ModeSymlink != 0 {
		return info, err
	}
	return fakeInfo{info, f.channel}, nil
}

func newFakeHost(t *testing.T, groups, passwd string) *fakeHost {
	t.Helper()
	f := &fakeHost{t: t, root: t.TempDir(), channel: rightChannel}
	f.put("/etc/group", groups)
	f.put("/etc/passwd", passwd)
	f.host = Host{Root: f.root, Run: f.run, Lstat: f.lstat}
	return f
}

const baseGroups = "root:x:0:\ndocker:x:998:alice\nqhost:x:997:\n"
const basePasswd = "root:x:0:0::/root:/bin/bash\nqhost:x:997:997::/var/lib/qhost:/usr/sbin/nologin\n"

func (f *fakeHost) put(p, content string) {
	f.t.Helper()
	full := filepath.Join(f.root, filepath.FromSlash(p))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fakeHost) read(p string) string {
	f.t.Helper()
	b, err := os.ReadFile(filepath.Join(f.root, filepath.FromSlash(p)))
	if err != nil {
		f.t.Fatal(err)
	}
	return string(b)
}

func (f *fakeHost) appendTo(p, line string) { f.put(p, f.read(p)+line) }

func (f *fakeHost) run(_ context.Context, name string, args ...string) error {
	line := name + " " + strings.Join(args, " ")
	if f.failOn != "" && strings.HasPrefix(line, f.failOn) {
		return errors.New("failed")
	}
	switch {
	case line == "systemctl is-enabled --quiet qhost-deploy":
		if !f.enabled {
			return errors.New("disabled")
		}
		return nil
	case line == "systemctl is-active --quiet qhost-deploy":
		if !f.active {
			return errors.New("inactive")
		}
		return nil
	}
	f.calls = append(f.calls, line)
	switch {
	case strings.HasPrefix(line, "systemd-tmpfiles "):
		f.channel = rightChannel
		if err := os.MkdirAll(filepath.Join(f.root, "run/qhost-deploy"), 0o755); err != nil {
			f.t.Fatal(err)
		}
	case line == "systemctl enable --now qhost-deploy":
		f.enabled, f.active = true, true
	}
	return nil
}

func (f *fakeHost) setUp() (Plan, error) {
	f.t.Helper()
	p, err := f.host.Plan(context.Background())
	if err != nil {
		return p, err
	}
	return p, p.Apply(context.Background())
}

func TestSetupCreatesEverythingAndRepeatsWithoutChange(t *testing.T) {
	f := newFakeHost(t, baseGroups, basePasswd)
	plan, err := f.setUp()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"systemd-tmpfiles --create /etc/tmpfiles.d/qhost-deploy.conf",
		"systemctl daemon-reload",
		"systemctl enable --now qhost-deploy",
	}
	if !slices.Equal(f.calls, want) {
		t.Fatalf("commands run:\n%v\nwant\n%v", f.calls, want)
	}
	if got := len(plan.Commands()); got != len(want)+3 {
		t.Fatalf("the plan lists %d steps, want %d: %v", got, len(want)+3, plan.Commands())
	}
	if i, j := slices.Index(plan.Commands(), "mkdir /etc/qhost-deploy (root, mode 0755)"), slices.Index(plan.Commands(), "systemctl enable --now qhost-deploy"); i < 0 || i > j {
		t.Fatalf("the plan does not create the configuration folder before the start: %v", plan.Commands())
	}
	if info, err := os.Stat(filepath.Join(f.root, "etc/qhost-deploy")); err != nil || !info.IsDir() || info.Mode().Perm() != 0o755 {
		t.Fatalf("configuration folder: %v %v", info, err)
	}
	if got := f.read("/etc/tmpfiles.d/qhost-deploy.conf"); got != "d /run/qhost-deploy 2750 root qhost -\n" {
		t.Fatalf("tmpfiles rule: %q", got)
	}
	if got := f.read(UnitPath()); got != string(packaging.DeployUnit) {
		t.Fatal("the installed unit differs from the embedded one")
	}
	if info, err := os.Stat(filepath.Join(f.root, filepath.FromSlash(UnitPath()))); err != nil || info.Mode().Perm() != 0o644 {
		t.Fatalf("unit mode: %v %v", info, err)
	}
	if !Installed(f.root) {
		t.Fatal("the unit is not reported as installed")
	}

	f.calls = nil
	again, err := f.setUp()
	if err != nil {
		t.Fatal(err)
	}
	if !again.Empty() || len(f.calls) != 0 {
		t.Fatalf("a repeated setup changes something: %v %v", again.Commands(), f.calls)
	}
}

// systemd may have created the configuration folder for the service user; the setup gives it back to root.
func TestSetupRepairsTheConfigurationFolder(t *testing.T) {
	f := newFakeHost(t, baseGroups, basePasswd)
	if _, err := f.setUp(); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(f.root, "etc/qhost-deploy")
	if err := os.Chmod(dir, 0o775); err != nil {
		t.Fatal(err)
	}
	var owner []int
	f.host.Chown = func(p string, uid, gid int) error {
		if p == dir {
			owner = []int{uid, gid}
		}
		return nil
	}
	p, err := f.setUp()
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Commands(); len(got) != 1 || got[0] != "mkdir /etc/qhost-deploy (root, mode 0755)" {
		t.Fatalf("plan: %v", got)
	}
	if !slices.Equal(owner, []int{os.Geteuid(), os.Getegid()}) {
		t.Fatalf("the folder was given to %v, want the user running the setup", owner)
	}
	if info, err := os.Stat(dir); err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("configuration folder: %v %v", info, err)
	}
}

// The service runs as root, so the setup neither creates nor changes an account or a group for it.
func TestSetupCreatesNoAccountForTheService(t *testing.T) {
	f := newFakeHost(t, baseGroups, basePasswd)
	p, err := f.setUp()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range append(f.calls, p.Commands()...) {
		if strings.HasPrefix(c, "groupadd") || strings.HasPrefix(c, "useradd") || strings.HasPrefix(c, "usermod") {
			t.Errorf("ran %q", c)
		}
	}
	if strings.Contains(f.read("/etc/group"), "qhost-deploy") || strings.Contains(f.read("/etc/passwd"), "qhost-deploy") {
		t.Error("the setup created an account or a group")
	}
}

func TestSetupRestartsNothingForAnUnchangedRunningService(t *testing.T) {
	f := newFakeHost(t, baseGroups, basePasswd)
	if _, err := f.setUp(); err != nil {
		t.Fatal(err)
	}
	f.calls = nil
	f.active = false
	p, err := f.host.Plan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Commands(); len(got) != 1 || got[0] != "systemctl enable --now qhost-deploy" {
		t.Fatalf("a stopped service needs only its start: %v", got)
	}
}

func TestSetupNeedsTheGroupDocker(t *testing.T) {
	f := newFakeHost(t, "root:x:0:\nqhost:x:997:\n", basePasswd)
	_, err := f.host.Plan(context.Background())
	if err == nil || !strings.Contains(err.Error(), "group docker") {
		t.Fatalf("err = %v", err)
	}
	if len(f.calls) != 0 {
		t.Fatalf("ran %v", f.calls)
	}
	if _, statErr := os.Stat(filepath.Join(f.root, "etc/tmpfiles.d")); statErr == nil {
		t.Fatal("the setup wrote a file")
	}
}

func TestSetupNeedsTheGroupQhost(t *testing.T) {
	f := newFakeHost(t, "root:x:0:\ndocker:x:998:\n", basePasswd)
	if _, err := f.host.Plan(context.Background()); err == nil || !strings.Contains(err.Error(), "group qhost") {
		t.Fatalf("err = %v", err)
	}
}

func TestSetupStopsAtTheFirstFailure(t *testing.T) {
	f := newFakeHost(t, baseGroups, basePasswd)
	f.failOn = "systemd-tmpfiles"
	if _, err := f.setUp(); err == nil || !strings.Contains(err.Error(), "systemd-tmpfiles") {
		t.Fatalf("err = %v", err)
	}
	if Installed(f.root) {
		t.Fatal("the unit was installed after a failed step")
	}
	for _, c := range f.calls {
		if strings.HasPrefix(c, "systemctl") {
			t.Errorf("ran %q after the failure", c)
		}
	}
}

func TestPlanNeverAddsTheServiceUserToQhost(t *testing.T) {
	f := newFakeHost(t, baseGroups, basePasswd)
	p, err := f.host.Plan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range p.Commands() {
		if strings.Contains(c, "groups qhost") || strings.Contains(c, "-G qhost") {
			t.Errorf("command %q touches the group qhost", c)
		}
	}
}
