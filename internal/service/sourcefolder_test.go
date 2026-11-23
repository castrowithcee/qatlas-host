package service_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

func (h *harness) commitFolder(into, folder, head string, files ...string) (control.SourceFolderCommitResult, error) {
	var res control.SourceFolderCommitResult
	err := h.call(control.SourceFolderCommitCommand, control.SourceFolderCommitArgs{Path: into, Folder: folder, Files: files, Head: head, SudoUID: 1000}, &res)
	return res, err
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestFolderCommitTakesThePlanOfThePreviewWithItsIgnoreLines(t *testing.T) {
	h, into := moveSource(t, &fakeDocker{}, nil)
	preview := h.move(project(t, "myapp", projectFiles()), into)
	if len(preview.Blocks) != 0 || preview.Ignore == nil {
		t.Fatalf("preview = %+v", preview)
	}
	// The folder is put into the source as the move will do it.
	writeTree(t, filepath.Join(into, "myapp"), projectFiles())
	head := headOf(t, into)

	res, err := h.commitFolder(into, "myapp", head, preview.PlannedFiles...)
	if err != nil || res.Commit != headOf(t, into) || res.Commit == head || res.Ignore == nil || !slices.Equal(res.Ignore.Lines, preview.Ignore.Lines) {
		t.Fatalf("commit = %+v, %v", res, err)
	}
	if want := []string{"myapp/.env", "myapp/.gitignore", "myapp/compose.yaml"}; !slices.Equal(slices.Sorted(slices.Values(res.Committed)), want) {
		t.Fatalf("committed = %v", res.Committed)
	}
	names := gitOutput(t, into, "show", "--name-only", "--format=%B", "HEAD")
	for _, left := range []string{"myapp/data", "myapp/notes.txt"} {
		if strings.Contains(names, left) {
			t.Errorf("the commit holds %s: %s", left, names)
		}
	}
	text := fileText(t, filepath.Join(into, "myapp/.gitignore"))
	for _, line := range preview.Ignore.Lines {
		if !strings.Contains(text, line+"\n") {
			t.Errorf(".gitignore lacks %q: %q", line, text)
		}
	}
	if st := gitOutput(t, into, "status", "--porcelain", "--untracked-files=all"); st != "" {
		t.Fatalf("the working copy is not clean: %s", st)
	}
	if got, _ := h.changes(into); len(got.Changes) != 0 {
		t.Fatalf("changes after the commit: %+v", got.Changes)
	}
}

func TestFolderCommitRefusesADeviatingListAMovedHeadAndLeavesTheFolderAsItWas(t *testing.T) {
	h, into := moveSource(t, &fakeDocker{}, nil)
	writeTree(t, filepath.Join(into, "myapp"), projectFiles())
	head := headOf(t, into)
	good := []string{".env", "compose.yaml"}

	for _, bad := range [][]string{{".env"}, {".env", "compose.yaml", "notes.txt"}, {".env", "compose.yaml", "gone.txt"}} {
		_, err := h.commitFolder(into, "myapp", head, bad...)
		wantCode(t, err, control.CodeInvalid)
	}
	_, err := h.commitFolder(into, "myapp", strings.Repeat("a", 40), good...)
	wantCode(t, err, control.CodeHeadMoved)
	for _, folder := range []string{"", ".hidden", "a/b", "..", "missing"} {
		_, err = h.commitFolder(into, folder, head, good...)
		wantCode(t, err, control.CodeInvalid)
	}
	_, err = h.commitFolder(into, "myapp", head)
	wantCode(t, err, control.CodeInvalid)
	_, err = h.commitFolder(into+"-other", "myapp", head, good...)
	wantCode(t, err, control.CodeNoSuchSource)

	if headOf(t, into) != head {
		t.Fatal("a refused commit moved the branch")
	}
	if _, err := os.Lstat(filepath.Join(into, "myapp/.gitignore")); err == nil {
		t.Fatal("a refused commit left a .gitignore")
	}
	if got := fileText(t, filepath.Join(into, "myapp/notes.txt")); got != "x" {
		t.Fatalf("notes.txt = %q", got)
	}
}

func TestFolderCommitPutsAnExistingIgnoreFileBackWhenTheCommitFails(t *testing.T) {
	h, into := moveSource(t, &fakeDocker{}, nil)
	files := projectFiles()
	files[".gitignore"] = "*.log\n"
	writeTree(t, filepath.Join(into, "myapp"), files)
	head := headOf(t, into)

	// The source branch moves between the preview and the commit: the stale head is refused after the plan.
	_, err := h.commitFolder(into, "myapp", strings.Repeat("b", 40), ".env", ".gitignore", "compose.yaml")
	wantCode(t, err, control.CodeHeadMoved)
	if got := fileText(t, filepath.Join(into, "myapp/.gitignore")); got != "*.log\n" {
		t.Fatalf(".gitignore = %q", got)
	}

	res, err := h.commitFolder(into, "myapp", head, ".env", ".gitignore", "compose.yaml")
	if err != nil || len(res.Committed) != 3 {
		t.Fatalf("commit = %+v, %v", res, err)
	}
	if got := fileText(t, filepath.Join(into, "myapp/.gitignore")); !strings.HasPrefix(got, "*.log\n") || !strings.Contains(got, "/notes.txt\n") {
		t.Fatalf(".gitignore = %q", got)
	}
}

func TestFolderCommitIsRootOnly(t *testing.T) {
	p := &peers{}
	var seen atomic.Pointer[control.Registry]
	h := realGitService(t, withPeers(t, p, &seen))
	into := localRepo(t, map[string]string{"other/compose.yaml": moveCompose, "other/.env": "WEB_VERSION=1.2.3\n"})
	if _, err := h.sourceAdd(into); err != nil {
		t.Fatal(err)
	}
	writeTree(t, filepath.Join(into, "myapp"), projectFiles())
	head := headOf(t, into)

	p.as(exampleAdmin)
	_, err := h.commitFolder(into, "myapp", head, ".env", "compose.yaml")
	wantCode(t, err, control.CodeNotRoot)
	if headOf(t, into) != head {
		t.Fatal("a commit without root moved the branch")
	}
	c, ok := seen.Load().Lookup(control.SourceFolderCommitCommand)
	if !ok || c.Class != control.ClassChange || c.Scope != control.ScopeDeploy {
		t.Fatalf("source.commit-folder = %+v, %v", c, ok)
	}
	p.as(0)
}
