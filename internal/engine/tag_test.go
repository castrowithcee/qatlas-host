package engine

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/castrowithcee/qatlas-host/internal/gitrepo"
	"github.com/castrowithcee/qatlas-host/internal/stacks"
)

var errTagRefused = errors.New("tags are protected on the demo remote")

// tagRig is a rollbackRig whose engine names the host as a long mixed-case name.
func tagRig(t *testing.T) *rig {
	t.Helper()
	r := newRig(t, "web")
	r.cfg.Host = "Demo01.example.test"
	r.eng = r.start()
	r.deployAll()
	r.copy.set(func(f *fakeCopy) {
		f.remote = commit(2)
		f.folders[[2]string{commit(1), commit(2)}] = []string{"web"}
		f.calls = nil
	})
	return r
}

func TestSuccessfulRunSetsTheTag(t *testing.T) {
	r := tagRig(t)
	first := r.stack("web")
	if first.LastRun.Tag != "qhost/demo01/web/"+first.LastRun.ID || first.DeployedTag != first.LastRun.Tag {
		t.Fatalf("first run: %+v", first)
	}
	r.poll()
	st := r.stack("web")
	want := gitrepo.RunTag("demo01", "web", st.LastRun.ID)
	if st.LastRun.Tag != want || st.DeployedTag != want || st.Deployed != commit(2) {
		t.Fatalf("web = %+v / %+v", st, st.LastRun)
	}
	if got := r.copy.tags[len(r.copy.tags)-1]; got != (tagCall{want, commit(2)}) {
		t.Fatalf("tag call = %+v", got)
	}
	if hasFinding(st, FindingTagRejected) {
		t.Fatal("finding without a failure")
	}
	// The backup of the run names the tag of the state it saves.
	if got := r.dep.backupTags; len(got) != 1 || got[0] != first.LastRun.Tag {
		t.Fatalf("backup tags = %q, want %q", got, first.LastRun.Tag)
	}
	if text := r.runLog(st); !strings.Contains(text, want) {
		t.Fatalf("log lacks the tag:\n%s", text)
	}
}

func TestTagFailureKeepsTheSuccessAndMakesAFinding(t *testing.T) {
	r := tagRig(t)
	events := r.withEvents()
	r.copy.set(func(f *fakeCopy) { f.tagErrs = []error{errTagRefused} })
	r.poll()
	wantSteps(t, r, "stop", "backup", "pull", "up", "check")
	st := r.stack("web")
	if st.LastRun.Result != ResultSucceeded || st.Deployed != commit(2) || st.LastRun.Tag != "" || st.DeployedTag != "" || st.Paused != nil {
		t.Fatalf("web = %+v / %+v", st, st.LastRun)
	}
	if !hasFinding(st, FindingTagRejected) {
		t.Fatalf("no finding: %+v", st.Findings)
	}
	if ev := events(); len(ev) != 1 || ev[0].Kind != ResultSucceeded {
		t.Fatalf("events = %+v", ev)
	}
	// The next poll replaces the poll's findings but keeps this one.
	r.poll()
	if !hasFinding(r.stack("web"), FindingTagRejected) {
		t.Fatal("a poll dropped the finding")
	}
	// The next run whose tag goes through clears it.
	r.copy.set(func(f *fakeCopy) {
		f.remote = commit(3)
		f.folders[[2]string{commit(2), commit(3)}] = []string{"web"}
	})
	r.poll()
	st = r.stack("web")
	if hasFinding(st, FindingTagRejected) || st.LastRun.Tag == "" || st.DeployedTag != st.LastRun.Tag {
		t.Fatalf("web = %+v / %+v", st, st.LastRun)
	}
}

func TestRollbackTagsTheRevertCommit(t *testing.T) {
	r := tagRig(t)
	r.dep.fail("check", errBoom)
	r.poll()
	wantSteps(t, r, "stop", "backup", "pull", "up", "check", "stop", "restore "+backupID, "up", "check")
	st := wantRolledBack(t, r, commit(1001))
	want := gitrepo.RunTag("demo01", "web", st.LastRun.ID)
	if st.LastRun.Tag != want || st.DeployedTag != want {
		t.Fatalf("tag = %q / %q, want %q", st.LastRun.Tag, st.DeployedTag, want)
	}
	if got := r.copy.tags[len(r.copy.tags)-1]; got != (tagCall{want, commit(1001)}) {
		t.Fatalf("tag call = %+v", got)
	}
}

func TestTagFailureLeavesTheRollbackAlone(t *testing.T) {
	r := tagRig(t)
	r.copy.set(func(f *fakeCopy) { f.tagErrs = []error{errTagRefused} })
	r.dep.fail("check", errBoom)
	r.poll()
	wantSteps(t, r, "stop", "backup", "pull", "up", "check", "stop", "restore "+backupID, "up", "check")
	st := wantRolledBack(t, r, commit(1001))
	if st.LastRun.Tag != "" || !hasFinding(st, FindingTagRejected) {
		t.Fatalf("web = %+v / %+v", st, st.LastRun)
	}
	if !slices.Equal(r.copy.pushed, []string{commit(1001)}) {
		t.Fatalf("pushed = %v", r.copy.pushed)
	}
}

func TestFailedAndPausedRunsSetNoTag(t *testing.T) {
	r := tagRig(t)
	n := len(r.copy.tags)
	r.dep.fail("stop", errBoom)
	r.poll()
	if r.stack("web").LastRun.Result != ResultFailed || len(r.copy.tags) != n {
		t.Fatalf("tags = %+v", r.copy.tags)
	}
}

func TestStaleRunIDs(t *testing.T) {
	ids := []string{"20260101T000000Z-00000001", "20260110T000000Z-00000002", "20260119T000000Z-00000003", "20260120T000000Z-00000004"}
	now := time.Date(2026, 1, 20, 12, 0, 0, 0, time.UTC)
	cases := map[string]struct {
		rules *stacks.Backup
		want  []string
	}{
		"no rules keeps 50": {nil, nil},
		"keep 2":            {&stacks.Backup{Keep: 2}, ids[:2]},
		"keep 1":            {&stacks.Backup{Keep: 1}, ids[:3]},
		"max age 5 days":    {&stacks.Backup{MaxAgeDays: 5}, ids[:2]},
		"max age 1 day":     {&stacks.Backup{MaxAgeDays: 1}, ids[:3]},
		"both limits":       {&stacks.Backup{Keep: 3, MaxAgeDays: 5}, ids[:2]},
	}
	for name, c := range cases {
		got := staleRunIDs(ids, c.rules, now)
		slices.Sort(got)
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: got %v, want %v", name, got, c.want)
		}
	}
	if got := staleRunIDs(ids[3:], &stacks.Backup{Keep: 1, MaxAgeDays: 1}, now.AddDate(1, 0, 0)); got != nil {
		t.Errorf("the newest tag was chosen: %v", got)
	}
	many := make([]string, 53)
	for i := range many {
		many[i] = "20260101T0000" + string(rune('0'+i/10)) + string(rune('0'+i%10)) + "Z-00000000"
	}
	got := staleRunIDs(many, nil, now)
	slices.Sort(got)
	if !slices.Equal(got, many[:3]) {
		t.Errorf("default keep: got %v", got)
	}
}

func TestPruneIsAskedAfterThePushedTag(t *testing.T) {
	r := tagRig(t)
	r.copy.set(func(f *fakeCopy) { f.pruneIDs, f.pruneCalls = []string{"20200101T000000Z-00000000"}, nil })
	r.poll()
	if got := r.copy.pruneCalls; len(got) != 1 || got[0] != "Demo01.example.test/web" {
		t.Fatalf("prune calls = %v", got)
	}
}

func TestPruneRefusalMakesAFindingWithoutFailingTheRun(t *testing.T) {
	r := tagRig(t)
	r.copy.set(func(f *fakeCopy) { f.pruneErrs = []error{errTagRefused} })
	r.poll()
	st := r.stack("web")
	if st.LastRun.Result != ResultSucceeded || st.LastRun.Tag == "" || !hasFinding(st, FindingTagRejected) {
		t.Fatalf("web = %+v / %+v", st, st.LastRun)
	}
}

func TestNoPruneWhenTheTagWasNotPushed(t *testing.T) {
	r := tagRig(t)
	r.copy.set(func(f *fakeCopy) { f.tagErrs = []error{errTagRefused}; f.pruneCalls = nil })
	r.poll()
	if len(r.copy.pruneCalls) != 0 {
		t.Fatalf("prune calls = %v", r.copy.pruneCalls)
	}
}
