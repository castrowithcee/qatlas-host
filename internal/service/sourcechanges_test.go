package service_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/service"
)

// confCompose mounts ./data and ./conf, so new files below them are the host's own.
const confCompose = bindCompose + "      - ./conf:/etc/demo\n"

// changesHarness binds a local folder that holds the stacks web and db, and waits until both are listed.
func changesHarness(t *testing.T, opts ...func(*service.Config)) (*harness, string) {
	h, dir, _ := changesHarnessClock(t, opts...)
	return h, dir
}

// changesHarnessClock is changesHarness on a clock the test moves, which makes the service poll.
func changesHarnessClock(t *testing.T, opts ...func(*service.Config)) (*harness, string, *svcClock) {
	t.Helper()
	clock := &svcClock{now: noon}
	h := realGitService(t, append([]func(*service.Config){withClock(clock, nil)}, opts...)...)
	dir := localRepo(t, map[string]string{
		"web/compose.yaml": confCompose, "web/.env": "WEB_VERSION=1.2.3\n", "web/data/db.bin": "x", "web/conf/app.ini": "a=1\n", "web/old.txt": "old\n",
		"db/compose.yaml": webCompose, "db/.env": "WEB_VERSION=1.0.0\n",
	})
	if _, err := h.sourceAdd(dir); err != nil {
		t.Fatal(err)
	}
	h.waitStatus("stacks listed", func(st control.StatusResult) bool {
		return len(stacksNamed(st, "web")) == 1 && len(stacksNamed(st, "db")) == 1
	})
	return h, dir, clock
}

func (h *harness) changes(dir string) (control.SourceChangesResult, error) {
	var res control.SourceChangesResult
	err := h.call(control.SourceChangesCommand, control.SourceChangesArgs{Path: dir}, &res)
	return res, err
}

func (h *harness) discard(dir string, files ...string) (control.SourceDiscardResult, error) {
	var res control.SourceDiscardResult
	err := h.call(control.SourceDiscardCommand, control.SourceDiscardArgs{Path: dir, Files: files}, &res)
	return res, err
}

func (h *harness) diff(dir, file string) (control.SourceDiffResult, error) {
	var res control.SourceDiffResult
	err := h.call(control.SourceDiffCommand, control.SourceDiffArgs{Path: dir, File: file}, &res)
	return res, err
}

func hasLocalChanges(st control.StatusResult, stack string) bool {
	for _, s := range stacksNamed(st, stack) {
		if slices.ContainsFunc(s.Findings, func(f control.FindingInfo) bool { return f.Kind == "local-changes" }) {
			return true
		}
	}
	return false
}

func TestChangesListAllStacksOfTheFolderAndLeaveOutTheHostsOwnFiles(t *testing.T) {
	h, dir := changesHarness(t)
	writeTree(t, dir, map[string]string{
		"web/.env": "WEB_VERSION=9.9.9\n", "web/notes.txt": "n\n", "db/.env": "WEB_VERSION=2.0.0\n",
		"web/secrets.env": "TOKEN=demo\n", "web/data/new.bin": "y", "web/conf/untracked.ini": "u\n",
	})
	if err := os.Remove(filepath.Join(dir, "web/old.txt")); err != nil {
		t.Fatal(err)
	}
	res, err := h.changes(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []control.SourceChange{
		{Path: "db/.env", Kind: "modified", Stack: "db"}, {Path: "web/.env", Kind: "modified", Stack: "web"},
		{Path: "web/notes.txt", Kind: "new", Stack: "web"}, {Path: "web/old.txt", Kind: "deleted", Stack: "web"},
	}
	if res.Path != dir || !slices.Equal(res.Changes, want) {
		t.Fatalf("changes = %+v, want %+v", res, want)
	}
	// a changed versioned file below a bind mount counts, a new one does not
	writeTree(t, dir, map[string]string{"web/conf/app.ini": "a=2\n"})
	if res, _ := h.changes(dir); !slices.Contains(res.Changes, control.SourceChange{Path: "web/conf/app.ini", Kind: "modified", Stack: "web"}) {
		t.Fatalf("changes = %+v", res)
	}
	wantCode(t, func() error { _, err := h.changes(dir + "-other"); return err }(), control.CodeNoSuchSource)
	wantCode(t, func() error { _, err := h.changes("relative"); return err }(), control.CodeInvalid)
	diff, err := h.diff(dir, "web/.env")
	if err != nil || !strings.Contains(diff.Diff, "+WEB_VERSION=9.9.9") || strings.Contains(diff.Diff, "TOKEN") || diff.Truncated {
		t.Fatalf("diff = %+v, %v", diff, err)
	}
	if diff, err := h.diff(dir, "web/notes.txt"); err != nil || !strings.Contains(diff.Diff, "+n") {
		t.Fatalf("diff of a new file = %+v, %v", diff, err)
	}
	for _, p := range []string{"web/secrets.env", "web/data/new.bin", "web/conf/untracked.ini", "../db/.env", "/etc/passwd", "", "web/conf", "web", ".env"} {
		_, err := h.diff(dir, p)
		wantCode(t, err, control.CodeNotChanged)
	}
}

func TestDiscardTakesBackTheNamedFilesAndTheFindingsGo(t *testing.T) {
	h, dir, clock := changesHarnessClock(t)
	writeTree(t, dir, map[string]string{"web/.env": "WEB_VERSION=9.9.9\n", "web/notes.txt": "n\n", "web/other.txt": "o\n",
		"db/.env": "WEB_VERSION=2.0.0\n", "web/secrets.env": "TOKEN=demo\n", "web/data/new.bin": "y"})
	if err := os.Remove(filepath.Join(dir, "web/old.txt")); err != nil {
		t.Fatal(err)
	}
	h.advanceUntil(clock, time.Hour, "local changes found", func(st control.StatusResult) bool { return hasLocalChanges(st, "web") && hasLocalChanges(st, "db") })

	// nothing happens when one file is no change, not even for the valid ones
	for _, bad := range []string{"web/secrets.env", "web/data/new.bin", "../db/.env", "/etc/passwd", "web/conf", "web", "", ".", "web/missing.txt"} {
		wantCode(t, func() error { _, err := h.discard(dir, "web/.env", bad); return err }(), control.CodeNotChanged)
	}
	if got := fileText(t, filepath.Join(dir, "web/.env")); got != "WEB_VERSION=9.9.9\n" {
		t.Fatalf(".env = %q", got)
	}
	wantCode(t, func() error { _, err := h.discard(dir); return err }(), control.CodeInvalid)
	wantCode(t, func() error { _, err := h.discard(dir+"-other", "web/.env"); return err }(), control.CodeNoSuchSource)

	// the files of two stacks go back with one call
	res, err := h.discard(dir, "web/.env", "web/old.txt", "web/notes.txt", "web/other.txt", "db/.env")
	if err != nil || !slices.Equal(res.Discarded, []string{"web/.env", "web/old.txt", "web/notes.txt", "web/other.txt", "db/.env"}) {
		t.Fatalf("discard = %+v, %v", res, err)
	}
	for file, want := range map[string]string{"web/.env": "WEB_VERSION=1.2.3\n", "web/old.txt": "old\n", "db/.env": "WEB_VERSION=1.0.0\n"} {
		if got := fileText(t, filepath.Join(dir, file)); got != want {
			t.Fatalf("%s = %q", file, got)
		}
	}
	// the host's own files stay
	for _, keep := range []string{"web/secrets.env", "web/data/new.bin"} {
		if _, err := os.Stat(filepath.Join(dir, keep)); err != nil {
			t.Errorf("%s: %v", keep, err)
		}
	}
	for _, gone := range []string{"web/notes.txt", "web/other.txt"} {
		if _, err := os.Stat(filepath.Join(dir, gone)); !os.IsNotExist(err) {
			t.Errorf("%s: %v", gone, err)
		}
	}
	if res, _ := h.changes(dir); len(res.Changes) != 0 {
		t.Fatalf("still changed: %+v", res.Changes)
	}
	h.waitStatus("the findings are gone", func(st control.StatusResult) bool { return !hasLocalChanges(st, "web") && !hasLocalChanges(st, "db") })
}

func TestDiffAndDiscardAreRootOnlyAndTheListIsForAdministrators(t *testing.T) {
	p := &peers{}
	var seen atomic.Pointer[control.Registry]
	h, dir := changesHarness(t, withPeers(t, p, &seen))
	writeTree(t, dir, map[string]string{"web/.env": "WEB_VERSION=9.9.9\n"})

	p.as(exampleAdmin)
	if res, err := h.changes(dir); err != nil || len(res.Changes) != 1 {
		t.Fatalf("an administrator reads the list: %+v, %v", res, err)
	}
	_, err := h.diff(dir, "web/.env")
	wantCode(t, err, control.CodeNotRoot)
	wantCode(t, func() error { _, err := h.discard(dir, "web/.env"); return err }(), control.CodeNotRoot)
	if got := fileText(t, filepath.Join(dir, "web/.env")); got != "WEB_VERSION=9.9.9\n" {
		t.Fatalf(".env = %q", got)
	}
	for name, want := range map[string]control.Class{
		control.SourceChangesCommand: control.ClassRead, control.SourceDiffCommand: control.ClassChange,
		control.SourceDiscardCommand: control.ClassChange,
	} {
		c, ok := seen.Load().Lookup(name)
		if !ok || c.Class != want || c.Scope != control.ScopeDeploy {
			t.Errorf("%s = %+v, %v", name, c, ok)
		}
	}
	p.as(0)
}

func fileText(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
