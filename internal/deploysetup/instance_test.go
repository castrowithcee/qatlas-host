package deploysetup

import (
	"context"
	"errors"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/packaging"
)

const (
	instanceGroups = "root:x:0:\nsudo:x:27:carol\ndocker:x:998:dave\nqhost:x:997:erin\nfrank:x:1005:\n"
	instancePasswd = "root:x:0:0::/root:/bin/bash\n" +
		"alice:x:1001:1001::/home/alice:/bin/bash\n" +
		"carol:x:1002:1002::/home/carol:/bin/bash\n" +
		"dave:x:1003:1003::/home/dave:/bin/bash\n" +
		"erin:x:1004:1004::/home/erin:/bin/bash\n" +
		"frank:x:1005:998::/home/frank:/bin/bash\n"
	instanceSubIDs = "alice:100000:65536\ncarol:165536:65536\ndave:231072:65536\nerin:296608:65536\nfrank:362144:65536\n"
)

// instanceHost is a host with rootless engines set up for its users and a runner that keeps the linger files,
// the channel directories and the unit state like the real programs would.
type instanceHost struct {
	*fakeHost
	running map[string]bool
}

func newInstanceHost(t *testing.T) *instanceHost {
	t.Helper()
	h := &instanceHost{fakeHost: newFakeHost(t, instanceGroups, instancePasswd), running: map[string]bool{}}
	h.put("/etc/subuid", instanceSubIDs)
	h.put("/etc/subgid", instanceSubIDs)
	for _, u := range []string{"alice", "carol", "dave", "erin", "frank"} {
		h.put(path.Join("/home", u, userUnit), "[Service]\n")
	}
	h.host.Run = h.runInstance
	return h
}

func (h *instanceHost) runInstance(_ context.Context, name string, args ...string) error {
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
	case name == "loginctl":
		h.put(path.Join(lingerDir, args[1]), "")
	case name == "systemd-tmpfiles":
		user := strings.TrimSuffix(strings.TrimPrefix(path.Base(args[1]), Service+"-"), ".conf")
		if err := os.MkdirAll(h.host.path("/run/"+Service+"-"+user), 0o755); err != nil {
			h.t.Fatal(err)
		}
	case name == "systemctl" && args[0] == "enable":
		h.running[args[2]] = true
	}
	return nil
}

func (h *instanceHost) setUpInstance(user string) (Plan, error) {
	h.t.Helper()
	p, err := h.host.PlanInstance(context.Background(), user)
	if err != nil {
		return p, err
	}
	return p, p.Apply(context.Background())
}

// refused checks that the plan for user fails with a message holding want and that nothing ran or was written.
func (h *instanceHost) refused(user, want string) {
	h.t.Helper()
	_, err := h.host.PlanInstance(context.Background(), user)
	if err == nil || !strings.Contains(err.Error(), want) {
		h.t.Fatalf("plan for %q: err = %v, want %q", user, err, want)
	}
	if len(h.calls) != 0 {
		h.t.Fatalf("ran %v", h.calls)
	}
	if list, _ := Instances(h.root); len(list) != 0 || h.host.exists(TemplatePath()) {
		h.t.Fatalf("the refused setup wrote something: %v", list)
	}
}

func TestInstanceDerivesItsNames(t *testing.T) {
	i, err := NewInstance("e2e-user_1")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range [][2]string{
		{i.User(), "e2e-user_1"},
		{i.Unit(), "qhost-deploy@e2e-user_1.service"},
		{i.StateDir(), "/var/lib/qhost-deploy-e2e-user_1"},
		{i.ConfigDir(), "/etc/qhost-deploy-e2e-user_1"},
		{i.ChannelDir(), "/run/qhost-deploy-e2e-user_1"},
		{i.TmpfilesConf(), "/etc/tmpfiles.d/qhost-deploy-e2e-user_1.conf"},
		{i.DropInDir(), "/etc/systemd/system/qhost-deploy@e2e-user_1.service.d"},
		{EngineSocket(1001), "/run/user/1001/docker.sock"},
		{TemplatePath(), "/usr/local/lib/systemd/system/qhost-deploy@.service"},
	} {
		if c[0] != c[1] {
			t.Errorf("got %q, want %q", c[0], c[1])
		}
	}
	ep := i.Endpoint()
	if ep.Dir != i.ChannelDir() || ep.ServiceUser != "e2e-user_1" || ep.Group != "qhost" || ep.SocketBase != "control" {
		t.Fatalf("endpoint = %+v", ep)
	}
}

func TestInstanceRefusesInvalidNames(t *testing.T) {
	for _, name := range []string{"", "Alice", "1abc", "-abc", "a/b", "a.b", "a b", "a%I", "../x", strings.Repeat("a", 33)} {
		if _, err := NewInstance(name); err == nil {
			t.Errorf("accepted %q", name)
		}
	}
	if _, err := NewInstance("_" + strings.Repeat("a", 31)); err != nil {
		t.Errorf("refused a name of 32 characters: %v", err)
	}
}

// The derived names and the template must agree, or the service would listen where no client looks.
func TestTemplateRendersTheDerivedNames(t *testing.T) {
	i, _ := NewInstance("alice")
	unit := unitLines(strings.ReplaceAll(string(packaging.DeployInstanceUnit), "%i", i.User()))
	for _, want := range []string{
		"ReadWritePaths=" + i.ChannelDir(),
		"StateDirectory=" + path.Base(i.StateDir()),
		"ConfigurationDirectory=" + path.Base(i.ConfigDir()),
		"Environment=DOCKER_CONFIG=" + path.Join(i.StateDir(), "docker"),
		"ExecStart=/usr/local/bin/" + Program + " deploy-service run --instance " + i.User(),
		"User=" + i.User(),
	} {
		if !unit[want] {
			t.Errorf("rendered template lacks %q", want)
		}
	}
}

func unitLines(unit string) map[string]bool {
	out := map[string]bool{}
	for _, l := range strings.Split(unit, "\n") {
		out[strings.TrimSpace(l)] = true
	}
	return out
}

func TestInstancesListsTheRules(t *testing.T) {
	root := t.TempDir()
	if list, err := Instances(root); err != nil || len(list) != 0 {
		t.Fatalf("no rule directory: %v %v", list, err)
	}
	dir := filepath.Join(root, "etc", "tmpfiles.d")
	for _, name := range []string{"qhost-deploy.conf", "qhost-deploy-bob.conf", "qhost-deploy-alice.conf", "qhost-deploy-Bad.conf",
		"other.conf", "qhost-deploy-carol.conf.bak"} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "qhost-deploy-dave.conf"), 0o755); err != nil {
		t.Fatal(err)
	}
	list, err := Instances(root)
	if err != nil {
		t.Fatal(err)
	}
	var users []string
	for _, i := range list {
		users = append(users, i.User())
	}
	if !slices.Equal(users, []string{"alice", "bob"}) {
		t.Fatalf("instances = %v", users)
	}
}

func TestInstanceSetupCreatesEverythingAndRepeatsWithoutChange(t *testing.T) {
	h := newInstanceHost(t)
	plan, err := h.setUpInstance("alice")
	if err != nil {
		t.Fatal(err)
	}
	wantRun := []string{
		"loginctl enable-linger alice",
		"systemctl daemon-reload",
		"systemd-tmpfiles --create /etc/tmpfiles.d/qhost-deploy-alice.conf",
		"systemctl enable --now " + instanceUnit("alice"),
	}
	if !slices.Equal(h.calls, wantRun) {
		t.Fatalf("commands run:\n%v\nwant\n%v", h.calls, wantRun)
	}
	wantPlan := []string{
		"loginctl enable-linger alice",
		"write /usr/local/lib/systemd/system/qhost-deploy@.service (mode 0644)",
		"systemctl daemon-reload",
		"write /etc/tmpfiles.d/qhost-deploy-alice.conf (mode 0644): d /run/qhost-deploy-alice 2750 alice qhost -",
		"systemd-tmpfiles --create /etc/tmpfiles.d/qhost-deploy-alice.conf",
		"systemctl enable --now " + instanceUnit("alice"),
	}
	if !slices.Equal(plan.Commands(), wantPlan) {
		t.Fatalf("plan:\n%v\nwant\n%v", plan.Commands(), wantPlan)
	}
	if len(plan.Warnings) != 0 || !strings.Contains(plan.Sentence(), "rootless Docker engine of the user alice") ||
		!strings.Contains(plan.Sentence(), "/run/user/1001/docker.sock") {
		t.Fatalf("sentence %q, warnings %v", plan.Sentence(), plan.Warnings)
	}
	if got := h.read(TemplatePath()); got != string(packaging.DeployInstanceUnit) {
		t.Fatal("the installed template differs from the embedded one")
	}
	if got := h.read("/etc/tmpfiles.d/qhost-deploy-alice.conf"); got != "d /run/qhost-deploy-alice 2750 alice qhost -\n" {
		t.Fatalf("tmpfiles rule: %q", got)
	}
	if list, err := Instances(h.root); err != nil || len(list) != 1 || list[0].User() != "alice" {
		t.Fatalf("instances = %v %v", list, err)
	}
	if Installed(h.root) {
		t.Fatal("an instance counts as the deploy service of the system engine")
	}
	if h.read("/etc/group") != instanceGroups || h.read("/etc/passwd") != instancePasswd {
		t.Fatal("the setup changed the user database")
	}

	h.calls = nil
	again, err := h.setUpInstance("alice")
	if err != nil {
		t.Fatal(err)
	}
	if !again.Empty() || len(h.calls) != 0 {
		t.Fatalf("a repeated setup changes something: %v %v", again.Commands(), h.calls)
	}
}

func TestInstanceSetupKeepsLingeringThatIsOn(t *testing.T) {
	h := newInstanceHost(t)
	h.put("/var/lib/systemd/linger/alice", "")
	plan, err := h.host.PlanInstance(context.Background(), "alice")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range plan.Commands() {
		if strings.HasPrefix(c, "loginctl") {
			t.Fatalf("plan enables lingering again: %v", plan.Commands())
		}
	}
}

func TestInstanceSetupStartsOnlyAStoppedInstance(t *testing.T) {
	h := newInstanceHost(t)
	if _, err := h.setUpInstance("alice"); err != nil {
		t.Fatal(err)
	}
	h.running[instanceUnit("alice")] = false
	plan, err := h.host.PlanInstance(context.Background(), "alice")
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.Commands(); len(got) != 1 || got[0] != "systemctl enable --now "+instanceUnit("alice") {
		t.Fatalf("a stopped instance needs only its start: %v", got)
	}
}

func TestInstanceSetupFindsTheEngineBySocketOrUserUnit(t *testing.T) {
	h := newInstanceHost(t)
	if err := os.Remove(h.host.path(path.Join("/home/alice", userUnit))); err != nil {
		t.Fatal(err)
	}
	h.refused("alice", "no rootless Docker engine")
	h.put("/run/user/1001/docker.sock", "")
	if _, err := h.host.PlanInstance(context.Background(), "alice"); err != nil {
		t.Fatalf("the socket alone is an engine: %v", err)
	}
}

func TestInstanceSetupNeedsSubordinateIDs(t *testing.T) {
	h := newInstanceHost(t)
	h.put("/etc/subuid", "carol:165536:65536\n")
	h.refused("alice", "/etc/subuid")
	h.put("/etc/subuid", "1001:100000:65536\n")
	h.put("/etc/subgid", "")
	h.refused("alice", "/etc/subgid")
	if err := os.Remove(h.host.path("/etc/subgid")); err != nil {
		t.Fatal(err)
	}
	h.refused("alice", "/etc/subgid")
	h.put("/etc/subgid", "alice:100000:65536\n")
	if _, err := h.host.PlanInstance(context.Background(), "alice"); err != nil {
		t.Fatalf("a range by uid counts: %v", err)
	}
}

func TestInstanceSetupRefusesUsersThatReachMore(t *testing.T) {
	for user, want := range map[string]string{
		"dave":  "in the group docker",
		"frank": "in the group docker",
		"erin":  "in the group qhost",
		"root":  "is root",
		"ghost": "does not exist",
		"Alice": "must match",
	} {
		t.Run(user, func(t *testing.T) {
			h := newInstanceHost(t)
			h.refused(user, want)
		})
	}
	h := newInstanceHost(t)
	_, err := h.host.PlanInstance(context.Background(), "dave")
	if err == nil || !strings.Contains(err.Error(), "area System") {
		t.Fatalf("the refusal does not point to the area System: %v", err)
	}
}

func TestInstanceSetupNeedsTheGroupQhost(t *testing.T) {
	h := newInstanceHost(t)
	h.put("/etc/group", "root:x:0:\ndocker:x:998:\n")
	h.refused("alice", "group qhost")
}

func TestInstanceSetupWarnsAboutAnAdministrator(t *testing.T) {
	h := newInstanceHost(t)
	plan, err := h.host.PlanInstance(context.Background(), "carol")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Warnings) != 1 || !strings.Contains(plan.Warnings[0], "administrator") || plan.Empty() {
		t.Fatalf("warnings %v, plan %v", plan.Warnings, plan.Commands())
	}
}

func TestInstanceSetupStopsAtTheFirstFailure(t *testing.T) {
	h := newInstanceHost(t)
	h.failOn = "systemctl daemon-reload"
	if _, err := h.setUpInstance("alice"); err == nil || !strings.Contains(err.Error(), "daemon-reload") {
		t.Fatalf("err = %v", err)
	}
	if list, _ := Instances(h.root); len(list) != 0 {
		t.Fatal("the instance counts as set up after a failed step")
	}
}

// instanceUnit is the unit name of the instance of user.
func instanceUnit(user string) string { return "qhost-deploy@" + user + ".service" }
