package nofollow

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// fixture is a stack folder "root/stack" and an outside folder holding one file the tests must never change.
type fixture struct {
	root, outside, secret string
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	base := t.TempDir()
	f := fixture{root: filepath.Join(base, "root"), outside: filepath.Join(base, "outside")}
	for _, d := range []string{filepath.Join(f.root, "stack"), f.outside} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	f.secret = filepath.Join(f.outside, "target")
	if err := os.WriteFile(f.secret, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f fixture) link(t *testing.T, to, at string) {
	t.Helper()
	if err := os.Symlink(to, at); err != nil {
		t.Fatal(err)
	}
}

// untouched fails when the outside folder holds more than the original file or the file changed.
func (f fixture) untouched(t *testing.T) {
	t.Helper()
	entries, err := os.ReadDir(f.outside)
	if err != nil || len(entries) != 1 {
		t.Fatalf("outside folder changed: %v %v", entries, err)
	}
	if b, _ := os.ReadFile(f.secret); string(b) != "outside" {
		t.Fatalf("outside file changed: %q", b)
	}
}

func wantLink(t *testing.T, err error, path string) {
	t.Helper()
	if !errors.Is(err, ErrLink) {
		t.Fatalf("want ErrLink, got %v", err)
	}
	if !strings.Contains(err.Error(), path) {
		t.Fatalf("error %q does not name %s", err, path)
	}
}

func TestNormalUse(t *testing.T) {
	f := newFixture(t)
	d, err := Open(f.root, "stack")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	w, err := d.Create("a.txt", 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.WriteString("one")
	_ = w.Close()
	if _, err := d.Create("a.txt", 0o644); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("second create: %v", err)
	}
	tmp, err := d.CreateTemp(".a.txt.*", 0o600)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = tmp.WriteString("two")
	_ = tmp.Close()
	if err := d.Rename(filepath.Base(tmp.Name()), "a.txt"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(f.root, "stack", "a.txt")); string(b) != "two" {
		t.Fatalf("content %q", b)
	}
	info, err := d.Lstat("a.txt")
	if err != nil || !info.Mode.IsRegular() || info.Mode.Perm() != 0o600 {
		t.Fatalf("lstat %v %v", info, err)
	}
}

func TestFoldersAndRemoval(t *testing.T) {
	f := newFixture(t)
	d, err := Open(f.root, "stack")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	a, _ := d.Create("a.txt", 0o644)
	_ = a.Close()
	sub, err := d.MkdirSub("info", 0o755)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.MkdirSub("info", 0o755); err != nil {
		t.Fatalf("existing folder: %v", err)
	}
	g, _ := sub.Create("x", 0o644)
	_ = g.Close()
	names, _ := d.Names()
	slices.Sort(names)
	if !slices.Equal(names, []string{"a.txt", "info"}) {
		t.Fatalf("names %v", names)
	}
	_ = sub.Close()
	if err := d.Sync(); err != nil {
		t.Fatal(err)
	}
	if err := d.Remove("a.txt"); err != nil {
		t.Fatal(err)
	}
	if err := d.RemoveAll("info"); err != nil {
		t.Fatal(err)
	}
	if err := d.RemoveAll("info"); err != nil {
		t.Fatalf("missing name: %v", err)
	}
	if names, _ := d.Names(); len(names) != 0 {
		t.Fatalf("left %v", names)
	}
}

func TestLinkAsRoot(t *testing.T) {
	f := newFixture(t)
	root := filepath.Join(filepath.Dir(f.root), "rootlink")
	f.link(t, f.outside, root)
	_, err := Open(root, "")
	wantLink(t, err, root)
	_, err = Open(root, "stack")
	wantLink(t, err, root)
	f.untouched(t)
}

func TestLinkAsParentOnTheWay(t *testing.T) {
	f := newFixture(t)
	f.link(t, f.outside, filepath.Join(f.root, "stack", "up"))
	_, err := Open(f.root, "stack/up/deeper")
	wantLink(t, err, filepath.Join(f.root, "stack", "up"))
	f.link(t, f.outside, filepath.Join(f.root, "linked"))
	_, err = Open(f.root, "linked")
	wantLink(t, err, filepath.Join(f.root, "linked"))
	f.untouched(t)
}

func TestLinkAsFolderToCreateInto(t *testing.T) {
	f := newFixture(t)
	d, _ := Open(f.root, "stack")
	defer d.Close()
	f.link(t, f.outside, filepath.Join(f.root, "stack", "info"))
	_, err := d.MkdirSub("info", 0o755)
	wantLink(t, err, filepath.Join(f.root, "stack", "info"))
	f.link(t, filepath.Join(f.outside, "missing"), filepath.Join(f.root, "stack", "dangling"))
	_, err = d.MkdirSub("dangling", 0o755)
	wantLink(t, err, filepath.Join(f.root, "stack", "dangling"))
	f.untouched(t)
}

func TestLinkAsTargetFile(t *testing.T) {
	f := newFixture(t)
	d, _ := Open(f.root, "stack")
	defer d.Close()
	at := filepath.Join(f.root, "stack", "f")
	f.link(t, f.secret, at)

	_, err := d.Create("f", 0o644)
	wantLink(t, err, at)
	_, err = d.OpenFile("f", os.O_RDWR, 0)
	wantLink(t, err, at)

	tmp, _ := d.CreateTemp(".f.*", 0o600)
	_, _ = tmp.WriteString("replacement")
	_ = tmp.Close()
	wantLink(t, d.Rename(filepath.Base(tmp.Name()), "f"), at)
	if info, _ := d.Lstat("f"); info.Mode&fs.ModeSymlink == 0 {
		t.Fatal("the link was replaced")
	}
	f.untouched(t)
}

func TestRemoveAllDoesNotEnterLinks(t *testing.T) {
	f := newFixture(t)
	d, _ := Open(f.root, "stack")
	defer d.Close()
	if err := os.MkdirAll(filepath.Join(f.root, "stack", "tree", "in"), 0o755); err != nil {
		t.Fatal(err)
	}
	f.link(t, f.outside, filepath.Join(f.root, "stack", "tree", "in", "out"))
	if err := d.RemoveAll("tree"); err != nil {
		t.Fatal(err)
	}
	f.link(t, f.outside, filepath.Join(f.root, "stack", "direct"))
	if err := d.RemoveAll("direct"); err != nil {
		t.Fatal(err)
	}
	if names, _ := d.Names(); len(names) != 0 {
		t.Fatalf("left %v", names)
	}
	f.untouched(t)
}

func TestBadPathsAndNames(t *testing.T) {
	f := newFixture(t)
	if _, err := Open("relative", ""); !errors.Is(err, ErrBadPath) {
		t.Fatalf("relative root: %v", err)
	}
	if _, err := Open(f.root+"/../root", ""); !errors.Is(err, ErrBadPath) {
		t.Fatalf("unclean root: %v", err)
	}
	if _, err := Open(f.root, "stack/../.."); !errors.Is(err, ErrBadPath) {
		t.Fatalf("dotdot: %v", err)
	}
	d, _ := Open(f.root, "")
	defer d.Close()
	for _, name := range []string{"", ".", "..", "a/b"} {
		if _, err := d.Create(name, 0o644); !errors.Is(err, ErrBadPath) {
			t.Fatalf("name %q: %v", name, err)
		}
	}
	if _, err := Open(f.root, "missing"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing: %v", err)
	}
}

func TestErrorMasksControlCharacters(t *testing.T) {
	err := &Error{"open", "/a/b\x1b[31m\nc", ErrLink}
	if strings.ContainsAny(err.Error(), "\x1b\n") {
		t.Fatalf("not masked: %q", err.Error())
	}
}
