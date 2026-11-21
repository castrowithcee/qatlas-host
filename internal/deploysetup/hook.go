package deploysetup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/castrowithcee/qatlas-kit/sysservice"

	"github.com/castrowithcee/qatlas-host/internal/hookservice"
	"github.com/castrowithcee/qatlas-host/packaging"
)

// The setup of the webhook receiver lives next to the setup of the deploy service because both share the host
// access, the plan and its steps, and because a new deploy instance must add its folder for the receiver in the
// same plan.
const (
	// HookService is the name of the unit, the user and the group of the webhook receiver.
	HookService = "qhost-hook"
	// DefaultHookListen is the address of a first setup without --listen: local only, on a fixed port above 1024.
	DefaultHookListen = "127.0.0.1:8480"
	// HookConfigDir holds the configuration of the receiver.
	HookConfigDir = "/etc/qhost-hook"
	// HookConfigPath is the configuration file, owned by root and readable by everybody.
	HookConfigPath = hookservice.DefaultConfigPath
	// HookRunDir holds one folder of markers per deploy instance.
	HookRunDir = hookservice.DefaultRunDir
	// HookTmpfilesConf holds the rules that create the marker folders at every boot.
	HookTmpfilesConf = tmpfilesDir + "/qhost-hook.conf"

	hookDirMode    = 0o755
	hookConfigMode = 0o644
	hookFolderMode = "0770"
	// minHookPort keeps the receiver, which has no capabilities, off the privileged ports.
	minHookPort = 1025
)

var hostnamePattern = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?$`)

// HookUnitPath is the path of the installed unit of the receiver on the host.
func HookUnitPath() string { return sysservice.ServicePaths(Program, HookService).Unit }

// HookInstalled reports whether the setup installed the unit of the receiver below root.
func HookInstalled(root string) bool {
	info, err := os.Lstat(hostPath(root, HookUnitPath()))
	return err == nil && info.Mode().IsRegular()
}

// HookConfig reads the configuration of the receiver below root; a missing file is an error that wraps
// fs.ErrNotExist. Reading needs no root.
func HookConfig(root string) (hookservice.Config, error) {
	data, err := os.ReadFile(hostPath(root, HookConfigPath))
	if err != nil {
		return hookservice.Config{}, err
	}
	return hookservice.ParseConfig(data)
}

// ValidateHookListen checks a listen address of the receiver: host:port with a port above 1024, because the
// receiver runs without capabilities. local reports whether the address reaches only this host.
func ValidateHookListen(addr string) (local bool, err error) {
	host, portText, err := net.SplitHostPort(addr)
	if err != nil {
		return false, fmt.Errorf("the address %q is not host:port", addr)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < minHookPort || port > 65535 {
		return false, fmt.Errorf("the port of %q must be a number from %d to 65535", addr, minHookPort)
	}
	if host == "" {
		return false, nil
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback(), nil
	}
	if !hostnamePattern.MatchString(host) {
		return false, fmt.Errorf("the host of %q is neither an IP address nor a host name", addr)
	}
	return host == "localhost", nil
}

// validateHookUnit is the check of the unit that qatlas-kit cannot do: its rule wants a state and a
// configuration directory named like the service, which the receiver does not have.
func validateHookUnit(unit []byte) error {
	lines := map[string]bool{}
	for _, l := range strings.Split(string(unit), "\n") {
		lines[strings.TrimSpace(l)] = true
	}
	for _, want := range []string{"User=" + HookService, "Group=" + HookService, "NoNewPrivileges=yes",
		"ProtectSystem=strict", "CapabilityBoundingSet=", "ReadWritePaths=" + HookRunDir,
		"ExecStart=" + sysservice.ServicePaths(Program, "").Program + " hook-service run"} {
		if !lines[want] {
			return fmt.Errorf("the line %q is missing", want)
		}
	}
	for l := range lines {
		for _, bad := range []string{"SupplementaryGroups=", "AmbientCapabilities=", "DynamicUser=", "User=root", "Group=root"} {
			if strings.HasPrefix(l, bad) {
				return fmt.Errorf("the line %q is not allowed", l)
			}
		}
	}
	return nil
}

func (h Host) chown(p string, uid, gid int) error {
	if h.Chown != nil {
		return h.Chown(p, uid, gid)
	}
	return os.Lchown(p, uid, gid)
}

// chownHook gives the path to root and the group of the receiver, which exists once the step that creates it ran.
func (h Host) chownHook(p string) error {
	groups, err := h.readGroupDB()
	if err != nil {
		return err
	}
	g, ok := groups[HookService]
	if !ok {
		return fmt.Errorf("the group %s does not exist", HookService)
	}
	return h.chown(p, 0, int(g.gid))
}

// PlanHook checks the host and returns the steps the webhook receiver still needs. listen is the address to set;
// empty keeps the stored one, or takes the default at the first setup. It changes nothing; an error means that
// the setup cannot start, with nothing changed.
func (h Host) PlanHook(ctx context.Context, listen string) (Plan, error) {
	if err := validateHookUnit(packaging.HookUnit); err != nil {
		return Plan{}, fmt.Errorf("the webhook receiver unit of this program is not valid: %w", err)
	}
	groups, err := h.readGroupDB()
	if err != nil {
		return Plan{}, err
	}
	accounts, err := h.readAccounts()
	if err != nil {
		return Plan{}, err
	}
	if err := checkHookUser(groups, accounts); err != nil {
		return Plan{}, err
	}

	cfg, exists, err := h.readHookConfig()
	if err != nil {
		return Plan{}, err
	}
	if listen == "" {
		listen = DefaultHookListen
		if exists {
			listen = cfg.Listen
		}
	}
	local, err := ValidateHookListen(listen)
	if err != nil {
		return Plan{}, err
	}
	listenChanged := exists && cfg.Listen != listen
	cfg.Version, cfg.Listen = 1, listen
	if cfg.Hooks == nil {
		cfg.Hooks = []hookservice.Hook{}
	}
	configJSON, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return Plan{}, err
	}
	configJSON = append(configJSON, '\n')

	plan := Plan{Listen: listen}
	plan.sentence = hookSentence(listen, local, listenChanged)
	add := func(command string, do func(ctx context.Context) error) {
		plan.Steps = append(plan.Steps, Step{Command: command, do: do})
	}
	command := func(name string, args ...string) {
		add(name+" "+strings.Join(args, " "), func(ctx context.Context) error { return h.run(ctx, name, args...) })
	}

	if _, ok := groups[HookService]; !ok {
		command("groupadd", "--system", HookService)
	}
	if _, ok := accounts[HookService]; !ok {
		command("useradd", "--system", "--gid", HookService, "--no-create-home", "--home-dir", "/nonexistent",
			"--shell", noLogin, HookService)
	}

	if info, err := os.Stat(h.path(HookConfigDir)); err != nil || info.Mode().Perm() != hookDirMode || !info.IsDir() {
		add("mkdir "+HookConfigDir+" (root:"+HookService+", mode 0755)", func(context.Context) error {
			dir := h.path(HookConfigDir)
			if err := os.MkdirAll(dir, hookDirMode); err != nil {
				return err
			}
			if err := os.Chmod(dir, hookDirMode); err != nil {
				return err
			}
			return h.chownHook(dir)
		})
	}
	if info, err := os.Stat(h.path(HookConfigPath)); !exists || err != nil || listenChanged || info.Mode().Perm() != hookConfigMode {
		add("write "+HookConfigPath+" (root:"+HookService+", mode 0644): listen "+listen, func(context.Context) error {
			return h.writeOwned(HookConfigPath, configJSON, h.chownHook)
		})
	}

	rule, dirs, err := h.hookRule(groups, accounts)
	if err != nil {
		return Plan{}, err
	}
	h.addHookRule(&plan, rule, dirs)

	unitChanged := h.differs(HookUnitPath(), packaging.HookUnit)
	if unitChanged {
		add("write "+HookUnitPath()+" (mode 0644)", func(context.Context) error { return h.write(HookUnitPath(), packaging.HookUnit) })
		command("systemctl", "daemon-reload")
	}
	enabled := h.run(ctx, "systemctl", "is-enabled", "--quiet", HookService) == nil
	active := h.run(ctx, "systemctl", "is-active", "--quiet", HookService) == nil
	if unitChanged || !enabled || !active {
		command("systemctl", "enable", "--now", HookService)
	}
	// A running receiver keeps its old address and unit until it restarts.
	if active && (listenChanged || unitChanged) {
		command("systemctl", "restart", HookService)
	}
	return plan, nil
}

// hookSentence says what the receiver setup does; a non-local address names the plain HTTP.
func hookSentence(listen string, local, changed bool) string {
	s := "Set up the webhook receiver: it runs as its own system user qhost-hook without login, home or extra groups, " +
		"listens on " + listen + ", only drops empty markers for stacks in " + HookRunDir + ", and starts at boot. " +
		"It reaches neither the container engine nor the host service."
	if changed {
		s += " The address changes, so the receiver restarts."
	}
	if !local {
		s += " The address is not local and the receiver speaks plain HTTP: a reverse proxy should provide HTTPS."
	}
	return s
}

// checkHookUser refuses an existing user that does not match: the receiver user has its own primary group and
// no other group.
func checkHookUser(groups map[string]group, accounts map[string]account) error {
	a, ok := accounts[HookService]
	if !ok {
		return nil
	}
	g, ok := groups[HookService]
	if !ok || g.gid != a.gid {
		return fmt.Errorf("the user %s exists, but its primary group is not the group %s; remove or fix the user first",
			HookService, HookService)
	}
	var names []string
	for n, other := range groups {
		if n != HookService && contains(other.members, HookService) {
			names = append(names, n)
		}
	}
	if len(names) > 0 {
		sort.Strings(names)
		return fmt.Errorf("the user %s is in the group %s; it must be in no other group, remove it from there first",
			HookService, strings.Join(names, ", "))
	}
	return nil
}

// readHookConfig reads the stored configuration; exists is false when there is none. A file that does not parse
// stops the setup, so that hooks are never overwritten.
func (h Host) readHookConfig() (cfg hookservice.Config, exists bool, err error) {
	f, err := os.Open(h.path(HookConfigPath))
	if errors.Is(err, fs.ErrNotExist) {
		return hookservice.Config{}, false, nil
	}
	if err != nil {
		return hookservice.Config{}, false, fmt.Errorf("read %s: %w", HookConfigPath, err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 1<<20))
	if err != nil {
		return hookservice.Config{}, false, fmt.Errorf("read %s: %w", HookConfigPath, err)
	}
	cfg, err = hookservice.ParseConfig(data)
	if err != nil {
		return hookservice.Config{}, false, fmt.Errorf("%s is not valid and stays as it is: %w", HookConfigPath, err)
	}
	return cfg, true, nil
}

// hookRule is the content of the tmpfiles file: the folder of the receiver and one marker folder for the system
// instance, when its deploy service is set up, and for every user instance with a rule, plus extra names. Each
// folder belongs to the receiver and to the group of the instance's service user.
func (h Host) hookRule(groups map[string]group, accounts map[string]account, extra ...string) ([]byte, []string, error) {
	groupOf := map[string]string{}
	if Installed(h.Root) {
		groupOf[hookservice.SystemInstance] = Service
	}
	insts, err := Instances(h.Root)
	if err != nil {
		return nil, nil, err
	}
	names := []string{}
	for _, i := range insts {
		names = append(names, i.user)
	}
	names = append(names, extra...)
	for _, n := range names {
		if n == hookservice.SystemInstance {
			groupOf[n] = Service
			continue
		}
		a, ok := accounts[n]
		if !ok {
			continue // an instance whose user is gone has no folder to make
		}
		// A primary group without a name is written by its number, which tmpfiles accepts.
		gname := strconv.FormatUint(uint64(a.gid), 10)
		for name, g := range groups {
			if g.gid == a.gid {
				gname = name
			}
		}
		groupOf[n] = gname
	}
	order := make([]string, 0, len(groupOf))
	for n := range groupOf {
		order = append(order, n)
	}
	sort.Slice(order, func(a, b int) bool {
		if (order[a] == hookservice.SystemInstance) != (order[b] == hookservice.SystemInstance) {
			return order[a] == hookservice.SystemInstance
		}
		return order[a] < order[b]
	})
	var b bytes.Buffer
	fmt.Fprintf(&b, "d %s 0755 %s %s -\n", HookRunDir, HookService, HookService)
	dirs := []string{HookRunDir}
	for _, n := range order {
		dir := path.Join(HookRunDir, n)
		fmt.Fprintf(&b, "d %s %s %s %s -\n", dir, hookFolderMode, HookService, groupOf[n])
		dirs = append(dirs, dir)
	}
	return b.Bytes(), dirs, nil
}

// addHookRule adds the steps that write the tmpfiles file and create the folders where either is missing.
func (h Host) addHookRule(plan *Plan, rule []byte, dirs []string) {
	changed := h.differs(HookTmpfilesConf, rule)
	if changed {
		plan.Steps = append(plan.Steps, Step{
			Command: "write " + HookTmpfilesConf + " (mode 0644): " + strings.ReplaceAll(strings.TrimSpace(string(rule)), "\n", "; "),
			do:      func(context.Context) error { return h.write(HookTmpfilesConf, rule) }})
	}
	missing := false
	for _, d := range dirs {
		if !h.exists(d) {
			missing = true
		}
	}
	if changed || missing {
		args := []string{"--create", HookTmpfilesConf}
		plan.Steps = append(plan.Steps, Step{Command: "systemd-tmpfiles " + strings.Join(args, " "),
			do: func(ctx context.Context) error { return h.run(ctx, "systemd-tmpfiles", args...) }})
	}
}

// planHookInstances adds, where the receiver is set up, the marker folder of the instance that is being set up
// (the name system or a user name) to the plan. Where the receiver is not set up it adds nothing.
func (h Host) planHookInstances(plan *Plan, instance string) error {
	if !HookInstalled(h.Root) {
		return nil
	}
	groups, err := h.readGroupDB()
	if err != nil {
		return err
	}
	accounts, err := h.readAccounts()
	if err != nil {
		return err
	}
	rule, dirs, err := h.hookRule(groups, accounts, instance)
	if err != nil {
		return err
	}
	h.addHookRule(plan, rule, dirs)
	return nil
}
