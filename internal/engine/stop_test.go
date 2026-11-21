package engine

import (
	"slices"
	"strings"
	"testing"
)

func TestFirstRunDoesNotStopTheStack(t *testing.T) {
	r := newRig(t, "web")
	r.poll()
	wantSteps(t, r, "pull", "up", "check")
	if r.stack("web").Deployed != commit(1) {
		t.Fatalf("web = %+v", r.stack("web"))
	}
}

func TestRunWithoutBackupRulesDoesNotStopTheStack(t *testing.T) {
	r := rollbackRig(t, "web")
	r.dep.noRules = true
	r.poll()
	wantSteps(t, r, "pull", "up", "check")
	st := r.stack("web")
	if st.Deployed != commit(2) || !strings.Contains(r.runLog(st), "none: the stack has no backup rules") {
		t.Fatalf("web = %+v\n%s", st, r.runLog(st))
	}
}

func TestRunWithBackupRulesStopsBeforeTheBackup(t *testing.T) {
	r := rollbackRig(t, "web")
	r.poll()
	wantSteps(t, r, "stop", "backup", "pull", "up", "check")
	if st := r.stack("web"); st.LastRun.Backup != backupID {
		t.Fatalf("run = %+v", st.LastRun)
	}
}

func TestUpFailureWithoutBackupRulesStopsAndStartsThePreviousState(t *testing.T) {
	r := rollbackRig(t, "web")
	r.dep.noRules = true
	r.dep.fail("up", errBoom)
	r.poll()
	wantSteps(t, r, "pull", "up", "stop", "up", "check")
	st := wantRolledBack(t, r, commit(1001))
	if !strings.Contains(r.runLog(st), "skipped: the stack has no backup rules") {
		t.Fatalf("log:\n%s", r.runLog(st))
	}
}

func TestNoBackupAfterAPredictedBackupGoesOn(t *testing.T) {
	r := rollbackRig(t, "web")
	r.dep.lateNoRules = true
	r.poll()
	if got := r.dep.stepsOf("web"); !slices.Equal(got, []string{"stop", "backup", "pull", "up", "check"}) {
		t.Fatalf("steps = %q", got)
	}
	if st := r.stack("web"); st.Deployed != commit(2) || st.LastRun.Backup != "" {
		t.Fatalf("web = %+v", st)
	}
}
