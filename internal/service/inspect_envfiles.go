package service

import (
	"context"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"syscall"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/stacks"
)

// noteUnreadableEnv records in acc the unversioned env files of the stacks that this service cannot read: the
// secrets.env of each stack and its files named by env_file. A file a Compose secret or a writable mount names,
// a link, a file with further hard links, and anything in a bind-mount source is left out, as is a name that
// cannot be shown as it is. Only the mode bits decide, never the content.
func noteUnreadableEnv(ctx context.Context, acc *control.SourceAccess, root string, v verdict) {
	if acc == nil || v.view == nil || v.loadErr != nil {
		return
	}
	found := map[string]bool{}
	for _, st := range v.list {
		tracked, err := v.view.TrackedFiles(ctx, st.Folder)
		if err != nil {
			return
		}
		var mounts []string
		for _, bm := range st.BindMounts {
			mounts = append(mounts, path.Join(st.Folder, bm))
		}
		for _, rel := range append([]string{path.Join(st.Folder, stacks.SecretsFile)}, st.Refs.EnvFiles...) {
			switch {
			case slices.Contains(tracked, rel), slices.Contains(st.Refs.Secrets, rel), slices.Contains(st.Refs.Writable, rel),
				slices.ContainsFunc(mounts, func(m string) bool { return underPath(m, rel) }):
			case unreadablePlainFile(root, rel):
				found[rel] = true
			}
		}
	}
	for rel := range found {
		acc.UnreadableEnvFiles = append(acc.UnreadableEnvFiles, rel)
	}
	sort.Strings(acc.UnreadableEnvFiles)
}

// unreadablePlainFile reports whether rel, a clean path below root, is a regular file with no further hard link
// that no symbolic link leads to and that this process cannot read.
func unreadablePlainFile(root, rel string) bool {
	if rel != maskControl(rel) || !filepath.IsLocal(rel) {
		return false
	}
	cur := root
	parts := strings.Split(rel, "/")
	for i, part := range parts {
		cur = filepath.Join(cur, part)
		info, err := os.Lstat(cur)
		if err != nil || info.Mode()&fs.ModeSymlink != 0 {
			return false
		}
		if i < len(parts)-1 {
			if !info.IsDir() {
				return false
			}
			continue
		}
		st, ok := info.Sys().(*syscall.Stat_t)
		return ok && info.Mode().IsRegular() && st.Nlink == 1 && syscall.Access(cur, accessRead) != nil
	}
	return false
}

// mapAll applies f to every element; an empty list stays nil.
func mapAll(v []string, f func(string) string) []string {
	var out []string
	for _, e := range v {
		out = append(out, f(e))
	}
	return out
}
