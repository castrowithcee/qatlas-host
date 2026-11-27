package gitrepo

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// rootOnlyGit takes write for group and others, and setgid on folders, from .git and everything in it, in the
// service of the system engine. Git creates its files with the umask of the service, which this does not trust:
// a .git that others can write is one root must not run git in. Links are neither followed nor changed.
//
// shortcut: .git is looked at once before the walk, so someone who can replace .git in the folder in that moment
// makes it clear write bits elsewhere, nothing else; upgrade to a descriptor based walk if more is ever done here.
func (r *Repo) rootOnlyGit() error {
	if !enforceGitSafe() {
		return nil
	}
	gitDir := filepath.Join(r.dir, ".git")
	if fi, err := os.Lstat(gitDir); err != nil || !fi.IsDir() {
		return fmt.Errorf("make .git root-only: %s is not a folder", gitDir)
	}
	return filepath.WalkDir(gitDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("make .git root-only: %w", err)
		}
		info, err := d.Info()
		if err != nil || info.Mode()&fs.ModeSymlink != 0 {
			return err
		}
		if info.Mode().Perm()&0o022 == 0 && info.Mode()&fs.ModeSetgid == 0 {
			return nil
		}
		err = unix.Fchmodat(unix.AT_FDCWD, path, uint32(info.Mode().Perm()&^0o022), unix.AT_SYMLINK_NOFOLLOW)
		if err != nil && !errors.Is(err, unix.ENOENT) {
			return fmt.Errorf("make .git root-only: %w", err)
		}
		return nil
	})
}
