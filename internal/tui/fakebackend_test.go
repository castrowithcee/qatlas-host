package tui

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/diagnose"
)

// fakeBackend answers from fixed results. A set error makes that call fail.
type fakeBackend struct {
	mu sync.Mutex
	// engines are the deploy services Engines lists, the system engine when nil; engineLog records the engine
	// every changing call was addressed to, and credsBy and notifyBy answer the reads by that engine.
	engines   []string
	engineLog []string
	// inspectEngines is the engine each inspection of a folder was addressed to, empty for none.
	inspectEngines []string
	// proposalEngines records each call about a proposal with the engine it was addressed to, as "<call>:<id>@<engine>".
	proposalEngines []string
	credsBy         map[string][]control.CredentialInfo
	// deployLogs answer DeployLog by engine; deployLogArgs records the engine of each call.
	deployLogs    map[string][]control.DeployLogEntry
	deployLogArgs []string
	notifyBy      map[string][]control.NotifyChannelInfo
	status        control.StatusResult
	stacks        control.StacksResult
	check         control.StacksCheckResult
	updates       control.UpdatesResult
	runs          control.RunsResult
	statusErr     error
	stacksErr     error
	// logs, changelogs and backups answer by run ID or stack name; an error set for the key fails that call.
	logs       map[string]string
	changelogs map[string]control.ChangelogResult
	backups    map[string]control.BackupListResult
	errs       map[string]error
	calls      map[string]int
	// acts records the changing calls in order as "<call>:<stack>"; an error set in errs for that key fails
	// the call, and updateRuns and holds answer by stack.
	acts       []string
	updateRuns map[string]control.StackUpdateRun
	holds      map[string]control.HoldResult

	// The settings: what the service holds. A key name of errs such as "probe:" fails that call.
	probe control.RepoProbeResult
	key   *control.KeyResult
	// dirKeys are the keys of the other stack folders; keyArgs records every generate and set call.
	dirKeys map[string]control.KeyResult
	keyArgs []control.KeySetArgs
	creds   []control.CredentialInfo
	window  control.WindowResult
	// windowBy, when set, answers Window and takes the changes by engine; hostWindows are the host window changes.
	windowBy    map[string]control.WindowResult
	hostWindows []control.WindowSetArgs
	notify      []control.NotifyChannelInfo
	repoSet     []control.RepoSetArgs
	// The stack folders: inspects answers by path (unknown paths are rejected), folderList is the bound list,
	// compare the answer of source.compare. fetchDirs, sourceAdds and removes record the calls in order; the
	// reads (inspect, compare, list) are only counted, so a test can tell that nothing changing was called.
	inspects   map[string]control.SourceInspectResult
	folderList control.SourceListResult
	compare    control.SourceCompareResult
	fetchDirs  []string
	sourceAdds []control.SourceAddArgs
	// compares records the comparisons asked for; remotes the source.remote calls, and remoteRes is their answer.
	compares []control.SourceCompareArgs
	// inspectIncludes records the choice of unclear files of every inspection, in order.
	inspectIncludes [][]string
	remotes         []control.SourceRemoteArgs
	removes         []string
	// host is the host side the model gets; nil gives it none.
	host *fakeFolders
	// sent are the arguments of the credential and channel writes in order, kept to check what reached the
	// service and that nothing else did.
	credSets []control.CredentialSetArgs
	adds     []control.NotifyAddArgs
	windows  []control.WindowSetArgs

	// secrets holds the key names by stack; secretSets and restores record the writes in order.
	secrets    map[string][]string
	secretSets []control.SecretSetArgs
	restores   []control.BackupRestoreArgs
	// returns records the calls of Return; returnRes, when set, is the answer.
	returns   []control.ReturnArgs
	checks    []control.ReturnArgs
	returnRes *control.ReturnRunResult

	// stackSettings holds the settings by stack; a stack without an entry is only deployed. settingsSets and
	// settingsRemoves record the writes in order; errs "settings:<stack>", "settingsset:<stack>" and
	// "settingsremove:<stack>" fail the calls.
	stackSettings   map[string]control.StackSettings
	settingsSets    []control.StackSettingsSetArgs
	settingsRemoves []string

	// containers is the list the service reads from the container engine, and containerDetails the detail by
	// name; errs "containers:" and "container:<name>" fail those calls.
	containers control.ContainerListResult
	// proposals is the list the service holds, newest first, and proposalContents the content by ID; errs
	// "proposals:" and "proposal:<id>" fail those calls. proposalActs records approve, reject and result calls.
	proposals        []control.ProposalInfo
	proposalContents map[string]string
	proposalActs     []string
	// proposalApproves, proposalRejects and proposalResults keep the arguments of those calls.
	proposalApproves []control.ProposalApproveArgs
	proposalRejects  []control.ProposalRejectArgs
	proposalResults  []control.ProposalResultArgs
	containerDetails map[string]control.ContainerDetail
	// ctNotReady is the reason a start or restart answers with as NotReady; "" means ready.
	ctNotReady string
	// ctLogLines are all container log lines in time order, ctLogPage the most lines one answer holds (zero:
	// no limit) and ctLogArgs the arguments of the calls made.
	ctLogLines []control.ContainerLogLine
	ctLogPage  int
	ctLogArgs  []control.ContainerLogsArgs
	// svcLog are the entries of the service log in time order, svcPage the most entries one answer holds (zero:
	// no limit) and svcArgs the arguments of the calls made.
	svcLog  []control.ServiceLogEntry
	svcPage int
	svcArgs []control.ServiceLogArgs
	// diagCalls counts the packages asked for; diagCall is what they are built from (nil: every part answers
	// empty) and diagGate, when set, holds the build until it is closed.
	diagCalls int
	diagCall  diagnose.Call
	diagGate  chan struct{}
}

func (f *fakeBackend) count(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls[name]++
}

func (f *fakeBackend) called(name string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[name]
}

func (f *fakeBackend) Status(context.Context) (control.StatusResult, error) {
	f.count("status")
	return f.status, f.statusErr
}

func (f *fakeBackend) Stacks(context.Context) (control.StacksResult, error) {
	f.count("stacks")
	return f.stacks, f.stacksErr
}

func (f *fakeBackend) Check(context.Context, string) (control.StacksCheckResult, error) {
	f.count("check")
	return f.check, nil
}

func (f *fakeBackend) Updates(ctx context.Context, args control.UpdatesArgs) (control.UpdatesResult, error) {
	f.count("updates")
	if args.Refresh {
		if err := f.actCtx(ctx, "refresh", "*"); err != nil {
			return control.UpdatesResult{}, err
		}
	}
	return f.updates, nil
}

func (f *fakeBackend) Runs(context.Context, string) (control.RunsResult, error) {
	f.count("runs")
	return f.runs, nil
}

// fail returns the error set for a call and key, "log:<run>", "changelog:<stack>" or "backups:<stack>".
func (f *fakeBackend) fail(call, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.errs[call+":"+key]
}

// setLog replaces the log of a run, as the service does while the run writes.
func (f *fakeBackend) setLog(run, text string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.logs == nil {
		f.logs = map[string]string{}
	}
	f.logs[run] = text
}

func (f *fakeBackend) Log(ctx context.Context, run string) (control.LogResult, error) {
	f.count("log")
	f.noteEngine(ctx, "log", run)
	if err := f.fail("log", run); err != nil {
		return control.LogResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	text, ok := f.logs[run]
	if !ok {
		return control.LogResult{}, control.NewError(control.CodeNoSuchRun, "no such run")
	}
	return control.LogResult{Text: text}, nil
}

func (f *fakeBackend) Changelog(ctx context.Context, stack string) (control.ChangelogResult, error) {
	f.count("changelog")
	f.noteEngine(ctx, "changelog", stack)
	if err := f.fail("changelog", stack); err != nil {
		return control.ChangelogResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if res, ok := f.changelogs[stack]; ok {
		return res, nil
	}
	return control.ChangelogResult{Stack: stack}, nil
}

func (f *fakeBackend) Backups(ctx context.Context, stack string) (control.BackupListResult, error) {
	f.count("backups")
	f.noteEngine(ctx, "backups", stack)
	if err := f.fail("backups", stack); err != nil {
		return control.BackupListResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return control.BackupListResult{Stack: stack, Backups: f.backups[stack].Backups}, nil
}

// act records a changing call and returns the error set for it.
func (f *fakeBackend) act(call, stack string) error {
	f.count(call)
	f.mu.Lock()
	f.acts = append(f.acts, call+":"+stack)
	f.mu.Unlock()
	return f.fail(call, stack)
}

func (f *fakeBackend) actions() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.acts...)
}

func (f *fakeBackend) Deploy(ctx context.Context, stack string) (control.DeployResult, error) {
	if err := f.actCtx(ctx, "deploy", stack); err != nil {
		return control.DeployResult{}, err
	}
	return control.DeployResult{ID: "20261031T100000Z-" + stack, Stack: stack, Commit: commitB}, nil
}

func (f *fakeBackend) Update(ctx context.Context, stacks []string) (control.UpdateRunResult, error) {
	var res control.UpdateRunResult
	f.count("update.call")
	for _, stack := range stacks {
		if err := f.actCtx(ctx, "update", stack); err != nil {
			return control.UpdateRunResult{}, err
		}
		f.mu.Lock()
		run, ok := f.updateRuns[stack]
		f.mu.Unlock()
		if !ok {
			run = control.StackUpdateRun{Name: stack, Steps: []control.UpdateStepInfo{{Variable: "APP_VERSION", From: "1.4.2",
				To: "1.5.0", Run: "20261031T100000Z-" + stack, Result: "succeeded"}}}
		}
		res.Stacks = append(res.Stacks, run)
	}
	return res, nil
}

func (f *fakeBackend) hold(call, stack string, active bool) (control.HoldResult, error) {
	if err := f.act(call, stack); err != nil {
		return control.HoldResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if res, ok := f.holds[call+":"+stack]; ok {
		return res, nil
	}
	return control.HoldResult{Stack: stack, Active: active, Changed: true}, nil
}

func (f *fakeBackend) Pause(_ context.Context, stack string) (control.HoldResult, error) {
	return f.hold("pause", stack, true)
}

func (f *fakeBackend) Unpause(_ context.Context, stack string) (control.HoldResult, error) {
	return f.hold("unpause", stack, false)
}

func (f *fakeBackend) Pin(_ context.Context, stack string) (control.HoldResult, error) {
	return f.hold("pin", stack, true)
}

func (f *fakeBackend) Unpin(_ context.Context, stack string) (control.HoldResult, error) {
	return f.hold("unpin", stack, false)
}

func (f *fakeBackend) Resume(ctx context.Context, stack string) (control.ResumeResult, error) {
	if err := f.actCtx(ctx, "resume", stack); err != nil {
		return control.ResumeResult{}, err
	}
	return control.ResumeResult{Stack: stack, Pause: &control.PauseInfo{Since: at(29, 3, 12), Run: "20261029T031000Z-0000000a"}}, nil
}

func (f *fakeBackend) stackAction(call, stack string, stopped bool) (control.StackActionResult, error) {
	if err := f.act(call, stack); err != nil {
		return control.StackActionResult{}, err
	}
	return control.StackActionResult{Stack: stack, Action: call, Stopped: stopped}, nil
}

func (f *fakeBackend) StackStart(_ context.Context, stack string) (control.StackActionResult, error) {
	return f.stackAction("start", stack, false)
}

func (f *fakeBackend) StackStop(_ context.Context, stack string) (control.StackActionResult, error) {
	return f.stackAction("stop", stack, true)
}

func (f *fakeBackend) StackRestart(_ context.Context, stack string) (control.StackActionResult, error) {
	return f.stackAction("restart", stack, false)
}

// containerAction records "<call>:<id>", where id is what the interface passed (the container ID), and answers
// with the container as it is afterwards: running after a start or restart, exited after a stop.
func (f *fakeBackend) containerAction(call, id string) (control.ContainerActionResult, error) {
	if err := f.act(call, id); err != nil {
		return control.ContainerActionResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	res := control.ContainerActionResult{}
	for _, d := range f.containerDetails {
		if d.ID == id || d.Name == id {
			res.ContainerSummary, res.Kind = d.ContainerSummary, d.Kind
		}
	}
	if call == "ctstop" {
		res.State, res.EngineState, res.UptimeSeconds, res.Health = control.ContainerStopped, "exited", 0, ""
		return res, nil
	}
	res.State, res.EngineState, res.UptimeSeconds = control.ContainerOK, "running", 2
	res.NotReady = f.ctNotReady
	if res.NotReady != "" {
		res.State, res.EngineState = control.ContainerStopped, "exited"
	}
	return res, nil
}

func (f *fakeBackend) ContainerStart(_ context.Context, id string) (control.ContainerActionResult, error) {
	return f.containerAction("ctstart", id)
}

func (f *fakeBackend) ContainerStop(_ context.Context, id string) (control.ContainerActionResult, error) {
	return f.containerAction("ctstop", id)
}

func (f *fakeBackend) ContainerRestart(_ context.Context, id string) (control.ContainerActionResult, error) {
	return f.containerAction("ctrestart", id)
}

func (f *fakeBackend) Window(ctx context.Context) (control.WindowResult, error) {
	f.count("window")
	if err := f.fail("window", EngineOf(ctx)); err != nil {
		return control.WindowResult{}, err
	}
	if err := f.fail("window", ""); err != nil {
		return control.WindowResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.windowBy != nil {
		return f.windowBy[engineOrSystem(ctx)], nil
	}
	return f.window, nil
}

func engineOrSystem(ctx context.Context) string {
	if e := EngineOf(ctx); e != "" {
		return e
	}
	return control.EngineSystem
}

func applyWindowArgs(w control.WindowResult, args control.WindowSetArgs) control.WindowResult {
	if args.Days != nil {
		w.Days = args.Days
	}
	if args.Start != "" {
		w.Start = args.Start
	}
	if args.Duration != "" {
		w.Duration = args.Duration
	}
	if args.Zone != "" {
		w.Zone = args.Zone
	}
	return w
}

func (f *fakeBackend) WindowReset(ctx context.Context) (control.WindowResult, error) {
	if err := f.actCtx(ctx, "windowreset", ""); err != nil {
		return control.WindowResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	w := f.windowBy[engineOrSystem(ctx)]
	w.Origin = control.WindowOriginHost
	w.Days, w.Start, w.Duration, w.Zone = f.window.Days, f.window.Start, f.window.Duration, f.window.Zone
	if f.windowBy != nil {
		f.windowBy[engineOrSystem(ctx)] = w
	}
	return w, nil
}

func (f *fakeBackend) HostWindow(_ context.Context) (control.WindowResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w := f.window
	w.Origin = control.WindowOriginHost
	return w, nil
}

func (f *fakeBackend) HostWindowSet(ctx context.Context, args control.WindowSetArgs) (control.WindowResult, error) {
	if err := f.actCtx(ctx, "hostwindowset", ""); err != nil {
		return control.WindowResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hostWindows = append(f.hostWindows, args)
	f.window = applyWindowArgs(f.window, args)
	for e, w := range f.windowBy {
		if w.Origin != control.WindowOriginOwn {
			f.windowBy[e] = applyWindowArgs(w, args)
		}
	}
	res := f.window
	res.Origin = control.WindowOriginHost
	return res, nil
}

// at is a fixed point in time of the fixtures.
func at(day, hour, minute int) time.Time {
	return time.Date(2026, 10, day, hour, minute, 0, 0, time.UTC)
}

const (
	commitA = "1111111111111111111111111111111111111111"
	commitB = "2222222222222222222222222222222222222222"
)

// demoBackend is a host with stacks in every state the overview shows.
func demoBackend() *fakeBackend {
	since := &control.HoldInfo{Since: at(28, 9, 0)}
	return &fakeBackend{
		status: control.StatusResult{Version: "9.9.9", StartedAt: at(27, 8, 0),
			Window: &control.WindowResult{Days: []string{"sun"}, Start: "03:00", Duration: "2h", Zone: "UTC", NextOpening: at(31, 3, 0)},
			Engine: control.EngineStatus{LastPoll: at(30, 14, 30), Stacks: []control.StackStatus{
				{Name: "demo-stack", Deployed: commitA, LastRun: "succeeded", Findings: []control.FindingInfo{}},
				{Name: "mail-relay", Deployed: commitA, Pending: commitB, LastRun: "succeeded", Findings: []control.FindingInfo{}},
				{Name: "old-tool", Deployed: commitA, Removed: true, LastRun: "succeeded",
					Findings: []control.FindingInfo{{Kind: "removed", Detail: "the folder is gone from the repository"}}},
				{Name: "photo-album", Deployed: commitA, LastRun: "rollback-failed",
					Paused:   &control.PauseInfo{Since: at(29, 3, 12), Run: "20261029T031000Z-0000000a"},
					Findings: []control.FindingInfo{{Kind: "paused", Detail: "a rollback failed"}}},
				{Name: "web-shop", Deployed: commitA, ActiveRun: "20261030T150000Z-0000000b", LastRun: "rolled-back", Findings: []control.FindingInfo{}},
				{Name: "wiki", Deployed: commitA, Pinned: since, LastRun: "succeeded", Findings: []control.FindingInfo{}},
			}}},
		stacks: control.StacksResult{Stacks: []control.StackInfo{
			{Name: "demo-stack", Update: "automatic", Images: []control.ImageInfo{{Service: "app", Image: "example/app", Var: "APP_VERSION", Version: "1.4.2"}}},
			{Name: "mail-relay", Update: "manual", Images: []control.ImageInfo{{Service: "relay", Image: "example/relay", Var: "RELAY_VERSION", Version: "3.1"}}},
			{Name: "photo-album", Update: "automatic", Images: []control.ImageInfo{{Service: "web", Image: "example/album", Var: "ALBUM_VERSION", Version: "2.0.1"}}},
			{Name: "web-shop", Update: "automatic", Images: []control.ImageInfo{
				{Service: "shop", Image: "example/shop", Var: "SHOP_VERSION", Version: "5.2.0"},
				{Service: "db", Image: "example/db", Var: "DB_VERSION", Version: "16.4"}}},
			{Name: "wiki", Update: "none", Images: []control.ImageInfo{{Service: "wiki", Image: "example/wiki", Var: "WIKI_VERSION", Version: "1.41.0"}}},
		}},
		check: control.StacksCheckResult{Stacks: []control.StackReport{
			{Stack: "demo-stack", Functions: []control.StackFunction{
				{Function: "deploy", Works: true, Reason: "Compose starts the stack from compose.yaml"},
				{Function: "update", Works: true, Mode: "automatic", Reason: "the maintenance window updates APP_VERSION by itself"},
				{Function: "rollback", Works: true, Reason: "a failed run goes back to the previous commit and restores the backup it made before the change"},
				{Function: "backup", Works: true, Reason: "a backup saves the data directories data/db"},
				{Function: "redact", Works: true, Reason: "the values of secrets.env (2 key(s)) are redacted in logs and messages"},
			}, Findings: []control.CheckFinding{
				{Stack: "demo-stack", Rule: "secrets-env-not-ignored", Severity: "warning", Cause: "secrets.env is not listed in .gitignore", Fix: "Add the line 'secrets.env' to the .gitignore."},
			}},
			{Stack: "photo-album", Findings: []control.CheckFinding{{Stack: "photo-album", Rule: "unset-image-variable", Severity: "error"}}},
			{Stack: "wiki", Functions: []control.StackFunction{
				{Function: "deploy", Works: true, Reason: "Compose starts the stack from compose.yaml"},
				{Function: "update", Mode: "none", Reason: "the image versions (WIKI_VERSION) can be updated, but updates are not switched on",
					Action: "Switch on updates in the stack's settings in the qhost TUI."},
			}, Findings: []control.CheckFinding{
				{Stack: "wiki", Rule: "duplicate-key", Severity: "warning"},
				{Stack: "wiki", Rule: "tracked-data", Severity: "warning"},
			}},
		}, Errors: 1, Warnings: 3},
		updates: control.UpdatesResult{Stacks: []control.StackUpdates{
			{Name: "demo-stack", Update: "automatic", Checked: at(30, 3, 5), Items: []control.UpdateInfo{
				{Variable: "APP_VERSION", Image: "example/app", Current: "1.4.2", Selected: "1.5.0", Status: "update"}}},
			{Name: "mail-relay", Update: "manual", Checked: at(30, 3, 5), Items: []control.UpdateInfo{
				{Variable: "RELAY_VERSION", Image: "example/relay", Current: "3.1", Selected: "3.1", Status: "current"}}},
			{Name: "photo-album", Update: "automatic", Checked: at(30, 3, 5), Items: []control.UpdateInfo{
				{Variable: "ALBUM_VERSION", Image: "example/album", Current: "2.0.1", Status: "error", Error: "registry unreachable"}}},
			{Name: "web-shop", Update: "automatic", Checked: at(30, 3, 5), Items: []control.UpdateInfo{
				{Variable: "SHOP_VERSION", Image: "example/shop", Current: "5.2.0", Selected: "5.2.0", MajorAvailable: "6.0.0", Status: "current"},
				{Variable: "DB_VERSION", Image: "example/db", Current: "16.4", Selected: "16.4", Status: "current"}}},
			{Name: "wiki", Update: "none"},
		}},
		runs: control.RunsResult{Runs: []control.RunInfo{
			{ID: "20261030T150000Z-0000000b", Stack: "web-shop", Started: at(30, 15, 0), Trigger: "manual"},
			{ID: "20261030T140200Z-0000000c", Stack: "demo-stack", Started: at(30, 14, 2), Trigger: "update", DurationMS: 48000, Result: "succeeded"},
			{ID: "20261030T120000Z-0000000d", Stack: "web-shop", Started: at(30, 12, 0), Trigger: "poll", DurationMS: 215000, Result: "rolled-back",
				Error: "health check of shop failed", Tag: "qhost/example-host/web-shop/20261030T120000Z-0000000d"},
			{ID: "20261029T031000Z-0000000a", Stack: "photo-album", Started: at(29, 3, 10), Trigger: "poll", DurationMS: 9000, Result: "rollback-failed"},
			{ID: "20261029T020000Z-00000010", Stack: "demo-stack", Started: at(29, 2, 0), Trigger: "poll", DurationMS: 850, Result: "failed",
				Error: "compose pull failed: registry unreachable"},
			{ID: "20261028T031000Z-0000000e", Stack: "mail-relay", Started: at(28, 3, 10), Trigger: "poll", DurationMS: 31000, Result: "succeeded",
				Tag: "qhost/example-host/mail-relay/20261028T031000Z-0000000e"},
			{ID: "20261027T031000Z-0000000f", Stack: "wiki", Started: at(27, 3, 10), Trigger: "manual", DurationMS: 12000, Result: "succeeded"},
		}},
		logs: map[string]string{
			"20261030T140200Z-0000000c": demoLog,
			"20261030T150000Z-0000000b": demoActiveLog,
		},
		changelogs: map[string]control.ChangelogResult{
			"demo-stack": {Stack: "demo-stack", Checked: at(30, 3, 5), Items: []control.ChangelogInfo{{
				Variable: "APP_VERSION", Current: "1.4.2", Target: "1.5.0", Source: "example/app",
				Link: "https://example.invalid/app/releases",
				Releases: []control.ChangelogRelease{
					{Version: "1.5.0", Name: "Spring release", Published: at(29, 9, 0),
						Body: "Adds a dark theme.\n\n- faster start\n- fixes the upload limit"},
					{Version: "1.4.3", Published: at(20, 9, 0), Body: "Fixes a crash on empty input."},
				}}}},
		},
		backups: map[string]control.BackupListResult{
			"demo-stack": {Stack: "demo-stack", Backups: []control.BackupInfo{
				{ID: "20261030T140100Z-0123abcd", Stack: "demo-stack", Created: at(30, 14, 1), Size: 12582912},
				{ID: "20261023T031000Z-4567cdef", Stack: "demo-stack", Created: at(23, 3, 10), Size: 11534336},
				{ID: "20261016T031000Z-89abcdef", Stack: "demo-stack", Incomplete: true},
			}},
		},
		secrets: map[string][]string{"demo-stack": {"API_SIGNING_KEY", "DB_PASSWORD"}},
		errs:    map[string]error{},
	}
}

// demoLog is the log of a finished run in the format of the service.
const demoLog = `2026-10-30T14:02:00Z step check
2026-10-30T14:02:01Z info compose file is valid
2026-10-30T14:02:02Z step backup
2026-10-30T14:02:09Z info backup 20261030T140100Z-0123abcd, 12.0 MiB
2026-10-30T14:02:10Z step deploy
2026-10-30T14:02:11Z info pull example/app:1.4.2
2026-10-30T14:02:40Z info up -d finished
2026-10-30T14:02:45Z step verify
2026-10-30T14:02:50Z info all services healthy
`

// demoActiveLog is the log of the run that is still active.
const demoActiveLog = `2026-10-30T15:00:00Z step check
2026-10-30T15:00:01Z info compose file is valid
2026-10-30T15:00:02Z step backup
2026-10-30T15:00:12Z info backup 20261030T150001Z-0123abcd, 48.0 MiB
2026-10-30T15:00:13Z step deploy
2026-10-30T15:00:14Z info pull example/shop:5.2.0
`

func (f *fakeBackend) RepoProbe(ctx context.Context, url string) (control.RepoProbeResult, error) {
	if err := f.actCtx(ctx, "probe", url); err != nil {
		return control.RepoProbeResult{}, err
	}
	return f.probe, nil
}

func (f *fakeBackend) RepoSet(ctx context.Context, args control.RepoSetArgs) (control.RepoSetResult, error) {
	if err := f.actCtx(ctx, "reposet", args.URL); err != nil {
		return control.RepoSetResult{}, err
	}
	f.mu.Lock()
	f.repoSet = append(f.repoSet, args)
	f.mu.Unlock()
	return control.RepoSetResult{URL: args.URL, Dir: args.Dir, Fingerprint: args.Fingerprint, Head: commitA, Cloned: true}, nil
}

func (f *fakeBackend) RepoFetch(ctx context.Context, dir string) (control.RepoFetchResult, error) {
	if err := f.actCtx(ctx, "fetch", dir); err != nil {
		return control.RepoFetchResult{}, err
	}
	f.mu.Lock()
	f.fetchDirs = append(f.fetchDirs, dir)
	f.mu.Unlock()
	return control.RepoFetchResult{Dir: stackDir, Head: commitA, RemoteHead: commitB, UpdatesAvailable: true}, nil
}

func (f *fakeBackend) SourceInspect(ctx context.Context, path, url string, include []string) (control.SourceInspectResult, error) {
	f.count("sourceinspect")
	f.mu.Lock()
	f.inspectEngines = append(f.inspectEngines, EngineOf(ctx))
	f.mu.Unlock()
	if err := f.fail("sourceinspect", path); err != nil {
		return control.SourceInspectResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inspectIncludes = append(f.inspectIncludes, slices.Clone(include))
	if res, ok := f.inspects[path]; ok {
		return planWithInclude(res, include)
	}
	return control.SourceInspectResult{Path: path, Case: control.SourceCaseReject, Reason: "no such folder in the fake",
		Stacks: []control.SourceStack{}}, nil
}

func (f *fakeBackend) SourceCompare(_ context.Context, args control.SourceCompareArgs) (control.SourceCompareResult, error) {
	f.count("sourcecompare")
	if err := f.fail("sourcecompare", args.Path); err != nil {
		return control.SourceCompareResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.compares = append(f.compares, args)
	return f.compare, nil
}

func (f *fakeBackend) SourceRemote(ctx context.Context, args control.SourceRemoteArgs) (control.SourceRemoteResult, error) {
	if err := f.actCtx(ctx, "sourceremote", args.Path); err != nil {
		return control.SourceRemoteResult{}, err
	}
	f.mu.Lock()
	f.remotes = append(f.remotes, args)
	f.mu.Unlock()
	return control.SourceRemoteResult{ID: "demo-id", Path: args.Path, URL: args.URL, State: args.ExpectState, Branch: "main",
		Head: commitA, Pushed: args.ExpectState != control.SourceRemoteEqual}, nil
}

func (f *fakeBackend) SourceAdd(ctx context.Context, args control.SourceAddArgs) (control.SourceAddResult, error) {
	if err := f.actCtx(ctx, "sourceadd", args.Path); err != nil {
		return control.SourceAddResult{}, err
	}
	f.mu.Lock()
	f.sourceAdds = append(f.sourceAdds, args)
	f.mu.Unlock()
	return control.SourceAddResult{ID: "demo-id", Path: args.Path, Kind: "local", Head: commitA}, nil
}

func (f *fakeBackend) SourceList(context.Context) (control.SourceListResult, error) {
	f.count("sourcelist")
	if err := f.fail("sourcelist", ""); err != nil {
		return control.SourceListResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.folderList, nil
}

func (f *fakeBackend) SourceRemove(ctx context.Context, path string) (control.SourceRemoveResult, error) {
	if err := f.actCtx(ctx, "sourceremove", path); err != nil {
		return control.SourceRemoveResult{}, err
	}
	f.mu.Lock()
	f.removes = append(f.removes, path)
	f.mu.Unlock()
	return control.SourceRemoveResult{ID: "demo-id", Path: path}, nil
}

// defaultKeyDir says dir names the default key: no folder, or the first bound one.
func (f *fakeBackend) defaultKeyDir(dir string) bool {
	src := f.status.Engine.Sources
	return dir == "" || len(src) > 0 && dir == src[0].Path
}

// keySlot is the stored key of a stack folder: key for the default one, dirKeys for the others. The caller
// holds f.mu.
func (f *fakeBackend) keySlot(dir string) *control.KeyResult {
	if f.defaultKeyDir(dir) {
		return f.key
	}
	if k, ok := f.dirKeys[dir]; ok {
		return &k
	}
	return nil
}

func (f *fakeBackend) putKey(dir string, k control.KeyResult) {
	if f.defaultKeyDir(dir) {
		f.key = &k
		return
	}
	if f.dirKeys == nil {
		f.dirKeys = map[string]control.KeyResult{}
	}
	f.dirKeys[dir] = k
}

func (f *fakeBackend) KeyShow(_ context.Context, dir string) (control.KeyResult, error) {
	f.count("keyshow")
	if err := f.fail("keyshow", dir); err != nil {
		return control.KeyResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if k := f.keySlot(dir); k != nil {
		return *k, nil
	}
	return control.KeyResult{}, control.NewError(control.CodeNoSuchKey, "no deploy key exists")
}

func (f *fakeBackend) KeyGenerate(ctx context.Context, dir string, replace bool) (control.KeyResult, error) {
	if err := f.actCtx(ctx, "keygen", dir); err != nil {
		return control.KeyResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.keyArgs = append(f.keyArgs, control.KeySetArgs{Replace: replace, Dir: dir})
	if f.keySlot(dir) != nil && !replace {
		return control.KeyResult{}, control.NewError(control.CodeKeyExists, "a deploy key already exists")
	}
	k := control.KeyResult{PublicKey: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExampleKeyForTestsOnly qhost",
		Fingerprint: "SHA256:ExampleKeyFingerprintForTestsOnly000000000"}
	f.putKey(dir, k)
	return k, nil
}

func (f *fakeBackend) KeySet(ctx context.Context, dir, key string, replace bool) (control.KeyResult, error) {
	if err := f.actCtx(ctx, "keyset", dir); err != nil {
		return control.KeyResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.keyArgs = append(f.keyArgs, control.KeySetArgs{Key: key, Replace: replace, Dir: dir})
	if f.keySlot(dir) != nil && !replace {
		return control.KeyResult{}, control.NewError(control.CodeKeyExists, "a deploy key already exists")
	}
	k := control.KeyResult{PublicKey: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIStoredKeyForTestsOnly stored",
		Fingerprint: "SHA256:StoredKeyFingerprintForTestsOnly000000000"}
	f.putKey(dir, k)
	return k, nil
}

func (f *fakeBackend) Credentials(ctx context.Context) (control.CredentialListResult, error) {
	f.count("credentials")
	if err := f.fail("credentials", ""); err != nil {
		return control.CredentialListResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.engineLog = append(f.engineLog, "credentials:@"+EngineOf(ctx))
	if f.credsBy != nil {
		return control.CredentialListResult{Credentials: f.credsBy[EngineOf(ctx)]}, nil
	}
	return control.CredentialListResult{Credentials: append([]control.CredentialInfo(nil), f.creds...)}, nil
}

func (f *fakeBackend) CredentialSet(ctx context.Context, name, value string) error {
	if err := f.actCtx(ctx, "credset", name); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.credSets = append(f.credSets, control.CredentialSetArgs{Name: name, Value: value})
	f.creds = append(f.creds, control.CredentialInfo{Name: name, Modified: at(31, 9, 0)})
	return nil
}

func (f *fakeBackend) CredentialDelete(ctx context.Context, name string) error {
	if err := f.actCtx(ctx, "creddel", name); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.creds = slices.DeleteFunc(f.creds, func(c control.CredentialInfo) bool { return c.Name == name })
	return nil
}

func (f *fakeBackend) WindowSet(ctx context.Context, args control.WindowSetArgs) (control.WindowResult, error) {
	if err := f.actCtx(ctx, "windowset", ""); err != nil {
		return control.WindowResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.windows = append(f.windows, args)
	if f.windowBy != nil {
		w := applyWindowArgs(f.windowBy[engineOrSystem(ctx)], args)
		w.Origin = control.WindowOriginOwn
		f.windowBy[engineOrSystem(ctx)] = w
		return w, nil
	}
	f.window = applyWindowArgs(f.window, args)
	return f.window, nil
}

func (f *fakeBackend) Notifications(ctx context.Context) (control.NotifyListResult, error) {
	f.count("notifications")
	if err := f.fail("notifications", ""); err != nil {
		return control.NotifyListResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.engineLog = append(f.engineLog, "notifications:@"+EngineOf(ctx))
	if f.notifyBy != nil {
		return control.NotifyListResult{Channels: f.notifyBy[EngineOf(ctx)]}, nil
	}
	return control.NotifyListResult{Channels: append([]control.NotifyChannelInfo(nil), f.notify...)}, nil
}

func (f *fakeBackend) NotifyAdd(ctx context.Context, args control.NotifyAddArgs) (control.NotifyChannelInfo, error) {
	if err := f.actCtx(ctx, "notifyadd", args.Name); err != nil {
		return control.NotifyChannelInfo{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.adds = append(f.adds, args)
	info := control.NotifyChannelInfo{Name: args.Name, Type: args.Type, Events: args.Events, Host: args.Host, Port: args.Port,
		Security: args.Security, User: args.User, From: args.From, To: args.To, ChatID: args.ChatID, BaseURL: args.BaseURL}
	f.notify = append(f.notify, info)
	return info, nil
}

func (f *fakeBackend) NotifyRemove(ctx context.Context, name string) error {
	if err := f.actCtx(ctx, "notifyremove", name); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.notify = slices.DeleteFunc(f.notify, func(c control.NotifyChannelInfo) bool { return c.Name == name })
	return nil
}

func (f *fakeBackend) NotifyTest(ctx context.Context, name string) (control.NotifyTestResult, error) {
	if err := f.actCtx(ctx, "notifytest", name); err != nil {
		return control.NotifyTestResult{}, err
	}
	return control.NotifyTestResult{Name: name, Type: control.NotifyWebhook}, nil
}

func (f *fakeBackend) Secrets(ctx context.Context, stack string) (control.SecretListResult, error) {
	f.count("secrets")
	f.noteEngine(ctx, "secrets", stack)
	if err := f.fail("secrets", stack); err != nil {
		return control.SecretListResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return control.SecretListResult{Stack: stack, Keys: slices.Clone(f.secrets[stack])}, nil
}

func (f *fakeBackend) SecretSet(ctx context.Context, stack, key, value string) (control.SecretResult, error) {
	if err := f.actCtx(ctx, "secretset", stack+"/"+key); err != nil {
		return control.SecretResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.secretSets = append(f.secretSets, control.SecretSetArgs{Stack: stack, Key: key, Value: value})
	replaced := slices.Contains(f.secrets[stack], key)
	if !replaced {
		if f.secrets == nil {
			f.secrets = map[string][]string{}
		}
		f.secrets[stack] = append(f.secrets[stack], key)
		slices.Sort(f.secrets[stack])
	}
	return control.SecretResult{Stack: stack, Key: key, Replaced: replaced}, nil
}

func (f *fakeBackend) SecretDelete(ctx context.Context, stack, key string) (control.SecretResult, error) {
	if err := f.actCtx(ctx, "secretdelete", stack+"/"+key); err != nil {
		return control.SecretResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.secrets[stack] = slices.DeleteFunc(f.secrets[stack], func(k string) bool { return k == key })
	return control.SecretResult{Stack: stack, Key: key}, nil
}

func (f *fakeBackend) BackupRestore(ctx context.Context, stack, id string) (control.BackupRestoreResult, error) {
	if err := f.actCtx(ctx, "restore", stack+"/"+id); err != nil {
		return control.BackupRestoreResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.restores = append(f.restores, control.BackupRestoreArgs{Stack: stack, ID: id, Confirm: true})
	return control.BackupRestoreResult{Backup: control.BackupInfo{ID: id, Stack: stack}}, nil
}

func (f *fakeBackend) ReturnCheck(_ context.Context, args control.ReturnArgs) (control.ReturnCheckResult, error) {
	if err := f.fail("returncheck", args.Stack+"/"+args.Run); err != nil {
		return control.ReturnCheckResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.checks = append(f.checks, args)
	return control.ReturnCheckResult{Stack: args.Stack, Run: args.Run, Data: args.Data,
		Steps: []control.ReturnStepInfo{{Variable: "APP_VERSION", From: "2.0.0", To: "1.9.0"}, {Variable: "DB_VERSION", From: "16.2", To: "16.1"}}}, nil
}

func (f *fakeBackend) Return(ctx context.Context, args control.ReturnArgs) (control.ReturnRunResult, error) {
	if err := f.actCtx(ctx, "return", args.Stack+"/"+args.Run); err != nil {
		return control.ReturnRunResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.returns = append(f.returns, args)
	if f.returnRes != nil {
		return *f.returnRes, nil
	}
	for i := range f.status.Engine.Stacks {
		if st := &f.status.Engine.Stacks[i]; st.Name == args.Stack {
			st.Pinned = &control.HoldInfo{Since: at(31, 1, 0)}
		}
	}
	return control.ReturnRunResult{ReturnCheckResult: control.ReturnCheckResult{Stack: args.Stack, Run: args.Run, Data: args.Data,
		Steps: []control.ReturnStepInfo{{Variable: "APP_VERSION", From: "2.0.0", To: "1.9.0"}}}, ReturnRun: "20261101T040000Z-00000001", Result: "succeeded"}, nil
}

func (f *fakeBackend) StackSettings(ctx context.Context, stack string) (control.StackSettingsResult, error) {
	f.count("settings")
	f.noteEngine(ctx, "settings", stack)
	if err := f.fail("settings", stack); err != nil {
		return control.StackSettingsResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	res := control.StackSettingsResult{Stack: stack}
	if s, ok := f.stackSettings[stack]; ok {
		res.Settings = &s
	}
	return res, nil
}

func (f *fakeBackend) StackSettingsSet(ctx context.Context, stack string, settings control.StackSettings) (control.StackSettingsSetResult, error) {
	if err := f.actCtx(ctx, "settingsset", stack); err != nil {
		return control.StackSettingsSetResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.settingsSets = append(f.settingsSets, control.StackSettingsSetArgs{Stack: stack, Settings: settings})
	old, had := f.stackSettings[stack]
	if f.stackSettings == nil {
		f.stackSettings = map[string]control.StackSettings{}
	}
	f.stackSettings[stack] = settings
	changed := !had || !reflect.DeepEqual(old, settings)
	res := control.StackSettingsSetResult{Stack: stack, Settings: settings, Changed: changed}
	if changed {
		res.Commit = commitB
	}
	return res, nil
}

func (f *fakeBackend) StackSettingsRemove(ctx context.Context, stack string) (control.StackSettingsRemoveResult, error) {
	if err := f.actCtx(ctx, "settingsremove", stack); err != nil {
		return control.StackSettingsRemoveResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.stackSettings[stack]; !ok {
		return control.StackSettingsRemoveResult{}, control.NewError(control.CodeNoSettings, "the stack has no settings; it is already deployed only")
	}
	delete(f.stackSettings, stack)
	f.settingsRemoves = append(f.settingsRemoves, stack)
	return control.StackSettingsRemoveResult{Stack: stack, Commit: commitB}, nil
}

func (f *fakeBackend) Containers(context.Context) (control.ContainerListResult, error) {
	f.count("containers")
	if err := f.fail("containers", ""); err != nil {
		return control.ContainerListResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.containers, nil
}

func (f *fakeBackend) Container(_ context.Context, name string) (control.ContainerDetail, error) {
	f.count("container")
	if err := f.fail("container", name); err != nil {
		return control.ContainerDetail{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	d, ok := f.containerDetails[name]
	if !ok {
		return control.ContainerDetail{}, control.NewError(control.CodeNotFound, "no such container")
	}
	return d, nil
}

// ContainerLogs answers like the service from ctLogLines: it filters by container or stack (a source that
// starts with "<stack>-"), by time and cursor, keeps the last Tail lines and cuts a page at ctLogPage lines.
// The arguments of every call are kept in ctLogArgs; an error set in errs under "containerlogs:<name or stack>"
// fails the call.
func (f *fakeBackend) ContainerLogs(_ context.Context, a control.ContainerLogsArgs) (control.ContainerLogsResult, error) {
	f.count("containerlogs")
	if err := f.fail("containerlogs", a.Name+a.Stack); err != nil {
		return control.ContainerLogsResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ctLogArgs = append(f.ctLogArgs, a)
	var sel []control.ContainerLogLine
	for _, l := range f.ctLogLines {
		switch {
		case a.Name != "" && l.Source != a.Name,
			a.Stack != "" && !strings.HasPrefix(l.Source, a.Stack+"-"),
			!a.Since.IsZero() && l.Time.Before(a.Since),
			!a.Until.IsZero() && l.Time.After(a.Until),
			!a.Cursor.IsZero() && l.Time.Before(a.Cursor):
			continue
		}
		sel = append(sel, l)
	}
	if a.Tail > 0 && len(sel) > a.Tail {
		sel = sel[len(sel)-a.Tail:]
	}
	res := control.ContainerLogsResult{Lines: []control.ContainerLogLine{}}
	if f.ctLogPage > 0 && len(sel) > f.ctLogPage {
		sel, res.More = sel[:f.ctLogPage], true
	}
	res.Lines = append(res.Lines, sel...)
	if len(sel) > 0 {
		res.Cursor = sel[len(sel)-1].Time.Add(time.Nanosecond)
	}
	return res, nil
}

var svcLevelRank = map[string]int{control.LogLevelDebug: 0, control.LogLevelInfo: 1, control.LogLevelWarn: 2, control.LogLevelError: 3}

// ServiceLog answers like the service from svcLog: by level, time and cursor (the number of entries already
// looked at), keeps the last Tail entries and cuts a page at svcPage entries. An error set in errs under
// "servicelog:" fails the call.
func (f *fakeBackend) Engines(context.Context) ([]string, error) {
	if f.engines != nil {
		return f.engines, nil
	}
	return []string{control.EngineSystem}, nil
}

// actCtx is act that also records the engine the call was addressed to, as "<call>:<arg>@<engine>".
func (f *fakeBackend) actCtx(ctx context.Context, call, arg string) error {
	f.mu.Lock()
	f.engineLog = append(f.engineLog, call+":"+arg+"@"+EngineOf(ctx))
	f.mu.Unlock()
	return f.act(call, arg)
}

// DeployLog answers from deployLogs by engine and records the engine of the call; an error under "deploylog:<engine>"
// fails it.
func (f *fakeBackend) DeployLog(ctx context.Context, a control.HostLogListArgs) (control.DeployLogListResult, error) {
	f.count("deploylog")
	engine := EngineOf(ctx)
	if err := f.fail("deploylog", engine); err != nil {
		return control.DeployLogListResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deployLogArgs = append(f.deployLogArgs, engine)
	return control.DeployLogListResult{Entries: f.deployLogs[engine]}, nil
}

func (f *fakeBackend) ServiceLog(_ context.Context, a control.ServiceLogArgs) (control.ServiceLogResult, error) {
	f.count("servicelog")
	if err := f.fail("servicelog", ""); err != nil {
		return control.ServiceLogResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.svcArgs = append(f.svcArgs, a)
	from := 0
	if a.Cursor != "" {
		from, _ = strconv.Atoi(a.Cursor)
	}
	res := control.ServiceLogResult{Entries: []control.ServiceLogEntry{}}
	var idx []int
	for i := from; i < len(f.svcLog); i++ {
		e := f.svcLog[i]
		if !a.Since.IsZero() && e.Time.Before(a.Since) || a.Level != "" && svcLevelRank[e.Level] < svcLevelRank[a.Level] {
			continue
		}
		idx = append(idx, i)
	}
	if a.Tail > 0 && len(idx) > a.Tail {
		idx = idx[len(idx)-a.Tail:]
	}
	pos := len(f.svcLog)
	if f.svcPage > 0 && len(idx) > f.svcPage {
		idx, res.More = idx[:f.svcPage], true
		pos = idx[len(idx)-1] + 1
	}
	for _, i := range idx {
		res.Entries = append(res.Entries, f.svcLog[i])
	}
	res.Cursor = strconv.Itoa(pos)
	return res, nil
}

func (f *fakeBackend) ProposalList(context.Context) (control.ProposalListResult, error) {
	f.count("proposals")
	if err := f.fail("proposals", ""); err != nil {
		return control.ProposalListResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return control.ProposalListResult{Proposals: slices.Clone(f.proposals)}, nil
}

func (f *fakeBackend) ProposalShow(ctx context.Context, id string) (control.ProposalShowResult, error) {
	f.count("proposal")
	f.mu.Lock()
	f.acts = append(f.acts, "proposalshow:"+id)
	f.proposalEngines = append(f.proposalEngines, "show:"+id+"@"+EngineOf(ctx))
	f.mu.Unlock()
	if err := f.fail("proposal", id); err != nil {
		return control.ProposalShowResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, p := range f.proposals {
		if p.ID == id {
			return control.ProposalShowResult{ProposalInfo: p, Content: []byte(f.proposalContents[id])}, nil
		}
	}
	return control.ProposalShowResult{}, control.NewError(control.CodeNotFound, "no such proposal")
}

func (f *fakeBackend) proposalAct(call string, id string) error {
	f.count(call)
	if err := f.fail(call, id); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.proposalActs = append(f.proposalActs, call+":"+id)
	f.acts = append(f.acts, call+":"+id)
	return nil
}

func (f *fakeBackend) ProposalApprove(ctx context.Context, args control.ProposalApproveArgs) (control.ProposalInfo, error) {
	err := f.proposalAct("proposalapprove", args.ID)
	f.mu.Lock()
	f.proposalEngines = append(f.proposalEngines, "approve:"+args.ID+"@"+EngineOf(ctx))
	f.mu.Unlock()
	f.mu.Lock()
	f.proposalApproves = append(f.proposalApproves, args)
	f.mu.Unlock()
	return control.ProposalInfo{ID: args.ID}, err
}

func (f *fakeBackend) ProposalReject(ctx context.Context, args control.ProposalRejectArgs) (control.ProposalInfo, error) {
	err := f.proposalAct("proposalreject", args.ID)
	f.mu.Lock()
	f.proposalEngines = append(f.proposalEngines, "reject:"+args.ID+"@"+EngineOf(ctx))
	f.mu.Unlock()
	f.mu.Lock()
	f.proposalRejects = append(f.proposalRejects, args)
	f.mu.Unlock()
	return control.ProposalInfo{ID: args.ID}, err
}

func (f *fakeBackend) ProposalResult(ctx context.Context, args control.ProposalResultArgs) (control.ProposalInfo, error) {
	err := f.proposalAct("proposalresult", args.ID)
	f.mu.Lock()
	f.proposalEngines = append(f.proposalEngines, "result:"+args.ID+"@"+EngineOf(ctx))
	f.mu.Unlock()
	f.mu.Lock()
	f.proposalResults = append(f.proposalResults, args)
	f.mu.Unlock()
	return control.ProposalInfo{ID: args.ID}, err
}

func (f *fakeBackend) Diagnose(ctx context.Context) *diagnose.Package {
	f.mu.Lock()
	f.diagCalls++
	call, gate := f.diagCall, f.diagGate
	f.mu.Unlock()
	if gate != nil {
		<-gate
	}
	if call == nil {
		call = func(context.Context, string, any, any) error { return nil }
	}
	return diagnose.Build(ctx, diagnose.Options{Call: call, Version: "1.2.3", Now: func() time.Time { return clogStart },
		Hostname: func() (string, error) { return "demo", nil }})
}

// noteEngine records the engine a read was addressed to, as "<call>:<arg>@<engine>".
func (f *fakeBackend) noteEngine(ctx context.Context, call, arg string) {
	f.mu.Lock()
	f.engineLog = append(f.engineLog, call+":"+arg+"@"+EngineOf(ctx))
	f.mu.Unlock()
}

// planWithInclude is the plan as the service makes it with a choice of unclear files: each chosen file moves from
// the left-out files to the included ones and the planned files. A path that is not left out as unclear is refused.
func planWithInclude(res control.SourceInspectResult, include []string) (control.SourceInspectResult, error) {
	if len(include) == 0 {
		return res, nil
	}
	res.Excluded, res.PlannedFiles = slices.Clone(res.Excluded), slices.Clone(res.PlannedFiles)
	for _, p := range include {
		i := slices.IndexFunc(res.Excluded, func(e control.SourceExcluded) bool {
			return e.Path == p && control.SourceUnclear(e.Reason)
		})
		if i < 0 {
			return res, errors.New("not an unclear file: " + p)
		}
		res.Included = append(res.Included, res.Excluded[i])
		res.Excluded = slices.Delete(res.Excluded, i, i+1)
		res.PlannedFiles = append(res.PlannedFiles, p)
	}
	slices.Sort(res.PlannedFiles)
	return res, nil
}
