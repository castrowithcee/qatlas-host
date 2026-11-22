package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/engine"
	"github.com/castrowithcee/qatlas-host/internal/stacks"
	"github.com/castrowithcee/qatlas-host/internal/window"
)

// UpdateNotice tells that the maintenance window or the regular registry check found an update for a stack in
// manual mode, which it leaves to the operator. Items are the variables with an update.
type UpdateNotice struct {
	Stack string
	Items []engine.UpdateItem
}

// windowService runs the work of each opening of the maintenance window: a registry check of every stack,
// then the updates of the stacks in automatic mode, one after another, through the path 'qhost stack update' takes.
type windowService struct {
	stackUpdater
	sched *window.Scheduler
}

// stackUpdater reports and updates the stacks a scheduled registry check found updates for: the maintenance
// window or the regular check. label begins its log lines.
type stackUpdater struct {
	upd    *updatesService
	log    *slog.Logger
	label  string
	notify func(UpdateNotice)
}

func newWindowService(stateDir, hostFile string, hostPoll time.Duration, upd *updatesService, clock window.Clock, log *slog.Logger, notify func(UpdateNotice)) (*windowService, error) {
	w := &windowService{stackUpdater: stackUpdater{upd: upd, log: log, label: "maintenance window", notify: notify}}
	sched, err := window.New(window.Config{StateDir: stateDir, HostFile: hostFile, HostPoll: hostPoll, Job: w.run, Clock: clock, Logger: log})
	if err != nil {
		return nil, err
	}
	w.sched = sched
	return w, nil
}

func registerWindowCommands(r *control.Registry, w *windowService) error {
	if err := r.Register(control.WindowShowCommand, reading(w.handleShow)); err != nil {
		return err
	}
	if err := r.Register(control.WindowSetCommand, changing(w.handleSet)); err != nil {
		return err
	}
	return r.Register(control.WindowResetCommand, changing(w.handleReset))
}

// handleReset removes the own window of this service.
func (w *windowService) handleReset(_ context.Context, raw json.RawMessage) (any, error) {
	if err := decodeArgs(raw, &struct{}{}); err != nil {
		return nil, err
	}
	st, err := w.sched.Reset()
	if err != nil {
		w.log.Error("window setting not stored", "error", err)
		return nil, errWindowUnreadable
	}
	return windowResult(st), nil
}

var errWindowUnreadable = control.NewError(control.CodeInternal,
	"the stored window setting cannot be read; see the service log")

func (w *windowService) handleShow(_ context.Context, raw json.RawMessage) (any, error) {
	if err := decodeArgs(raw, &struct{}{}); err != nil {
		return nil, err
	}
	st, err := w.sched.Status()
	if err != nil {
		w.log.Error("window setting not readable", "error", err)
		return nil, errWindowUnreadable
	}
	return windowResult(st), nil
}

// handleSet validates every part before anything is stored; the texts of the answers are fixed.
func (w *windowService) handleSet(_ context.Context, raw json.RawMessage) (any, error) {
	var a control.WindowSetArgs
	if err := decodeArgs(raw, &a); err != nil {
		return nil, err
	}
	c, err := windowChange(a)
	var st window.Status
	if err == nil {
		st, err = w.sched.Set(c)
	}
	var ie *window.InvalidError
	switch {
	case errors.As(err, &ie):
		return nil, control.NewError(control.CodeInvalid, ie.Text)
	case err != nil:
		w.log.Error("window setting not stored", "error", err)
		return nil, errWindowUnreadable
	}
	return windowResult(st), nil
}

// windowChange parses the parts a request names.
func windowChange(a control.WindowSetArgs) (window.Change, error) {
	return window.ParseChange(a.Days, a.Start, a.Duration, a.Zone)
}

// status is the window part of the status answer; nil when the setting cannot be read.
func (w *windowService) status() *control.WindowResult {
	st, err := w.sched.Status()
	if err != nil {
		return nil
	}
	res := windowResult(st)
	return &res
}

func windowResult(st window.Status) control.WindowResult {
	return control.WindowResult{
		Days: st.Window.Days.Names(), Start: window.FormatStart(st.Window.Start),
		Duration: window.FormatDuration(st.Window.Duration), Zone: st.Window.Zone,
		Origin: string(st.Origin), Warning: st.Warning,
		Open: st.Open, OpenUntil: st.OpenUntil, NextOpening: st.Next, LastRun: st.LastRun, Running: st.Running,
	}
}

// run is the work of one opening. Only stacks with the update trigger window are handled; the others are
// skipped without a notice, with a debug log line. Every such stack is checked at the registries and the result stored, as
// 'qhost stack updates --refresh' does, also for a paused or pinned stack, so an available update stays visible.
// A stack in manual mode is only reported. The stacks in automatic mode are updated in name order, each once
// the one before is done; a stack is skipped, with a log line that says why, when its marks forbid it, when it
// has nothing to update, or when 'qhost stack update' would refuse it. Once the window has closed nothing new
// begins: no further stack and no further version step; a step that has begun runs to its end.
func (w *windowService) run(ctx context.Context, open func() bool) {
	list, err := w.upd.loadStacks("")
	if err != nil {
		w.log.Warn("maintenance window: the stacks cannot be read; nothing is checked or updated", "error", err)
		return
	}
	list = slices.DeleteFunc(list, func(st stacks.Stack) bool {
		if trigger, _ := st.UpdateTrigger(); trigger != stacks.TriggerWindow {
			w.log.Debug("maintenance window: stack skipped", "stack", st.Name, "reason", "the update trigger is "+trigger)
			return true
		}
		return false
	})
	cctx, cancel := context.WithTimeout(ctx, control.UpdatesRefreshTimeout)
	fresh, err := w.upd.detector.Check(cctx, list)
	cancel()
	if err != nil {
		w.log.Warn("maintenance window: the registry check did not finish; nothing is updated", "error", err)
		return
	}
	for _, st := range list {
		if check, ok := fresh[st.Name]; ok {
			if err := w.upd.eng.RecordUpdates(st.Name, check); err != nil {
				w.log.Warn("maintenance window: the registry check cannot be stored", "stack", st.Name, "error", err)
			}
		}
	}
	var updated []string
	for _, st := range list {
		if ctx.Err() != nil {
			return
		}
		switch {
		case st.Err != nil:
			w.skip(st.Name, "the stack cannot be read")
			continue
		case st.EnvErr != nil:
			w.skip(st.Name, "the stack is deploy-only because its .env cannot be read")
			continue
		case st.UpdateMode() == stacks.ModeManual:
			w.report(st.Name, fresh[st.Name])
			continue
		case st.UpdateMode() != stacks.ModeAutomatic:
			continue
		}
		if !open() {
			w.log.Info("maintenance window: stack waits for the next window", "stack", st.Name, "reason", "the window has closed")
			continue
		}
		if w.update(ctx, st.Name, fresh[st.Name], open) {
			updated = append(updated, st.Name)
		}
	}
	w.upd.recheck(ctx, updated)
}

func (u stackUpdater) skip(name, reason string) {
	u.log.Info(u.label+": stack skipped", "stack", name, "reason", reason)
}

// report emits the notice of a stack in manual mode with an available update. It sends nothing anywhere.
func (u stackUpdater) report(name string, check engine.UpdateCheck) {
	var items []engine.UpdateItem
	for _, it := range check.Items {
		if it.Status == engine.UpdateAvailable {
			items = append(items, it)
			u.log.Info("update available", "event", "update-available", "stack", name, "mode", stacks.ModeManual,
				"variable", it.Variable, "current", it.Current, "available", it.Selected)
		}
	}
	if len(items) > 0 && u.notify != nil {
		u.notify(UpdateNotice{Stack: name, Items: items})
	}
}

// update updates one stack in automatic mode and tells whether a step succeeded. open tells whether a new step
// may begin; the regular check passes one that always allows it.
func (u stackUpdater) update(ctx context.Context, name string, check engine.UpdateCheck, open func() bool) bool {
	eng := u.upd.eng
	verdict, err := eng.WindowEligibility(name)
	switch {
	case err != nil:
		u.skip(name, "the engine state cannot be read")
		return false
	case !verdict.Allowed:
		u.skip(name, "marked "+strings.Join(verdict.Reasons, ", "))
		return false
	}
	if reason := nothingToUpdate(check); reason != "" {
		u.skip(name, reason)
		return false
	}
	pctx, cancel := context.WithTimeout(ctx, control.UpdatesRefreshTimeout)
	defer cancel()
	st, err := eng.CheckUpdate(pctx, name)
	var steps []engine.VersionStep
	if err == nil {
		steps, err = u.upd.detector.Plan(pctx, st, "", "")
	}
	switch {
	case err != nil:
		u.skip(name, refusal(pctx, err))
		return false
	case len(steps) == 0:
		u.skip(name, "nothing to update")
		return false
	}
	results, err := eng.UpdateWhile(ctx, name, steps, open)
	if err != nil {
		u.skip(name, refusal(ctx, err))
		return false
	}
	succeeded, failed := false, false
	for _, r := range results {
		switch {
		case r.Run != "":
			u.log.Info(u.label+": update step finished", "stack", name, "variable", r.Variable,
				"from", r.From, "to", r.To, "run", r.Run, "result", r.Result)
			succeeded = succeeded || r.Result == engine.ResultSucceeded
			failed = failed || r.Result != engine.ResultSucceeded
		case failed:
			u.log.Info(u.label+": update step not attempted after a failed step", "stack", name,
				"variable", r.Variable, "to", r.To)
		default:
			u.log.Info(u.label+": update step waits for the next window", "stack", name,
				"variable", r.Variable, "to", r.To, "reason", "the window has closed")
		}
	}
	return succeeded
}

// nothingToUpdate says why a stored check offers no update, empty when it offers one.
func nothingToUpdate(check engine.UpdateCheck) string {
	reason := "nothing to update"
	for _, it := range check.Items {
		switch it.Status {
		case engine.UpdateAvailable:
			return ""
		case engine.UpdateError:
			reason = "the registry check failed for a variable"
		}
	}
	return reason
}

// refusal is the fixed text of why a stack cannot be updated.
func refusal(ctx context.Context, err error) string {
	var ce *control.Error
	if errors.As(updateAnswer(ctx, err), &ce) {
		return ce.Message
	}
	return "the stack cannot be updated now"
}
