package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// gitOwner is the owner and group .git must have; only a test changes it, because root cannot be made one.
var gitOwner = struct{ uid, gid int }{0, 0}

// gitSecure is what makes the .git of a stack folder of the system engine one that only root can change: every
// entry owned by root:root, no write for group and others, no setgid on folders, no core.sharedRepository. The
// deploy service of that engine runs as root and trusts no other .git. The work tree is not part of it.
type gitSecure struct {
	root string // the stack folder
	// entries are the paths in .git that change, folders before what they hold.
	entries []string
	// shared is the value of core.sharedRepository to remove, empty when it is not set.
	shared string
	// gids are the groups of the entries that change, and othersWritable is the first of them that others can
	// write, empty when there is none. They tell what the releases before the root service left (oldStackGits).
	gids           map[uint32]bool
	othersWritable string
}

func (s *gitSecure) count() int {
	if s == nil {
		return 0
	}
	n := len(s.entries)
	if s.shared != "" {
		n++
	}
	return n
}

func (s *gitSecure) summary() []string {
	lines := []string{fmt.Sprintf("%d entries of .git change so that only root can change it: owner and group root, no write for group and others, no setgid on folders; the work tree stays as it is", len(s.entries))}
	if len(s.entries) > 0 {
		names := make([]string, len(s.entries))
		for i, e := range s.entries {
			rel, _ := filepath.Rel(s.root, e)
			names[i] = fmt.Sprintf("%q", rel)
		}
		lines = append(lines, "  "+limitedList(names))
	}
	if s.shared != "" {
		lines = append(lines, "  git config core.sharedRepository "+fmt.Sprintf("%q", s.shared)+" is removed")
	}
	return lines
}

// planSystemGit plans the change of an existing .git of a folder that the system engine takes over: nil when
// nothing needs to change. A clone and a new repository get a root-only .git from the service itself.
func planSystemGit(res control.SourceInspectResult) (*permPlan, error) {
	if res.Path == "" || (res.Case != control.SourceCaseAdopt && res.Case != control.SourceCaseLocal) {
		return nil, nil
	}
	s, err := planGitSecure(context.Background(), res.Path)
	if err != nil || s.count() == 0 {
		return nil, err
	}
	return &permPlan{dir: res.Path, secure: s}, nil
}

// planGitSecure reads .git of dir without following a link and computes the changes; it changes nothing. A
// folder without .git has none. What cannot be repaired is refused with the reason.
func planGitSecure(ctx context.Context, dir string) (*gitSecure, error) {
	s := &gitSecure{root: dir, gids: map[uint32]bool{}}
	gitDir := filepath.Join(dir, ".git")
	if _, err := os.Lstat(gitDir); errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err := requireRealDir(gitDir); err != nil {
		return nil, err
	}
	err := scanGit(gitDir, func(path string, mode fs.FileMode, gid uint32) {
		s.entries = append(s.entries, path)
		s.gids[gid] = true
		if mode.Perm()&0o002 != 0 && s.othersWritable == "" {
			s.othersWritable = path
		}
	})
	if err != nil {
		return nil, err
	}
	out, err := runGit(ctx, dir, "config", "--local", "--get", "core.sharedRepository")
	var ee *exec.ExitError
	switch {
	case errors.As(err, &ee) && ee.ExitCode() == 1:
	case err != nil:
		return nil, folderFailure("cannot read the git configuration of "+dir, err)
	default:
		s.shared = strings.TrimSpace(string(out))
	}
	return s, nil
}

// scanGit calls found for every entry of .git that is not root-only yet, a folder before what it holds. A link, a
// special file or a file with further hard links, which may lie outside, cannot be repaired and is refused.
func scanGit(gitDir string, found func(path string, mode fs.FileMode, gid uint32)) error {
	refuse := func(path, why string) error {
		return usageFailure(fmt.Sprintf("%q %s, so .git cannot be made root-only; nothing was changed", path, why),
			"Look at the entry, remove or replace it yourself, then run the command again.")
	}
	return filepath.WalkDir(gitDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return folderFailure("cannot read "+gitDir, err)
		}
		info, err := d.Info()
		if err != nil {
			return folderFailure("cannot read "+path, err)
		}
		st, _ := info.Sys().(*syscall.Stat_t)
		switch {
		case info.Mode()&fs.ModeSymlink != 0:
			return refuse(path, "is a symbolic link")
		case !info.IsDir() && !info.Mode().IsRegular():
			return refuse(path, "is neither a file nor a folder")
		case !info.IsDir() && !singleLink(info):
			return refuse(path, "has further hard links, which may lie outside the folder")
		case st == nil:
			return folderFailure("cannot read the owner of "+path, nil)
		}
		if int(st.Uid) != gitOwner.uid || int(st.Gid) != gitOwner.gid || info.Mode().Perm()&0o022 != 0 || info.Mode()&fs.ModeSetgid != 0 {
			found(path, info.Mode(), st.Gid)
		}
		return nil
	})
}

// secureGit applies the plan: every entry gets root and the mode without group and others write, a folder before
// what it holds, so nobody else can swap an entry below it any more. Only then core.sharedRepository is removed,
// so that git runs as root in a root-only .git; git keeps the mode of the configuration it rewrites. An entry that
// became a link, or lies behind one, since the plan is skipped; the scan afterwards finds it.
func (h folderHost) secureGit(ctx context.Context, s *gitSecure) error {
	for _, path := range s.entries {
		info, err := os.Lstat(path)
		if err != nil {
			continue
		}
		isDir := info.IsDir()
		if _, ok := plainBelow(s.root, path, isDir); !ok {
			continue
		}
		if err := h.lchown(path, gitOwner.uid, gitOwner.gid); err != nil {
			return folderFailure("cannot change the owner of "+path, err)
		}
		// Anything may have changed since the check; chmod follows links, so look again.
		if again, ok := plainBelow(s.root, path, isDir); !ok || !os.SameFile(info, again) {
			continue
		}
		keep := info.Mode() & (fs.ModePerm | fs.ModeSetuid | fs.ModeSticky)
		if err := h.chmod(path, keep&^0o022); err != nil {
			return folderFailure("cannot change the mode of "+path, err)
		}
	}
	if s.shared != "" {
		if _, err := runGit(ctx, s.root, "config", "--local", "--unset-all", "core.sharedRepository"); err != nil {
			return folderFailure("cannot remove core.sharedRepository in "+s.root, err)
		}
	}
	var left []string
	if err := scanGit(filepath.Join(s.root, ".git"), func(path string, _ fs.FileMode, _ uint32) { left = append(left, path) }); err != nil {
		return err
	}
	if len(left) > 0 {
		return folderFailure(fmt.Sprintf("%d entries of .git are still not root-only, the first is %q", len(left), left[0]), nil)
	}
	return nil
}
