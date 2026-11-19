// Package uninstall removes both services from the host. It runs as root, deletes only the fixed paths named
// below and starts programs without a shell; nothing it deletes or runs is built from input. The stack
// folders, their data and the containers are never part of it.
package uninstall

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
)

// The units, users and groups of the two services.
const (
	hostService   = "qhost"
	deployService = "qhost-deploy"
)

// What the installer and the setup of the deploy service put on the host, and what the services create.
const (
	binaryPath     = "/usr/local/bin/qhost"
	hostUnitPath   = "/usr/local/lib/systemd/system/qhost.service"
	deployUnitPath = "/usr/local/lib/systemd/system/qhost-deploy.service"
	// manDir holds the manpages of the installer: qhost.1 and one qhost-<command>.1 for each command.
	manDir = "/usr/local/share/man/man1"
	// tmpfilesPath is the rule that creates the channel directory of the deploy service at boot.
	tmpfilesPath = "/etc/tmpfiles.d/qhost-deploy.conf"
	// channelDir is that channel directory; the host service's own one belongs to its unit and goes with it.
	channelDir = "/run/qhost-deploy"
	// dropInFile is the drop-in that lists the stack folders the deploy unit may write; dropInDir stays if
	// another drop-in of the administrator is in it.
	dropInFile = "/etc/systemd/system/qhost-deploy.service.d/10-stack-folders.conf"
	dropInDir  = "/etc/systemd/system/qhost-deploy.service.d"

	hostStateDir   = "/var/lib/qhost"
	deployStateDir = "/var/lib/qhost-deploy"
	// journalDir is the root-only journal of the changes of the host, below the folder of that name.
	journalDir = "/var/lib/qhost-host"
)

var pagePattern = regexp.MustCompile(`^qhost(-[a-z0-9]+)*\.1$`)

// Host is everything the removal takes from the host. The zero value is the real host; tests set Root and Run
// so that no real path, systemctl or user database is involved.
type Host struct {
	// Root is prepended to every host path; empty in production.
	Root string
	// Run starts a program with an argument list and no shell; nil starts the real one.
	Run func(ctx context.Context, name string, args ...string) error
}

func (h Host) path(p string) string {
	if h.Root == "" {
		return p
	}
	return filepath.Join(h.Root, filepath.FromSlash(p))
}

func (h Host) run(ctx context.Context, name string, args ...string) error {
	if h.Run != nil {
		return h.Run(ctx, name, args...)
	}
	return exec.CommandContext(ctx, name, args...).Run()
}

// Step is one change of the removal. Command is the exact command or the removal it stands for, as shown to
// the person.
type Step struct {
	Command string
	do      func(ctx context.Context) error
}

// Plan is the list of steps the removal needs on this host; it is empty when nothing is left to remove.
type Plan struct {
	Steps []Step
	Purge bool
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

// Sentence says in plain words what the removal does and what stays.
func (p Plan) Sentence() string {
	s := "Remove qhost and the deploy service from this host: both services are stopped and disabled, and the program, " +
		"the units, the drop-in of the deploy unit, the rule of its channel and the manpages are deleted."
	keep := " The state of both services, the journal of the host changes, the users and the group stay."
	if p.Purge {
		s += " The state and the log of both services, including credentials and backups, the journal of the host " +
			"changes, the system users qhost and qhost-deploy and the group qhost are deleted too, and they cannot be restored."
		keep = ""
	}
	return s + keep + " The stack folders, their data and the containers are not touched."
}

// Apply carries out the steps in order and stops at the first failure; what ran stays in place, and a repeated
// removal continues from there.
func (p Plan) Apply(ctx context.Context) error {
	for _, s := range p.Steps {
		if err := s.do(ctx); err != nil {
			return fmt.Errorf("%s: %w", s.Command, err)
		}
	}
	return nil
}

// Targets lists every path the removal can delete on a host that has everything, for pages the manpages of
// the program. It is the only list of what is deleted.
func Targets(purge bool, pages []string) []string {
	out := []string{hostUnitPath, deployUnitPath, dropInFile, dropInDir, tmpfilesPath, channelDir}
	for _, page := range validPages(pages) {
		out = append(out, manDir+"/"+page)
	}
	out = append(out, binaryPath)
	if purge {
		out = append(out, hostStateDir, deployStateDir, journalDir)
	}
	return out
}

func validPages(pages []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, p := range pages {
		if pagePattern.MatchString(p) && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

// Plan checks the host and returns the steps still needed. It changes nothing; an error means the removal
// cannot start, with nothing changed. pages are the file names of the manpages of the program. keep are paths
// that must stay, the bound stack folders: a plan whose deletions would reach one, or lie in one, is refused.
func (h Host) Plan(purge bool, pages, keep []string) (Plan, error) {
	// Only the directories go with everything in them, so only they can reach a stack folder.
	recursive := []string{channelDir}
	if purge {
		recursive = append(recursive, hostStateDir, deployStateDir, journalDir)
	}
	for _, target := range recursive {
		for _, k := range keep {
			if overlaps(target, k) {
				return Plan{}, fmt.Errorf("%s would delete the stack folder %s or a part of it; nothing was changed", target, k)
			}
		}
	}
	plan := Plan{Purge: purge}
	add := func(command string, do func(ctx context.Context) error) {
		plan.Steps = append(plan.Steps, Step{Command: command, do: do})
	}
	command := func(name string, args ...string) {
		add(name+" "+strings.Join(args, " "), func(ctx context.Context) error { return h.run(ctx, name, args...) })
	}
	exists := func(p string) (bool, error) {
		parent := filepath.Dir(h.path(p))
		if info, err := os.Lstat(parent); err == nil && info.Mode()&fs.ModeSymlink != 0 {
			return false, fmt.Errorf("%s is a symbolic link; nothing was changed", filepath.Dir(p))
		}
		info, err := os.Lstat(h.path(p))
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return false, nil
		case err != nil:
			return false, err
		case info.Mode()&fs.ModeSymlink != 0:
			return false, fmt.Errorf("%s is a symbolic link; nothing was changed", p)
		}
		return true, nil
	}
	file := func(p string) error {
		ok, err := exists(p)
		if err != nil || !ok {
			return err
		}
		if info, err := os.Lstat(h.path(p)); err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("%s is not a regular file; nothing was changed", p)
		}
		add("rm "+p, func(context.Context) error { return h.removeFile(p) })
		return nil
	}

	units := map[string]string{hostService: hostUnitPath, deployService: deployUnitPath}
	var reload bool
	for _, name := range []string{deployService, hostService} {
		ok, err := exists(units[name])
		if err != nil {
			return Plan{}, err
		}
		if ok {
			command("systemctl", "disable", "--now", name)
			reload = true
		}
	}
	for _, p := range []string{hostUnitPath, deployUnitPath, dropInFile} {
		if err := file(p); err != nil {
			return Plan{}, err
		}
	}
	if ok, err := exists(dropInFile); err != nil {
		return Plan{}, err
	} else if ok {
		reload = true
		add("rmdir "+dropInDir+" if it is empty", func(context.Context) error { return h.removeEmptyDir(dropInDir) })
	}
	if err := file(tmpfilesPath); err != nil {
		return Plan{}, err
	}
	if err := h.dir(&plan, channelDir, "rm -r "+channelDir, exists); err != nil {
		return Plan{}, err
	}
	var found []string
	for _, page := range validPages(pages) {
		p := manDir + "/" + page
		ok, err := exists(p)
		if err != nil {
			return Plan{}, err
		}
		if ok {
			if info, err := os.Lstat(h.path(p)); err != nil || !info.Mode().IsRegular() {
				return Plan{}, fmt.Errorf("%s is not a regular file; nothing was changed", p)
			}
			found = append(found, p)
		}
	}
	if len(found) > 0 {
		add(fmt.Sprintf("rm the %d manpages qhost.1 and qhost-<command>.1 in %s", len(found), manDir), func(context.Context) error {
			for _, p := range found {
				if err := h.removeFile(p); err != nil {
					return err
				}
			}
			return nil
		})
	}
	if err := file(binaryPath); err != nil {
		return Plan{}, err
	}
	if reload || len(plan.Steps) > 0 {
		command("systemctl", "daemon-reload")
	}

	if purge {
		for _, d := range []string{hostStateDir, deployStateDir, journalDir} {
			if err := h.dir(&plan, d, "rm -r "+d, exists); err != nil {
				return Plan{}, err
			}
		}
		users, err := h.names("/etc/passwd")
		if err != nil {
			return Plan{}, err
		}
		groups, err := h.names("/etc/group")
		if err != nil {
			return Plan{}, err
		}
		for _, name := range []string{deployService, hostService} {
			if users[name] {
				command("userdel", name)
			}
		}
		for _, name := range []string{deployService, hostService} {
			if groups[name] {
				name := name
				add("groupdel "+name+" if userdel did not remove it", func(ctx context.Context) error {
					if now, err := h.names("/etc/group"); err != nil || !now[name] {
						return err
					}
					return h.run(ctx, "groupdel", name)
				})
			}
		}
	}
	return plan, nil
}

// dir adds the removal of the directory p if it exists as a plain directory.
func (h Host) dir(plan *Plan, p, command string, exists func(string) (bool, error)) error {
	ok, err := exists(p)
	if err != nil || !ok {
		return err
	}
	if info, err := os.Lstat(h.path(p)); err != nil || !info.IsDir() {
		return fmt.Errorf("%s is not a directory; nothing was changed", p)
	}
	plan.Steps = append(plan.Steps, Step{Command: command, do: func(context.Context) error { return h.removeDir(p) }})
	return nil
}

// removeFile deletes the regular file at the host path p; a symbolic link or anything else stays.
func (h Host) removeFile(p string) error {
	info, err := os.Lstat(h.path(p))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return err
	case !info.Mode().IsRegular():
		return fmt.Errorf("%s is not a regular file", p)
	}
	return os.Remove(h.path(p))
}

// removeDir deletes the directory at the host path p with everything in it; a symbolic link stays.
func (h Host) removeDir(p string) error {
	info, err := os.Lstat(h.path(p))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return err
	case !info.IsDir():
		return fmt.Errorf("%s is not a directory", p)
	}
	return os.RemoveAll(h.path(p))
}

// removeEmptyDir deletes the directory at the host path p only when nothing is in it.
func (h Host) removeEmptyDir(p string) error {
	err := os.Remove(h.path(p))
	if err == nil || errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTEMPTY) {
		return nil
	}
	return err
}

// names returns the first fields of a colon-separated account file below the root.
func (h Host) names(p string) (map[string]bool, error) {
	f, err := os.Open(h.path(p))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", p, err)
	}
	defer f.Close()
	out := map[string]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if line := sc.Text(); line != "" && !strings.HasPrefix(line, "#") {
			out[strings.SplitN(line, ":", 2)[0]] = true
		}
	}
	return out, sc.Err()
}

// overlaps reports whether a and b are the same path or one lies inside the other.
func overlaps(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	return a == b || strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/")
}
