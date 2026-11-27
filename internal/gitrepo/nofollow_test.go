package gitrepo

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/nofollow"
)

// outsideFolder holds what a write through a link would reach.
type outsideFolder struct{ dir string }

func newOutside(t *testing.T, files map[string]string) outsideFolder {
	t.Helper()
	o := outsideFolder{filepath.Join(t.TempDir(), "outside")}
	writeTree(t, o.dir, files)
	return o
}

func (o outsideFolder) snapshot(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(o.dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			b, rerr := os.ReadFile(p)
			out[p], err = string(b), rerr
		} else if err == nil {
			out[p] = "<dir>"
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func (o outsideFolder) unchanged(t *testing.T, before map[string]string) {
	t.Helper()
	after := o.snapshot(t)
	if len(after) != len(before) {
		t.Fatalf("outside entries changed: %v -> %v", before, after)
	}
	for p, b := range before {
		if after[p] != b {
			t.Fatalf("%s changed: %q -> %q", p, b, after[p])
		}
	}
}

func wantNamed(t *testing.T, err error, path string) {
	t.Helper()
	if err == nil {
		t.Fatal("the write followed a link")
	}
	if !errors.Is(err, nofollow.ErrLink) {
		t.Fatalf("want a link refusal, got %v", err)
	}
	if !strings.Contains(err.Error(), path) {
		t.Fatalf("error %q does not name %s", err, path)
	}
}

func link(t *testing.T, to, at string) {
	t.Helper()
	if err := os.Symlink(to, at); err != nil {
		t.Fatal(err)
	}
}

// The .gitignore of a folder is created, changed and restored without following a link on the folder or on the
// file. The folder is the root of the walk, so no level lies between it and the file.
func TestIgnoreWritesFollowNoLink(t *testing.T) {
	const old = "node_modules\n"
	plan := func(exists bool) IgnorePlan {
		p := IgnorePlan{Write: true, Exists: exists, content: []byte(old + "/new\n")}
		if exists {
			p.old = []byte(old)
		}
		return p
	}
	for _, exists := range []bool{false, true} {
		for _, c := range []struct {
			name  string
			setup func(t *testing.T, o outsideFolder) (dir, named string)
		}{
			{"folder is a link", func(t *testing.T, o outsideFolder) (string, string) {
				l := filepath.Join(t.TempDir(), "folder")
				link(t, o.dir, l)
				return l, l
			}},
			{".gitignore is a link", func(t *testing.T, o outsideFolder) (string, string) {
				dir := t.TempDir()
				link(t, filepath.Join(o.dir, "target"), filepath.Join(dir, IgnoreName))
				return dir, filepath.Join(dir, IgnoreName)
			}},
		} {
			for _, op := range []string{"apply", "revert"} {
				t.Run(c.name+"/"+op+map[bool]string{false: "/new", true: "/existing"}[exists], func(t *testing.T) {
					o := newOutside(t, map[string]string{"target": old, IgnoreName: old})
					before := o.snapshot(t)
					dir, named := c.setup(t, o)
					p := plan(exists)
					var err error
					if op == "apply" {
						err = p.Apply(dir)
					} else {
						err = p.Revert(dir)
					}
					if op == "revert" && !exists && c.name == ".gitignore is a link" {
						// Removing the entry unlinks the link itself and never reaches its target.
						if err != nil {
							t.Fatal(err)
						}
					} else {
						wantNamed(t, err, named)
					}
					o.unchanged(t, before)
				})
			}
		}
	}
}

// .git/info/attributes is written below .git of the folder without following a link on any level.
func TestPutAttributesFollowsNoLink(t *testing.T) {
	for _, c := range []struct {
		name  string
		setup func(t *testing.T, o outsideFolder) (dir, named string)
	}{
		{"folder is a link", func(t *testing.T, o outsideFolder) (string, string) {
			l := filepath.Join(t.TempDir(), "folder")
			link(t, o.dir, l)
			return l, l
		}},
		{".git is a link", func(t *testing.T, o outsideFolder) (string, string) {
			dir := t.TempDir()
			link(t, o.dir, filepath.Join(dir, ".git"))
			return dir, filepath.Join(dir, ".git")
		}},
		{".git/info is a link", func(t *testing.T, o outsideFolder) (string, string) {
			dir := t.TempDir()
			if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
				t.Fatal(err)
			}
			link(t, o.dir, filepath.Join(dir, ".git", "info"))
			return dir, filepath.Join(dir, ".git", "info")
		}},
		{"attributes is a link", func(t *testing.T, o outsideFolder) (string, string) {
			dir := t.TempDir()
			if err := os.MkdirAll(filepath.Join(dir, ".git", "info"), 0o755); err != nil {
				t.Fatal(err)
			}
			at := filepath.Join(dir, ".git", "info", "attributes")
			link(t, filepath.Join(o.dir, "target"), at)
			return dir, at
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			o := newOutside(t, map[string]string{"target": "outside\n"})
			before := o.snapshot(t)
			dir, named := c.setup(t, o)
			info, err := putAttributes(dir)
			if err == nil {
				info.Close()
			}
			wantNamed(t, err, named)
			o.unchanged(t, before)
		})
	}
}

// A failed start removes its .git without entering a link; a link as the folder or as .git leaves the outside
// as it is. A fresh .git is no link and has no level between it and the folder, so those cases cannot arise
// from a start that passed claimGit.
func TestRemoveGitFollowsNoLink(t *testing.T) {
	o := newOutside(t, map[string]string{"target": "outside\n", "sub/file": "x"})
	before := o.snapshot(t)

	l := filepath.Join(t.TempDir(), "folder")
	link(t, o.dir, l)
	r := &Repo{dir: l}
	wantNamed(t, r.removeGit(), l)
	o.unchanged(t, before)

	dir := t.TempDir()
	link(t, o.dir, filepath.Join(dir, ".git"))
	if err := (&Repo{dir: dir}).removeGit(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(dir, ".git")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf(".git link stays: %v", err)
	}
	o.unchanged(t, before)

	dir = t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git", "objects"), 0o755); err != nil {
		t.Fatal(err)
	}
	link(t, o.dir, filepath.Join(dir, ".git", "objects", "alt"))
	if err := (&Repo{dir: dir}).removeGit(); err != nil {
		t.Fatal(err)
	}
	o.unchanged(t, before)
}

// A start in a folder that is a link stops before git writes anything.
func TestInitRefusesFolderLink(t *testing.T) {
	o := newOutside(t, map[string]string{"a.txt": "a\n"})
	before := o.snapshot(t)
	l := filepath.Join(t.TempDir(), "folder")
	link(t, o.dir, l)
	_, err := Init(context.Background(), Config{Local: true}, l, []string{"a.txt"}, nil)
	wantNamed(t, err, l)
	o.unchanged(t, before)
}
