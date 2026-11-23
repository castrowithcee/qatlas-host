// Package tui is the terminal interface of qhost. It acts only through a Backend, which reaches the service
// over its channel; it never touches the stacks repository, Docker or the files of the service. It changes
// something only after the person confirmed it.
package tui

import (
	"context"
	"time"

	"github.com/castrowithcee/qatlas-kit/localipc"

	"github.com/castrowithcee/qatlas-host/internal/client"
	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/diagnose"
)

// Backend is what the interface asks of the service, one method per channel command. Calls run off the event
// loop and may block; a failure the service reports is a *control.Error.
//
// A host can have several deploy services, one per Docker engine: the one of the system engine and an instance
// for each rootless engine. A Backend over all of them (the one the qhost command builds) routes each call:
//   - The reads of the lists (Status, Stacks, Runs, SourceList, Containers, Check and Updates without a stack)
//     merge every deploy service, mark each stack, stack folder, run and container group with its engine
//     (control.EngineSystem or "rootless:<user>") and name a deploy service that could not be asked in the field
//     NotChecked of the result, with its engine, instead of failing. Only when none answers is that an error.
//     As root, Containers adds the Docker engines that have no deploy service as groups marked ReadOnly; without
//     root it sets EnginesUnread.
//   - A call about one stack or one stack folder goes to the deploy service that has it. A name several engines
//     have is an error that names them; WithEngine chooses one. Nothing falls back to another engine.
//   - A call about a whole engine (credentials, notification channels, the window and every call that names
//     neither stack nor folder) goes to the engine WithEngine names; without one, to the system engine, else to
//     the only instance, else it is an error that asks for the engine.
//
// A Backend over one channel serves the system engine and ignores WithEngine.
type Backend interface {
	// Engines lists the deploy services of the host by engine id: the system engine when its service is set up,
	// then the rootless engines by user.
	Engines(ctx context.Context) ([]string, error)
	Status(ctx context.Context) (control.StatusResult, error)
	Stacks(ctx context.Context) (control.StacksResult, error)
	// Check checks one stack, or every stack for an empty name.
	Check(ctx context.Context, stack string) (control.StacksCheckResult, error)
	// Updates reads the stored update detection; args.Refresh asks the registries first.
	Updates(ctx context.Context, args control.UpdatesArgs) (control.UpdatesResult, error)
	// Runs lists the runs of one stack, or of every stack for an empty name, newest first.
	Runs(ctx context.Context, stack string) (control.RunsResult, error)
	Log(ctx context.Context, run string) (control.LogResult, error)
	Changelog(ctx context.Context, stack string) (control.ChangelogResult, error)
	// Backups lists the backups of one stack, newest first.
	Backups(ctx context.Context, stack string) (control.BackupListResult, error)
	Window(ctx context.Context) (control.WindowResult, error)
	// Secrets lists the key names of the secrets of one stack; never a value.
	Secrets(ctx context.Context, stack string) (control.SecretListResult, error)
	// Containers lists the containers of the host in groups, without readings of CPU and memory.
	Containers(ctx context.Context) (ContainerList, error)
	// Container reads one container by name; the result holds the names of its environment variables, never values.
	Container(ctx context.Context, name string) (control.ContainerDetail, error)
	// ContainerLogs reads a page of the output of one container or of all containers of an own stack.
	ContainerLogs(ctx context.Context, args control.ContainerLogsArgs) (control.ContainerLogsResult, error)
	// ServiceLog reads a page of the log of the service itself.
	ServiceLog(ctx context.Context, args control.ServiceLogArgs) (control.ServiceLogResult, error)
	// DeployLog reads a page of the action log of one deploy service, newest first: the one WithEngine names,
	// else the one a call about a whole engine reaches.
	DeployLog(ctx context.Context, args control.HostLogListArgs) (control.DeployLogListResult, error)
	// Diagnose collects the diagnostic package from read-only calls; it never fails, a part that could not be
	// collected carries its reason.
	Diagnose(ctx context.Context) *diagnose.Package

	// The calls below change something in the service; the interface makes them only after a confirmation.

	// Deploy starts a manual run of one stack and returns as soon as it is started.
	Deploy(ctx context.Context, stack string) (control.DeployResult, error)
	// Update moves the image versions of the stacks and returns when the last step is done.
	Update(ctx context.Context, stacks []string) (control.UpdateRunResult, error)
	Pause(ctx context.Context, stack string) (control.HoldResult, error)
	Unpause(ctx context.Context, stack string) (control.HoldResult, error)
	Pin(ctx context.Context, stack string) (control.HoldResult, error)
	Unpin(ctx context.Context, stack string) (control.HoldResult, error)
	// Resume clears the pause a failed rollback left on a stack.
	Resume(ctx context.Context, stack string) (control.ResumeResult, error)
	// StackStart, StackStop and StackRestart act on the containers of a stack and return when the service is done.
	StackStart(ctx context.Context, stack string) (control.StackActionResult, error)
	StackStop(ctx context.Context, stack string) (control.StackActionResult, error)
	StackRestart(ctx context.Context, stack string) (control.StackActionResult, error)
	// ContainerStart, ContainerStop and ContainerRestart act on one container, named by its ID, and return when
	// the service is done; a start or restart waits for the container to be ready.
	ContainerStart(ctx context.Context, id string) (control.ContainerActionResult, error)
	ContainerStop(ctx context.Context, id string) (control.ContainerActionResult, error)
	ContainerRestart(ctx context.Context, id string) (control.ContainerActionResult, error)

	// The settings calls. Reads are free; the others the interface makes only after a confirmation. None of
	// the results carries a stored secret.

	// RepoProbe reads the host key the server of url presents; it stores nothing.
	RepoProbe(ctx context.Context, url string) (control.RepoProbeResult, error)
	RepoSet(ctx context.Context, args control.RepoSetArgs) (control.RepoSetResult, error)
	// RepoFetch fetches one stack folder, or every one for an empty dir.
	RepoFetch(ctx context.Context, dir string) (control.RepoFetchResult, error)
	// The key calls take the stack folder the key is for; empty is the default key, the one of the folder bound
	// first. KeyGenerate and KeySet keep an existing key and refuse unless replace is set.
	KeyShow(ctx context.Context, dir string) (control.KeyResult, error)
	KeyGenerate(ctx context.Context, dir string, replace bool) (control.KeyResult, error)
	// KeySet stores the private key text as the deploy key; the text is never logged or shown.
	KeySet(ctx context.Context, dir, key string, replace bool) (control.KeyResult, error)
	Credentials(ctx context.Context) (control.CredentialListResult, error)
	CredentialSet(ctx context.Context, name, value string) error
	CredentialDelete(ctx context.Context, name string) error
	// WindowSet makes the result the own window of the deploy service the context names, WindowReset removes it, so
	// the service follows the host window again; HostWindowSet changes the host window, a file only root writes,
	// and reaches no deploy service.
	WindowSet(ctx context.Context, args control.WindowSetArgs) (control.WindowResult, error)
	WindowReset(ctx context.Context) (control.WindowResult, error)
	HostWindowSet(ctx context.Context, args control.WindowSetArgs) (control.WindowResult, error)
	// HostWindow reads the host window from its file; it reaches no deploy service and needs no root.
	HostWindow(ctx context.Context) (control.WindowResult, error)
	Notifications(ctx context.Context) (control.NotifyListResult, error)
	NotifyAdd(ctx context.Context, args control.NotifyAddArgs) (control.NotifyChannelInfo, error)
	NotifyRemove(ctx context.Context, name string) error
	NotifyTest(ctx context.Context, name string) (control.NotifyTestResult, error)

	// The stack-detail writes. The value of SecretSet travels only in this call.
	SecretSet(ctx context.Context, stack, key, value string) (control.SecretResult, error)
	SecretDelete(ctx context.Context, stack, key string) (control.SecretResult, error)
	// The stack folder calls. SourceInspect, SourceCompare and SourceList only read; SourceAdd and SourceRemove
	// the interface makes only after a confirmation, and SourceAdd only after the host side has granted the folder.

	// SourceInspect classifies a folder; url is the SSH address to clone or to expect as origin, and may be empty.
	SourceInspect(ctx context.Context, path, url string, include []string) (control.SourceInspectResult, error)
	SourceCompare(ctx context.Context, args control.SourceCompareArgs) (control.SourceCompareResult, error)
	SourceAdd(ctx context.Context, args control.SourceAddArgs) (control.SourceAddResult, error)
	// SourceRemote gives a local stack folder its remote, as the comparison the person was shown.
	SourceRemote(ctx context.Context, args control.SourceRemoteArgs) (control.SourceRemoteResult, error)
	SourceList(ctx context.Context) (control.SourceListResult, error)
	// SourceRemove releases a stack folder; no file of it is touched.
	SourceRemove(ctx context.Context, path string) (control.SourceRemoveResult, error)

	// ReturnCheck changes nothing: it refuses as Return would and names the versions the return sets.
	ReturnCheck(ctx context.Context, args control.ReturnArgs) (control.ReturnCheckResult, error)
	// Return sets a stack back to the versions before a successful update run and returns when the return's run
	// has ended; it pins the stack after a success.
	Return(ctx context.Context, args control.ReturnArgs) (control.ReturnRunResult, error)

	// BackupRestore replaces the data of a stack with a backup and starts the stack again.
	BackupRestore(ctx context.Context, stack, id string) (control.BackupRestoreResult, error)

	// The stack-settings calls. StackSettings is a read; its result has no Settings for a stack that is only
	// deployed. StackSettingsSet replaces all settings of the stack, StackSettingsRemove deletes them; both
	// commit to the stacks repository and push when it has a remote. A refusal such as a field that does not
	// fit is a *control.Error with CodeInvalid whose message names the field path, such as "update.tags.X: ...";
	// removing the settings of a stack without any is a *control.Error with CodeNoSettings.
	StackSettings(ctx context.Context, stack string) (control.StackSettingsResult, error)
	StackSettingsSet(ctx context.Context, stack string, settings control.StackSettings) (control.StackSettingsSetResult, error)
	StackSettingsRemove(ctx context.Context, stack string) (control.StackSettingsRemoveResult, error)

	// ProposalList lists the proposals, newest first; ProposalShow reads one with its content.
	ProposalList(ctx context.Context) (control.ProposalListResult, error)
	ProposalShow(ctx context.Context, id string) (control.ProposalShowResult, error)
	// ProposalApprove, ProposalReject and ProposalResult are root's; the interface calls them only after the person decided.
	ProposalApprove(ctx context.Context, args control.ProposalApproveArgs) (control.ProposalInfo, error)
	ProposalReject(ctx context.Context, args control.ProposalRejectArgs) (control.ProposalInfo, error)
	ProposalResult(ctx context.Context, args control.ProposalResultArgs) (control.ProposalInfo, error)
}

type engineKey struct{}

// WithEngine returns ctx that makes a routing Backend send the call to the deploy service of engine, an id as
// Engines lists it. A named engine is never replaced by another one; if it cannot be reached the call fails.
func WithEngine(ctx context.Context, engine string) context.Context {
	return context.WithValue(ctx, engineKey{}, engine)
}

// EngineOf is the engine WithEngine named for ctx, or empty.
func EngineOf(ctx context.Context) string {
	e, _ := ctx.Value(engineKey{}).(string)
	return e
}

// updateCallLimit bounds one update call, which waits for every step of the stack; it is the limit of the
// update command.
const updateCallLimit = 24 * time.Hour

// remoteCallLimit bounds the calls that wait on the git server; it is the limit of the repo command.
const remoteCallLimit = 3 * time.Minute

// restoreCallLimit bounds a restore, which waits for the stack to stop, unpack and start again; it is the limit
// of the backup command.
const restoreCallLimit = 6*time.Hour + time.Minute

// containerCallLimit bounds the container reads. They are one request to the container engine and repeat every
// few seconds, so a call that hangs gives way early; the engine's own limit is longer and its answer, if any, is
// only seen when it comes sooner.
const containerCallLimit = 10 * time.Second

// ChannelBackend is the Backend over the service channel. Every call dials a connection of its own, so calls
// may run concurrently.
type ChannelBackend struct {
	endpoint localipc.ServiceEndpoint
	version  string
}

// NewBackend returns the Backend over the channel at ep.
func NewBackend(ep localipc.ServiceEndpoint) *ChannelBackend { return &ChannelBackend{endpoint: ep} }

// WithVersion sets the version of qhost that a diagnostic package states.
func (b *ChannelBackend) WithVersion(v string) *ChannelBackend {
	b.version = v
	return b
}

// Engines names the engine of the one channel: the system engine.
func (b *ChannelBackend) Engines(context.Context) ([]string, error) {
	return []string{control.EngineSystem}, nil
}

func (b *ChannelBackend) Diagnose(ctx context.Context) *diagnose.Package {
	return diagnose.Build(ctx, diagnose.Options{Call: b.call, Version: b.version})
}

func (b *ChannelBackend) call(ctx context.Context, command string, args, result any) error {
	return client.Do(ctx, b.endpoint, client.CallTimeout, command, args, result)
}

func (b *ChannelBackend) Status(ctx context.Context) (res control.StatusResult, err error) {
	err = b.call(ctx, control.StatusCommand, nil, &res)
	return res, err
}

func (b *ChannelBackend) Stacks(ctx context.Context) (res control.StacksResult, err error) {
	err = b.call(ctx, control.StacksListCommand, nil, &res)
	return res, err
}

func (b *ChannelBackend) Check(ctx context.Context, stack string) (res control.StacksCheckResult, err error) {
	err = b.call(ctx, control.StacksCheckCommand, control.StacksCheckArgs{Stack: stack}, &res)
	return res, err
}

func (b *ChannelBackend) Updates(ctx context.Context, args control.UpdatesArgs) (res control.UpdatesResult, err error) {
	limit := client.CallTimeout
	if args.Refresh {
		limit += control.UpdatesRefreshTimeout
	}
	if args.Changelog {
		limit += control.ChangelogTimeout
	}
	err = client.Do(ctx, b.endpoint, limit, control.UpdatesListCommand, args, &res)
	return res, err
}

func (b *ChannelBackend) Runs(ctx context.Context, stack string) (res control.RunsResult, err error) {
	err = b.call(ctx, control.RunsCommand, control.RunsArgs{Stack: stack}, &res)
	return res, err
}

func (b *ChannelBackend) Log(ctx context.Context, run string) (res control.LogResult, err error) {
	err = b.call(ctx, control.LogCommand, control.LogArgs{ID: run}, &res)
	return res, err
}

func (b *ChannelBackend) Changelog(ctx context.Context, stack string) (res control.ChangelogResult, err error) {
	err = client.Do(ctx, b.endpoint, client.CallTimeout+control.ChangelogTimeout, control.ChangelogShowCommand,
		control.ChangelogArgs{Stack: stack}, &res)
	return res, err
}

func (b *ChannelBackend) Window(ctx context.Context) (res control.WindowResult, err error) {
	err = b.call(ctx, control.WindowShowCommand, nil, &res)
	return res, err
}

func (b *ChannelBackend) Backups(ctx context.Context, stack string) (res control.BackupListResult, err error) {
	err = b.call(ctx, control.BackupListCommand, control.BackupArgs{Stack: stack}, &res)
	return res, err
}

func (b *ChannelBackend) Deploy(ctx context.Context, stack string) (res control.DeployResult, err error) {
	err = b.call(ctx, control.DeployCommand, control.DeployArgs{Stack: stack}, &res)
	return res, err
}

func (b *ChannelBackend) Update(ctx context.Context, stacks []string) (res control.UpdateRunResult, err error) {
	err = client.Do(ctx, b.endpoint, updateCallLimit, control.UpdateRunCommand, control.UpdateRunArgs{Stacks: stacks}, &res)
	return res, err
}

func (b *ChannelBackend) hold(ctx context.Context, command, stack string) (res control.HoldResult, err error) {
	err = b.call(ctx, command, control.HoldArgs{Stack: stack}, &res)
	return res, err
}

func (b *ChannelBackend) Pause(ctx context.Context, stack string) (control.HoldResult, error) {
	return b.hold(ctx, control.PauseCommand, stack)
}

func (b *ChannelBackend) Unpause(ctx context.Context, stack string) (control.HoldResult, error) {
	return b.hold(ctx, control.UnpauseCommand, stack)
}

func (b *ChannelBackend) Pin(ctx context.Context, stack string) (control.HoldResult, error) {
	return b.hold(ctx, control.PinCommand, stack)
}

func (b *ChannelBackend) Unpin(ctx context.Context, stack string) (control.HoldResult, error) {
	return b.hold(ctx, control.UnpinCommand, stack)
}

func (b *ChannelBackend) Resume(ctx context.Context, stack string) (res control.ResumeResult, err error) {
	err = b.call(ctx, control.ResumeCommand, control.ResumeArgs{Stack: stack}, &res)
	return res, err
}

func (b *ChannelBackend) stackAction(ctx context.Context, command, stack string) (res control.StackActionResult, err error) {
	err = client.Do(ctx, b.endpoint, control.StackActionTimeout, command, control.StackActionArgs{Stack: stack}, &res)
	return res, err
}

func (b *ChannelBackend) StackStart(ctx context.Context, stack string) (control.StackActionResult, error) {
	return b.stackAction(ctx, control.StackStartCommand, stack)
}

func (b *ChannelBackend) StackStop(ctx context.Context, stack string) (control.StackActionResult, error) {
	return b.stackAction(ctx, control.StackStopCommand, stack)
}

func (b *ChannelBackend) StackRestart(ctx context.Context, stack string) (control.StackActionResult, error) {
	return b.stackAction(ctx, control.StackRestartCommand, stack)
}

func (b *ChannelBackend) containerAction(ctx context.Context, command, id string) (res control.ContainerActionResult, err error) {
	err = client.Do(ctx, b.endpoint, control.ContainerActionTimeout+client.CallTimeout, command, control.ContainerActionArgs{Name: id}, &res)
	return res, err
}

func (b *ChannelBackend) ContainerStart(ctx context.Context, id string) (control.ContainerActionResult, error) {
	return b.containerAction(ctx, control.ContainerStartCommand, id)
}

func (b *ChannelBackend) ContainerStop(ctx context.Context, id string) (control.ContainerActionResult, error) {
	return b.containerAction(ctx, control.ContainerStopCommand, id)
}

func (b *ChannelBackend) ContainerRestart(ctx context.Context, id string) (control.ContainerActionResult, error) {
	return b.containerAction(ctx, control.ContainerRestartCommand, id)
}

func (b *ChannelBackend) RepoProbe(ctx context.Context, url string) (res control.RepoProbeResult, err error) {
	err = client.Do(ctx, b.endpoint, remoteCallLimit, control.RepoProbeCommand, control.RepoProbeArgs{URL: url}, &res)
	return res, err
}

func (b *ChannelBackend) RepoSet(ctx context.Context, args control.RepoSetArgs) (res control.RepoSetResult, err error) {
	err = client.Do(ctx, b.endpoint, remoteCallLimit, control.RepoSetCommand, args, &res)
	return res, err
}

func (b *ChannelBackend) RepoFetch(ctx context.Context, dir string) (res control.RepoFetchResult, err error) {
	err = client.Do(ctx, b.endpoint, remoteCallLimit, control.RepoFetchCommand, control.RepoFetchArgs{Dir: dir}, &res)
	return res, err
}

func (b *ChannelBackend) SourceInspect(ctx context.Context, path, url string, include []string) (res control.SourceInspectResult, err error) {
	err = b.call(ctx, control.SourceInspectCommand, control.SourceInspectArgs{Path: path, URL: url, Include: include}, &res)
	return res, err
}

func (b *ChannelBackend) SourceCompare(ctx context.Context, args control.SourceCompareArgs) (res control.SourceCompareResult, err error) {
	err = client.Do(ctx, b.endpoint, remoteCallLimit, control.SourceCompareCommand, args, &res)
	return res, err
}

func (b *ChannelBackend) SourceRemote(ctx context.Context, args control.SourceRemoteArgs) (res control.SourceRemoteResult, err error) {
	err = client.Do(ctx, b.endpoint, remoteCallLimit, control.SourceRemoteCommand, args, &res)
	return res, err
}

func (b *ChannelBackend) SourceAdd(ctx context.Context, args control.SourceAddArgs) (res control.SourceAddResult, err error) {
	err = client.Do(ctx, b.endpoint, remoteCallLimit, control.SourceAddCommand, args, &res)
	return res, err
}

func (b *ChannelBackend) SourceList(ctx context.Context) (res control.SourceListResult, err error) {
	err = b.call(ctx, control.SourceListCommand, nil, &res)
	return res, err
}

func (b *ChannelBackend) SourceRemove(ctx context.Context, path string) (res control.SourceRemoveResult, err error) {
	err = b.call(ctx, control.SourceRemoveCommand, control.SourceRemoveArgs{Path: path}, &res)
	return res, err
}

func (b *ChannelBackend) KeyShow(ctx context.Context, dir string) (res control.KeyResult, err error) {
	err = b.call(ctx, control.KeyShowCommand, control.KeyShowArgs{Dir: dir}, &res)
	return res, err
}

func (b *ChannelBackend) KeyGenerate(ctx context.Context, dir string, replace bool) (res control.KeyResult, err error) {
	err = b.call(ctx, control.KeyGenerateCommand, control.KeyGenerateArgs{Replace: replace, Dir: dir}, &res)
	return res, err
}

func (b *ChannelBackend) KeySet(ctx context.Context, dir, key string, replace bool) (res control.KeyResult, err error) {
	err = b.call(ctx, control.KeySetCommand, control.KeySetArgs{Key: key, Replace: replace, Dir: dir}, &res)
	return res, err
}

func (b *ChannelBackend) Credentials(ctx context.Context) (res control.CredentialListResult, err error) {
	err = b.call(ctx, control.CredentialListCommand, nil, &res)
	return res, err
}

func (b *ChannelBackend) CredentialSet(ctx context.Context, name, value string) error {
	return b.call(ctx, control.CredentialSetCommand, control.CredentialSetArgs{Name: name, Value: value}, nil)
}

func (b *ChannelBackend) CredentialDelete(ctx context.Context, name string) error {
	return b.call(ctx, control.CredentialDeleteCommand, control.CredentialNameArgs{Name: name}, nil)
}

func (b *ChannelBackend) WindowReset(ctx context.Context) (res control.WindowResult, err error) {
	err = b.call(ctx, control.WindowResetCommand, nil, &res)
	return res, err
}

func (b *ChannelBackend) WindowSet(ctx context.Context, args control.WindowSetArgs) (res control.WindowResult, err error) {
	err = b.call(ctx, control.WindowSetCommand, args, &res)
	return res, err
}

func (b *ChannelBackend) Notifications(ctx context.Context) (res control.NotifyListResult, err error) {
	err = b.call(ctx, control.NotifyListCommand, nil, &res)
	return res, err
}

func (b *ChannelBackend) NotifyAdd(ctx context.Context, args control.NotifyAddArgs) (res control.NotifyChannelInfo, err error) {
	err = b.call(ctx, control.NotifyAddCommand, args, &res)
	return res, err
}

func (b *ChannelBackend) NotifyRemove(ctx context.Context, name string) error {
	return b.call(ctx, control.NotifyRemoveCommand, control.NotifyNameArgs{Name: name}, nil)
}

func (b *ChannelBackend) NotifyTest(ctx context.Context, name string) (res control.NotifyTestResult, err error) {
	err = client.Do(ctx, b.endpoint, client.CallTimeout+control.NotifyTestTimeout, control.NotifyTestCommand,
		control.NotifyNameArgs{Name: name}, &res)
	return res, err
}

func (b *ChannelBackend) Secrets(ctx context.Context, stack string) (res control.SecretListResult, err error) {
	err = b.call(ctx, control.SecretListCommand, control.SecretListArgs{Stack: stack}, &res)
	return res, err
}

func (b *ChannelBackend) Containers(ctx context.Context) (res ContainerList, err error) {
	err = client.Do(ctx, b.endpoint, containerCallLimit, control.ContainerListCommand, control.ContainerListArgs{}, &res.ContainerListResult)
	return res, err
}

// ContainerList is the answer of Backend.Containers: the groups, and what only a Backend over several deploy
// services knows. NotChecked names the engines whose service could not be asked; EnginesUnread is set when the
// reader is not root, so it cannot tell whether engines without a deploy service exist.
type ContainerList struct {
	control.ContainerListResult
	NotChecked    []control.EngineNotChecked
	EnginesUnread bool
}

func (b *ChannelBackend) Container(ctx context.Context, name string) (res control.ContainerDetail, err error) {
	err = client.Do(ctx, b.endpoint, containerCallLimit, control.ContainerGetCommand, control.ContainerGetArgs{Name: name}, &res)
	return res, err
}

func (b *ChannelBackend) ContainerLogs(ctx context.Context, args control.ContainerLogsArgs) (res control.ContainerLogsResult, err error) {
	err = client.Do(ctx, b.endpoint, control.ContainerLogsTimeout+client.CallTimeout, control.ContainerLogsCommand, args, &res)
	return res, err
}

func (b *ChannelBackend) ServiceLog(ctx context.Context, args control.ServiceLogArgs) (res control.ServiceLogResult, err error) {
	err = b.call(ctx, control.ServiceLogCommand, args, &res)
	return res, err
}

func (b *ChannelBackend) DeployLog(ctx context.Context, args control.HostLogListArgs) (res control.DeployLogListResult, err error) {
	err = b.call(ctx, control.HostLogListCommand, args, &res)
	return res, err
}

func (b *ChannelBackend) SecretSet(ctx context.Context, stack, key, value string) (res control.SecretResult, err error) {
	err = b.call(ctx, control.SecretSetCommand, control.SecretSetArgs{Stack: stack, Key: key, Value: value}, &res)
	return res, err
}

func (b *ChannelBackend) SecretDelete(ctx context.Context, stack, key string) (res control.SecretResult, err error) {
	err = b.call(ctx, control.SecretDeleteCommand, control.SecretKeyArgs{Stack: stack, Key: key}, &res)
	return res, err
}

func (b *ChannelBackend) BackupRestore(ctx context.Context, stack, id string) (res control.BackupRestoreResult, err error) {
	err = client.Do(ctx, b.endpoint, restoreCallLimit, control.BackupRestoreCommand,
		control.BackupRestoreArgs{Stack: stack, ID: id, Confirm: true}, &res)
	return res, err
}

func (b *ChannelBackend) ReturnCheck(ctx context.Context, args control.ReturnArgs) (res control.ReturnCheckResult, err error) {
	err = client.Do(ctx, b.endpoint, updateCallLimit, control.ReturnCheckCommand, args, &res)
	return res, err
}

func (b *ChannelBackend) Return(ctx context.Context, args control.ReturnArgs) (res control.ReturnRunResult, err error) {
	err = client.Do(ctx, b.endpoint, updateCallLimit, control.ReturnRunCommand, args, &res)
	return res, err
}

func (b *ChannelBackend) StackSettings(ctx context.Context, stack string) (res control.StackSettingsResult, err error) {
	err = b.call(ctx, control.StackSettingsGetCommand, control.StackSettingsArgs{Stack: stack}, &res)
	return res, err
}

func (b *ChannelBackend) StackSettingsSet(ctx context.Context, stack string, settings control.StackSettings) (res control.StackSettingsSetResult, err error) {
	err = client.Do(ctx, b.endpoint, control.StackSettingsTimeout, control.StackSettingsSetCommand,
		control.StackSettingsSetArgs{Stack: stack, Settings: settings}, &res)
	return res, err
}

func (b *ChannelBackend) StackSettingsRemove(ctx context.Context, stack string) (res control.StackSettingsRemoveResult, err error) {
	err = client.Do(ctx, b.endpoint, control.StackSettingsTimeout, control.StackSettingsRemoveCommand,
		control.StackSettingsArgs{Stack: stack}, &res)
	return res, err
}

func (b *ChannelBackend) ProposalList(ctx context.Context) (res control.ProposalListResult, err error) {
	err = b.call(ctx, control.ProposalListCommand, nil, &res)
	return res, err
}

func (b *ChannelBackend) ProposalShow(ctx context.Context, id string) (res control.ProposalShowResult, err error) {
	err = b.call(ctx, control.ProposalShowCommand, control.ProposalIDArgs{ID: id}, &res)
	return res, err
}

func (b *ChannelBackend) ProposalApprove(ctx context.Context, args control.ProposalApproveArgs) (res control.ProposalInfo, err error) {
	err = b.call(ctx, control.ProposalApproveCommand, args, &res)
	return res, err
}

func (b *ChannelBackend) ProposalReject(ctx context.Context, args control.ProposalRejectArgs) (res control.ProposalInfo, err error) {
	err = b.call(ctx, control.ProposalRejectCommand, args, &res)
	return res, err
}

func (b *ChannelBackend) ProposalResult(ctx context.Context, args control.ProposalResultArgs) (res control.ProposalInfo, err error) {
	err = b.call(ctx, control.ProposalResultCommand, args, &res)
	return res, err
}

// Folders is the host side of binding a stack folder. It acts on the host, not through the service, because the
// service runs sandboxed and never creates a folder itself.
type Folders interface {
	// Missing reports whether the folder does not exist yet.
	Missing(dir string) (bool, error)
	// Grant creates the folder when create is set, which the person confirmed, and lets the service write to it.
	Grant(ctx context.Context, dir string, create bool) error
	// PlanPermissions works out which permissions the folder needs for the service to write it, from the
	// answer of SourceInspect; it changes nothing. The plan is empty when the service can write already.
	PlanPermissions(ctx context.Context, res control.SourceInspectResult) (PermPlan, error)
	// ApplyPermissions carries out a plan the person confirmed.
	ApplyPermissions(ctx context.Context, plan PermPlan) error
	// ReadOnly reports whether the file system of the folder, or of its nearest existing parent, is read-only;
	// only the host can tell, because the service sees every folder it may not write as read-only.
	ReadOnly(path string) (bool, error)
	// Engines adds to the answer of SourceInspect the containers that run in the stack folders in the rootless
	// engines of other users, and the engines that could not be asked; only the host can read them, and only
	// as root. It only reads and returns res unchanged for a rejection or an answer without stacks.
	Engines(ctx context.Context, res control.SourceInspectResult) control.SourceInspectResult
}

// PermPlan is a plan of permission changes for a folder. Changed is the number of paths it changes and Summary
// the lines to show the person; the handle is the host side's own data and opaque here.
type PermPlan struct {
	Changed int
	Summary []string
	handle  any
}

// NewPermPlan returns a plan that carries handle for the host side.
func NewPermPlan(changed int, summary []string, handle any) PermPlan {
	return PermPlan{Changed: changed, Summary: summary, handle: handle}
}

// Handle returns what NewPermPlan was given.
func (p PermPlan) Handle() any { return p.handle }

// Empty reports whether the plan changes nothing.
func (p PermPlan) Empty() bool { return p.handle == nil }
