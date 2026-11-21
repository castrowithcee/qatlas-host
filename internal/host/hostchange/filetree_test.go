package hostchange

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func (f *fileFixture) change(t *testing.T, id string, args ...string) Result {
	t.Helper()
	res, err := f.runner.Apply(t.Context(), id, args, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Entry.Result != ResultApplied || res.Entry.Check != CheckPassed {
		t.Fatalf("%s %q: %+v", id, args, res.Entry)
	}
	return res
}

func (f *fileFixture) undo(t *testing.T, id string) Result {
	t.Helper()
	res, err := f.runner.Undo(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func wantGone(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("%s must not exist: %v", path, err)
	}
}

func wantLink(t *testing.T, path, target string) {
	t.Helper()
	got, err := os.Readlink(path)
	if err != nil || got != target {
		t.Fatalf("%s: link to %q (%v), want a link to %q", path, got, err, target)
	}
}

func modeOf(t *testing.T, path string) uint32 { t.Helper(); return statOf(t, path).Mode & 0o7777 }

// names lists a folder, to see that no hidden leftover of a step stays behind.
func names(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return strings.Join(out, " ")
}

func wantArgError(t *testing.T, what string, err error) {
	t.Helper()
	var arg *ArgumentError
	if !errors.As(err, &arg) {
		t.Errorf("%s: want an argument error, got %v", what, err)
	}
}

func TestFileRenameAndUndo(t *testing.T) {
	f := newFileFixture(t)
	p := filepath.Join(f.root, "app.conf")
	mkfile(t, p, "a=1\n", 0o640)
	ino := statOf(t, p).Ino
	q := filepath.Join(f.root, "app.conf.old")
	res := f.change(t, ActionFileRename, p, q)
	if !res.Entry.Reversible || strings.Join(res.Entry.Commands, ";") != "[builtin] file-rename "+p+" "+q {
		t.Fatalf("%+v", res.Entry)
	}
	wantGone(t, p)
	if content(t, q) != "a=1\n" || statOf(t, q).Ino != ino {
		t.Fatal("the renamed file must keep its content and inode")
	}
	undo := f.undo(t, res.Entry.ID)
	if undo.Entry.Result != ResultApplied || strings.Join(undo.Entry.Commands, ";") != "[builtin] file-rename "+q+" "+p+" < <recorded state>" {
		t.Fatalf("undo %+v", undo.Entry)
	}
	wantGone(t, q)
	if statOf(t, p).Ino != ino {
		t.Fatal("the undo must bring the same file back")
	}

	data := filepath.Join(f.root, "data")
	mkdir(t, data, 0o750)
	mkfile(t, filepath.Join(data, "x"), "x\n", 0o644)
	archive := filepath.Join(f.root, "archive")
	mkdir(t, archive, 0o755)
	f.change(t, ActionFileRename, data, filepath.Join(archive, "data"))
	if content(t, filepath.Join(archive, "data", "x")) != "x\n" {
		t.Fatal("the folder must move with its content")
	}

	link := filepath.Join(f.root, "current")
	if err := os.Symlink("demo-missing-target", link); err != nil {
		t.Fatal(err)
	}
	f.change(t, ActionFileRename, link, filepath.Join(f.root, "previous"))
	wantLink(t, filepath.Join(f.root, "previous"), "demo-missing-target")

	res = f.change(t, ActionFileRename, p, q)
	mkfile(t, p, "new\n", 0o644)
	undo = f.undo(t, res.Entry.ID)
	if undo.Entry.Result != ResultFailed || !strings.Contains(undo.Entry.Error, "exists already") || content(t, q) != "a=1\n" {
		t.Fatalf("an undo onto an occupied source: %+v", undo.Entry)
	}
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(q); err != nil {
		t.Fatal(err)
	}
	mkfile(t, q, "other\n", 0o644)
	undo = f.undo(t, res.Entry.ID)
	if undo.Entry.Result != ResultFailed || !strings.Contains(undo.Entry.Error, "no longer the entry") || content(t, q) != "other\n" {
		t.Fatalf("an undo of a replaced target: %+v", undo.Entry)
	}
	wantGone(t, p)
}

// otherFileSystem is a fresh folder on another file system than dir, or skips.
func otherFileSystem(t *testing.T, dir string) string {
	t.Helper()
	other, err := os.MkdirTemp("/dev/shm", "qhost-test-")
	if err != nil {
		t.Skipf("no second file system: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(other) })
	if other, err = filepath.EvalSymlinks(other); err != nil {
		t.Fatal(err)
	}
	if statOf(t, other).Dev == statOf(t, dir).Dev {
		t.Skip("no second file system")
	}
	return other
}

func TestFileRenameRefusals(t *testing.T) {
	f := newFileFixture(t)
	p := filepath.Join(f.root, "a.conf")
	mkfile(t, p, "a\n", 0o644)
	q := filepath.Join(f.root, "b.conf")
	mkfile(t, q, "b\n", 0o644)
	_, err := f.runner.Apply(t.Context(), ActionFileRename, []string{p, q}, nil)
	wantRefusal(t, "existing target", err, "exists already")
	if content(t, p) != "a\n" || content(t, q) != "b\n" {
		t.Fatal("an existing target must stay")
	}
	if err := runFileStep(Command{Argv: []string{stepFileRename, p, q}, Builtin: true}, f.ops()); err == nil || content(t, q) != "b\n" {
		t.Fatalf("the step must not replace on its own: %v", err)
	}
	_, err = f.runner.Apply(t.Context(), ActionFileRename, []string{filepath.Join(f.root, "missing"), filepath.Join(f.root, "c")}, nil)
	wantRefusal(t, "missing source", err, "does not exist")
	_, err = f.runner.Apply(t.Context(), ActionFileRename, []string{p, filepath.Join(f.root, "missing", "a.conf")}, nil)
	wantRefusal(t, "missing target folder", err, "does not exist")
	_, err = f.runner.Registry.Describe(ActionFileRename, []string{f.root, filepath.Join(f.root, "inner")}, nil)
	wantArgError(t, "target inside the source", err)

	t.Run("other file system", func(t *testing.T) {
		other := otherFileSystem(t, f.root)
		target := filepath.Join(other, "a.conf")
		_, err := f.runner.Apply(t.Context(), ActionFileRename, []string{p, target}, nil)
		wantRefusal(t, "other file system", err, "different file systems", "copy it instead")
		err = runFileStep(Command{Argv: []string{stepFileRename, p, target}, Builtin: true}, f.ops())
		if err == nil || !strings.Contains(err.Error(), "different file systems") {
			t.Errorf("the step must not copy across file systems: %v", err)
		}
		wantGone(t, target)
		if content(t, p) != "a\n" {
			t.Error("the source must stay")
		}
	})
}

func TestFileCopyFile(t *testing.T) {
	f := newFileFixture(t)
	src := filepath.Join(f.root, "tool")
	mkfile(t, src, "#!/bin/sh\necho demo\n", 0o755)
	// os.Chmod takes setuid as os.ModeSetuid; the plain bits go through unix.
	if err := unix.Chmod(src, 0o4755); err != nil || modeOf(t, src) != 0o4755 {
		t.Fatalf("set setuid: %v", err)
	}
	shared := filepath.Join(f.root, "shared")
	mkdir(t, shared, 0o2770)
	dst := filepath.Join(shared, "tool")
	res := f.change(t, ActionFileCopy, src, dst)
	if !res.Entry.Reversible || strings.Join(res.Entry.Commands, ";") != "[builtin] file-copy "+src+" "+dst {
		t.Fatalf("%+v", res.Entry)
	}
	st, dir := statOf(t, dst), statOf(t, shared)
	if content(t, dst) != content(t, src) || st.Mode&0o7777 != 0o755 || st.Ino == statOf(t, src).Ino {
		t.Errorf("copy %q mode %04o: want the content and the mode without setuid", content(t, dst), st.Mode&0o7777)
	}
	if want := fmt.Sprintf("%d %d:%d", st.Ino, dir.Uid, dir.Gid); strings.Join(f.chowns, ";") != want {
		t.Errorf("owner changes %q, want %q", f.chowns, want)
	}
	if names(t, shared) != "tool" {
		t.Errorf("the target folder holds %q", names(t, shared))
	}

	undo := f.undo(t, res.Entry.ID)
	if undo.Entry.Result != ResultApplied || strings.Join(undo.Entry.Commands, ";") != "[builtin] file-remove-copy "+dst+" < <recorded state>" {
		t.Fatalf("undo %+v", undo.Entry)
	}
	wantGone(t, dst)
	if modeOf(t, src) != 0o4755 {
		t.Error("the source must stay")
	}

	res = f.change(t, ActionFileCopy, src, dst)
	mkfile(t, dst, "edited later\n", 0o755)
	undo = f.undo(t, res.Entry.ID)
	if undo.Entry.Result != ResultFailed || !strings.Contains(undo.Entry.Error, "changed after the copy") || content(t, dst) != "edited later\n" {
		t.Fatalf("an undo of a changed copy: %+v", undo.Entry)
	}
	_, err := f.runner.Apply(t.Context(), ActionFileCopy, []string{src, dst}, nil)
	wantRefusal(t, "existing target", err, "exists already")
}

func TestFileCopyFolderKeepsLinksAsLinks(t *testing.T) {
	f := newFileFixture(t)
	outside, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	shadow := filepath.Join(outside, "shadow")
	mkfile(t, shadow, "root:demo-hash:1:0:99999:7:::\n", 0o600)
	src := filepath.Join(f.root, "app")
	mkdir(t, src, 0o750)
	mkdir(t, filepath.Join(src, "conf"), 0o700)
	mkfile(t, filepath.Join(src, "conf", "app.env"), "DEMO=1\n", 0o640)
	if err := os.Symlink(shadow, filepath.Join(src, "shadow-link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("conf/app.env", filepath.Join(src, "rel")); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(f.root, "backup")
	mkdir(t, backup, 0o755)
	dst := filepath.Join(backup, "app")
	res := f.change(t, ActionFileCopy, src, dst)

	wantLink(t, filepath.Join(dst, "shadow-link"), shadow)
	wantLink(t, filepath.Join(dst, "rel"), "conf/app.env")
	if content(t, filepath.Join(dst, "conf", "app.env")) != "DEMO=1\n" {
		t.Error("the file must be copied")
	}
	for p, want := range map[string]uint32{dst: 0o750, filepath.Join(dst, "conf"): 0o700, filepath.Join(dst, "conf", "app.env"): 0o640} {
		if m := modeOf(t, p); m != want {
			t.Errorf("%s has %04o, want %04o", p, m, want)
		}
	}
	owner := statOf(t, backup)
	if len(f.chowns) != 5 {
		t.Errorf("owner changes %q, want one for each of the five entries", f.chowns)
	}
	for _, c := range f.chowns {
		if !strings.HasSuffix(c, fmt.Sprintf(" %d:%d", owner.Uid, owner.Gid)) {
			t.Errorf("owner change %q is not to the owner of the target folder", c)
		}
	}
	if content(t, shadow) != "root:demo-hash:1:0:99999:7:::\n" || names(t, outside) != "shadow" {
		t.Error("the target of a link was touched")
	}
	if names(t, backup) != "app" {
		t.Errorf("the target folder holds %q", names(t, backup))
	}

	undo := f.undo(t, res.Entry.ID)
	if undo.Entry.Result != ResultApplied || names(t, backup) != "" {
		t.Fatalf("undo %+v, left %q", undo.Entry, names(t, backup))
	}
	if names(t, outside) != "shadow" || content(t, filepath.Join(src, "conf", "app.env")) != "DEMO=1\n" {
		t.Error("the undo touched more than the copy")
	}

	res = f.change(t, ActionFileCopy, src, dst)
	mkfile(t, filepath.Join(dst, "conf", "added.txt"), "added\n", 0o644)
	undo = f.undo(t, res.Entry.ID)
	if undo.Entry.Result != ResultFailed || content(t, filepath.Join(dst, "conf", "added.txt")) != "added\n" {
		t.Fatalf("an undo of a copy with an added file: %+v", undo.Entry)
	}
}

func TestFileCopyRefusesSpecialEntriesAndLeavesNothingBehind(t *testing.T) {
	f := newFileFixture(t)
	src := filepath.Join(f.root, "app")
	mkdir(t, src, 0o755)
	mkfile(t, filepath.Join(src, "a.txt"), "a\n", 0o644)
	pipe := filepath.Join(src, "pipe")
	if err := unix.Mkfifo(pipe, 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(f.root, "target")
	mkdir(t, target, 0o755)
	dst := filepath.Join(target, "app")
	_, err := f.runner.Apply(t.Context(), ActionFileCopy, []string{src, dst}, nil)
	wantRefusal(t, "a FIFO", err, "device, FIFO or socket")
	if err := runFileStep(Command{Argv: []string{stepFileCopy, src, dst}, Builtin: true}, f.ops()); err == nil {
		t.Error("the step must refuse a FIFO on its own")
	}
	if names(t, target) != "" {
		t.Errorf("a refused copy left %q", names(t, target))
	}
	_, err = f.runner.Apply(t.Context(), ActionFileRemove, []string{src}, nil)
	wantRefusal(t, "remove a FIFO", err, "device, FIFO or socket")

	if err := os.Remove(pipe); err != nil {
		t.Fatal(err)
	}
	f.chownErr = errors.New("simulated chown failure")
	res, err := f.runner.Apply(t.Context(), ActionFileCopy, []string{src, dst}, nil)
	if err != nil || res.Entry.Result != ResultFailed {
		t.Fatalf("%v %+v", err, res.Entry)
	}
	if names(t, target) != "" {
		t.Errorf("a failed copy left %q", names(t, target))
	}
}

func TestFileRemoveAndUndo(t *testing.T) {
	f := newFileFixture(t)
	p := filepath.Join(f.root, "app.conf")
	mkfile(t, p, "a=1\n", 0o640)
	when := time.Date(2029, 5, 6, 7, 8, 9, 0, time.UTC)
	if err := os.Chtimes(p, when, when); err != nil {
		t.Fatal(err)
	}
	res := f.change(t, ActionFileRemove, p)
	if !res.Entry.Reversible || strings.Join(res.Entry.Commands, ";") != "[builtin] file-remove "+p {
		t.Fatalf("%+v", res.Entry)
	}
	wantGone(t, p)
	if names(t, f.root) != "" {
		t.Errorf("the removal left %q", names(t, f.root))
	}
	undo := f.undo(t, res.Entry.ID)
	if undo.Entry.Result != ResultApplied || strings.Join(undo.Entry.Commands, ";") != "[builtin] file-restore-tree "+p+" < <previous content>" {
		t.Fatalf("undo %+v", undo.Entry)
	}
	st := statOf(t, p)
	if content(t, p) != "a=1\n" || st.Mode&0o7777 != 0o640 || st.Mtim.Sec != when.Unix() {
		t.Errorf("restored %q mode %04o time %d", content(t, p), st.Mode&0o7777, st.Mtim.Sec)
	}
	if want := fmt.Sprintf("%d %d:%d", st.Ino, st.Uid, st.Gid); strings.Join(f.chowns, ";") != want {
		t.Errorf("owner changes %q, want %q", f.chowns, want)
	}

	data := filepath.Join(f.root, "data")
	mkdir(t, data, 0o750)
	mkdir(t, filepath.Join(data, "sub"), 0o700)
	mkdir(t, filepath.Join(data, "empty"), 0o755)
	mkfile(t, filepath.Join(data, "sub", "db.sqlite"), "demo-db\x00\x01", 0o600)
	if err := os.Symlink("sub/db.sqlite", filepath.Join(data, "current")); err != nil {
		t.Fatal(err)
	}
	f.chowns = nil
	res = f.change(t, ActionFileRemove, data)
	wantGone(t, data)
	undo = f.undo(t, res.Entry.ID)
	if undo.Entry.Result != ResultApplied {
		t.Fatalf("undo %+v", undo.Entry)
	}
	wantLink(t, filepath.Join(data, "current"), "sub/db.sqlite")
	if content(t, filepath.Join(data, "sub", "db.sqlite")) != "demo-db\x00\x01" {
		t.Error("the content must come back")
	}
	for p, want := range map[string]uint32{data: 0o750, filepath.Join(data, "sub"): 0o700, filepath.Join(data, "empty"): 0o755,
		filepath.Join(data, "sub", "db.sqlite"): 0o600} {
		if m := modeOf(t, p); m != want {
			t.Errorf("%s has %04o, want %04o", p, m, want)
		}
	}
	if len(f.chowns) != 5 {
		t.Errorf("owner changes %q, want one for each of the five entries", f.chowns)
	}

	res = f.change(t, ActionFileRemove, data)
	mkdir(t, data, 0o755)
	undo = f.undo(t, res.Entry.ID)
	if undo.Entry.Result != ResultFailed || !strings.Contains(undo.Entry.Error, "not overwritten") || names(t, data) != "" {
		t.Fatalf("an undo onto an occupied path: %+v", undo.Entry)
	}
}

func TestFileRestoreTreeFindsAnUntouchedTree(t *testing.T) {
	f := newFileFixture(t)
	data := filepath.Join(f.root, "data")
	mkdir(t, data, 0o755)
	mkfile(t, filepath.Join(data, "a.txt"), "a\n", 0o644)
	st, err := captureRemove(data, false)
	if err != nil {
		t.Fatal(err)
	}
	inverse, err := inverseRemove(mustJSON(t, st))
	if err != nil {
		t.Fatal(err)
	}
	// The restore after a removal that did not start finds the tree as it was and leaves it.
	if err := runFileStep(inverse.Commands[0], f.ops()); err != nil {
		t.Fatalf("restore over the untouched tree: %v", err)
	}
	mkfile(t, filepath.Join(data, "a.txt"), "changed\n", 0o644)
	if err := runFileStep(inverse.Commands[0], f.ops()); err == nil || content(t, filepath.Join(data, "a.txt")) != "changed\n" {
		t.Fatalf("a changed tree must stay: %v", err)
	}
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestFileTreeUndoLimit(t *testing.T) {
	f := newFileFixture(t)
	big := filepath.Join(f.root, "big")
	mkdir(t, big, 0o755)
	img := filepath.Join(big, "disk.img")
	mkfile(t, img, "", 0o644)
	// A sparse file: the limit counts the size of the files, not the blocks they use.
	if err := os.Truncate(img, MaxUndoBytes); err != nil {
		t.Fatal(err)
	}
	mkfile(t, filepath.Join(big, "note.txt"), "x", 0o644)
	if n, size, err := MeasureTree(big); err != nil || n != 3 || size != MaxUndoBytes+1 {
		t.Fatalf("measured %d entries %d bytes: %v", n, size, err)
	}
	size := formatSize(MaxUndoBytes + 1)
	_, err := f.runner.Apply(t.Context(), ActionFileRemove, []string{big}, nil)
	wantRefusal(t, "remove over the limit", err, "3 entries", size, "permanent")
	_, err = f.runner.Apply(t.Context(), ActionFileCopy, []string{big, filepath.Join(f.root, "copy")}, nil)
	wantRefusal(t, "copy over the limit", err, "3 entries", size, "permanent")
	wantGone(t, filepath.Join(f.root, "copy"))

	for _, args := range [][]string{{ActionFileRemove, big, optionPermanent}, {ActionFileCopy, big, filepath.Join(f.root, "copy"), optionPermanent}} {
		plan, err := f.runner.Registry.Describe(args[0], args[1:], nil)
		if err != nil || plan.NotReversible == "" || !strings.Contains(plan.Sentence, "cannot be undone") {
			t.Errorf("%q: %v %q %q", args, err, plan.NotReversible, plan.Sentence)
		}
	}
	res := f.change(t, ActionFileRemove, big, optionPermanent)
	if res.Entry.Reversible || res.Entry.NotReversible == "" || strings.Join(res.Entry.Commands, ";") != "[builtin] file-remove "+big+" permanent" {
		t.Fatalf("%+v", res.Entry)
	}
	if string(res.Entry.Previous) != `{"path":"`+big+`","permanent":true}` {
		t.Errorf("a permanent removal must keep nothing of the tree: %s", res.Entry.Previous)
	}
	wantGone(t, big)
	if _, err := f.runner.Undo(t.Context(), res.Entry.ID); err == nil || !strings.Contains(err.Error(), "cannot be undone") {
		t.Errorf("undo of a permanent removal: %v", err)
	}

	many := filepath.Join(f.root, "many")
	mkdir(t, many, 0o755)
	for i := range MaxFolderEntries {
		mkfile(t, filepath.Join(many, fmt.Sprintf("f%04d", i)), "", 0o644)
	}
	_, err = f.runner.Apply(t.Context(), ActionFileRemove, []string{many}, nil)
	wantRefusal(t, "too many entries for an undo", err, fmt.Sprintf("%d entries", MaxFolderEntries+1), "permanent")
}

func TestFileTreeActionsRefuseLinksOnTheWay(t *testing.T) {
	f := newFileFixture(t)
	outside, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	victim := filepath.Join(outside, "victim")
	mkdir(t, victim, 0o755)
	mkfile(t, filepath.Join(victim, "keep.txt"), "keep\n", 0o644)
	data := filepath.Join(f.root, "data")
	mkdir(t, data, 0o777)
	if err := os.Symlink(outside, filepath.Join(data, "conf")); err != nil {
		t.Fatal(err)
	}
	real := filepath.Join(f.root, "real.txt")
	mkfile(t, real, "real\n", 0o644)
	middle := filepath.Join(data, "conf", "victim")

	for name, args := range map[string][]string{
		"rename from behind a link": {ActionFileRename, middle, filepath.Join(f.root, "moved")},
		"rename to behind a link":   {ActionFileRename, real, filepath.Join(data, "conf", "planted.txt")},
		"copy from behind a link":   {ActionFileCopy, middle, filepath.Join(f.root, "copied")},
		"copy to behind a link":     {ActionFileCopy, real, filepath.Join(data, "conf", "planted.txt")},
		"remove behind a link":      {ActionFileRemove, middle},
		"remove below a link":       {ActionFileRemove, filepath.Join(middle, "keep.txt")},
	} {
		_, err := f.runner.Apply(t.Context(), args[0], args[1:], nil)
		wantRefusal(t, name, err, "link")
		step := map[string]string{ActionFileRename: stepFileRename, ActionFileCopy: stepFileCopy, ActionFileRemove: stepFileRemove}[args[0]]
		if err := runFileStep(Command{Argv: append([]string{step}, args[1:]...), Builtin: true}, f.ops()); err == nil {
			t.Errorf("%s: the step followed the link", name)
		}
	}
	if names(t, outside) != "victim" || names(t, victim) != "keep.txt" || content(t, filepath.Join(victim, "keep.txt")) != "keep\n" {
		t.Fatal("something behind a link changed")
	}
	wantGone(t, filepath.Join(f.root, "moved"))
	wantGone(t, filepath.Join(f.root, "copied"))

	// The link itself as the last entry is removed as a link.
	f.change(t, ActionFileRemove, filepath.Join(data, "conf"))
	wantGone(t, filepath.Join(data, "conf"))
	if names(t, victim) != "keep.txt" {
		t.Fatal("the removal followed the link")
	}
}

func TestFileTreeLockedPaths(t *testing.T) {
	f := newFileFixture(t)
	for name, args := range map[string][]string{
		"remove .git":                 {ActionFileRemove, "/srv/demo/.git"},
		"remove stack settings":       {ActionFileRemove, "/srv/demo/.qatlas/qhost/settings.yaml"},
		"rename into .git":            {ActionFileRename, "/srv/demo/a", "/srv/demo/.git/a"},
		"rename out of .git":          {ActionFileRename, "/srv/demo/.git/config", "/srv/demo/config"},
		"copy into stack settings":    {ActionFileCopy, "/srv/demo/a", "/srv/demo/.qatlas/qhost/a"},
		"copy into a nested .git":     {ActionFileCopy, "/srv/demo/a", "/srv/demo/data/repo/.git/a"},
		"rename stack settings":       {ActionFileRename, "/srv/demo/.qatlas/qhost", "/srv/demo/.qatlas/old"},
		"remove below stack settings": {ActionFileRemove, "/srv/demo/.qatlas/qhost/new/x"},
	} {
		_, err := f.runner.Registry.Describe(args[0], args[1:], nil)
		wantRefusal(t, name, err, "locked")
	}
	if _, err := f.runner.Registry.Describe(ActionFileCopy, []string{"/srv/demo/.git/config", "/srv/demo/config.bak"}, nil); err != nil {
		t.Errorf("copying out of .git only reads it: %v", err)
	}

	repo := filepath.Join(f.root, "repo")
	mkdir(t, repo, 0o755)
	mkdir(t, filepath.Join(repo, ".git"), 0o755)
	mkfile(t, filepath.Join(repo, ".git", "HEAD"), "ref: refs/heads/main\n", 0o644)
	for name, args := range map[string][]string{
		"remove a repository": {ActionFileRemove, repo},
		"rename a repository": {ActionFileRename, repo, filepath.Join(f.root, "repo2")},
		"copy a repository":   {ActionFileCopy, repo, filepath.Join(f.root, "repo-copy")},
	} {
		_, err := f.runner.Apply(t.Context(), args[0], args[1:], nil)
		wantRefusal(t, name, err, "locked", ".git")
		step := map[string]string{ActionFileRename: stepFileRename, ActionFileCopy: stepFileCopy, ActionFileRemove: stepFileRemove}[args[0]]
		if err := runFileStep(Command{Argv: append([]string{step}, args[1:]...), Builtin: true}, f.ops()); err == nil {
			t.Errorf("%s: the step must refuse on its own", name)
		}
	}
	if names(t, f.root) != "repo" || content(t, filepath.Join(repo, ".git", "HEAD")) != "ref: refs/heads/main\n" {
		t.Fatalf("the repository changed: %q", names(t, f.root))
	}
	f.change(t, ActionFileCopy, filepath.Join(repo, ".git", "HEAD"), filepath.Join(f.root, "HEAD.bak"))
	if content(t, filepath.Join(f.root, "HEAD.bak")) != "ref: refs/heads/main\n" {
		t.Error("a copy out of .git must work")
	}
}

func TestFileTreeSystemPathNeedsTheAcknowledgement(t *testing.T) {
	f := newFileFixture(t)
	for name, tc := range map[string]struct {
		args        []string
		consequence string
	}{
		"rename in /etc":        {[]string{ActionFileRename, "/etc/demo.conf", "/etc/demo.conf.old"}, "stop services"},
		"rename into /etc":      {[]string{ActionFileRename, "/srv/demo/a", "/etc/demo"}, "stop services"},
		"copy out of /etc":      {[]string{ActionFileCopy, "/etc/demo", "/srv/demo-copy"}, "keys or passwords"},
		"copy into /usr":        {[]string{ActionFileCopy, "/srv/demo/a", "/usr/local/demo"}, "change how the system"},
		"remove in /boot":       {[]string{ActionFileRemove, "/boot/demo"}, "stop services"},
		"remove permanent /etc": {[]string{ActionFileRemove, "/etc/demo", optionPermanent}, "stop services"},
	} {
		_, err := f.runner.Registry.Describe(tc.args[0], tc.args[1:], nil)
		wantRefusal(t, name, err, "system path", tc.consequence)
		plan, err := f.runner.Registry.Describe(tc.args[0], append(tc.args[1:], optionSystemPath), nil)
		if err != nil || !strings.Contains(plan.Sentence, systemSentence) {
			t.Errorf("%s acknowledged: %v %q", name, err, plan.Sentence)
		}
	}
	for name, args := range map[string][]string{
		"remove in /proc":   {ActionFileRemove, "/proc/sys/kernel/demo", optionSystemPath},
		"copy out of /sys":  {ActionFileCopy, "/sys/kernel/demo", "/srv/demo", optionSystemPath},
		"rename into /proc": {ActionFileRename, "/srv/demo", "/proc/demo", optionSystemPath},
	} {
		_, err := f.runner.Registry.Describe(args[0], args[1:], nil)
		wantRefusal(t, name, err, "virtual file system")
	}
	for name, args := range map[string][]string{
		"remove /etc":     {ActionFileRemove, "/etc", optionSystemPath, optionPermanent},
		"rename /usr":     {ActionFileRename, "/usr", "/usr-old", optionSystemPath},
		"copy over /boot": {ActionFileCopy, "/srv/demo", "/boot", optionSystemPath},
	} {
		_, err := f.runner.Registry.Describe(args[0], args[1:], nil)
		wantRefusal(t, name, err, "system folder")
	}
	for name, args := range map[string][]string{
		"rename one path":        {ActionFileRename, "/srv/demo"},
		"rename permanent":       {ActionFileRename, "/srv/a", "/srv/b", optionPermanent},
		"copy one path":          {ActionFileCopy, "/srv/a"},
		"copy into itself":       {ActionFileCopy, "/srv/a", "/srv/a/b"},
		"copy onto itself":       {ActionFileCopy, "/srv/a", "/srv/a"},
		"remove unknown option":  {ActionFileRemove, "/srv/a", "force"},
		"remove twice permanent": {ActionFileRemove, "/srv/a", optionPermanent, optionPermanent},
		"remove relative":        {ActionFileRemove, "srv/a"},
		"remove root":            {ActionFileRemove, "/"},
		"remove dotdot":          {ActionFileRemove, "/srv/../etc"},
		"remove nothing":         {ActionFileRemove},
	} {
		_, err := f.runner.Registry.Describe(args[0], args[1:], nil)
		wantArgError(t, name, err)
	}
}

func TestFileTreeContentIsNeverShown(t *testing.T) {
	f := newFileFixture(t)
	const marker = "DEMO_TOKEN=demo-secret-value-37\n"
	encoded := base64.StdEncoding.EncodeToString([]byte(marker))
	src := filepath.Join(f.root, "app.env")
	mkfile(t, src, marker, 0o600)

	res := f.change(t, ActionFileCopy, src, filepath.Join(f.root, "app.env.bak"))
	if strings.Contains(f.journalText(t), "demo-secret-value") || strings.Contains(f.journalText(t), encoded) {
		t.Error("a copy must keep no content in the journal")
	}
	f.undo(t, res.Entry.ID)

	res = f.change(t, ActionFileRemove, src)
	if !strings.Contains(f.journalText(t), encoded) {
		t.Error("the journal must keep the removed content for the undo")
	}
	inverse, err := inverseRemove(res.Entry.Previous)
	if err != nil {
		t.Fatal(err)
	}
	if len(inverse.Redact) != 1 || string(inverse.Redact[0]) != marker {
		t.Errorf("the inverse must redact the removed content: %q", inverse.Redact)
	}
	cmd := inverse.Commands[0]
	encodedCmd, _ := json.Marshal(cmd)
	undo := f.undo(t, res.Entry.ID)
	audited, _ := json.Marshal(f.audited)
	shown := fmt.Sprintf("%v %+v %#v %s %s", cmd, cmd, cmd, encodedCmd, inverse.Sentence) + string(audited) +
		res.Entry.Sentence + strings.Join(res.Entry.Commands, "") + undo.Entry.Sentence + strings.Join(undo.Entry.Commands, "") + undo.Entry.Error
	if strings.Contains(shown, "demo-secret-value") || strings.Contains(shown, encoded) {
		t.Errorf("the content was shown: %q", shown)
	}
	if content(t, src) != marker {
		t.Error("the undo must bring the content back")
	}
}

func TestFileJournalFolderLocked(t *testing.T) {
	f := newFileFixture(t)
	const j = "/var/lib/qhost-host"
	for name, args := range map[string][]string{
		"write":                {ActionFileWrite, j},
		"write in journal":     {ActionFileWrite, j + "/journal/x.json"},
		"create folder":        {ActionFileCreateFolder, j + "/journal"},
		"create folder below":  {ActionFileCreateFolder, j + "/journal/new"},
		"rename source":        {ActionFileRename, j + "/journal/x", "/srv/demo/x"},
		"rename source folder": {ActionFileRename, j, "/srv/demo/old"},
		"rename target":        {ActionFileRename, "/srv/demo/a", j + "/journal/x"},
		"copy target":          {ActionFileCopy, "/srv/demo/a", j + "/journal/x"},
		"copy target folder":   {ActionFileCopy, "/srv/demo/a", j},
		"remove folder":        {ActionFileRemove, j},
		"remove journal":       {ActionFileRemove, j + "/journal"},
		"remove journal file":  {ActionFileRemove, j + "/journal/x.json"},
	} {
		var data []byte
		if args[0] == ActionFileWrite {
			data = []byte("x\n")
		}
		_, err := f.runner.Registry.Describe(args[0], args[1:], data)
		wantRefusal(t, name, err, "locked", "journal")
	}
	for _, id := range []string{ActionFileWrite, ActionFileCreateFolder} {
		step := map[string]string{ActionFileWrite: stepFileWrite, ActionFileCreateFolder: stepFolderCreate}[id]
		cmd := Command{Argv: []string{step, j + "/journal/x"}, Builtin: true, Secret: []byte(`{"content":"eAo="}`)}
		if err := runFileStep(cmd, fileOps{chown: f.chown}); err == nil || !strings.Contains(err.Error(), "locked") {
			t.Errorf("the %s step must refuse on its own: %v", id, err)
		}
	}
	for name, tc := range map[string]struct {
		step string
		args []string
	}{
		"rename": {stepFileRename, []string{j + "/journal/x", "/srv/demo/x"}},
		"copy":   {stepFileCopy, []string{"/srv/demo/a", j + "/journal/x"}},
		"remove": {stepFileRemove, []string{j + "/journal/x"}},
	} {
		err := runFileStep(Command{Argv: append([]string{tc.step}, tc.args...), Builtin: true}, f.ops())
		if err == nil || !strings.Contains(err.Error(), "locked") {
			t.Errorf("the %s step must refuse on its own: %v", name, err)
		}
	}
	for _, p := range []string{"/var/lib/qhost-hostx/a", "/var/lib/qhost/a"} {
		if _, err := f.runner.Registry.Describe(ActionFileWrite, []string{p}, []byte("x\n")); err != nil {
			t.Errorf("%s: %v", p, err)
		}
	}
	if _, err := f.runner.Registry.Describe(ActionFileCopy, []string{j + "/journal/x.json", "/srv/demo/x.bak"}, nil); err != nil {
		t.Errorf("copying out of the journal only reads it: %v", err)
	}

	// A folder that holds the journal folder is neither removed, moved nor overwritten.
	old := journalStateDir
	journalStateDir = filepath.Join(f.root, "var", "lib", "qhost-host")
	t.Cleanup(func() { journalStateDir = old })
	lib := filepath.Join(f.root, "var", "lib")
	mkdir(t, filepath.Join(f.root, "var"), 0o755)
	mkdir(t, lib, 0o755)
	mkdir(t, journalStateDir, 0o755)
	mkdir(t, filepath.Join(f.root, "other"), 0o755)
	mkfile(t, filepath.Join(journalStateDir, "entry.json"), "{}\n", 0o600)
	for name, args := range map[string][]string{
		"remove /var/lib": {ActionFileRemove, lib},
		"rename /var/lib": {ActionFileRename, lib, filepath.Join(f.root, "lib2")},
	} {
		_, err := f.runner.Apply(t.Context(), args[0], args[1:], nil)
		wantRefusal(t, name, err, "locked", "journal")
	}
	if content(t, filepath.Join(journalStateDir, "entry.json")) != "{}\n" {
		t.Fatal("the journal changed")
	}
}
