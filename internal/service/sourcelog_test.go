package service_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/gitrepo"
)

func (h *harness) sourceLog(dir string, limit int) (control.SourceLogResult, error) {
	var res control.SourceLogResult
	err := h.call(control.SourceLogCommand, control.SourceLogArgs{Path: dir, Limit: limit}, &res)
	return res, err
}

func (h *harness) sourceShow(dir, commit string) (control.SourceShowResult, error) {
	var res control.SourceShowResult
	err := h.call(control.SourceShowCommand, control.SourceShowArgs{Path: dir, Commit: commit}, &res)
	return res, err
}

func TestLogMasksForeignTextAndListsPathsWithKinds(t *testing.T) {
	h, dir := changesHarness(t)
	if err := os.Remove(filepath.Join(dir, "web/old.txt")); err != nil {
		t.Fatal(err)
	}
	writeTree(t, dir, map[string]string{"web/.env": "WEB_VERSION=2.0.0\n", "web/new.txt": "n\n", "web/bad\x1bname.txt": "b\n"})
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "--author=Mal\x1b[31mlory <m@example.test>", "-m", "fix\x1b]0;x\x07 it\n\nbody")

	res, err := h.sourceLog(dir, 0)
	if err != nil || res.Path != dir || len(res.Commits) != 2 || res.Truncated {
		t.Fatalf("log = %+v, %v", res, err)
	}
	c := res.Commits[0]
	if c.ID != headOf(t, dir) || c.Author != "Mal [31mlory" || c.Subject != "fix ]0;x it" || strings.ContainsAny(c.Author+c.Subject, "\x1b\x07") {
		t.Fatalf("commit = %+v", c)
	}
	if _, err := time.Parse(time.RFC3339, c.Date); err != nil {
		t.Fatalf("date %q: %v", c.Date, err)
	}
	want := map[string]string{"web/.env": "modified", "web/new.txt": "new", "web/old.txt": "deleted", "web/bad?name.txt": "new"}
	if len(c.Paths) != len(want) {
		t.Fatalf("paths = %+v", c.Paths)
	}
	for _, p := range c.Paths {
		if want[p.Path] != p.Kind {
			t.Errorf("path %+v, want kind %q", p, want[p.Path])
		}
	}
}

func TestLogLimitDefaultsToTwentyAndIsBounded(t *testing.T) {
	h, dir := changesHarness(t)
	for i := range 22 {
		gitIn(t, dir, "commit", "-q", "--allow-empty", "-m", fmt.Sprintf("empty %d", i))
	}
	if res, err := h.sourceLog(dir, 0); err != nil || len(res.Commits) != control.DefaultLogLimit {
		t.Fatalf("default = %d commits, %v", len(res.Commits), err)
	}
	if res, err := h.sourceLog(dir, 3); err != nil || len(res.Commits) != 3 || res.Commits[0].Subject != "empty 21" {
		t.Fatalf("limit 3 = %+v, %v", res, err)
	}
	if res, err := h.sourceLog(dir, control.MaxLogLimit); err != nil || len(res.Commits) != 23 {
		t.Fatalf("max = %d commits, %v", len(res.Commits), err)
	}
	for _, limit := range []int{control.MaxLogLimit + 1, -1} {
		_, err := h.sourceLog(dir, limit)
		wantCode(t, err, control.CodeInvalid)
	}
	_, err := h.sourceLog(dir+"-other", 1)
	wantCode(t, err, control.CodeNoSuchSource)
}

func TestLogCutsTheListOfPathsOfACommit(t *testing.T) {
	h, dir := changesHarness(t)
	files := map[string]string{}
	for i := range gitrepo.MaxLogPaths + 1 {
		files[fmt.Sprintf("web/gen/f%03d.txt", i)] = "x\n"
	}
	writeTree(t, dir, files)
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", "many")
	res, err := h.sourceLog(dir, 1)
	if err != nil || len(res.Commits) != 1 || len(res.Commits[0].Paths) != gitrepo.MaxLogPaths || !res.Commits[0].PathsTruncated {
		t.Fatalf("log = %d paths, truncated %v, %v", len(res.Commits[0].Paths), res.Commits[0].PathsTruncated, err)
	}
}

func TestShowRefusesAnUnreachableCommitAndCutsALargeDiff(t *testing.T) {
	h, dir := changesHarness(t)
	writeTree(t, dir, map[string]string{"web/big.txt": strings.Repeat("0123456789abcdef\n", gitrepo.MaxDiffBytes/16)})
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", "big")
	res, err := h.sourceShow(dir, headOf(t, dir))
	if err != nil || !res.Truncated || len(res.Diff) > gitrepo.MaxDiffBytes || !strings.Contains(res.Diff, "+++ b/web/big.txt") {
		t.Fatalf("show = truncated %v, %d bytes, %v", res.Truncated, len(res.Diff), err)
	}
	_, err = h.sourceShow(dir, strings.Repeat("a", 40))
	wantCode(t, err, control.CodeNotFound)
	_, err = h.sourceShow(dir, "HEAD")
	wantCode(t, err, control.CodeInvalid)
	_, err = h.sourceShow(dir+"-other", headOf(t, dir))
	wantCode(t, err, control.CodeNoSuchSource)
}

func TestShowIsRootOnlyAndTheLogIsForAdministrators(t *testing.T) {
	p := &peers{}
	var seen atomic.Pointer[control.Registry]
	h, dir := changesHarness(t, withPeers(t, p, &seen))
	head := headOf(t, dir)

	p.as(exampleAdmin)
	if res, err := h.sourceLog(dir, 1); err != nil || len(res.Commits) != 1 {
		t.Fatalf("an administrator reads the log: %+v, %v", res, err)
	}
	_, err := h.sourceShow(dir, head)
	wantCode(t, err, control.CodeNotRoot)
	for name, want := range map[string]control.Class{control.SourceLogCommand: control.ClassRead, control.SourceShowCommand: control.ClassChange} {
		c, ok := seen.Load().Lookup(name)
		if !ok || c.Class != want || c.Scope != control.ScopeDeploy {
			t.Errorf("%s = %+v, %v", name, c, ok)
		}
	}
	p.as(0)
	if res, err := h.sourceShow(dir, head); err != nil || !strings.Contains(res.Diff, "+++ b/web/compose.yaml") {
		t.Fatalf("root show = %+v, %v", res, err)
	}
}

// folderReads are the read commands that name a bound stack folder.
func folderReads(dir string) map[string][]any {
	return map[string][]any{
		control.SourceChangesCommand: {control.SourceChangesArgs{Path: dir}},
		control.SourceLogCommand:     {control.SourceLogArgs{Path: dir}},
	}
}
