package uninstall

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/castrowithcee/qatlas-kit/sysservice"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/deploysetup"
	"github.com/castrowithcee/qatlas-host/internal/host/hostchange"
	"github.com/castrowithcee/qatlas-host/internal/hostservice"
	"github.com/castrowithcee/qatlas-host/internal/service"
	"github.com/castrowithcee/qatlas-host/internal/window"
)

var testPages = []string{"qhost.1", "qhost-stack.1", "qhost-stack-list.1"}

func TestTargetsAreTheFixedPaths(t *testing.T) {
	want := []string{
		"/usr/local/lib/systemd/system/qhost.service",
		"/usr/local/lib/systemd/system/qhost-deploy.service",
		"/etc/systemd/system/qhost-deploy.service.d/10-stack-folders.conf",
		"/etc/systemd/system/qhost-deploy.service.d",
		"/etc/tmpfiles.d/qhost-deploy.conf",
		"/run/qhost-deploy",
		"/usr/local/lib/systemd/system/qhost-hook.service",
		"/etc/tmpfiles.d/qhost-hook.conf",
		"/run/qhost-hook",
		"/usr/local/share/man/man1/qhost.1",
		"/usr/local/share/man/man1/qhost-stack.1",
		"/usr/local/share/man/man1/qhost-stack-list.1",
		"/usr/local/bin/qhost",
	}
	if got := Targets(false, testPages); !slices.Equal(got, want) {
		t.Errorf("targets = %q, want %q", got, want)
	}
	purge := append(slices.Clone(want), "/var/lib/qhost", "/var/lib/qhost-deploy", "/etc/qhost-deploy", "/var/lib/qhost-host", "/etc/qhost-hook")
	if got := Targets(true, testPages); !slices.Equal(got, purge) {
		t.Errorf("purge targets = %q, want %q", got, purge)
	}
}

func TestPagesThatAreNotManpagesOfTheProgramAreIgnored(t *testing.T) {
	got := Targets(false, []string{"../../etc/passwd", "qhost.1", "qhost-/x.1", "other.1", "qhost-a.1/..", "qhost.1"})
	if !slices.Contains(got, "/usr/local/share/man/man1/qhost.1") || len(got) != 11 {
		t.Fatalf("targets = %q", got)
	}
}

// The constants of the removal must name what the installer, the setup and the services use.
func TestConstantsMatchTheInstalledLayout(t *testing.T) {
	host := sysservice.ServicePaths("qhost", "")
	deploy := sysservice.ServicePaths("qhost", "qhost-deploy")
	for name, pair := range map[string][2]string{
		"binary":       {binaryPath, host.Program},
		"host unit":    {hostUnitPath, host.Unit},
		"deploy unit":  {deployUnitPath, deploy.Unit},
		"deploy unit2": {deployUnitPath, deploysetup.UnitPath()},
		"host state":   {hostStateDir, host.State},
		"deploy state": {deployStateDir, deploy.State},
		"host state 2": {hostStateDir, hostservice.DefaultStateDir},
		"deploy st. 2": {deployStateDir, service.DefaultStateDir},
		"deploy conf.": {deployConfigDir, deploy.Config},
		"host window":  {deployConfigDir, filepath.Dir(window.HostFilePath)},
		"channel":      {channelDir, control.DeployEndpoint().Dir},
		"journal":      {journalDir, filepath.Dir(hostchange.DefaultJournalDir)},
		"host name":    {hostService, "qhost"},
		"deploy name":  {deployService, deploysetup.Service},
	} {
		if pair[0] != pair[1] {
			t.Errorf("%s: %q, installed layout %q", name, pair[0], pair[1])
		}
	}
}

type fakeHost struct {
	root string
	ran  []string
	mu   sync.Mutex
}

func (f *fakeHost) host() Host {
	return Host{Root: f.root, Run: func(_ context.Context, name string, args ...string) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.ran = append(f.ran, name+" "+strings.Join(args, " "))
		switch name {
		case "userdel":
			f.dropAccount("etc/passwd", args[0])
		case "groupdel":
			f.dropAccount("etc/group", args[0])
		}
		return nil
	}}
}

// dropAccount deletes the line of an account from the user or group file, like userdel and groupdel.
func (f *fakeHost) dropAccount(file, name string) {
	full := filepath.Join(f.root, file)
	data, err := os.ReadFile(full)
	if err != nil {
		return
	}
	var keep []string
	for _, l := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(l, name+":") {
			keep = append(keep, l)
		}
	}
	_ = os.WriteFile(full, []byte(strings.Join(keep, "\n")), 0o644)
}

// newHost builds an installed host below a temporary root, with a stack folder and bystander files that must
// survive every removal. It returns the host, the stack folder as a host path and the bystander host paths.
func newHost(t *testing.T) (*fakeHost, string, []string) {
	t.Helper()
	f := &fakeHost{root: t.TempDir()}
	write := func(p, content string) {
		full := filepath.Join(f.root, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range Targets(true, testPages) {
		if filepath.Ext(p) == ".d" || strings.HasPrefix(p, "/var/lib") || p == channelDir || p == deploysetup.HookRunDir || p == deploysetup.HookConfigDir || p == deployConfigDir {
			continue
		}
		write(p, "invented")
	}
	write("/run/qhost-deploy/control.sock", "invented")
	write("/run/qhost-hook/system/marker", "invented")
	write("/etc/qhost-hook/config.json", "{}")
	write("/etc/qhost-deploy/window.json", "{}")
	write("/var/lib/qhost/state.json", "invented")
	write("/var/lib/qhost-deploy/secrets", "invented")
	write("/var/lib/qhost-host/journal/1.json", "invented")
	write("/etc/passwd", "root:x:0:0::/root:/bin/sh\nqhost:x:997:997::/var/lib/qhost:/usr/sbin/nologin\nqhost-deploy:x:996:996::/x:/usr/sbin/nologin\nqhost-hook:x:995:995::/nonexistent:/usr/sbin/nologin\n")
	write("/etc/group", "root:x:0:\nqhost:x:997:alice\nqhost-deploy:x:996:\nqhost-hook:x:995:\ndocker:x:998:qhost-deploy\n")
	stack := "/srv/stacks"
	bystanders := []string{stack + "/app/compose.yaml", stack + "/app/data/db", "/etc/qhost/keep", "/usr/local/bin/other",
		"/usr/local/share/man/man1/other.1", "/etc/systemd/system/other.service"}
	for _, p := range bystanders {
		write(p, "invented")
	}
	return f, stack, bystanders
}

func tree(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		out = append(out, "/"+filepath.ToSlash(strings.TrimPrefix(p, root+string(filepath.Separator))))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(out)
	return out
}

func exists(root, p string) bool {
	_, err := os.Lstat(filepath.Join(root, p))
	return err == nil
}

func apply(t *testing.T, f *fakeHost, purge bool, keep ...string) Plan {
	t.Helper()
	plan, err := f.host().Plan(purge, testPages, keep)
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	return plan
}

// Everything that vanishes must be one of the fixed targets or lie below a fixed directory target.
func assertOnlyTargetsVanished(t *testing.T, before, after []string, purge bool) {
	t.Helper()
	targets := Targets(purge, testPages)
	dirs := []string{channelDir, deploysetup.HookRunDir}
	if purge {
		dirs = append(dirs, hostStateDir, deployStateDir, deployConfigDir, journalDir, deploysetup.HookConfigDir)
	}
	kept := map[string]bool{}
	for _, p := range after {
		kept[p] = true
	}
	for _, p := range before {
		if kept[p] {
			continue
		}
		ok := slices.Contains(targets, p)
		for _, d := range dirs {
			ok = ok || strings.HasPrefix(p, d+"/")
		}
		// Directories that held only targets are not deleted by name; they must be a parent of a target.
		if !ok {
			t.Errorf("%s vanished but is not a fixed target", p)
		}
	}
}

func TestUninstallRemovesServicesAndKeepsStateAndStacks(t *testing.T) {
	f, _, bystanders := newHost(t)
	before := tree(t, f.root)
	plan := apply(t, f, false)
	after := tree(t, f.root)
	assertOnlyTargetsVanished(t, before, after, false)

	for _, p := range Targets(false, testPages) {
		if p != dropInDir && exists(f.root, p) {
			t.Errorf("%s was not removed", p)
		}
	}
	if exists(f.root, dropInDir) {
		t.Error("the empty drop-in directory stays")
	}
	for _, p := range append(bystanders, "/var/lib/qhost/state.json", "/var/lib/qhost-deploy/secrets",
		"/var/lib/qhost-host/journal/1.json", "/etc/passwd", "/etc/group") {
		if !exists(f.root, p) {
			t.Errorf("%s was removed", p)
		}
	}
	want := []string{"systemctl disable --now qhost-hook", "systemctl disable --now qhost-deploy", "systemctl disable --now qhost", "systemctl daemon-reload",
		"userdel qhost-deploy", "groupdel qhost-deploy"}
	if !slices.Equal(f.ran, want) {
		t.Errorf("commands = %q, want %q", f.ran, want)
	}
	for _, p := range []string{"/etc/qhost-hook/config.json", "/etc/passwd"} {
		if !exists(f.root, p) {
			t.Errorf("%s was removed without --purge", p)
		}
	}
	if !strings.Contains(plan.Sentence(), "qhost-hook is stopped and disabled") || !strings.Contains(plan.Sentence(), "/run/qhost-hook") ||
		!strings.Contains(plan.Sentence(), "configuration, the user and the group qhost-hook stay") {
		t.Errorf("the sentence does not name the receiver: %q", plan.Sentence())
	}
	assertOnlyDeployAccountRemoved(t, plan)
	again, err := f.host().Plan(false, testPages, nil)
	if err != nil || !again.Empty() {
		t.Fatalf("repeated plan: %v, %q", err, again.Commands())
	}
}

func TestPurgeRemovesStateJournalUsersAndGroup(t *testing.T) {
	f, _, bystanders := newHost(t)
	before := tree(t, f.root)
	apply(t, f, true)
	assertOnlyTargetsVanished(t, before, tree(t, f.root), true)
	for _, p := range []string{"/var/lib/qhost", "/var/lib/qhost-deploy", "/var/lib/qhost-host", "/etc/qhost-hook", "/run/qhost-hook", "/etc/qhost-deploy"} {
		if exists(f.root, p) {
			t.Errorf("%s was not removed", p)
		}
	}
	for _, p := range bystanders {
		if !exists(f.root, p) {
			t.Errorf("%s was removed", p)
		}
	}
	want := []string{"systemctl disable --now qhost-hook", "systemctl disable --now qhost-deploy", "systemctl disable --now qhost", "systemctl daemon-reload",
		"userdel qhost-hook", "userdel qhost-deploy", "userdel qhost", "groupdel qhost-hook", "groupdel qhost-deploy", "groupdel qhost"}
	if !slices.Equal(f.ran, want) {
		t.Errorf("commands = %q, want %q", f.ran, want)
	}
}

func TestPurgeSkipsGroupThatUserdelRemoved(t *testing.T) {
	f, _, _ := newHost(t)
	plan, err := Host{Root: f.root, Run: func(_ context.Context, name string, args ...string) error {
		f.ran = append(f.ran, name+" "+strings.Join(args, " "))
		if name == "userdel" {
			return os.WriteFile(filepath.Join(f.root, "etc/group"), []byte("root:x:0:\n"), 0o644)
		}
		return nil
	}}.Plan(true, testPages, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, c := range f.ran {
		if strings.HasPrefix(c, "groupdel") {
			t.Errorf("ran %q although the group was gone", c)
		}
	}
}

func TestUninstallWithoutDeployServiceTouchesOnlyTheHostService(t *testing.T) {
	f, _, _ := newHost(t)
	for _, p := range []string{deployUnitPath, tmpfilesPath, dropInFile, deploysetup.HookUnitPath(), deploysetup.HookTmpfilesConf} {
		if err := os.Remove(filepath.Join(f.root, p)); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []string{deploysetup.HookRunDir, deploysetup.HookConfigDir} {
		if err := os.RemoveAll(filepath.Join(f.root, p)); err != nil {
			t.Fatal(err)
		}
	}
	f.dropAccount("etc/passwd", "qhost-deploy")
	f.dropAccount("etc/group", "qhost-deploy")
	plan := apply(t, f, false)
	if strings.Contains(plan.Sentence(), "qhost-hook") {
		t.Errorf("the sentence names a receiver that is not there: %q", plan.Sentence())
	}
	want := []string{"systemctl disable --now qhost", "systemctl daemon-reload"}
	if !slices.Equal(f.ran, want) {
		t.Errorf("commands = %q, want %q", f.ran, want)
	}
}

func TestPlanRefusesDeletionThatReachesAStackFolder(t *testing.T) {
	for _, keep := range []string{"/var/lib/qhost-deploy", "/var/lib/qhost-deploy/stacks", "/var/lib", "/run/qhost-deploy/x", "/var/lib/qhost-host/journal"} {
		f, _, _ := newHost(t)
		before := tree(t, f.root)
		if _, err := f.host().Plan(true, testPages, []string{keep}); err == nil || !strings.Contains(err.Error(), "stack folder") {
			t.Errorf("keep %s: error %v", keep, err)
		}
		if got := tree(t, f.root); !slices.Equal(got, before) || len(f.ran) != 0 {
			t.Errorf("keep %s changed the host", keep)
		}
	}
	f, _, _ := newHost(t)
	if _, err := f.host().Plan(false, testPages, []string{"/var/lib/qhost-deploy"}); err != nil {
		t.Errorf("without purge the state is not touched: %v", err)
	}
}

func TestPlanRefusesSymbolicLinksAndChangesNothing(t *testing.T) {
	for _, link := range []string{binaryPath, hostUnitPath, "/var/lib/qhost", "/usr/local/share/man/man1/qhost.1", channelDir} {
		f, _, _ := newHost(t)
		outside := filepath.Join(t.TempDir(), "outside")
		if err := os.MkdirAll(outside, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(outside, "file"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		full := filepath.Join(f.root, link)
		if err := os.RemoveAll(full); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, full); err != nil {
			t.Fatal(err)
		}
		_, err := f.host().Plan(true, testPages, nil)
		if err == nil || !strings.Contains(err.Error(), "symbolic link") {
			t.Errorf("%s: error %v", link, err)
		}
		if !exists(outside, "file") {
			t.Errorf("%s: the target of the link was touched", link)
		}
	}
}

func TestPlanRefusesSymbolicLinkParent(t *testing.T) {
	f, _, _ := newHost(t)
	outside := t.TempDir()
	dir := filepath.Join(f.root, "usr/local/lib/systemd/system")
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, dir); err != nil {
		t.Fatal(err)
	}
	if _, err := f.host().Plan(false, testPages, nil); err == nil {
		t.Fatal("a unit directory that is a symbolic link was accepted")
	}
}

func TestFailedDisableRemovesNothing(t *testing.T) {
	f, _, _ := newHost(t)
	before := tree(t, f.root)
	plan, err := Host{Root: f.root, Run: func(context.Context, string, ...string) error { return os.ErrPermission }}.Plan(true, testPages, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(context.Background()); err == nil {
		t.Fatal("a failing systemctl was ignored")
	}
	if got := tree(t, f.root); !slices.Equal(got, before) {
		t.Error("files were removed after the services could not be stopped")
	}
}

func TestTargetsIncludeTheInstancesPaths(t *testing.T) {
	inst, err := deploysetup.NewInstance("alice")
	if err != nil {
		t.Fatal(err)
	}
	got := Targets(true, nil, inst)
	for _, want := range []string{inst.DropInDir(), inst.TmpfilesConf(), inst.ChannelDir(), deploysetup.TemplatePath(), inst.StateDir(), inst.ConfigDir()} {
		if !slices.Contains(got, want) {
			t.Errorf("targets lack %s: %q", want, got)
		}
	}
	if slices.Contains(Targets(false, nil, inst), inst.StateDir()) {
		t.Error("the state is a target without purge")
	}
}

func TestPurgeNamesTheReceiverConfigurationUserAndGroup(t *testing.T) {
	f, _, _ := newHost(t)
	plan, err := f.host().Plan(true, testPages, nil)
	if err != nil {
		t.Fatal(err)
	}
	commands := strings.Join(plan.Commands(), "\n")
	for _, want := range []string{"systemctl disable --now qhost-hook", "rm " + deploysetup.HookUnitPath(), "rm " + deploysetup.HookTmpfilesConf,
		"rm -r /run/qhost-hook", "rm -r /etc/qhost-hook", "userdel qhost-hook", "groupdel qhost-hook"} {
		if !strings.Contains(commands, want) {
			t.Errorf("the plan lacks %q: %s", want, commands)
		}
	}
	if !strings.Contains(plan.Sentence(), "/etc/qhost-hook with the hooks, the system user and the group qhost-hook are deleted too") {
		t.Errorf("sentence = %q", plan.Sentence())
	}
}

func TestPurgeSkipsAReceiverUserAndGroupThatDoNotExist(t *testing.T) {
	f, _, _ := newHost(t)
	write := func(p, content string) {
		if err := os.WriteFile(filepath.Join(f.root, p), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("etc/passwd", "root:x:0:0::/root:/bin/sh\nqhost:x:997:997::/x:/bin/sh\nqhost-deploy:x:996:996::/x:/bin/sh\n")
	write("etc/group", "root:x:0:\nqhost:x:997:\nqhost-deploy:x:996:\n")
	apply(t, f, true)
	for _, c := range f.ran {
		if strings.Contains(c, "del qhost-hook") {
			t.Errorf("ran %q although the account does not exist", c)
		}
	}
}

func TestRemovalWithoutPurgeKeepsTheConfigurationOfTheDeployServices(t *testing.T) {
	f, _, _ := newHost(t)
	apply(t, f, false)
	if !exists(f.root, "/etc/qhost-deploy/window.json") {
		t.Error("the host window file was removed without --purge")
	}
}

func TestUninstallWithoutAnAccountOfTheDeployServiceRemovesNone(t *testing.T) {
	f, _, _ := newHost(t)
	f.dropAccount("etc/passwd", "qhost-deploy")
	f.dropAccount("etc/group", "qhost-deploy")
	plan, err := f.host().Plan(false, testPages, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range plan.Commands() {
		if strings.HasPrefix(c, "userdel") || strings.HasPrefix(c, "groupdel") {
			t.Errorf("plan runs %q although no account exists", c)
		}
	}
}

// Without --purge the users and groups of the receiver and the host service stay.
func assertOnlyDeployAccountRemoved(t *testing.T, plan Plan) {
	t.Helper()
	for _, c := range plan.Commands() {
		if (strings.HasPrefix(c, "userdel ") || strings.HasPrefix(c, "groupdel ")) && !strings.Contains(c, "qhost-deploy") {
			t.Errorf("plan without purge runs %q", c)
		}
	}
	if strings.Contains(plan.Sentence(), "cannot be restored") {
		t.Errorf("plan without purge: %q", plan.Sentence())
	}
}
