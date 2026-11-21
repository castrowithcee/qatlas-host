package hostchange

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// fileWorld runs the file steps on a temporary tree. It records owner changes instead of making them, since
// only root may give a file away.
type fileWorld struct {
	chowns   []string
	chownErr error
	// after runs once a write succeeded, to simulate a failure after the step changed the host.
	after func(Command) error
}

func (w *fileWorld) Run(_ context.Context, c Command) ([]byte, error) {
	if err := runFileStep(c, w.ops()); err != nil {
		return nil, err
	}
	if w.after != nil && c.Argv[0] == stepFileWrite {
		return nil, w.after(c)
	}
	return nil, nil
}

func (w *fileWorld) ops() fileOps { return fileOps{chown: w.chown, chownLink: w.chownLink} }

func (w *fileWorld) chownLink(dir int, name string, uid, gid int) error {
	if w.chownErr != nil {
		return w.chownErr
	}
	var st unix.Stat_t
	if err := unix.Fstatat(dir, name, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return err
	}
	if st.Mode&unix.S_IFMT != unix.S_IFLNK {
		return fmt.Errorf("%s is not a link", name)
	}
	w.chowns = append(w.chowns, fmt.Sprintf("%d %d:%d", st.Ino, uid, gid))
	return nil
}

func (w *fileWorld) chown(fd, uid, gid int) error {
	if w.chownErr != nil {
		return w.chownErr
	}
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return err
	}
	w.chowns = append(w.chowns, fmt.Sprintf("%d %d:%d", st.Ino, uid, gid))
	return nil
}

type fileFixture struct {
	*fileWorld
	runner  *Runner
	root    string
	journal string
	audited []AuditEntry
}

func newFileFixture(t *testing.T) *fileFixture {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, "tree")
	mkdir(t, root, 0o755)
	f := &fileFixture{fileWorld: &fileWorld{}, root: root, journal: filepath.Join(dir, "host", "journal")}
	reg := NewRegistry()
	if err := RegisterFileActions(reg); err != nil {
		t.Fatal(err)
	}
	j, err := OpenJournal(f.journal, os.Getuid())
	if err != nil {
		t.Fatal(err)
	}
	clock := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	f.runner = &Runner{
		Registry: reg, Journal: j, Exec: f.fileWorld, SudoUID: 1000,
		Now:    func() time.Time { clock = clock.Add(time.Second); return clock },
		Record: func(a AuditEntry) error { f.audited = append(f.audited, a); return nil },
	}
	return f
}

func (f *fileFixture) apply(t *testing.T, id, path, content string, extra ...string) Result {
	t.Helper()
	res, err := f.runner.Apply(t.Context(), id, append([]string{path}, extra...), []byte(content))
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// command is the step of the plan, as the runner would hand it to the executor.
func (f *fileFixture) command(t *testing.T, id, path, content string) Command {
	t.Helper()
	plan, err := f.runner.Registry.Describe(id, []string{path}, []byte(content))
	if err != nil {
		t.Fatal(err)
	}
	return plan.Commands[0]
}

func (f *fileFixture) journalText(t *testing.T) string {
	t.Helper()
	entries, err := os.ReadDir(f.journal)
	if err != nil {
		t.Fatal(err)
	}
	var all strings.Builder
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(f.journal, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		all.Write(b)
	}
	return all.String()
}

func mkdir(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func mkfile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func statOf(t *testing.T, path string) *syscall.Stat_t {
	t.Helper()
	fi, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	return fi.Sys().(*syscall.Stat_t)
}

func content(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func wantRefusal(t *testing.T, what string, err error, words ...string) {
	t.Helper()
	if _, ok := isRefusal(err); !ok {
		t.Fatalf("%s: want a refusal, got %v", what, err)
	}
	for _, w := range words {
		if !strings.Contains(err.Error(), w) {
			t.Errorf("%s: %q does not name %q", what, err, w)
		}
	}
}

func TestFileWriteKeepsInodeOwnerModeAndHardLinks(t *testing.T) {
	f := newFileFixture(t)
	p := filepath.Join(f.root, "app.conf")
	mkfile(t, p, "old=1\nmore=old\n", 0o640)
	second := filepath.Join(f.root, "second-name")
	if err := os.Link(p, second); err != nil {
		t.Fatal(err)
	}
	before := statOf(t, p)
	res := f.apply(t, ActionFileWrite, p, "new=2\n")
	if res.Entry.Result != ResultApplied || res.Entry.Check != CheckPassed || !res.Entry.Reversible {
		t.Fatalf("%+v", res.Entry)
	}
	after := statOf(t, p)
	if content(t, p) != "new=2\n" || content(t, second) != "new=2\n" {
		t.Errorf("content %q, second name %q", content(t, p), content(t, second))
	}
	if after.Ino != before.Ino || after.Uid != before.Uid || after.Gid != before.Gid || after.Mode&0o7777 != 0o640 || after.Nlink != 2 {
		t.Errorf("before %+v after %+v: inode, owner, mode and hard links must stay", before, after)
	}
	if len(f.chowns) != 0 {
		t.Errorf("an existing file must keep its owner: %q", f.chowns)
	}
	if want := "[builtin] file-write " + p + " < <content>"; strings.Join(res.Entry.Commands, ";") != want {
		t.Errorf("commands %q", res.Entry.Commands)
	}
}

func TestFileWriteNewFileGetsOwnerAndGroupOfItsFolder(t *testing.T) {
	f := newFileFixture(t)
	folder := filepath.Join(f.root, "data")
	mkdir(t, folder, 0o750)
	p := filepath.Join(folder, "new.env")
	res := f.apply(t, ActionFileWrite, p, "DEMO=1\n")
	if res.Entry.Result != ResultApplied || res.Entry.Check != CheckPassed {
		t.Fatalf("%+v", res.Entry)
	}
	st, dir := statOf(t, p), statOf(t, folder)
	if content(t, p) != "DEMO=1\n" || st.Mode&0o7777 != 0o640 {
		t.Errorf("content %q mode %04o", content(t, p), st.Mode&0o7777)
	}
	if want := fmt.Sprintf("%d %d:%d", st.Ino, dir.Uid, dir.Gid); strings.Join(f.chowns, ";") != want {
		t.Errorf("owner changes %q, want %q", f.chowns, want)
	}

	open := filepath.Join(f.root, "open")
	mkdir(t, open, 0o1777)
	f.apply(t, ActionFileWrite, filepath.Join(open, "note.txt"), "")
	if m := statOf(t, filepath.Join(open, "note.txt")).Mode & 0o7777; m != 0o664 {
		t.Errorf("a new file in a folder writable for everyone has %04o; it must not be writable for everyone", m)
	}

	_, err := f.runner.Apply(t.Context(), ActionFileWrite, []string{filepath.Join(f.root, "missing", "x.conf")}, []byte("x\n"))
	wantRefusal(t, "missing folder", err, "does not exist")
}

func TestFileUndoRestoresContentAndMetadata(t *testing.T) {
	f := newFileFixture(t)
	p := filepath.Join(f.root, "app.conf")
	mkfile(t, p, "old\n", 0o640)
	ino := statOf(t, p).Ino
	res := f.apply(t, ActionFileWrite, p, "new\n")
	if err := os.Chmod(p, 0o600); err != nil {
		t.Fatal(err)
	}
	undo, err := f.runner.Undo(t.Context(), res.Entry.ID)
	if err != nil || undo.Entry.Result != ResultApplied {
		t.Fatalf("undo: %v %+v", err, undo.Entry)
	}
	st := statOf(t, p)
	if content(t, p) != "old\n" || st.Mode&0o7777 != 0o640 || st.Ino != ino {
		t.Errorf("content %q mode %04o inode %d, want the previous content, mode and inode", content(t, p), st.Mode&0o7777, st.Ino)
	}
	if want := "[builtin] file-restore " + p + " < <previous content>"; strings.Join(undo.Entry.Commands, ";") != want {
		t.Errorf("undo commands %q", undo.Entry.Commands)
	}

	res = f.apply(t, ActionFileWrite, p, "second\n")
	mkfile(t, p, "edited later\n", 0o640)
	undo, err = f.runner.Undo(t.Context(), res.Entry.ID)
	if err != nil || undo.Entry.Result != ResultFailed || !strings.Contains(undo.Entry.Error, "changed after the change") {
		t.Fatalf("undo over a later edit: %v %+v", err, undo.Entry)
	}
	if content(t, p) != "edited later\n" {
		t.Errorf("a later edit was overwritten: %q", content(t, p))
	}
}

func TestFileUndoRemovesACreatedFileOnlyWhileUnchanged(t *testing.T) {
	f := newFileFixture(t)
	p := filepath.Join(f.root, "new.txt")
	res := f.apply(t, ActionFileWrite, p, "one\n")
	undo, err := f.runner.Undo(t.Context(), res.Entry.ID)
	if err != nil || undo.Entry.Result != ResultApplied {
		t.Fatalf("undo: %v %+v", err, undo.Entry)
	}
	if _, err := os.Lstat(p); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the created file must be gone: %v", err)
	}

	res = f.apply(t, ActionFileWrite, p, "two\n")
	mkfile(t, p, "edited later\n", 0o644)
	undo, err = f.runner.Undo(t.Context(), res.Entry.ID)
	if err != nil || undo.Entry.Result != ResultFailed || content(t, p) != "edited later\n" {
		t.Fatalf("a file edited after the change must stay: %v %+v", err, undo.Entry)
	}
}

func TestFileFailedChangeIsTakenBack(t *testing.T) {
	f := newFileFixture(t)
	p := filepath.Join(f.root, "app.conf")
	mkfile(t, p, "old\n", 0o644)
	f.after = func(Command) error { return errors.New("simulated failure after the write") }
	res := f.apply(t, ActionFileWrite, p, "half\n")
	if res.Entry.Result != ResultRestored || content(t, p) != "old\n" {
		t.Fatalf("content %q, %+v", content(t, p), res.Entry)
	}

	f.after = nil
	f.chownErr = errors.New("simulated chown failure")
	created := filepath.Join(f.root, "created.conf")
	res = f.apply(t, ActionFileWrite, created, "x\n")
	if res.Entry.Result == ResultApplied {
		t.Fatalf("%+v", res.Entry)
	}
	folder := filepath.Join(f.root, "created")
	res = f.apply(t, ActionFileCreateFolder, folder, "")
	if res.Entry.Result == ResultApplied {
		t.Fatalf("%+v", res.Entry)
	}
	for _, p := range []string{created, folder} {
		if _, err := os.Lstat(p); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("a failed creation left %s behind: %v", p, err)
		}
	}
}

func TestFileStepsRefuseLinks(t *testing.T) {
	f := newFileFixture(t)
	outsideDir := t.TempDir()
	shadow := filepath.Join(outsideDir, "shadow")
	mkfile(t, shadow, "root:demo-hash:1:0:99999:7:::\n", 0o600)
	// A data folder that a container may write to.
	data := filepath.Join(f.root, "data")
	mkdir(t, data, 0o777)
	last := filepath.Join(data, "app.conf")
	if err := os.Symlink(shadow, last); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideDir, filepath.Join(data, "conf")); err != nil {
		t.Fatal(err)
	}
	dangling := filepath.Join(data, "planted")
	if err := os.Symlink(filepath.Join(outsideDir, "planted"), dangling); err != nil {
		t.Fatal(err)
	}
	middle := filepath.Join(data, "conf", "shadow")

	for name, tc := range map[string]struct{ id, path string }{
		"link as last entry":   {ActionFileWrite, last},
		"dangling link":        {ActionFileWrite, dangling},
		"link on the way":      {ActionFileWrite, middle},
		"folder behind a link": {ActionFileCreateFolder, filepath.Join(data, "conf", "new")},
		"folder over a link":   {ActionFileCreateFolder, last},
	} {
		_, err := f.runner.Apply(t.Context(), tc.id, []string{tc.path}, []byte("pwned\n"))
		if _, ok := isRefusal(err); !ok {
			t.Errorf("%s: want a refusal, got %v", name, err)
		}
		// The step refuses on its own, as when the link appears after the previous state was read.
		if err := runFileStep(f.command(t, tc.id, tc.path, "pwned\n"), fileOps{chown: f.chown}); err == nil {
			t.Errorf("%s: the step followed the link", name)
		}
	}
	restore, err := json.Marshal(fileState{Path: middle, Existed: true, Content: []byte("pwned\n"), Meta: &fileMeta{Mode: 0o644}})
	if err != nil {
		t.Fatal(err)
	}
	if err := runFileStep(Command{Argv: []string{stepFileRestore, middle}, Builtin: true, Secret: restore}, fileOps{chown: f.chown}); err == nil {
		t.Error("the restore followed a link on the way")
	}
	if err := runFileStep(Command{Argv: []string{stepFolderRemove, filepath.Join(data, "conf")}, Builtin: true}, fileOps{}); err == nil {
		t.Error("the folder removal must refuse a link")
	}
	if content(t, shadow) != "root:demo-hash:1:0:99999:7:::\n" {
		t.Errorf("the file behind the link changed: %q", content(t, shadow))
	}
	if entries, _ := os.ReadDir(outsideDir); len(entries) != 1 {
		t.Errorf("something was created behind a link: %v", entries)
	}
	if len(f.chowns) != 0 {
		t.Errorf("owner changes %q", f.chowns)
	}
}

func TestFileLockedPaths(t *testing.T) {
	f := newFileFixture(t)
	for _, p := range []string{
		"/srv/demo/.git", "/srv/demo/.git/config", "/srv/demo/.git/hooks/post-merge", "/srv/demo/data/repo/.git/HEAD",
		"/srv/demo/.qatlas/qhost", "/srv/demo/.qatlas/qhost/settings.yaml", "/srv/demo/.qatlas/qhost/new/x",
	} {
		for _, id := range []string{ActionFileWrite, ActionFileCreateFolder} {
			_, err := f.runner.Registry.Describe(id, []string{p}, []byte("x\n"))
			wantRefusal(t, id+" "+p, err, "locked")
		}
		cmd := Command{Argv: []string{stepFileWrite, p}, Builtin: true, Secret: []byte(`{"content":"eAo="}`)}
		if err := runFileStep(cmd, fileOps{chown: f.chown}); err == nil || !strings.Contains(err.Error(), "locked") {
			t.Errorf("the step must refuse %s on its own: %v", p, err)
		}
	}
	for _, p := range []string{"/srv/demo/.gitignore", "/srv/demo/.github/workflows/ci.yml", "/srv/demo/.qatlas/other.yaml", "/srv/demo/qhost/x"} {
		if _, err := f.runner.Registry.Describe(ActionFileWrite, []string{p}, []byte("x\n")); err != nil {
			t.Errorf("%s: %v", p, err)
		}
	}

	git := filepath.Join(f.root, ".git")
	mkdir(t, git, 0o755)
	_, err := f.runner.Apply(t.Context(), ActionFileWrite, []string{filepath.Join(git, "config")}, []byte("x\n"))
	wantRefusal(t, "apply below .git", err, "locked")
	if _, err := os.Lstat(filepath.Join(git, "config")); !errors.Is(err, os.ErrNotExist) {
		t.Error("a file was written below .git")
	}
}

func TestFileContentLimits(t *testing.T) {
	f := newFileFixture(t)
	p := filepath.Join(f.root, "app.conf")
	for name, data := range map[string][]byte{
		"too large":     bytes.Repeat([]byte("a"), MaxFileContent+1),
		"NUL byte":      []byte("a\x00b\n"),
		"invalid UTF-8": {'o', 'k', 0xff, 0xfe, '\n'},
	} {
		_, err := f.runner.Registry.Describe(ActionFileWrite, []string{p}, data)
		var arg *ArgumentError
		if !errors.As(err, &arg) {
			t.Errorf("%s: want an argument error, got %v", name, err)
		}
	}
	if _, err := f.runner.Registry.Describe(ActionFileWrite, []string{p}, bytes.Repeat([]byte("ä"), MaxFileContent/2)); err != nil {
		t.Errorf("text of exactly the limit: %v", err)
	}
	cmd := Command{Argv: []string{stepFileWrite, p}, Builtin: true, Secret: []byte(`{"content":"AAE="}`)}
	if err := runFileStep(cmd, fileOps{chown: f.chown}); err == nil {
		t.Error("the step must refuse binary content on its own")
	}

	mkfile(t, p, "something\n", 0o644)
	if res := f.apply(t, ActionFileWrite, p, ""); res.Entry.Result != ResultApplied || content(t, p) != "" {
		t.Fatalf("an empty write: %q %+v", content(t, p), res.Entry)
	}

	big := filepath.Join(f.root, "big.log")
	mkfile(t, big, strings.Repeat("x", MaxFileContent+1), 0o644)
	_, err := f.runner.Apply(t.Context(), ActionFileWrite, []string{big}, []byte("x\n"))
	wantRefusal(t, "large previous file", err, "larger")
	bin := filepath.Join(f.root, "tool.bin")
	mkfile(t, bin, "\x7fELF\x00\x01", 0o755)
	_, err = f.runner.Apply(t.Context(), ActionFileWrite, []string{bin}, []byte("x\n"))
	wantRefusal(t, "binary previous file", err, "binary")
	_, err = f.runner.Apply(t.Context(), ActionFileWrite, []string{f.root}, []byte("x\n"))
	wantRefusal(t, "a folder", err, "not a plain file")
}

func TestFileSystemPathNeedsTheAcknowledgement(t *testing.T) {
	f := newFileFixture(t)
	for id, consequence := range map[string]string{ActionFileWrite: "stop services", ActionFileCreateFolder: "change how the system"} {
		for _, p := range []string{"/etc/demo-app.conf", "/usr/local/etc/demo", "/boot/demo", "/etc"} {
			_, err := f.runner.Registry.Describe(id, []string{p}, []byte("x\n"))
			wantRefusal(t, id+" "+p, err, "system path", consequence)
			plan, err := f.runner.Registry.Describe(id, []string{p, optionSystemPath}, []byte("x\n"))
			if err != nil || !strings.HasSuffix(plan.Sentence, systemSentence) {
				t.Errorf("%s %s acknowledged: %v %q", id, p, err, plan.Sentence)
			}
		}
		for _, p := range []string{"/proc/sys/kernel/demo", "/sys/kernel/demo"} {
			_, err := f.runner.Registry.Describe(id, []string{p, optionSystemPath}, []byte("x\n"))
			wantRefusal(t, id+" "+p, err, "virtual file system")
		}
		plan, err := f.runner.Registry.Describe(id, []string{"/srv/demo/app.conf"}, []byte("x\n"))
		if err != nil || strings.Contains(plan.Sentence, "system path") {
			t.Errorf("%s outside the system paths: %v %q", id, err, plan.Sentence)
		}
		for name, args := range map[string][]string{
			"no path": nil, "relative": {"srv/demo"}, "dotdot": {"/srv/demo/../x"}, "trailing": {"/srv/demo/"},
			"root": {"/"}, "newline": {"/srv/demo\nx"}, "unknown option": {"/srv/demo", "force"},
			"too many": {"/srv/demo", optionSystemPath, optionSystemPath},
		} {
			_, err := f.runner.Registry.Describe(id, args, []byte("x\n"))
			var arg *ArgumentError
			if !errors.As(err, &arg) {
				t.Errorf("%s %s: want an argument error, got %v", id, name, err)
			}
		}
	}
}

func TestFileContentIsNeverShown(t *testing.T) {
	f := newFileFixture(t)
	const marker = "DEMO_TOKEN=demo-secret-value-91\n"
	encoded := base64.StdEncoding.EncodeToString([]byte(marker))
	p := filepath.Join(f.root, "app.env")
	mkfile(t, p, "DEMO_TOKEN=old-demo-value\n", 0o600)

	cmd := f.command(t, ActionFileWrite, p, marker)
	plan, err := f.runner.Registry.Describe(ActionFileWrite, []string{p}, []byte(marker))
	if err != nil {
		t.Fatal(err)
	}
	encodedCmd, _ := json.Marshal(cmd)
	shown := fmt.Sprintf("%v %+v %#v %s %s %s", cmd, cmd, cmd, cmd, encodedCmd, plan.Sentence)
	if cmd.Display() != "[builtin] file-write "+p+" < <content>" || strings.Contains(shown, "demo-secret-value") || strings.Contains(shown, encoded) {
		t.Fatalf("shown %q", shown)
	}

	f.after = func(Command) error { return errors.New("simulated failure while writing " + marker) }
	res := f.apply(t, ActionFileWrite, p, marker)
	f.after = nil
	if res.Entry.Result != ResultRestored || strings.Contains(res.Entry.Error, "demo-secret-value") {
		t.Fatalf("%+v", res.Entry)
	}
	res = f.apply(t, ActionFileWrite, p, marker)
	if res.Entry.Result != ResultApplied {
		t.Fatalf("%+v", res.Entry)
	}
	audited, _ := json.Marshal(f.audited)
	all := f.journalText(t) + string(audited) + strings.Join(res.Entry.Commands, "") + res.Entry.Sentence
	if strings.Contains(all, "demo-secret-value") || strings.Contains(all, encoded) {
		t.Error("the new content reached the journal or the audit entry")
	}

	inverse, err := inverseFileWrite(res.Entry.Previous)
	if err != nil {
		t.Fatal(err)
	}
	if got := displays(inverse.Commands); strings.Join(got, "") != "[builtin] file-restore "+p+" < <previous content>" ||
		strings.Contains(inverse.Sentence+fmt.Sprint(inverse.Commands), "old-demo-value") {
		t.Errorf("inverse %q %q", inverse.Sentence, got)
	}
}

func TestFileCreateFolder(t *testing.T) {
	f := newFileFixture(t)
	shared := filepath.Join(f.root, "shared")
	mkdir(t, shared, 0o2770)
	p := filepath.Join(shared, "new")
	res := f.apply(t, ActionFileCreateFolder, p, "")
	if res.Entry.Result != ResultApplied || res.Entry.Check != CheckPassed || !res.Entry.Reversible {
		t.Fatalf("%+v", res.Entry)
	}
	st, parent := statOf(t, p), statOf(t, shared)
	if st.Mode&unix.S_IFMT != unix.S_IFDIR || st.Mode&0o7777 != parent.Mode&0o7777 {
		t.Errorf("mode %o, want a folder with %04o", st.Mode, parent.Mode&0o7777)
	}
	if want := fmt.Sprintf("%d %d:%d", st.Ino, parent.Uid, parent.Gid); strings.Join(f.chowns, ";") != want {
		t.Errorf("owner changes %q, want %q", f.chowns, want)
	}
	if want := "[builtin] folder-create " + p; strings.Join(res.Entry.Commands, ";") != want {
		t.Errorf("commands %q", res.Entry.Commands)
	}
	_, err := f.runner.Apply(t.Context(), ActionFileCreateFolder, []string{p}, nil)
	wantRefusal(t, "existing folder", err, "exists already")

	undo, err := f.runner.Undo(t.Context(), res.Entry.ID)
	if err != nil || undo.Entry.Result != ResultApplied {
		t.Fatalf("undo: %v %+v", err, undo.Entry)
	}
	if _, err := os.Lstat(p); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the folder must be gone: %v", err)
	}

	res = f.apply(t, ActionFileCreateFolder, p, "")
	mkfile(t, filepath.Join(p, "kept.txt"), "kept\n", 0o644)
	undo, err = f.runner.Undo(t.Context(), res.Entry.ID)
	if err != nil || undo.Entry.Result != ResultFailed || !strings.Contains(undo.Entry.Error, "not empty") {
		t.Fatalf("undo of a filled folder: %v %+v", err, undo.Entry)
	}
	if content(t, filepath.Join(p, "kept.txt")) != "kept\n" {
		t.Error("the content of the folder was touched")
	}
	_, err = f.runner.Apply(t.Context(), ActionFileCreateFolder, []string{filepath.Join(f.root, "missing", "x")}, nil)
	wantRefusal(t, "missing folder", err, "does not exist")
}
