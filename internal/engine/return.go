package engine

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/castrowithcee/qatlas-host/internal/stacks"
)

// ReturnData says what a return does with the data of the stack.
type ReturnData string

const (
	// DataRestore restores the backup the target update made; everything written since is lost.
	DataRestore ReturnData = "restore"
	// DataKeep leaves the data as it is.
	DataKeep ReturnData = "keep"
)

// Refusals of CheckReturn and Return, besides those of an update, before anything changes. ErrBackupMissing
// is also one.
var (
	ErrReturnData      = errors.New("the data choice must be restore or keep")
	ErrNotReturnable   = errors.New("the run is not a successful update of the stack's versions")
	ErrNothingToReturn = errors.New("the stack already has the versions from before that update")
	ErrNoCurrentBackup = errors.New("the stack has no backup rules, so its current data cannot be saved before the restore")
)

// ReturnPlan is what a return does: the variables it sets, from the value the stack has to the value before
// the target run's update, and, with DataRestore, the backup it restores.
type ReturnPlan struct {
	Run    string
	Steps  []VersionStep
	Data   ReturnData
	Backup string
}

// ReturnResult is the outcome of a return that was started. Commit is the return's pushed commit, or the
// deployed one when it failed before its push.
type ReturnResult struct {
	ReturnPlan
	RunID, Commit, Result, Error string
}

// returnJob is what a return run does beyond a secured run: the variables its commit sets and the backup it
// restores after the commit, empty for DataKeep.
type returnJob struct {
	changes []VersionStep
	restore string
	// message, when set, is the commit message instead of the return's.
	message string
}

// CheckReturn tells whether the stack can be returned as Return would, refusing it as Return does, and returns
// the plan. It changes nothing and holds nothing.
func (e *Engine) CheckReturn(ctx context.Context, name, runID string, data ReturnData) (ReturnPlan, error) {
	if e.cfg.Deployer == nil {
		return ReturnPlan{}, ErrNoDeployer
	}
	b, err := e.cfg.Source()
	if err != nil {
		return ReturnPlan{}, err
	}
	if err := e.lockCommand(ctx); err != nil {
		return ReturnPlan{}, err
	}
	defer e.lock.Unlock()
	st, head, err := e.updatable(ctx, b, name)
	if err != nil {
		return ReturnPlan{}, err
	}
	e.mu.Lock()
	_, busy := e.active[name]
	closed := e.closed
	e.mu.Unlock()
	switch {
	case closed:
		return ReturnPlan{}, ErrStopped
	case busy:
		return ReturnPlan{}, ErrRunActive
	}
	if _, err := e.updateBase(ctx, b, name, st.Folder, head, true); err != nil {
		return ReturnPlan{}, err
	}
	return e.returnPlan(ctx, b, st, runID, data)
}

// Return sets the stack back to the image versions it had before the update run runID, as a run of its own with
// trigger return under the stack's slot: stop, backup of the current data, a commit that sets exactly the
// version variables that update changed to their value before it, pushed like an update's, with DataRestore
// the update's backup restored, then pull, up and check. A failing run is rolled back like any run, with the
// backup of the current data and a revert of the return's commit. After a success the stack is pinned, so no
// window, poll or webhook moves it on; no observation time begins.
//
// runID must be a successful update step of the stack (ErrNoSuchRun, ErrNotReturnable); a stack the update
// left on other versions than the ones it returns to keeps the variables the update did not change. With
// DataRestore a backup that is gone is refused with ErrBackupMissing and a stack without backup rules with
// ErrNoCurrentBackup. ErrNothingToReturn when the stack has those versions already. The stack is refused as
// Update refuses it, except that a pinned stack is allowed. The run goes on in the caller's goroutine under
// the run time limit and independent of ctx.
func (e *Engine) Return(ctx context.Context, name, runID string, data ReturnData) (ReturnResult, error) {
	if e.cfg.Deployer == nil {
		return ReturnResult{}, ErrNoDeployer
	}
	b, err := e.cfg.Source()
	if err != nil {
		return ReturnResult{}, err
	}
	var (
		st       stacks.Stack
		plan     ReturnPlan
		deployed string
	)
	err = func() error {
		if err := e.lockCommand(ctx); err != nil {
			return err
		}
		defer e.lock.Unlock()
		var head string
		var err error
		if st, head, err = e.updatable(ctx, b, name); err != nil {
			return err
		}
		if err := e.take(name, "return"); err != nil {
			return err
		}
		if deployed, err = e.updateBase(ctx, b, name, st.Folder, head, true); err == nil {
			plan, err = e.returnPlan(ctx, b, st, runID, data)
		}
		if err != nil {
			e.free(name, true)
		}
		return err
	}()
	if err != nil {
		return ReturnResult{}, err
	}
	held, slotFree := true, false
	defer func() {
		if !slotFree {
			e.unslot(name)
		}
		e.finish(held)
	}()

	res := ReturnResult{ReturnPlan: plan}
	r, err := e.stepRun(b, st, deployed, TriggerReturn)
	if err != nil {
		res.Error = "the run log cannot be created"
		e.cfg.Logger.Error("return not started", "stack", name, "error", err)
		return res, nil
	}
	r.held, r.change, r.back = held, &plan.Steps[0], &returnJob{changes: plan.Steps}
	if data == DataRestore {
		r.back.restore = plan.Backup
	}
	out := r.perform()
	held = r.held
	rec := r.record(out, true)
	slotFree = true
	e.notify(Event{Kind: rec.Result, Stack: rec.Stack, Run: rec.ID, Commit: rec.Commit, Error: rec.Error})
	res.RunID, res.Commit, res.Result, res.Error = rec.ID, rec.Commit, rec.Result, rec.Error
	return res, nil
}

// returnPlan checks the target run and works out the versions to set. The caller holds the working-copy lock.
func (e *Engine) returnPlan(ctx context.Context, b Binding, st stacks.Stack, runID string, data ReturnData) (ReturnPlan, error) {
	if data != DataRestore && data != DataKeep {
		return ReturnPlan{}, ErrReturnData
	}
	runs, err := e.Runs(st.Name)
	if err != nil {
		return ReturnPlan{}, err
	}
	i := slices.IndexFunc(runs, func(r RunRecord) bool { return r.ID == runID })
	if i < 0 {
		return ReturnPlan{}, ErrNoSuchRun
	}
	target := runs[i]
	if !changesVersions(target.Trigger) || target.Result != ResultSucceeded {
		return ReturnPlan{}, ErrNotReturnable
	}
	steps, err := e.returnSteps(ctx, b, st, target.Commit)
	if err != nil {
		return ReturnPlan{}, err
	}
	plan := ReturnPlan{Run: target.ID, Steps: steps, Data: data}
	if data == DataKeep {
		return plan, nil
	}
	if target.Backup == "" {
		return ReturnPlan{}, fmt.Errorf("%w: the update made none", ErrBackupMissing)
	}
	if err := e.backupPresent(ctx, st, target.Backup); err != nil {
		return ReturnPlan{}, err
	}
	if p, ok := e.cfg.Deployer.(BackupPlanner); ok && !p.WillBackup(st) {
		return ReturnPlan{}, ErrNoCurrentBackup
	}
	plan.Backup = target.Backup
	return plan, nil
}

// returnSteps compares the .env of the update's commit with its parent's: every image version variable it
// changed goes back to the parent's value, unless the stack has that value already.
func (e *Engine) returnSteps(ctx context.Context, b Binding, st stacks.Stack, commit string) ([]VersionStep, error) {
	wc := b.Copy
	parent, err := wc.ParentOf(ctx, commit)
	if err != nil {
		return nil, fmt.Errorf("read the update's commit: %w", err)
	}
	path := st.RepoPath(envName)
	var env [2]map[string]string
	for i, c := range []string{parent, commit} {
		content, err := wc.FileAt(ctx, c, path)
		if err != nil {
			return nil, fmt.Errorf("read %s of the update: %w", envName, err)
		}
		if env[i], err = stacks.ParseEnv(content); err != nil {
			return nil, fmt.Errorf("read %s of the update: %w", envName, e.errText(err))
		}
	}
	var steps []VersionStep
	changed := false
	for _, img := range st.Images {
		if img.Source != stacks.SourceEnv || slices.ContainsFunc(steps, func(s VersionStep) bool { return s.Variable == img.Var }) {
			continue
		}
		before, had := env[0][img.Var]
		after, has := env[1][img.Var]
		if !had || !has || before == after || before == "" {
			continue
		}
		changed = true
		if img.Value != before {
			steps = append(steps, VersionStep{Variable: img.Var, From: img.Value, To: before})
		}
	}
	switch {
	case !changed:
		return nil, ErrNotReturnable
	case len(steps) == 0:
		return nil, ErrNothingToReturn
	}
	return steps, nil
}

func (e *Engine) errText(err error) error { return errors.New(e.text(err.Error())) }

// guardRestore stops a return with DataRestore before its commit when the current data was not backed up: the
// restore would overwrite it for good. noData says why no backup was made.
func (r *run) guardRestore(backupID, noData string) error {
	if r.back == nil || r.back.restore == "" || backupID != "" {
		return nil
	}
	if noData == "" {
		noData = "no backup was made"
	}
	return fmt.Errorf("the current data is not backed up (%s), so it cannot be overwritten by the restore", noData)
}

// restoreTarget puts the data back the way the target update's backup has it. The stack is stopped, as the
// backup step required.
func (r *run) restoreTarget(ctx context.Context, st stacks.Stack) error {
	if r.back.restore == "" {
		return nil
	}
	d := r.e.cfg.Deployer
	return r.step("restore", func() error {
		r.log.Printf("backup %s", r.back.restore)
		return d.Restore(ctx, st, r.back.restore, r.log)
	})
}
