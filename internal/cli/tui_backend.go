package cli

import (
	"context"
	"errors"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/castrowithcee/qatlas-kit/cli"

	"github.com/castrowithcee/qatlas-host/internal/client"
	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/diagnose"
	"github.com/castrowithcee/qatlas-host/internal/tui"
	"github.com/castrowithcee/qatlas-host/internal/window"
)

// engineBackend is the tui.Backend over every deploy service of the host: the one of the system engine, when it
// is set up, and the instance of each rootless engine. It routes with the rules of the commands: a call about a
// stack or a stack folder goes to the service that has it (locate), a call about a whole engine goes to the
// engine tui.WithEngine names, else to the system engine or the only instance (instanceEngine), and the lists
// merge every service. A call never goes to another service than the responsible one, and one that cannot be
// reached is not replaced.
type engineBackend struct {
	version string
}

var _ tui.Backend = (*engineBackend)(nil)

func newEngineBackend() *engineBackend { return &engineBackend{} }

func (b *engineBackend) withVersion(v string) *engineBackend {
	b.version = v
	return b
}

func (b *engineBackend) channel(t engineTarget) *tui.ChannelBackend {
	return tui.NewBackend(t.endpoint()).WithVersion(b.version)
}

// routeError is a refusal of the router as the interface shows it: the message, then the next step.
type routeError struct{ f *cli.Failure }

func (e routeError) Error() string {
	if e.f.NextStep == "" {
		return e.f.Message
	}
	return e.f.Message + ". " + e.f.NextStep
}

func (e routeError) Unwrap() error { return e.f.Cause }

func routeFailure(err error) error {
	if err == nil {
		return nil
	}
	var f *cli.Failure
	if errors.As(err, &f) {
		return routeError{f}
	}
	return err
}

// stackItems turns names into the items locate searches for.
func stackItems(stacks []string) []engineItem {
	items := make([]engineItem, len(stacks))
	for i, s := range stacks {
		items[i] = stackItem(s)
	}
	return items
}

// locateTarget finds the service that holds every item, or the one tui.WithEngine names.
func (b *engineBackend) locateTarget(ctx context.Context, items ...engineItem) (engineTarget, error) {
	t, err := folders.locate(ctx, io.Discard, tui.EngineOf(ctx), items...)
	return t, routeFailure(err)
}

func (b *engineBackend) locateBackend(ctx context.Context, items ...engineItem) (*tui.ChannelBackend, error) {
	t, err := b.locateTarget(ctx, items...)
	if err != nil {
		return nil, err
	}
	return b.channel(t), nil
}

// instanceBackend finds the service of a call about a whole engine.
func (b *engineBackend) instanceBackend(ctx context.Context) (*tui.ChannelBackend, error) {
	t, err := folders.instanceEngine(tui.EngineOf(ctx))
	if err != nil {
		return nil, routeFailure(err)
	}
	return b.channel(t), nil
}

func onItems[T any](b *engineBackend, ctx context.Context, items []engineItem, call func(*tui.ChannelBackend) (T, error)) (T, error) {
	c, err := b.locateBackend(ctx, items...)
	if err != nil {
		var zero T
		return zero, err
	}
	return call(c)
}

func onStack[T any](b *engineBackend, ctx context.Context, stack string, call func(*tui.ChannelBackend) (T, error)) (T, error) {
	return onItems(b, ctx, []engineItem{stackItem(stack)}, call)
}

func onInstance[T any](b *engineBackend, ctx context.Context, call func(*tui.ChannelBackend) (T, error)) (T, error) {
	c, err := b.instanceBackend(ctx)
	if err != nil {
		var zero T
		return zero, err
	}
	return call(c)
}

// onFolder is for a call about a stack folder; an empty dir is a call about the whole engine. A key may be
// asked for before its folder is bound, so with byDefault a folder no service has follows the rule of the
// whole engine.
func onFolder[T any](b *engineBackend, ctx context.Context, dir string, byDefault bool, call func(*tui.ChannelBackend) (T, error)) (T, error) {
	if dir == "" {
		return onInstance(b, ctx, call)
	}
	item := folderItem(dir)
	item.byDefault = byDefault
	return onItems(b, ctx, []engineItem{item}, call)
}

// engineTargetsOf is the services a read asks: the one tui.WithEngine names, else every service. A host
// without a deploy service has the system engine, whose call says that it is not set up.
func (b *engineBackend) engineTargetsOf(ctx context.Context) ([]engineTarget, error) {
	if e := tui.EngineOf(ctx); e != "" {
		t, err := folders.namedEngine(e)
		if err != nil {
			return nil, routeFailure(err)
		}
		return []engineTarget{t}, nil
	}
	targets, err := folders.engineTargets()
	if err != nil {
		return nil, routeFailure(err)
	}
	if len(targets) == 0 {
		targets = []engineTarget{{}}
	}
	return targets, nil
}

// overviewLimit bounds one read: a host with several services gives way early to one that hangs.
func overviewLimit(many bool) time.Duration {
	if many {
		return peerLimit
	}
	return client.CallTimeout
}

func (b *engineBackend) Engines(ctx context.Context) ([]string, error) {
	targets, err := folders.engineTargets()
	if err != nil {
		return nil, routeFailure(err)
	}
	ids := make([]string, len(targets))
	for i, t := range targets {
		ids[i] = t.id()
	}
	return ids, nil
}

// gathered is what the services answered to a read: the answers of those that could be asked, in the order
// of the targets, and the engines whose service could not.
type gathered[T any] struct {
	ok         []engineReply[T]
	notChecked []control.EngineNotChecked
}

// gather asks every service and merges the outcome as the overviews of the commands do: a service that cannot
// be asked is a note with its engine, a service without a bound stack folder adds nothing, and the read fails
// only when no service has an answer. A host with one service gets its answer or its error unchanged.
func gather[T any](ctx context.Context, b *engineBackend, command string, args any, limit func(many bool) time.Duration) (gathered[T], error) {
	targets, err := b.engineTargetsOf(ctx)
	if err != nil {
		return gathered[T]{}, err
	}
	ok, failed, empty := splitOverview(len(targets), askEach[T](ctx, limit(len(targets) > 1), targets, command, args))
	switch {
	case len(ok) == 0 && len(failed) == 0:
		return gathered[T]{}, empty[0].err
	case len(ok) == 0:
		return gathered[T]{}, failed[0].err
	}
	g := gathered[T]{ok: ok}
	for _, f := range failed {
		g.notChecked = append(g.notChecked, control.EngineNotChecked{Engine: f.target.id(), Reason: briefReason(f.err)})
	}
	return g, nil
}

func (b *engineBackend) Status(ctx context.Context) (control.StatusResult, error) {
	g, err := gather[control.StatusResult](ctx, b, control.StatusCommand, nil, overviewLimit)
	if err != nil {
		return control.StatusResult{}, err
	}
	// The scalars and the window are those of the first service that answered, as the first folder's are in
	// one service; the stack folders and stacks of all are listed, each with its engine.
	res := g.ok[0].value
	res.Engine.Sources, res.Engine.Stacks = nil, nil
	for _, r := range g.ok {
		id := r.target.id()
		for _, src := range r.value.Engine.Sources {
			src.Engine = id
			res.Engine.Sources = append(res.Engine.Sources, src)
		}
		for _, st := range r.value.Engine.Stacks {
			st.Engine = id
			res.Engine.Stacks = append(res.Engine.Stacks, st)
		}
	}
	res.NotChecked = g.notChecked
	return res, nil
}

func (b *engineBackend) Stacks(ctx context.Context) (control.StacksResult, error) {
	g, err := gather[control.StacksResult](ctx, b, control.StacksListCommand, nil, overviewLimit)
	if err != nil {
		return control.StacksResult{}, err
	}
	res := control.StacksResult{Dir: g.ok[0].value.Dir, NotChecked: g.notChecked}
	for _, r := range g.ok {
		for _, st := range r.value.Stacks {
			st.Engine = r.target.id()
			res.Stacks = append(res.Stacks, st)
		}
	}
	return res, nil
}

func (b *engineBackend) SourceList(ctx context.Context) (control.SourceListResult, error) {
	g, err := gather[control.SourceListResult](ctx, b, control.SourceListCommand, nil, overviewLimit)
	if err != nil {
		return control.SourceListResult{}, err
	}
	res := control.SourceListResult{NotChecked: g.notChecked}
	for _, r := range g.ok {
		for _, src := range r.value.Sources {
			src.Engine = r.target.id()
			res.Sources = append(res.Sources, src)
		}
	}
	return res, nil
}

// Runs lists the runs of one stack from the service that has it, or those of every stack from every service,
// newest first.
func (b *engineBackend) Runs(ctx context.Context, stack string) (control.RunsResult, error) {
	if stack != "" {
		t, err := b.locateTarget(ctx, stackItem(stack))
		if err != nil {
			return control.RunsResult{}, err
		}
		res, err := b.channel(t).Runs(ctx, stack)
		return tagRuns(res, t.id()), err
	}
	g, err := gather[control.RunsResult](ctx, b, control.RunsCommand, control.RunsArgs{}, overviewLimit)
	if err != nil {
		return control.RunsResult{}, err
	}
	res := control.RunsResult{NotChecked: g.notChecked}
	for _, r := range g.ok {
		res.Runs = append(res.Runs, tagRuns(r.value, r.target.id()).Runs...)
	}
	sort.SliceStable(res.Runs, func(i, j int) bool { return res.Runs[i].Started.After(res.Runs[j].Started) })
	return res, nil
}

func tagRuns(res control.RunsResult, engine string) control.RunsResult {
	for i := range res.Runs {
		res.Runs[i].Engine = engine
	}
	return res
}

// Check checks one stack at the service that has it, or every stack of every service.
func (b *engineBackend) Check(ctx context.Context, stack string) (control.StacksCheckResult, error) {
	if stack != "" {
		t, err := b.locateTarget(ctx, stackItem(stack))
		if err != nil {
			return control.StacksCheckResult{}, err
		}
		res, err := b.channel(t).Check(ctx, stack)
		for i := range res.Stacks {
			res.Stacks[i].Engine = t.id()
		}
		return res, err
	}
	g, err := gather[control.StacksCheckResult](ctx, b, control.StacksCheckCommand, control.StacksCheckArgs{}, overviewLimit)
	if err != nil {
		return control.StacksCheckResult{}, err
	}
	res := control.StacksCheckResult{Dir: g.ok[0].value.Dir, NotChecked: g.notChecked}
	for _, r := range g.ok {
		for _, st := range r.value.Stacks {
			st.Engine = r.target.id()
			res.Stacks = append(res.Stacks, st)
		}
		res.Errors += r.value.Errors
		res.Warnings += r.value.Warnings
	}
	return res, nil
}

// Updates reads the update detection of one stack from the service that has it, or of every stack of every
// service.
func (b *engineBackend) Updates(ctx context.Context, args control.UpdatesArgs) (control.UpdatesResult, error) {
	if args.Stack != "" {
		t, err := b.locateTarget(ctx, stackItem(args.Stack))
		if err != nil {
			return control.UpdatesResult{}, err
		}
		res, err := b.channel(t).Updates(ctx, args)
		for i := range res.Stacks {
			res.Stacks[i].Engine = t.id()
		}
		return res, err
	}
	limit := client.CallTimeout
	if args.Refresh {
		limit += control.UpdatesRefreshTimeout
	}
	if args.Changelog {
		limit += control.ChangelogTimeout
	}
	g, err := gather[control.UpdatesResult](ctx, b, control.UpdatesListCommand, args, func(bool) time.Duration { return limit })
	if err != nil {
		return control.UpdatesResult{}, err
	}
	res := control.UpdatesResult{NotChecked: g.notChecked}
	for _, r := range g.ok {
		for _, st := range r.value.Stacks {
			st.Engine = r.target.id()
			res.Stacks = append(res.Stacks, st)
		}
	}
	return res, nil
}

// Log reads the log of a run from the service that recorded it: it asks every service, as 'run log' does, and
// stops when several know the run.
func (b *engineBackend) Log(ctx context.Context, run string) (control.LogResult, error) {
	targets, err := b.engineTargetsOf(ctx)
	if err != nil {
		return control.LogResult{}, err
	}
	if len(targets) == 1 {
		return b.channel(targets[0]).Log(ctx, run)
	}
	known, unchecked, first := splitRun(askEach[control.LogResult](ctx, peerLimit, targets, control.LogCommand, control.LogArgs{ID: run}))
	switch {
	case len(known) > 1:
		ids := make([]string, len(known))
		for i, k := range known {
			ids[i] = k.target.id()
		}
		return control.LogResult{}, routeFailure(usageFailure("the run "+run+" exists in the engines "+strings.Join(ids, ", "),
			"Choose one engine."))
	case len(known) == 1:
		return known[0].value, nil
	case len(unchecked) == len(targets):
		return control.LogResult{}, unchecked[0].err
	case len(unchecked) > 0:
		return control.LogResult{}, routeFailure(&cli.Failure{Message: "the run " + run + " is in none of the engines that answered; " +
			uncheckedList(unchecked), NextStep: "Start that deploy service and read the log again.", Cause: unchecked[0].err})
	}
	return control.LogResult{}, first
}

func (b *engineBackend) Changelog(ctx context.Context, stack string) (control.ChangelogResult, error) {
	return onStack(b, ctx, stack, func(c *tui.ChannelBackend) (control.ChangelogResult, error) { return c.Changelog(ctx, stack) })
}

func (b *engineBackend) Backups(ctx context.Context, stack string) (control.BackupListResult, error) {
	return onStack(b, ctx, stack, func(c *tui.ChannelBackend) (control.BackupListResult, error) { return c.Backups(ctx, stack) })
}

func (b *engineBackend) Secrets(ctx context.Context, stack string) (control.SecretListResult, error) {
	return onStack(b, ctx, stack, func(c *tui.ChannelBackend) (control.SecretListResult, error) { return c.Secrets(ctx, stack) })
}

func (b *engineBackend) Window(ctx context.Context) (control.WindowResult, error) {
	return onInstance(b, ctx, func(c *tui.ChannelBackend) (control.WindowResult, error) { return c.Window(ctx) })
}

func (b *engineBackend) Containers(ctx context.Context) (control.ContainerListResult, error) {
	return onInstance(b, ctx, func(c *tui.ChannelBackend) (control.ContainerListResult, error) { return c.Containers(ctx) })
}

func (b *engineBackend) Container(ctx context.Context, name string) (control.ContainerDetail, error) {
	return onInstance(b, ctx, func(c *tui.ChannelBackend) (control.ContainerDetail, error) { return c.Container(ctx, name) })
}

func (b *engineBackend) ContainerLogs(ctx context.Context, args control.ContainerLogsArgs) (control.ContainerLogsResult, error) {
	if args.Stack != "" {
		return onStack(b, ctx, args.Stack, func(c *tui.ChannelBackend) (control.ContainerLogsResult, error) { return c.ContainerLogs(ctx, args) })
	}
	return onInstance(b, ctx, func(c *tui.ChannelBackend) (control.ContainerLogsResult, error) { return c.ContainerLogs(ctx, args) })
}

func (b *engineBackend) ServiceLog(ctx context.Context, args control.ServiceLogArgs) (control.ServiceLogResult, error) {
	return onInstance(b, ctx, func(c *tui.ChannelBackend) (control.ServiceLogResult, error) { return c.ServiceLog(ctx, args) })
}

func (b *engineBackend) DeployLog(ctx context.Context, args control.HostLogListArgs) (control.DeployLogListResult, error) {
	return onInstance(b, ctx, func(c *tui.ChannelBackend) (control.DeployLogListResult, error) { return c.DeployLog(ctx, args) })
}

func (b *engineBackend) Diagnose(ctx context.Context) *diagnose.Package {
	c, err := b.instanceBackend(ctx)
	if err != nil {
		return diagnose.Build(ctx, diagnose.Options{Version: b.version,
			Call: func(context.Context, string, any, any) error { return err }})
	}
	return c.Diagnose(ctx)
}

func (b *engineBackend) Deploy(ctx context.Context, stack string) (control.DeployResult, error) {
	return onStack(b, ctx, stack, func(c *tui.ChannelBackend) (control.DeployResult, error) { return c.Deploy(ctx, stack) })
}

// Update moves the stacks at the service that has all of them; stacks of different engines are refused.
func (b *engineBackend) Update(ctx context.Context, stacks []string) (control.UpdateRunResult, error) {
	return onItems(b, ctx, stackItems(stacks), func(c *tui.ChannelBackend) (control.UpdateRunResult, error) { return c.Update(ctx, stacks) })
}

func (b *engineBackend) Pause(ctx context.Context, stack string) (control.HoldResult, error) {
	return onStack(b, ctx, stack, func(c *tui.ChannelBackend) (control.HoldResult, error) { return c.Pause(ctx, stack) })
}

func (b *engineBackend) Unpause(ctx context.Context, stack string) (control.HoldResult, error) {
	return onStack(b, ctx, stack, func(c *tui.ChannelBackend) (control.HoldResult, error) { return c.Unpause(ctx, stack) })
}

func (b *engineBackend) Pin(ctx context.Context, stack string) (control.HoldResult, error) {
	return onStack(b, ctx, stack, func(c *tui.ChannelBackend) (control.HoldResult, error) { return c.Pin(ctx, stack) })
}

func (b *engineBackend) Unpin(ctx context.Context, stack string) (control.HoldResult, error) {
	return onStack(b, ctx, stack, func(c *tui.ChannelBackend) (control.HoldResult, error) { return c.Unpin(ctx, stack) })
}

func (b *engineBackend) Resume(ctx context.Context, stack string) (control.ResumeResult, error) {
	return onStack(b, ctx, stack, func(c *tui.ChannelBackend) (control.ResumeResult, error) { return c.Resume(ctx, stack) })
}

func (b *engineBackend) StackStart(ctx context.Context, stack string) (control.StackActionResult, error) {
	return onStack(b, ctx, stack, func(c *tui.ChannelBackend) (control.StackActionResult, error) { return c.StackStart(ctx, stack) })
}

func (b *engineBackend) StackStop(ctx context.Context, stack string) (control.StackActionResult, error) {
	return onStack(b, ctx, stack, func(c *tui.ChannelBackend) (control.StackActionResult, error) { return c.StackStop(ctx, stack) })
}

func (b *engineBackend) StackRestart(ctx context.Context, stack string) (control.StackActionResult, error) {
	return onStack(b, ctx, stack, func(c *tui.ChannelBackend) (control.StackActionResult, error) { return c.StackRestart(ctx, stack) })
}

func (b *engineBackend) ContainerStart(ctx context.Context, id string) (control.ContainerActionResult, error) {
	return onInstance(b, ctx, func(c *tui.ChannelBackend) (control.ContainerActionResult, error) { return c.ContainerStart(ctx, id) })
}

func (b *engineBackend) ContainerStop(ctx context.Context, id string) (control.ContainerActionResult, error) {
	return onInstance(b, ctx, func(c *tui.ChannelBackend) (control.ContainerActionResult, error) { return c.ContainerStop(ctx, id) })
}

func (b *engineBackend) ContainerRestart(ctx context.Context, id string) (control.ContainerActionResult, error) {
	return onInstance(b, ctx, func(c *tui.ChannelBackend) (control.ContainerActionResult, error) { return c.ContainerRestart(ctx, id) })
}

func (b *engineBackend) RepoProbe(ctx context.Context, url string) (control.RepoProbeResult, error) {
	return onInstance(b, ctx, func(c *tui.ChannelBackend) (control.RepoProbeResult, error) { return c.RepoProbe(ctx, url) })
}

func (b *engineBackend) RepoSet(ctx context.Context, args control.RepoSetArgs) (control.RepoSetResult, error) {
	return onInstance(b, ctx, func(c *tui.ChannelBackend) (control.RepoSetResult, error) { return c.RepoSet(ctx, args) })
}

func (b *engineBackend) RepoFetch(ctx context.Context, dir string) (control.RepoFetchResult, error) {
	return onFolder(b, ctx, dir, false, func(c *tui.ChannelBackend) (control.RepoFetchResult, error) { return c.RepoFetch(ctx, dir) })
}

func (b *engineBackend) KeyShow(ctx context.Context, dir string) (control.KeyResult, error) {
	return onFolder(b, ctx, dir, true, func(c *tui.ChannelBackend) (control.KeyResult, error) { return c.KeyShow(ctx, dir) })
}

func (b *engineBackend) KeyGenerate(ctx context.Context, dir string, replace bool) (control.KeyResult, error) {
	return onFolder(b, ctx, dir, true, func(c *tui.ChannelBackend) (control.KeyResult, error) { return c.KeyGenerate(ctx, dir, replace) })
}

func (b *engineBackend) KeySet(ctx context.Context, dir, key string, replace bool) (control.KeyResult, error) {
	return onFolder(b, ctx, dir, true, func(c *tui.ChannelBackend) (control.KeyResult, error) { return c.KeySet(ctx, dir, key, replace) })
}

func (b *engineBackend) Credentials(ctx context.Context) (control.CredentialListResult, error) {
	return onInstance(b, ctx, func(c *tui.ChannelBackend) (control.CredentialListResult, error) { return c.Credentials(ctx) })
}

func (b *engineBackend) CredentialSet(ctx context.Context, name, value string) error {
	_, err := onInstance(b, ctx, func(c *tui.ChannelBackend) (struct{}, error) { return struct{}{}, c.CredentialSet(ctx, name, value) })
	return err
}

func (b *engineBackend) CredentialDelete(ctx context.Context, name string) error {
	_, err := onInstance(b, ctx, func(c *tui.ChannelBackend) (struct{}, error) { return struct{}{}, c.CredentialDelete(ctx, name) })
	return err
}

func (b *engineBackend) WindowSet(ctx context.Context, args control.WindowSetArgs) (control.WindowResult, error) {
	return onInstance(b, ctx, func(c *tui.ChannelBackend) (control.WindowResult, error) { return c.WindowSet(ctx, args) })
}

func (b *engineBackend) WindowReset(ctx context.Context) (control.WindowResult, error) {
	return onInstance(b, ctx, func(c *tui.ChannelBackend) (control.WindowResult, error) { return c.WindowReset(ctx) })
}

// HostWindow reads the host window, from its file or what the state kept before it existed, as the CLI does.
func (b *engineBackend) HostWindow(_ context.Context) (control.WindowResult, error) {
	w, err := window.HostBase(windowHostFile(), stateDir())
	if err != nil {
		return control.WindowResult{}, routeFailure(err)
	}
	return control.WindowResult{Days: w.Days.Names(), Start: window.FormatStart(w.Start), Duration: window.FormatDuration(w.Duration),
		Zone: w.Zone, Origin: control.WindowOriginHost}, nil
}

// HostWindowSet writes the host window file, as 'sudo qhost window set' does; no deploy service is involved.
func (b *engineBackend) HostWindowSet(_ context.Context, args control.WindowSetArgs) (control.WindowResult, error) {
	w, err := changeHostWindow(args)
	if err != nil {
		return control.WindowResult{}, routeFailure(err)
	}
	res := control.WindowResult{Days: w.Days.Names(), Start: window.FormatStart(w.Start), Duration: window.FormatDuration(w.Duration),
		Zone: w.Zone, Origin: control.WindowOriginHost}
	if next, err := w.Next(time.Now()); err == nil {
		res.NextOpening = next.Start
	}
	return res, nil
}

func (b *engineBackend) Notifications(ctx context.Context) (control.NotifyListResult, error) {
	return onInstance(b, ctx, func(c *tui.ChannelBackend) (control.NotifyListResult, error) { return c.Notifications(ctx) })
}

func (b *engineBackend) NotifyAdd(ctx context.Context, args control.NotifyAddArgs) (control.NotifyChannelInfo, error) {
	return onInstance(b, ctx, func(c *tui.ChannelBackend) (control.NotifyChannelInfo, error) { return c.NotifyAdd(ctx, args) })
}

func (b *engineBackend) NotifyRemove(ctx context.Context, name string) error {
	_, err := onInstance(b, ctx, func(c *tui.ChannelBackend) (struct{}, error) { return struct{}{}, c.NotifyRemove(ctx, name) })
	return err
}

func (b *engineBackend) NotifyTest(ctx context.Context, name string) (control.NotifyTestResult, error) {
	return onInstance(b, ctx, func(c *tui.ChannelBackend) (control.NotifyTestResult, error) { return c.NotifyTest(ctx, name) })
}

func (b *engineBackend) SecretSet(ctx context.Context, stack, key, value string) (control.SecretResult, error) {
	return onStack(b, ctx, stack, func(c *tui.ChannelBackend) (control.SecretResult, error) { return c.SecretSet(ctx, stack, key, value) })
}

func (b *engineBackend) SecretDelete(ctx context.Context, stack, key string) (control.SecretResult, error) {
	return onStack(b, ctx, stack, func(c *tui.ChannelBackend) (control.SecretResult, error) { return c.SecretDelete(ctx, stack, key) })
}

// SourceInspect reads a folder at the service that has it bound; a folder no service has bound follows the
// rule of the whole engine.
func (b *engineBackend) SourceInspect(ctx context.Context, path, url string, include []string) (control.SourceInspectResult, error) {
	return onFolder(b, ctx, path, true, func(c *tui.ChannelBackend) (control.SourceInspectResult, error) {
		return c.SourceInspect(ctx, path, url, include)
	})
}

func (b *engineBackend) SourceCompare(ctx context.Context, args control.SourceCompareArgs) (control.SourceCompareResult, error) {
	return onFolder(b, ctx, args.Path, true, func(c *tui.ChannelBackend) (control.SourceCompareResult, error) { return c.SourceCompare(ctx, args) })
}

// SourceAdd binds a folder at the engine tui.WithEngine names, else by the rule of the whole engine; the choice
// of the engine for a new folder is the person's.
func (b *engineBackend) SourceAdd(ctx context.Context, args control.SourceAddArgs) (control.SourceAddResult, error) {
	return onInstance(b, ctx, func(c *tui.ChannelBackend) (control.SourceAddResult, error) { return c.SourceAdd(ctx, args) })
}

func (b *engineBackend) SourceRemote(ctx context.Context, args control.SourceRemoteArgs) (control.SourceRemoteResult, error) {
	return onFolder(b, ctx, args.Path, false, func(c *tui.ChannelBackend) (control.SourceRemoteResult, error) { return c.SourceRemote(ctx, args) })
}

func (b *engineBackend) SourceRemove(ctx context.Context, path string) (control.SourceRemoveResult, error) {
	return onFolder(b, ctx, path, false, func(c *tui.ChannelBackend) (control.SourceRemoveResult, error) { return c.SourceRemove(ctx, path) })
}

func (b *engineBackend) BackupRestore(ctx context.Context, stack, id string) (control.BackupRestoreResult, error) {
	return onStack(b, ctx, stack, func(c *tui.ChannelBackend) (control.BackupRestoreResult, error) {
		return c.BackupRestore(ctx, stack, id)
	})
}

func (b *engineBackend) StackSettings(ctx context.Context, stack string) (control.StackSettingsResult, error) {
	return onStack(b, ctx, stack, func(c *tui.ChannelBackend) (control.StackSettingsResult, error) { return c.StackSettings(ctx, stack) })
}

func (b *engineBackend) StackSettingsSet(ctx context.Context, stack string, settings control.StackSettings) (control.StackSettingsSetResult, error) {
	return onStack(b, ctx, stack, func(c *tui.ChannelBackend) (control.StackSettingsSetResult, error) {
		return c.StackSettingsSet(ctx, stack, settings)
	})
}

func (b *engineBackend) StackSettingsRemove(ctx context.Context, stack string) (control.StackSettingsRemoveResult, error) {
	return onStack(b, ctx, stack, func(c *tui.ChannelBackend) (control.StackSettingsRemoveResult, error) {
		return c.StackSettingsRemove(ctx, stack)
	})
}

// ProposalList merges the proposals of every deploy service, each marked with the engine of the service that
// stores it, newest first. A service that cannot be asked adds none.
func (b *engineBackend) ProposalList(ctx context.Context) (control.ProposalListResult, error) {
	g, err := gather[control.ProposalListResult](ctx, b, control.ProposalListCommand, nil, overviewLimit)
	if err != nil {
		return control.ProposalListResult{}, err
	}
	var res control.ProposalListResult
	for _, r := range g.ok {
		for _, p := range r.value.Proposals {
			p.Engine = r.target.id()
			res.Proposals = append(res.Proposals, p)
		}
		res.Unreadable = append(res.Unreadable, r.value.Unreadable...)
	}
	sort.SliceStable(res.Proposals, func(i, j int) bool { return res.Proposals[i].Submitted.After(res.Proposals[j].Submitted) })
	return res, nil
}

func (b *engineBackend) ProposalShow(ctx context.Context, id string) (control.ProposalShowResult, error) {
	return onInstance(b, ctx, func(c *tui.ChannelBackend) (control.ProposalShowResult, error) { return c.ProposalShow(ctx, id) })
}

func (b *engineBackend) ProposalApprove(ctx context.Context, args control.ProposalApproveArgs) (control.ProposalInfo, error) {
	return onInstance(b, ctx, func(c *tui.ChannelBackend) (control.ProposalInfo, error) { return c.ProposalApprove(ctx, args) })
}

func (b *engineBackend) ProposalReject(ctx context.Context, args control.ProposalRejectArgs) (control.ProposalInfo, error) {
	return onInstance(b, ctx, func(c *tui.ChannelBackend) (control.ProposalInfo, error) { return c.ProposalReject(ctx, args) })
}

func (b *engineBackend) ProposalResult(ctx context.Context, args control.ProposalResultArgs) (control.ProposalInfo, error) {
	return onInstance(b, ctx, func(c *tui.ChannelBackend) (control.ProposalInfo, error) { return c.ProposalResult(ctx, args) })
}
