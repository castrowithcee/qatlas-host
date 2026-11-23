package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/gitrepo"
)

const twoVarEnv = "APP_VERSION=1.0.0\nDB_VERSION=5.0.0\nOTHER=kept\n"

// backupOf names the backup of a commit by the end of its id, so every deployed commit has its own.
func backupOf(deployed string) string { return "backup-" + deployed[len(deployed)-4:] }

// returnRig deploys web with two image variables at commit 1 and updates APP_VERSION 1.0.0 to 1.1.0 (run a,
// commit 2001), then DB_VERSION 5.0.0 to 5.1.0 (run b, commit 2002), then APP_VERSION to 1.2.0 (run c, commit
// 2003).
func returnRig(t *testing.T) (r *rig, a, b, c string) {
	t.Helper()
	r = newRig(t, "web")
	body := "services:\n  app:\n    image: registry.example.test/demo/app:${APP_VERSION}\n  db:\n    image: registry.example.test/demo/db:${DB_VERSION}\n"
	for name, content := range map[string]string{"compose.yaml": body, ".env": twoVarEnv} {
		if err := os.WriteFile(filepath.Join(r.wc, "web", name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	r.copy.files[commit(1)] = map[string][]byte{"web/.env": []byte(twoVarEnv)}
	r.dep.backupName = backupOf
	r.deployAll()
	var ids []string
	for _, s := range []VersionStep{
		{Variable: "APP_VERSION", From: "1.0.0", To: "1.1.0"},
		{Variable: "DB_VERSION", From: "5.0.0", To: "5.1.0"},
		{Variable: "APP_VERSION", From: "1.1.0", To: "1.2.0"},
	} {
		res := r.update(s)
		if res[0].Result != ResultSucceeded {
			t.Fatalf("update %+v: %+v", s, res)
		}
		ids = append(ids, res[0].Run)
	}
	r.resetSteps()
	return r, ids[0], ids[1], ids[2]
}

func (r *rig) doReturn(run string, data ReturnData) (ReturnResult, error) {
	r.t.Helper()
	return r.eng.Return(context.Background(), "web", run, data)
}

func TestReturnTakesTheVersionsFromTheParentOfTheRunCommit(t *testing.T) {
	r, a, _, _ := returnRig(t)
	events := r.withEvents()
	plan, err := r.eng.CheckReturn(context.Background(), "web", a, DataKeep)
	want := []VersionStep{{Variable: "APP_VERSION", From: "1.2.0", To: "1.0.0"}}
	if err != nil || !slices.Equal(plan.Steps, want) || plan.Backup != "" {
		t.Fatalf("plan = %+v, %v", plan, err)
	}
	res, err := r.doReturn(a, DataKeep)
	if err != nil || res.Result != ResultSucceeded || res.Commit != commit(2004) || res.RunID == "" || !slices.Equal(res.Steps, want) {
		t.Fatalf("result = %+v, %v", res, err)
	}
	// DB_VERSION was changed by a later update and stays
	if got := r.env(); got != "APP_VERSION=1.0.0\nDB_VERSION=5.1.0\nOTHER=kept\n" {
		t.Fatalf("env = %q", got)
	}
	c := r.copy.commits[len(r.copy.commits)-1]
	if c.Path != "web/.env" || c.Message != "Return web APP_VERSION from 1.2.0 to 1.0.0" || c.Base != commit(2003) {
		t.Fatalf("commit = %+v", c)
	}
	wantSteps(t, r, "stop", "backup", "pull", "up", "check")
	st := r.stack("web")
	if st.Deployed != commit(2004) || st.LastRun.Trigger != TriggerReturn || st.LastRun.Result != ResultSucceeded ||
		st.LastRun.Backup != backupOf(commit(2003)) || st.Observe != nil || st.Pin == nil || st.Paused != nil || st.FailedCommit != "" {
		t.Fatalf("web = %+v / %+v", st, st.LastRun)
	}
	if ev := events(); len(ev) != 1 || ev[0].Kind != ResultSucceeded || ev[0].Run != res.RunID || ev[0].Commit != commit(2004) {
		t.Fatalf("events = %+v", ev)
	}
	if snap, _ := r.eng.Snapshot(); len(snap.Active) != 0 {
		t.Fatalf("still active: %v", snap.Active)
	}
	// pinned: an update is refused, and the stack list says so
	if _, err := r.eng.Update(context.Background(), "web", []VersionStep{{Variable: "APP_VERSION", From: "1.0.0", To: "1.1.0"}}); !errors.Is(err, ErrPinned) {
		t.Fatalf("update after the return: %v", err)
	}
	if v, _ := r.eng.WindowEligibility("web"); v.Allowed || !slices.Contains(v.Reasons, BlockPin) {
		t.Fatalf("verdict = %+v", v)
	}
}

func TestReturnCheckNamesOnlyImageVersions(t *testing.T) {
	other := "example-other-value-" + t.Name()
	r, a, _, _ := returnRig(t)
	// The value differs from the one the update's parent has, so an unfiltered plan would carry it.
	env := strings.Replace(r.env(), "OTHER=kept", "OTHER="+other, 1)
	if err := os.WriteFile(filepath.Join(r.wc, "web", ".env"), []byte(env), 0o644); err != nil {
		t.Fatal(err)
	}
	plan, err := r.eng.CheckReturn(context.Background(), "web", a, DataKeep)
	if err != nil || len(plan.Steps) != 1 || plan.Steps[0].Variable != "APP_VERSION" {
		t.Fatalf("plan = %+v, %v", plan, err)
	}
	if got := fmt.Sprintf("%+v", plan); strings.Contains(got, other) {
		t.Fatalf("plan holds the value: %s", got)
	}
}

func TestReturnToAnOlderRunUsesItsOwnParent(t *testing.T) {
	r, _, b, _ := returnRig(t)
	res, err := r.doReturn(b, DataKeep)
	if err != nil || res.Result != ResultSucceeded {
		t.Fatalf("result = %+v, %v", res, err)
	}
	if got := r.env(); got != "APP_VERSION=1.2.0\nDB_VERSION=5.0.0\nOTHER=kept\n" {
		t.Fatalf("env = %q", got)
	}
}

func TestReturnWithDataRestoresTheBackupOfTheTargetRunAfterTheCurrentBackup(t *testing.T) {
	r, a, _, _ := returnRig(t)
	res, err := r.doReturn(a, DataRestore)
	if err != nil || res.Result != ResultSucceeded || res.Backup != backupOf(commit(1)) {
		t.Fatalf("result = %+v, %v", res, err)
	}
	wantSteps(t, r, "has "+backupOf(commit(1)), "stop", "backup", "restore "+backupOf(commit(1)), "pull", "up", "check")
	if !strings.Contains(r.runLog(r.stack("web")), "step restore") {
		t.Fatal("the log has no restore step")
	}
	if got := r.dep.backedUp[len(r.dep.backedUp)-1]; got != commit(2003) {
		t.Fatalf("the current data was backed up at %s", got)
	}
}

func TestReturnKeepingDataRestoresNothing(t *testing.T) {
	r, a, _, _ := returnRig(t)
	if _, err := r.doReturn(a, DataKeep); err != nil {
		t.Fatal(err)
	}
	for _, s := range r.dep.stepsOf("web") {
		if strings.HasPrefix(s, "restore") || strings.HasPrefix(s, "has") {
			t.Fatalf("step %q", s)
		}
	}
}

func TestReturnRefusalsChangeNothing(t *testing.T) {
	ctx := context.Background()
	for name, tc := range map[string]struct {
		prepare func(r *rig, a, b string) (run string)
		data    ReturnData
		want    error
	}{
		"backup gone": {func(r *rig, a, _ string) string { r.dep.gone = []string{backupOf(commit(1))}; return a }, DataRestore, ErrBackupMissing},
		"other trigger": {func(r *rig, _, _ string) string {
			// the first run of the stack is the poll that deployed it
			runs, _ := r.eng.Runs("web")
			return runs[len(runs)-1].ID
		}, DataKeep, ErrNotReturnable},
		"unknown run":     {func(*rig, string, string) string { return "20260101T000000Z-00000000" }, DataKeep, ErrNoSuchRun},
		"no data choice":  {func(_ *rig, a, _ string) string { return a }, "", ErrReturnData},
		"no backup rules": {func(r *rig, a, _ string) string { r.dep.noRules = true; return a }, DataRestore, ErrNoCurrentBackup},
		"local changes": {func(r *rig, a, _ string) string {
			r.copy.set(func(f *fakeCopy) { f.changes = []gitrepo.Change{{Code: " M", Path: "web/.env"}} })
			return a
		}, DataKeep, ErrLocalChanges},
		"never deployed": {func(r *rig, a, _ string) string { writeStack(r.t, r.wc, "fresh"); return a }, DataKeep, ErrNotDeployed},
		"undeployed state": {func(r *rig, a, _ string) string {
			r.copy.set(func(f *fakeCopy) {
				f.head = commit(9)
				f.folders[[2]string{commit(2003), commit(9)}] = []string{"web"}
			})
			return a
		}, DataKeep, ErrUndeployed},
		"paused": {func(r *rig, a, _ string) string {
			_, err := r.eng.store.Update(func(s State) (State, error) {
				cur := s.Stacks["web"]
				cur.Paused = &Pause{Time: r.clock.Now(), Run: "20260102T030405Z-0000abcd"}
				s.Stacks["web"] = cur
				return s, nil
			})
			if err != nil {
				r.t.Fatal(err)
			}
			return a
		}, DataKeep, ErrPaused},
		"stopped": {func(r *rig, a, _ string) string {
			_, err := r.eng.store.Update(func(s State) (State, error) {
				cur := s.Stacks["web"]
				cur.Stopped = &Hold{Since: r.clock.Now()}
				s.Stacks["web"] = cur
				return s, nil
			})
			if err != nil {
				r.t.Fatal(err)
			}
			return a
		}, DataKeep, ErrStackStopped},
	} {
		t.Run(name, func(t *testing.T) {
			r, a, b, _ := returnRig(t)
			run := tc.prepare(r, a, b)
			stack := "web"
			if name == "never deployed" {
				stack = "fresh"
			}
			commits, pushed := len(r.copy.commits), len(r.copy.pushed)
			res, err := r.eng.Return(ctx, stack, run, tc.data)
			if !errors.Is(err, tc.want) || res.RunID != "" {
				t.Fatalf("return: %v %+v, want %v", err, res, tc.want)
			}
			if _, err := r.eng.CheckReturn(ctx, stack, run, tc.data); !errors.Is(err, tc.want) {
				t.Fatalf("check: %v, want %v", err, tc.want)
			}
			for _, s := range r.dep.stepsOf("web") {
				if !strings.HasPrefix(s, "has") {
					t.Fatalf("a step ran: %v", r.dep.stepsOf("web"))
				}
			}
			if len(r.copy.commits) != commits || len(r.copy.pushed) != pushed || r.stack("web").Pin != nil {
				t.Fatal("something changed")
			}
			if snap, _ := r.eng.Snapshot(); len(snap.Active) != 0 {
				t.Fatalf("slot kept: %v", snap.Active)
			}
		})
	}
}

func TestReturnRefusesARunOfAnotherTriggerAndAReturnRun(t *testing.T) {
	r, a, _, _ := returnRig(t)
	if _, err := r.eng.Trigger(context.Background(), "web"); err != nil {
		t.Fatal(err)
	}
	r.eng.Wait()
	manual := r.stack("web").LastRun
	if manual.Trigger != TriggerManual {
		t.Fatalf("run = %+v", manual)
	}
	res, err := r.doReturn(a, DataKeep)
	if err != nil || res.Result != ResultSucceeded {
		t.Fatalf("return: %+v, %v", res, err)
	}
	if _, err := r.eng.SetPin(context.Background(), "web", false); err != nil {
		t.Fatal(err)
	}
	r.resetSteps()
	commits := len(r.copy.commits)
	for _, id := range []string{manual.ID, res.RunID} {
		if _, err := r.doReturn(id, DataKeep); !errors.Is(err, ErrNotReturnable) {
			t.Fatalf("return to %s: %v", id, err)
		}
	}
	// the stack has the versions before that update already
	if _, err := r.doReturn(a, DataKeep); !errors.Is(err, ErrNothingToReturn) {
		t.Fatalf("second return: %v", err)
	}
	if len(r.dep.stepsOf("web")) != 0 || len(r.copy.commits) != commits {
		t.Fatal("something ran")
	}
}

func TestReturnIsAllowedForAPinnedStackAndRefusedWhileARunIsActive(t *testing.T) {
	r, a, _, _ := returnRig(t)
	if _, err := r.eng.SetPin(context.Background(), "web", true); err != nil {
		t.Fatal(err)
	}
	r.dep.block, r.dep.entered = make(chan struct{}), make(chan struct{}, 1)
	if _, err := r.eng.Trigger(context.Background(), "web"); err != nil {
		t.Fatal(err)
	}
	<-r.dep.entered
	_, cerr := r.eng.CheckReturn(context.Background(), "web", a, DataKeep)
	_, rerr := r.doReturn(a, DataKeep)
	close(r.dep.block)
	r.eng.Wait()
	if !errors.Is(cerr, ErrRunActive) || !errors.Is(rerr, ErrRunActive) {
		t.Fatalf("check %v, return %v", cerr, rerr)
	}
	pinned := r.stack("web").Pin
	if res, err := r.doReturn(a, DataKeep); err != nil || res.Result != ResultSucceeded {
		t.Fatalf("return of a pinned stack: %+v, %v", res, err)
	}
	if got := r.stack("web").Pin; got == nil || !got.Since.Equal(pinned.Since) {
		t.Fatalf("pin = %+v, was %+v", got, pinned)
	}
}

func TestFailedReturnRestoresTheCurrentDataRevertsAndDoesNotPin(t *testing.T) {
	for name, tc := range map[string]struct {
		failStep string
		steps    []string
	}{
		"check": {"check", []string{"stop", "backup", "restore " + backupOf(commit(1)), "pull", "up", "check",
			"stop", "restore " + backupOf(commit(2003)), "up", "check"}},
		"restore": {"restore " + backupOf(commit(1)), []string{"stop", "backup", "restore " + backupOf(commit(1)),
			"stop", "restore " + backupOf(commit(2003)), "up", "check"}},
	} {
		t.Run(name, func(t *testing.T) {
			r, a, _, _ := returnRig(t)
			events := r.withEvents()
			r.dep.fail(tc.failStep, errBoom)
			res, err := r.doReturn(a, DataRestore)
			if err != nil || res.Result != ResultRolledBack || !strings.Contains(res.Error, "boom") {
				t.Fatalf("result = %+v, %v", res, err)
			}
			steps := r.dep.stepsOf("web")
			if !slices.Equal(steps[1:], tc.steps) {
				t.Fatalf("steps = %q\nwant    %q", steps[1:], tc.steps)
			}
			if rv := r.copy.reverts; len(rv) != 1 || rv[0].Deployed != commit(2003) || rv[0].Failed != commit(2004) {
				t.Fatalf("reverts = %+v", rv)
			}
			st := r.stack("web")
			if st.Pin != nil || st.Paused != nil || st.LastRun.Trigger != TriggerReturn || st.LastRun.Result != ResultRolledBack ||
				st.Deployed != commit(1001) || st.FailedCommit != commit(2004) || r.env() != "APP_VERSION=1.2.0\nDB_VERSION=5.1.0\nOTHER=kept\n" {
				t.Fatalf("web = %+v / %+v, env %q", st, st.LastRun, r.env())
			}
			if ev := events(); len(ev) != 1 || ev[0].Kind != ResultRolledBack {
				t.Fatalf("events = %+v", ev)
			}
		})
	}
}

func TestFailedReturnBeforeItsPushStartsTheStateAgain(t *testing.T) {
	r, a, _, _ := returnRig(t)
	r.copy.commitErrs = []error{errBoom}
	res, err := r.doReturn(a, DataRestore)
	if err != nil || res.Result != ResultRolledBack {
		t.Fatalf("result = %+v, %v", res, err)
	}
	// the data was never touched, so the rollback restores none
	for _, s := range r.dep.stepsOf("web") {
		if strings.HasPrefix(s, "restore") {
			t.Fatalf("restored: %v", r.dep.stepsOf("web"))
		}
	}
	if len(r.copy.reverts) != 0 || r.stack("web").Pin != nil || r.stack("web").Deployed != commit(2003) {
		t.Fatalf("web = %+v", r.stack("web"))
	}
}

func TestReturnWithDataNeedsTheCurrentBackup(t *testing.T) {
	r, a, _, _ := returnRig(t)
	r.dep.lateNoRules = true // the backup answers ErrNoBackup although one was planned
	res, err := r.doReturn(a, DataRestore)
	if err != nil || res.Result != ResultRolledBack || !strings.Contains(res.Error, "not backed up") {
		t.Fatalf("result = %+v, %v", res, err)
	}
	for _, s := range r.dep.stepsOf("web") {
		if strings.HasPrefix(s, "restore") {
			t.Fatalf("restored: %v", r.dep.stepsOf("web"))
		}
	}
	if r.copy.called("push") != 3 || r.stack("web").Pin != nil {
		t.Fatalf("calls %v", r.copy.calls)
	}
}

func TestReturnWithDataRefusesAnUpdateThatMadeNoBackup(t *testing.T) {
	r := updateRig(t)
	r.copy.files[commit(1)] = map[string][]byte{"web/.env": []byte(demoEnv)}
	r.dep.noRules = true
	res := r.update(step("1.0.0", "1.1.0"))
	r.dep.noRules = false
	r.resetSteps()
	if _, err := r.doReturn(res[0].Run, DataRestore); !errors.Is(err, ErrBackupMissing) {
		t.Fatalf("return: %v", err)
	}
	if len(r.dep.stepsOf("web")) != 0 || r.stack("web").Pin != nil {
		t.Fatal("something ran")
	}
	// keeping the data works
	if got, err := r.doReturn(res[0].Run, DataKeep); err != nil || got.Result != ResultSucceeded {
		t.Fatalf("return: %+v, %v", got, err)
	}
}
