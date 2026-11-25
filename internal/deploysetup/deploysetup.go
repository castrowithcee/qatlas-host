// Package deploysetup sets up the deploy services on the host: the service of the system engine with its system
// user, channel directory and unit, and an instance for the rootless engine of an existing user. It sets up the
// webhook receiver too (hook.go), which shares the plan and adds a folder per deploy instance. The setup runs
// as root and starts programs without a shell; the only input in what it runs or writes is the user name of an
// instance, which matches UserPattern.
package deploysetup

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/castrowithcee/qatlas-kit/sysservice"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/hookservice"
	"github.com/castrowithcee/qatlas-host/packaging"
)

const (
	// Program is the executable the unit starts and Service the name of the unit, the user and the group.
	Program = "qhost"
	Service = "qhost-deploy"
	// dockerGroup is the supplementary group of the deploy service; the group is practically root.
	dockerGroup = "docker"
	// stateDir is the home of the user and the state directory of the unit.
	stateDir = "/var/lib/qhost-deploy"
	// noLogin is the shell of the user.
	noLogin = "/usr/sbin/nologin"
	// tmpfilesConf holds the rule that creates the channel directory at every boot.
	tmpfilesConf = "/etc/tmpfiles.d/qhost-deploy.conf"
	// configDir holds what every deploy service reads, the host window among it; only root writes there. The
	// setup creates it before the first start: for ConfigurationDirectory= systemd 252 gives a folder it creates
	// to the service user, but leaves an existing one alone.
	configDir     = "/etc/qhost-deploy"
	configDirMode = 0o755
)

// Host is everything the setup takes from the host. The zero value is the real host; tests set Root and Run so
// that no real /etc, systemctl or user database is involved.
type Host struct {
	// Root is prepended to every host path; empty in production.
	Root string
	// Run starts a program with an argument list and no shell; nil starts the real one. Only the exit status
	// counts: the output of a program is untrusted text and stays out of errors.
	Run func(ctx context.Context, name string, args ...string) error
	// Chown sets the owner of a path; nil changes the real owner.
	Chown func(path string, uid, gid int) error
}

// UnitPath is the path of the installed unit on the host.
func UnitPath() string { return sysservice.ServicePaths(Program, Service).Unit }

// Installed reports whether the setup installed the unit below root; a service that was never set up has none.
func Installed(root string) bool {
	info, err := os.Lstat(hostPath(root, UnitPath()))
	return err == nil && info.Mode().IsRegular()
}

func hostPath(root, p string) string {
	if root == "" {
		return p
	}
	return filepath.Join(root, filepath.FromSlash(p))
}

func (h Host) path(p string) string { return hostPath(h.Root, p) }

func (h Host) run(ctx context.Context, name string, args ...string) error {
	if h.Run != nil {
		return h.Run(ctx, name, args...)
	}
	return exec.CommandContext(ctx, name, args...).Run()
}

// Step is one change of the setup. Command is the exact command or the file write it stands for, as shown to
// the person.
type Step struct {
	Command string
	do      func(ctx context.Context) error
}

// Plan is the list of steps a setup still needs; it is empty when the service is set up already.
type Plan struct {
	Steps []Step
	// Warnings are said with the question; they do not stop the setup.
	Warnings []string
	// Listen is the address of the webhook receiver after the setup; it is empty for a deploy service plan.
	Listen   string
	sentence string
}

// Empty reports whether nothing is left to do.
func (p Plan) Empty() bool { return len(p.Steps) == 0 }

// Commands returns the exact commands of the steps, in order.
func (p Plan) Commands() []string {
	out := make([]string, len(p.Steps))
	for i, s := range p.Steps {
		out[i] = s.Command
	}
	return out
}

// Sentence says in plain words what the setup does.
func (p Plan) Sentence() string {
	if p.sentence != "" {
		return p.sentence
	}
	return "Set up the deploy service: it runs as its own system user qhost-deploy in the group docker, with its own " +
		"channel and state, and starts at boot. The host service does not get access to the container engine."
}

// Apply carries out the steps in order and stops at the first failure; what ran stays in place, and a
// repeated setup continues from there.
func (p Plan) Apply(ctx context.Context) error {
	for _, s := range p.Steps {
		if err := s.do(ctx); err != nil {
			return fmt.Errorf("%s: %w", s.Command, err)
		}
	}
	return nil
}

// Plan checks the unit and the host and returns the steps that are still needed. It changes nothing; an
// error means that the setup cannot start, with nothing changed.
func (h Host) Plan(ctx context.Context) (Plan, error) {
	if err := sysservice.ValidateUnit(packaging.DeployUnit,
		sysservice.UnitRules{Program: Program, Service: Service, Groups: []string{dockerGroup}}); err != nil {
		return Plan{}, fmt.Errorf("the deploy unit of this program is not valid: %w", err)
	}
	groups, err := h.readGroups()
	if err != nil {
		return Plan{}, err
	}
	ep := control.DeployEndpoint()
	if _, ok := groups[dockerGroup]; !ok {
		return Plan{}, fmt.Errorf("the group %s does not exist; install the container engine first", dockerGroup)
	}
	if _, ok := groups[ep.Group]; !ok {
		return Plan{}, fmt.Errorf("the group %s does not exist; install qhost with its installer first", ep.Group)
	}
	users, err := h.readUsers()
	if err != nil {
		return Plan{}, err
	}

	var plan Plan
	add := func(command string, do func(ctx context.Context) error) {
		plan.Steps = append(plan.Steps, Step{Command: command, do: do})
	}
	command := func(name string, args ...string) {
		add(name+" "+strings.Join(args, " "), func(ctx context.Context) error { return h.run(ctx, name, args...) })
	}

	if _, ok := groups[Service]; !ok {
		command("groupadd", "--system", Service)
	}
	if _, ok := users[Service]; !ok {
		command("useradd", "--system", "--gid", Service, "--no-create-home", "--home-dir", stateDir,
			"--shell", noLogin, Service)
	}
	if !contains(groups[dockerGroup], Service) {
		command("usermod", "--append", "--groups", dockerGroup, Service)
	}

	rule := tmpfilesRule(ep.Dir, ep.ServiceUser, ep.Group)
	ruleChanged := h.differs(tmpfilesConf, rule)
	if ruleChanged {
		add("write "+tmpfilesConf+" (mode 0644): "+strings.TrimSpace(string(rule)),
			func(context.Context) error { return h.write(tmpfilesConf, rule) })
	}
	if _, err := os.Stat(h.path(ep.Dir)); ruleChanged || err != nil {
		command("systemd-tmpfiles", "--create", tmpfilesConf)
	}

	if !h.ownDir(configDir, configDirMode) {
		add("mkdir "+configDir+" (root, mode 0755)", func(context.Context) error {
			dir := h.path(configDir)
			if err := os.MkdirAll(dir, configDirMode); err != nil {
				return err
			}
			if err := h.chown(dir, os.Geteuid(), os.Getegid()); err != nil {
				return err
			}
			return os.Chmod(dir, configDirMode)
		})
	}

	unitChanged := h.differs(UnitPath(), packaging.DeployUnit)
	if unitChanged {
		add("write "+UnitPath()+" (mode 0644)", func(context.Context) error { return h.write(UnitPath(), packaging.DeployUnit) })
		command("systemctl", "daemon-reload")
	}
	if err := h.planHookInstances(ctx, &plan, hookservice.SystemInstance); err != nil {
		return Plan{}, err
	}
	if unitChanged || h.run(ctx, "systemctl", "is-enabled", "--quiet", Service) != nil ||
		h.run(ctx, "systemctl", "is-active", "--quiet", Service) != nil {
		command("systemctl", "enable", "--now", Service)
	}
	return plan, nil
}

// tmpfilesRule creates the channel directory for the service user; the group carries through the setgid bit,
// so its members reach the socket without the service reaching them.
func tmpfilesRule(dir, user, group string) []byte {
	return []byte(fmt.Sprintf("d %s 2750 %s %s -\n", dir, user, group))
}

// ownDir reports whether the host path p is a folder with mode perm owned by the user running the setup (root in
// production).
func (h Host) ownDir(p string, perm os.FileMode) bool {
	info, err := os.Lstat(h.path(p))
	if err != nil || !info.IsDir() || info.Mode().Perm() != perm {
		return false
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(st.Uid) == os.Geteuid()
}

// differs reports whether the file at the host path p is missing or holds other content than want.
func (h Host) differs(p string, want []byte) bool {
	got, err := os.ReadFile(h.path(p))
	return err != nil || !bytes.Equal(got, want)
}

// write replaces the file at the host path p atomically with mode 0644.
func (h Host) write(p string, content []byte) error { return h.writeOwned(p, content, nil) }

// writeOwned is write with a step on the temporary file before it replaces the target, for the owner of a file.
func (h Host) writeOwned(p string, content []byte, prepare func(tmp string) error) error {
	target := h.path(p)
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".qhost-setup-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if prepare != nil {
		if err := prepare(tmp.Name()); err != nil {
			return err
		}
	}
	return os.Rename(tmp.Name(), target)
}

func contains(list []string, s string) bool {
	for _, e := range list {
		if e == s {
			return true
		}
	}
	return false
}

// readGroups maps every group name of /etc/group to its member names.
func (h Host) readGroups() (map[string][]string, error) {
	out := map[string][]string{}
	err := h.readColon("/etc/group", 4, func(f []string) {
		var members []string
		if f[3] != "" {
			members = strings.Split(f[3], ",")
		}
		out[f[0]] = members
	})
	return out, err
}

// readUsers maps every user name of /etc/passwd to itself; only the existence counts.
func (h Host) readUsers() (map[string]struct{}, error) {
	out := map[string]struct{}{}
	err := h.readColon("/etc/passwd", 2, func(f []string) { out[f[0]] = struct{}{} })
	return out, err
}

func (h Host) readColon(p string, fields int, each func([]string)) error {
	f, err := os.Open(h.path(p))
	if err != nil {
		return fmt.Errorf("read %s: %w", p, err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if parts := strings.Split(line, ":"); len(parts) >= fields {
			each(parts)
		}
	}
	if err := sc.Err(); err != nil {
		return errors.Join(fmt.Errorf("read %s", p), err)
	}
	return nil
}
