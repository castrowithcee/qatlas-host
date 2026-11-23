package service_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/service"
)

func (h *harness) commitPaths(dir, head string, files ...string) (control.SourceCommitResult, error) {
	var res control.SourceCommitResult
	err := h.call(control.SourceCommitCommand, control.SourceCommitArgs{Path: dir, Files: files, Head: head, SudoUID: 1000}, &res)
	return res, err
}

func TestCommitTakesExactlyTheNamedPathsAndTheStacksRunAgain(t *testing.T) {
	clock := &svcClock{now: noon}
	dep := &fakeDeployer{}
	h := startWith(t, dep, func(c *service.Config) { c.Git = nil }, withDocker(&fakeDocker{}), withClock(clock, nil))
	dir := localRepo(t, map[string]string{
		"web/compose.yaml": confCompose, "web/.env": "WEB_VERSION=1.2.3\n", "web/old.txt": "old\n",
		"db/compose.yaml": webCompose, "db/.env": "WEB_VERSION=1.0.0\n",
	})
	if _, err := h.sourceAdd(dir); err != nil {
		t.Fatal(err)
	}
	head := headOf(t, dir)
	clock.Advance(time.Hour)
	h.waitStatus("web deployed", func(st control.StatusResult) bool {
		s := stacksNamed(st, "web")
		return len(s) == 1 && s[0].Deployed == head
	})
	writeTree(t, dir, map[string]string{"web/.env": "WEB_VERSION=9.9.9\n", "db/.env": "WEB_VERSION=2.0.0\n",
		"web/secrets.env": "TOKEN=demo\n"})
	if err := os.Remove(filepath.Join(dir, "web/old.txt")); err != nil {
		t.Fatal(err)
	}

	res, err := h.commitPaths(dir, head, "web/old.txt", "web/.env")
	if err != nil || res.Commit != headOf(t, dir) || res.Commit == head || !slices.Equal(res.Committed, []string{"web/old.txt", "web/.env"}) {
		t.Fatalf("commit = %+v, %v", res, err)
	}
	out, err := exec.Command("git", "-C", dir, "show", "--name-status", "--format=%B", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"web/.env", "web/old.txt", "by ", "M\tweb/.env", "D\tweb/old.txt"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("the commit lacks %q: %s", want, out)
		}
	}
	if strings.Contains(string(out), "db/.env") {
		t.Errorf("the commit has more than the named paths: %s", out)
	}
	// the other changes are still local changes, and the host's own file is untouched
	if res, _ := h.changes(dir); len(res.Changes) != 1 {
		t.Fatalf("changes after the commit: %+v", res.Changes)
	}
	if got := fileText(t, filepath.Join(dir, "web/secrets.env")); got != "TOKEN=demo\n" {
		t.Fatalf("secrets.env = %q", got)
	}
	clock.Advance(time.Hour)
	h.waitStatus("the run of the new commit", func(st control.StatusResult) bool {
		s := stacksNamed(st, "web")
		return len(s) == 1 && s[0].Deployed == res.Commit && s[0].ActiveRun == "" && s[0].LastRun == "succeeded"
	})
	// db keeps a change that is not committed, so only web ran again
	if n := dep.pullsOf("db"); n != 1 {
		t.Fatalf("db pulled %d times", n)
	}
}

func TestCommitRefusesADeviatingListAMovedHeadAndAStackThatDoesNotLoad(t *testing.T) {
	h, dir := changesHarness(t)
	head := headOf(t, dir)
	writeTree(t, dir, map[string]string{"web/.env": "WEB_VERSION=9.9.9\n", "web/secrets.env": "TOKEN=demo\n", "web/data/new.bin": "y"})

	for _, bad := range []string{"web/secrets.env", "web/data/new.bin", "web/missing.txt", "../db/.env", "web", ""} {
		_, err := h.commitPaths(dir, head, "web/.env", bad)
		wantCode(t, err, control.CodeNotChanged)
	}
	_, err := h.commitPaths(dir, strings.Repeat("a", 40), "web/.env")
	wantCode(t, err, control.CodeHeadMoved)
	_, err = h.commitPaths(dir, "main", "web/.env")
	wantCode(t, err, control.CodeInvalid)
	_, err = h.commitPaths(dir, head)
	wantCode(t, err, control.CodeInvalid)
	_, err = h.commitPaths(dir+"-other", head, "web/.env")
	wantCode(t, err, control.CodeNoSuchSource)
	if headOf(t, dir) != head {
		t.Fatal("a refused commit moved the branch")
	}

	writeTree(t, dir, map[string]string{"web/compose.yaml": "services: [broken\n"})
	_, err = h.commitPaths(dir, head, "web/.env", "web/compose.yaml")
	wantCode(t, err, control.CodeUnreadable)
	if headOf(t, dir) != head {
		t.Fatal("a commit of a stack that does not load moved the branch")
	}
}

func TestCommitIsRootOnly(t *testing.T) {
	p := &peers{}
	var seen atomic.Pointer[control.Registry]
	h, dir := changesHarness(t, withPeers(t, p, &seen))
	head := headOf(t, dir)
	writeTree(t, dir, map[string]string{"web/.env": "WEB_VERSION=9.9.9\n"})

	p.as(exampleAdmin)
	_, err := h.commitPaths(dir, head, "web/.env")
	wantCode(t, err, control.CodeNotRoot)
	if headOf(t, dir) != head {
		t.Fatal("a commit without root moved the branch")
	}
	c, ok := seen.Load().Lookup(control.SourceCommitCommand)
	if !ok || c.Class != control.ClassChange || c.Scope != control.ScopeDeploy {
		t.Fatalf("source.commit = %+v, %v", c, ok)
	}
	p.as(0)
}
