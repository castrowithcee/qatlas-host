package cli

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"syscall"
)

// setPermissions gives the service group of a rootless engine the folder and the planned files, as source add
// does for a folder of files, and remembers group and mode of every path first; the system engine needs none.
func (m *moveRun) setPermissions() error {
	if targetOf(m.ctx).inst == nil {
		return nil
	}
	_, gid, err := folders.owner(targetOf(m.ctx).account())
	if err != nil {
		return fmt.Errorf("cannot find the service account %s: %w", targetOf(m.ctx).account(), err)
	}
	plan, err := planListedFiles(m.target, gid, m.res.PlannedFiles, m.res.BindMounts)
	if err != nil {
		return err
	}
	for _, set := range []struct {
		paths []string
		dir   bool
	}{{plan.dirs, true}, {plan.files, false}} {
		for _, p := range set.paths {
			info, ok := plainBelow(m.target, p, set.dir)
			if !ok {
				continue
			}
			rel, _ := filepath.Rel(m.target, p)
			m.perms = append(m.perms, pathMode{rel: rel, dir: set.dir, gid: int(info.Sys().(*syscall.Stat_t).Gid),
				mode: info.Mode() & (fs.ModePerm | fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky)})
		}
	}
	return folders.applyPermissions(m.ctx, plan)
}

// restorePermissions puts back group and mode in the old folder; a path that is gone or became a link is left.
func (m *moveRun) restorePermissions() error {
	for _, p := range m.perms {
		path := filepath.Join(m.path, p.rel)
		if _, ok := plainBelow(m.path, path, p.dir); !ok {
			continue
		}
		if err := folders.lchown(path, -1, p.gid); err != nil {
			return fmt.Errorf("cannot set the group of %s back: %w", path, err)
		}
		if _, ok := plainBelow(m.path, path, p.dir); !ok {
			continue
		}
		if err := folders.chmod(path, p.mode); err != nil {
			return fmt.Errorf("cannot set the mode of %s back: %w", path, err)
		}
	}
	return nil
}
