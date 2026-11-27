package deploysetup

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/castrowithcee/qatlas-kit/localipc"

	"github.com/castrowithcee/qatlas-host/internal/admins"
	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/packaging"
)

// UserPattern is what a user name must match to own an instance. It keeps the name usable as is in the unit
// name, in %i of the template and in every derived path.
var UserPattern = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)

const (
	// tmpfilesDir holds the channel rule of every instance; an instance is set up when its rule exists.
	tmpfilesDir = "/etc/tmpfiles.d"
	// lingerDir holds a file per user whose user manager runs without a login.
	lingerDir = "/var/lib/systemd/linger"
	// userUnit is the user unit of a rootless engine, relative to the home of its owner.
	userUnit = ".config/systemd/user/docker.service"
)

// Instance is the deploy service for the rootless engine of one user. Every name and path of the instance is
// derived here, so the setup, the service, the status and later the update and the removal agree on them.
type Instance struct {
	user string
}

// NewInstance returns the instance of user; a name outside UserPattern is refused.
func NewInstance(user string) (Instance, error) {
	if !UserPattern.MatchString(user) {
		return Instance{}, fmt.Errorf("the user name %q cannot own a deploy service instance: it must match %s", user, UserPattern)
	}
	return Instance{user: user}, nil
}

// User is the owner of the engine; the instance runs as this user.
func (i Instance) User() string { return i.user }

func (i Instance) name() string { return Service + "-" + i.user }

// Unit is the instance of the template unit.
func (i Instance) Unit() string { return Service + "@" + i.user + ".service" }

// StateDir is the state directory systemd creates for the user with mode 0700.
func (i Instance) StateDir() string { return "/var/lib/" + i.name() }

// ConfigDir is the configuration directory of the instance.
func (i Instance) ConfigDir() string { return "/etc/" + i.name() }

// ChannelDir is the channel directory, owned by the user and the group qhost through setgid.
func (i Instance) ChannelDir() string { return "/run/" + i.name() }

// TmpfilesConf holds the rule that creates the channel directory at every boot.
func (i Instance) TmpfilesConf() string { return tmpfilesDir + "/" + i.name() + ".conf" }

// DropInDir is the drop-in directory of the instance's unit.
func (i Instance) DropInDir() string { return "/etc/systemd/system/" + i.Unit() + ".d" }

// StackFoldersDropIn is the drop-in that lists the stack folders the instance may write.
func (i Instance) StackFoldersDropIn() string { return i.DropInDir() + "/10-stack-folders.conf" }

// Endpoint is the channel of the instance. Like the deploy service of the system engine it admits root and the
// group qhost, which its user is not a member of.
func (i Instance) Endpoint() localipc.ServiceEndpoint {
	return localipc.ServiceEndpoint{Dir: i.ChannelDir(), SocketBase: "control", ServiceUser: i.user,
		Group: control.ServiceGroup, ErrorName: i.Unit()}
}

// EngineSocket is the socket of the rootless engine of the user with uid.
func EngineSocket(uid uint32) string { return fmt.Sprintf("/run/user/%d/docker.sock", uid) }

// TemplatePath is the path of the installed template unit.
func TemplatePath() string { return path.Join(path.Dir(UnitPath()), Service+"@.service") }

// Instances lists the instances set up below root, sorted by user. A rule file whose name derives no valid
// instance is not one. Reading needs no root: the rules are readable by everybody.
func Instances(root string) ([]Instance, error) {
	entries, err := os.ReadDir(hostPath(root, tmpfilesDir))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", tmpfilesDir, err)
	}
	var out []Instance
	for _, e := range entries {
		user, ok := strings.CutPrefix(e.Name(), Service+"-")
		if !ok || !e.Type().IsRegular() {
			continue
		}
		if user, ok = strings.CutSuffix(user, ".conf"); !ok {
			continue
		}
		if i, err := NewInstance(user); err == nil {
			out = append(out, i)
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].user < out[b].user })
	return out, nil
}

// account is a user of /etc/passwd.
type account struct {
	uid, gid uint32
	home     string
}

// group is a group of /etc/group.
type group struct {
	gid     uint32
	members []string
}

// UserID returns the uid of user from /etc/passwd below root.
func (h Host) UserID(user string) (uint32, error) {
	users, err := h.readAccounts()
	if err != nil {
		return 0, err
	}
	a, ok := users[user]
	if !ok {
		return 0, fmt.Errorf("the user %s does not exist", user)
	}
	return a.uid, nil
}

func (h Host) readAccounts() (map[string]account, error) {
	out := map[string]account{}
	err := h.readColon("/etc/passwd", 7, func(f []string) {
		uid, err1 := strconv.ParseUint(f[2], 10, 32)
		gid, err2 := strconv.ParseUint(f[3], 10, 32)
		if err1 == nil && err2 == nil {
			out[f[0]] = account{uid: uint32(uid), gid: uint32(gid), home: f[5]}
		}
	})
	return out, err
}

func (h Host) readGroupDB() (map[string]group, error) {
	out := map[string]group{}
	err := h.readColon("/etc/group", 4, func(f []string) {
		gid, err := strconv.ParseUint(f[2], 10, 32)
		if err != nil {
			return
		}
		var members []string
		if f[3] != "" {
			members = strings.Split(f[3], ",")
		}
		out[f[0]] = group{gid: uint32(gid), members: members}
	})
	return out, err
}

// GroupIDs maps every group name of /etc/group below root to its id.
func (h Host) GroupIDs() (map[string]uint32, error) {
	groups, err := h.readGroupDB()
	if err != nil {
		return nil, err
	}
	out := make(map[string]uint32, len(groups))
	for name, g := range groups {
		out[name] = g.gid
	}
	return out, nil
}

// inGroup reports whether the user is a member of the group, as a supplementary or the primary group.
func inGroup(groups map[string]group, name, user string, a account) bool {
	g, ok := groups[name]
	return ok && (g.gid == a.gid || contains(g.members, user))
}

// hasSubIDs reports whether the file names a range for the user by name or uid. A missing file names none.
func (h Host) hasSubIDs(p, user string, uid uint32) (bool, error) {
	found := false
	err := h.readColon(p, 3, func(f []string) {
		if f[0] == user || f[0] == strconv.FormatUint(uint64(uid), 10) {
			found = true
		}
	})
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return found, err
}

// hasRootlessEngine reports whether the account has a rootless engine: its socket or its user unit exists.
func (h Host) hasRootlessEngine(a account) bool {
	if h.exists(EngineSocket(a.uid)) {
		return true
	}
	return filepath.IsAbs(a.home) && h.exists(path.Join(a.home, userUnit))
}

// RootlessUsers lists the users that can own an instance as far as the engine goes: not root, a valid name and a
// rootless engine. It is a list to choose from; PlanInstance still checks and refuses every user.
func (h Host) RootlessUsers() ([]string, error) {
	users, err := h.readAccounts()
	if err != nil {
		return nil, err
	}
	var out []string
	for name, a := range users {
		if a.uid != 0 && UserPattern.MatchString(name) && h.hasRootlessEngine(a) {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out, nil
}

func (h Host) exists(p string) bool {
	_, err := os.Lstat(h.path(p))
	return err == nil
}

// PlanInstance checks the host for an instance of user and returns the steps that are still needed. It never
// creates a user or changes a group. It changes nothing; an error means that the setup cannot start, with
// nothing changed.
func (h Host) PlanInstance(ctx context.Context, user string) (Plan, error) {
	inst, err := NewInstance(user)
	if err != nil {
		return Plan{}, err
	}
	groups, err := h.readGroupDB()
	if err != nil {
		return Plan{}, err
	}
	if _, ok := groups[control.ServiceGroup]; !ok {
		return Plan{}, fmt.Errorf("the group %s does not exist; install qhost with its installer first", control.ServiceGroup)
	}
	users, err := h.readAccounts()
	if err != nil {
		return Plan{}, err
	}
	a, ok := users[user]
	if !ok {
		return Plan{}, fmt.Errorf("the user %s does not exist; qhost does not create the owner of a rootless engine", user)
	}
	if a.uid == 0 {
		return Plan{}, fmt.Errorf("the user %s is root; an instance runs as the unprivileged owner of a rootless engine", user)
	}
	for _, g := range []struct{ name, reach string }{
		{control.ServiceGroup, "the channel of the host service"},
		{dockerGroup, "the system engine"},
	} {
		if inGroup(groups, g.name, user, a) {
			return Plan{}, fmt.Errorf("the user %s is in the group %s, and the instance would reach %s, because it runs with "+
				"the groups of its user; remove %s from the group in the area System first", user, g.name, g.reach, user)
		}
	}
	socket := EngineSocket(a.uid)
	if !h.hasRootlessEngine(a) {
		return Plan{}, fmt.Errorf("the user %s has no rootless Docker engine: neither %s nor %s exists; set up rootless Docker "+
			"for the user first", user, socket, path.Join(a.home, userUnit))
	}
	for _, p := range []string{"/etc/subuid", "/etc/subgid"} {
		ok, err := h.hasSubIDs(p, user, a.uid)
		if err != nil {
			return Plan{}, err
		}
		if !ok {
			return Plan{}, fmt.Errorf("the user %s has no entry in %s, which a rootless engine needs; add a range of "+
				"subordinate ids for the user first", user, p)
		}
	}

	plan := Plan{sentence: fmt.Sprintf("Set up a deploy service for the rootless Docker engine of the user %s: it runs as "+
		"%s with the unit %s, its own channel %s and state %s, uses only the engine at %s and starts at boot. %s joins no "+
		"group, and no user is created.", user, user, inst.Unit(), inst.ChannelDir(), inst.StateDir(), socket, user)}
	if inGroup(groups, admins.AdminGroup, user, a) {
		plan.Warnings = append(plan.Warnings, fmt.Sprintf("%s is an administrator (group %s): the instance runs as %s, so "+
			"the damage of a taken-over stack or instance is not limited to an unprivileged user.", user, admins.AdminGroup, user))
	}
	add := func(command string, do func(ctx context.Context) error) {
		plan.Steps = append(plan.Steps, Step{Command: command, do: do})
	}
	command := func(name string, args ...string) {
		add(name+" "+strings.Join(args, " "), func(ctx context.Context) error { return h.run(ctx, name, args...) })
	}

	if !h.exists(path.Join(lingerDir, user)) {
		command("loginctl", "enable-linger", user)
	}
	templateChanged := h.differs(TemplatePath(), packaging.DeployInstanceUnit)
	if templateChanged {
		add("write "+TemplatePath()+" (mode 0644)",
			func(context.Context) error { return h.write(TemplatePath(), packaging.DeployInstanceUnit) })
		command("systemctl", "daemon-reload")
	}
	// The rule comes after the template: an instance counts as set up once its rule exists.
	rule := tmpfilesRule(inst.ChannelDir(), user, control.ServiceGroup)
	ruleChanged := h.differs(inst.TmpfilesConf(), rule)
	if ruleChanged {
		add("write "+inst.TmpfilesConf()+" (mode 0644): "+strings.TrimSpace(string(rule)),
			func(context.Context) error { return h.write(inst.TmpfilesConf(), rule) })
	}
	if ruleChanged || !h.exists(inst.ChannelDir()) {
		command("systemd-tmpfiles", "--create", inst.TmpfilesConf())
	}
	if err := h.planHookInstances(ctx, &plan, user); err != nil {
		return Plan{}, err
	}
	if templateChanged || h.run(ctx, "systemctl", "is-enabled", "--quiet", inst.Unit()) != nil ||
		h.run(ctx, "systemctl", "is-active", "--quiet", inst.Unit()) != nil {
		command("systemctl", "enable", "--now", inst.Unit())
	}
	return plan, nil
}
