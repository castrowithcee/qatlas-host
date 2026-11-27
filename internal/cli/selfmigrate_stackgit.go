package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/castrowithcee/qatlas-kit/cli"

	"github.com/castrowithcee/qatlas-host/internal/service"
)

const stackGitName = "stack-git-root-only"

// stackGitTransition makes the .git of the bound stack folders of the system engine root-only where the releases
// before the deploy service ran as root shared it with the group of its account. The service runs git only in a
// root-only .git, so such a folder would otherwise stay at the finding git-unsafe after the update. It runs in the
// start step only, before the service runs git, and never fails: a folder it cannot secure keeps the finding and
// is reported with the next step. Folders it refuses are reported by done, so that a start that secures nothing
// does not show the step as applied.
var stackGitTransition = transition{
	name: stackGitName,
	done: func(ctx context.Context, root string) (bool, error) {
		return len(oldStackGits(ctx, root, reportOf(ctx))) == 0, nil
	},
	apply: func(ctx context.Context, root string) error {
		report := reportOf(ctx)
		for _, g := range oldStackGits(ctx, root, io.Discard) {
			if err := folders.secureGit(ctx, g.plan); err != nil {
				_, _ = fmt.Fprintf(report, "%s: could not make .git of %q root-only: %s; run 'sudo qhost source secure %s'\n",
					stackGitName, g.dir, reasonOf(err), g.dir)
				continue
			}
			_, _ = fmt.Fprintf(report, "%s: made .git of %q root-only (%d entries, core.sharedRepository removed)\n",
				stackGitName, g.dir, len(g.plan.entries))
		}
		return nil
	},
}

// oldStackGit is a stack folder of the system engine, as the host names it, with the plan that secures its .git.
type oldStackGit struct {
	dir  string
	plan *gitSecure
}

// oldStackGits lists the bound stack folders below root whose .git carries exactly what the releases before the
// root service left: core.sharedRepository=group, and every entry that is not root-only differs only in owner,
// group write, setgid and its group, which is root, qhost-deploy or an id that has no group any more. A folder
// with core.sharedRepository=group that does not fit, or cannot be read, is reported on report and left out;
// folders without it, missing folders and links are not looked at.
func oldStackGits(ctx context.Context, root string, report io.Writer) []oldStackGit {
	var out []oldStackGit
	for _, dir := range service.BoundDirs(filepath.Join(root, service.DefaultStateDir)) {
		path := filepath.Join(root, dir)
		if !groupShared(ctx, path) {
			continue
		}
		plan, err := planGitSecure(ctx, path)
		if err == nil {
			err = notOldSetup(root, plan)
		}
		if err != nil {
			_, _ = fmt.Fprintf(report, "%s: left .git of %q as it is: %s; run 'sudo qhost source secure %s'\n",
				stackGitName, dir, reasonOf(err), dir)
			continue
		}
		out = append(out, oldStackGit{dir: dir, plan: plan})
	}
	return out
}

// groupShared reports whether path is a real folder whose git configuration sets core.sharedRepository to group.
func groupShared(ctx context.Context, path string) bool {
	if info, err := os.Lstat(path); err != nil || !info.IsDir() {
		return false
	}
	if _, err := os.Lstat(filepath.Join(path, ".git")); err != nil {
		return false
	}
	out, err := runGit(ctx, path, "config", "--local", "--get", "core.sharedRepository")
	return err == nil && strings.TrimSpace(string(out)) == "group"
}

// notOldSetup tells why the plan changes more than the releases before the root service did.
func notOldSetup(root string, plan *gitSecure) error {
	if plan.othersWritable != "" {
		return fmt.Errorf("%q can be written by others", plan.othersWritable)
	}
	groups, err := deployHost(root).GroupIDs()
	if err != nil {
		return err
	}
	var foreign []string
	for gid := range plan.gids {
		if name := foreignGroup(gid, groups); name != "" {
			foreign = append(foreign, name)
		}
	}
	if len(foreign) > 0 {
		sort.Strings(foreign)
		return fmt.Errorf("entries have the group %s, which is neither root nor %s", strings.Join(foreign, ", "), serviceAccount)
	}
	return nil
}

// foreignGroup is the name of the group gid when it exists and is neither root nor the group of the account of the
// earlier releases; empty otherwise. An id without a group is the one that account left behind.
func foreignGroup(gid uint32, groups map[string]uint32) string {
	var names []string
	for name, id := range groups {
		if id != gid {
			continue
		}
		if name == "root" || name == serviceAccount {
			return ""
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// reasonOf is the message of err with its cause.
func reasonOf(err error) string {
	var f *cli.Failure
	if errors.As(err, &f) && f.Cause != nil {
		return f.Message + ": " + f.Cause.Error()
	}
	return err.Error()
}
