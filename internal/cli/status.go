package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-kit/cli"
	"github.com/castrowithcee/qatlas-kit/cobracli"
	"github.com/castrowithcee/qatlas-kit/localipc"
	"github.com/castrowithcee/qatlas-kit/output"

	"github.com/castrowithcee/qatlas-host/internal/client"
	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/deploysetup"
	"github.com/castrowithcee/qatlas-host/internal/hostservice"
	"github.com/castrowithcee/qatlas-host/internal/service"
)

// ExitServiceNotRunning is the exit code of a command that found no running service. The kit's exit contract
// knows only success, runtime failure and usage, so Run maps this one case on top of it.
const ExitServiceNotRunning = 3

// serviceEndpoint names the channel the commands talk to; tests replace it.
var serviceEndpoint = control.DeployEndpoint

// instanceEndpoint names the channel of a deploy service instance; tests replace it.
var instanceEndpoint = deploysetup.Instance.Endpoint

// notRunning records that the command failed because no service listens.
type notRunning struct{ seen bool }

func newStatusCommand(program *cobracli.Program, state *notRunning) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show whether the qhost services are running",
		Long: "status shows the host service and the deploy service. For the host service it shows whether it runs and its version\n" +
			"and start time; for a user of the group qhost who is no administrator it shows that it runs and is reserved for administrators. The deploy\n" +
			"service is shown as not set up until 'sudo qhost deploy-service setup'; then status asks it for its version and start time, the state of the last poll of each stack folder\n" +
			"and, per stack, what is deployed, pending or running, the last run result, a pause after a failed rollback, the operator's pause and pin, the observation time after an update, and the findings,\n" +
			"and whether the maintenance window is open and when it opens next (see 'qhost window'). A stack is listed as\n" +
			"stack.<name>; when another stack folder holds its name, as source.<id>.stack.<name> (see 'qhost help sources').\n" +
			"A stack folder of the system engine with the finding git-unsafe shows the reason in git_unsafe and the next step in\n" +
			"git_unsafe_fix: 'sudo qhost source secure <path>' (see 'Git in a stack folder' in 'qhost help sources').\n" +
			"Every stack folder and stack has the field engine, system here. Every instance for the rootless engine of a user that is\n" +
			"set up is listed as instance.<user>, with its unit, whether it runs and, when it does, its version and start time,\n" +
			"and its stack folders and stacks as instance.<user>.source.<id> and instance.<user>.stack.<name>, in the same form\n" +
			"with the field engine rootless:<user>. An instance that cannot be asked shows its state in instance.<user>.deploy_service.\n" +
			"The webhook receiver is shown as hook_service: not set up until 'sudo qhost hook-service setup', then running or not running\n" +
			"as systemd reports it, with its address (hook_listen) and the number of hooks (hook_count); it needs no root.\n" +
			"Root and administrators in the group qhost may read; run it without sudo. A set up deploy service that is not running ends with exit code 3.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			host := append(hostFields(c.Context()), instanceFields(c.Context())...)
			host = append(host, hookFields(c.Context())...)
			if !deployInstalled() {
				fields := append([]output.Field{{Name: "running", Value: false}, {Name: "deploy_service", Value: "not set up"}}, host...)
				return output.Encode(c.OutOrStdout(), program.Format(), output.Object{Fields: fields})
			}
			var st control.StatusResult
			if err := call(c.Context(), serviceEndpoint(), control.StatusCommand, nil, &st); err != nil {
				if errors.Is(err, client.ErrNotRunning) {
					state.seen = true
					return withStateCause(failureOf(err), stateDir())
				}
				return failureOf(err)
			}
			fields := []output.Field{
				{Name: "running", Value: true},
				{Name: "version", Value: st.Version},
				{Name: "started_at", Value: st.StartedAt.UTC().Format(time.RFC3339)},
			}
			fields = append(fields, engineFields(control.EngineSystem, st.Engine)...)
			if w := st.Window; w != nil {
				fields = append(fields, output.Field{Name: "window_open", Value: w.Open},
					output.Field{Name: "window_next_opening", Value: w.NextOpening.Format(time.RFC3339)})
			}
			fields = append(fields, output.Field{Name: "deploy_service", Value: "running"})
			return output.Encode(c.OutOrStdout(), program.Format(), output.Object{Fields: append(fields, host...)})
		},
	}
}

// hostFields describes the host service. A failed reading is a state of the host service, never a failure of
// the command: a user who reaches the channel but is no administrator gets "running" without details.
func hostFields(ctx context.Context) []output.Field {
	var st hostservice.StatusResult
	err := call(ctx, hostEndpoint(), control.StatusCommand, nil, &st)
	var remote *control.Error
	switch {
	case err == nil:
		return []output.Field{{Name: "host_service", Value: "running"}, {Name: "host_version", Value: st.Version},
			{Name: "host_started_at", Value: st.StartedAt.UTC().Format(time.RFC3339)}}
	case errors.Is(err, client.ErrNotRunning):
		return []output.Field{{Name: "host_service", Value: "not running"}}
	case errors.As(err, &remote) && remote.Code == control.CodeNotAdmin:
		return []output.Field{{Name: "host_service", Value: "running (reserved for administrators)"}}
	case errors.Is(err, client.ErrNoAccess):
		return []output.Field{{Name: "host_service", Value: "unknown (no access to its channel)"}}
	}
	return []output.Field{{Name: "host_service", Value: "unreachable"}}
}

// hookActive reports whether systemd runs the webhook receiver; it reads the state and changes nothing, so it
// needs no root. Tests replace it.
var hookActive = func(ctx context.Context) bool {
	return exec.CommandContext(ctx, "systemctl", "is-active", "--quiet", deploysetup.HookService+".service").Run() == nil
}

// hookFields describes the webhook receiver: not set up, or its state from systemd with the address and the
// number of hooks from its configuration, which everybody may read. Nothing here fails the command.
func hookFields(ctx context.Context) []output.Field {
	root := deploySetup.host.Root
	if !deploysetup.HookInstalled(root) {
		return []output.Field{{Name: "hook_service", Value: "not set up"}}
	}
	state := "not running"
	if hookActive(ctx) {
		state = "running"
	}
	out := []output.Field{{Name: "hook_service", Value: state}}
	cfg, err := deploysetup.HookConfig(root)
	if err != nil {
		return append(out, output.Field{Name: "hook_config", Value: "unreadable: " + err.Error()})
	}
	return append(out, output.Field{Name: "hook_listen", Value: cfg.Listen}, output.Field{Name: "hook_count", Value: len(cfg.Hooks)})
}

// instanceFields describes every deploy service instance set up on the host. As for the host service, a failed
// reading is a state of the instance, never a failure of the command.
func instanceFields(ctx context.Context) []output.Field {
	list, err := deploysetup.Instances(deploySetup.host.Root)
	if err != nil {
		return []output.Field{{Name: "instances", Value: "unknown: " + err.Error()}}
	}
	var out []output.Field
	for _, inst := range list {
		p := "instance." + inst.User() + "."
		out = append(out, output.Field{Name: p + "unit", Value: inst.Unit()})
		var st control.StatusResult
		err := call(ctx, instanceEndpoint(inst), control.StatusCommand, nil, &st)
		var remote *control.Error
		state := "unreachable"
		switch {
		case err == nil:
			out = append(out, output.Field{Name: p + "deploy_service", Value: "running"},
				output.Field{Name: p + "version", Value: st.Version},
				output.Field{Name: p + "started_at", Value: st.StartedAt.UTC().Format(time.RFC3339)})
			out = append(out, entryFields(p, rootlessPrefix+inst.User(), st.Engine)...)
			continue
		case errors.Is(err, client.ErrNotRunning):
			state = "not running"
			if cause, readable := stateCause(inst.StateDir()); cause != "" {
				out = append(out, output.Field{Name: p + "state", Value: "unreadable: " + cause})
			} else if !readable {
				out = append(out, output.Field{Name: p + "state", Value: stateHint})
			}
		case errors.As(err, &remote) && (remote.Code == control.CodeNoAccess || remote.Code == control.CodeNotAdmin):
			state = "running (reserved for administrators)"
		case errors.Is(err, client.ErrNoAccess):
			state = "unknown (no access to its channel)"
		}
		out = append(out, output.Field{Name: p + "deploy_service", Value: state})
	}
	return out
}

// stateHint is shown where the state files of a service that does not run could not be checked.
const stateHint = "unknown (run 'sudo qhost status' to check its state files)"

// stateCause checks the state files of a deploy service that does not run, changing nothing. cause names the
// file when a format number refusal or another read error is found; readable is false when the files could not
// be read for lack of permission, which only root has.
func stateCause(dir string) (cause string, readable bool) {
	err := service.CheckState(dir)
	switch {
	case err == nil:
		return "", true
	case errors.Is(err, fs.ErrPermission):
		return "", false
	}
	return err.Error(), true
}

// withStateCause adds to the failure of a deploy service that is not running the file that keeps it from starting,
// or the way to find out.
func withStateCause(err error, dir string) error {
	var f *cli.Failure
	if !deployInstalled() || !errors.As(err, &f) {
		return err
	}
	if cause, readable := stateCause(dir); cause != "" {
		f.Message += "; its state cannot be read: " + cause
		f.NextStep = "Fix or move aside the file named above (see 'qhost help troubleshooting'), then start it with 'sudo systemctl start qhost-deploy'."
	} else if !readable {
		f.NextStep += " If it does not start, run 'sudo qhost status' to check its state files."
	}
	return err
}

// engineFields flattens the engine part into scalar fields, which every output format can carry. The stack
// folders and stacks carry the engine of the deploy service that was asked.
func engineFields(engine string, e control.EngineStatus) []output.Field {
	var out []output.Field
	if !e.LastPoll.IsZero() {
		out = append(out, output.Field{Name: "last_poll", Value: e.LastPoll.UTC().Format(time.RFC3339)})
	}
	for _, f := range [][2]string{{"poll_error", e.PollError}, {"note", e.Note}, {"head", e.Head}, {"remote_head", e.RemoteHead}} {
		if f[1] != "" {
			out = append(out, output.Field{Name: f[0], Value: f[1]})
		}
	}
	return append(out, entryFields("", engine, e)...)
}

// entryFields flattens the stack folders and stacks of the engine part. The keys start with prefix, which is
// empty for the deploy service of the system engine and instance.<user>. for an instance, so that one name
// never stands for two engines.
func entryFields(prefix, engine string, e control.EngineStatus) []output.Field {
	out := sourceFields(prefix, engine, e.Sources)
	for i, s := range e.Stacks {
		// The entry that stands for a name keeps the short key; another source's entry of it carries its source.
		p := prefix + "stack." + s.Name + "."
		if !control.Primary(e.Stacks, i) {
			p = prefix + "source." + s.Source + ".stack." + s.Name + "."
		}
		out = append(out, output.Field{Name: p + "engine", Value: engine}, output.Field{Name: p + "source", Value: s.Source},
			output.Field{Name: p + "deployed", Value: s.Deployed}, output.Field{Name: p + "pending", Value: s.Pending})
		if s.ActiveRun != "" {
			out = append(out, output.Field{Name: p + "active_run", Value: s.ActiveRun})
		}
		if s.LastRun != "" {
			out = append(out, output.Field{Name: p + "last_run", Value: s.LastRun})
		}
		out = append(out, findingsField(p, s.Findings)...)
		out = append(out, imageFields(p, s.Images)...)
		if s.Removed {
			out = append(out, output.Field{Name: p + "removed", Value: true})
		}
		if s.Paused != nil {
			out = append(out, output.Field{Name: p + "paused",
				Value: "since " + s.Paused.Since.UTC().Format(time.RFC3339) + ", run " + s.Paused.Run})
		}
		if s.OperatorPause != nil {
			out = append(out, output.Field{Name: p + "operator_pause", Value: "since " + s.OperatorPause.Since.UTC().Format(time.RFC3339)})
		}
		if s.Pinned != nil {
			out = append(out, output.Field{Name: p + "pinned", Value: "since " + s.Pinned.Since.UTC().Format(time.RFC3339)})
		}
		if s.Stopped != nil {
			out = append(out, output.Field{Name: p + "stopped", Value: "since " + s.Stopped.Since.UTC().Format(time.RFC3339)})
		}
		if r := s.Repair; r != nil {
			out = append(out, output.Field{Name: p + "repair", Value: repairText(r)})
		}
		out = append(out, updateFields(p, s)...)
	}
	return out
}

// sourceFields flattens the stack folders of the engine part.
func sourceFields(prefix, engine string, sources []control.SourceStatus) []output.Field {
	var out []output.Field
	for _, src := range sources {
		p := prefix + "source." + src.ID + "."
		out = append(out, output.Field{Name: p + "path", Value: src.Path}, output.Field{Name: p + "engine", Value: engine},
			output.Field{Name: p + "kind", Value: src.Kind})
		if !src.LastPoll.IsZero() {
			out = append(out, output.Field{Name: p + "last_poll", Value: src.LastPoll.UTC().Format(time.RFC3339)})
		}
		for _, f := range [][2]string{{"poll_error", src.PollError}, {"note", src.Note}, {"git_unsafe", src.GitUnsafe}, {"head", src.Head}, {"remote_head", src.RemoteHead}} {
			if f[1] != "" {
				out = append(out, output.Field{Name: p + f[0], Value: f[1]})
			}
		}
		// Only the system engine checks .git, and source secure is for its folders alone.
		if src.GitUnsafe != "" && prefix == "" {
			out = append(out, output.Field{Name: p + "git_unsafe_fix", Value: "sudo qhost source secure " + src.Path})
		}
		if src.Unreadable != "" {
			out = append(out, output.Field{Name: p + "state", Value: "unreadable: " + src.Unreadable})
		}
	}
	return out
}

// updateFields are the observation time and the locked versions of a stack, as far as it has them.
func updateFields(p string, s control.StackStatus) []output.Field {
	var out []output.Field
	if o := s.Observe; o != nil {
		out = append(out, output.Field{Name: p + "observe", Value: "until " + o.Until.UTC().Format(time.RFC3339) + ", update " + o.Commit})
	}
	if len(s.FailedVersions) > 0 {
		out = append(out, output.Field{Name: p + "failed_versions", Value: control.FailedVersionsText(s.FailedVersions)})
	}
	return out
}

// repairText is the repair state as one line: halted since when, or the attempts made and the next one due.
func repairText(r *control.RepairInfo) string {
	text := fmt.Sprintf("%d of %d attempts", r.Attempts, r.Max)
	if r.Halted != nil {
		return "halted since " + r.Halted.Since.UTC().Format(time.RFC3339) + " after " + text
	}
	if !r.Next.IsZero() {
		text += ", next " + r.Next.UTC().Format(time.RFC3339)
	}
	return text
}

func call(ctx context.Context, ep localipc.ServiceEndpoint, command string, args, result any) error {
	return callFor(ctx, client.CallTimeout, ep, command, args, result)
}

// callFor is call with its own time limit, for commands that wait on a remote server.
func callFor(ctx context.Context, limit time.Duration, ep localipc.ServiceEndpoint, command string, args, result any) error {
	return client.Do(client.WithOrigin(ctx, control.OriginCLI), ep, limit, command, args, result)
}

// remoteNextStep is the next step for a refusal of the service that no case of failureOf names. The caller does
// not know which service answered, so the log hint names both units.
func remoteNextStep(code string) string {
	switch code {
	case control.CodeInternal:
		return "Read the cause in the service log: 'journalctl -u qhost-deploy', or 'journalctl -u qhost-deploy@<user>' for an instance."
	case control.CodeVersion, control.CodeUnknownCommand:
		return "Check that the client and the service are the same version."
	}
	return "Read the message above, correct what it names and run the command again."
}

// failureOf turns a channel error into a diagnostic with a next step.
func failureOf(err error) error {
	var remote *control.Error
	switch {
	case errors.Is(err, client.ErrNotRunning):
		if !deployInstalled() {
			return notSetUp(err)
		}
		return &cli.Failure{Code: output.CodeUnreachable, Message: "the deploy service is not running",
			NextStep: "Start it with 'sudo systemctl start qhost-deploy', then run the command again.", Cause: err}
	case errors.As(err, &remote) && remote.Code == control.CodeNotRoot:
		return &cli.Failure{Code: output.CodePermission, Message: "only root may use the service; run the command with sudo",
			NextStep: "Run the same command with sudo.", Cause: err}
	case errors.As(err, &remote) && remote.Code == control.CodeNoAccess:
		return &cli.Failure{Code: output.CodePermission, Message: "qhost is reserved for administrators (sudo)",
			NextStep: "Ask an administrator for access, or run the command as root.", Cause: err}
	case errors.Is(err, client.ErrNoAccess):
		return &cli.Failure{Code: output.CodePermission, Message: "this user may not use the qhost service channel",
			NextStep: "Ask an administrator to give this user access to qhost, or run the command with sudo.", Cause: err}
	case errors.As(err, &remote) && remote.Code == control.CodeKeyExists:
		return &cli.Failure{Code: output.CodeConfirmationRequired, Message: remote.Message, Usage: true,
			NextStep: "Run 'qhost key generate --replace' to replace it; the old key stops working at once.", Cause: err}
	case errors.As(err, &remote) && remote.Code == control.CodeInvalid:
		return &cli.Failure{Code: output.CodeInvalidRequest, Message: remote.Message, Usage: true,
			NextStep: "Fix the argument and run the command again.", Cause: err}
	case errors.As(err, &remote) && remote.Code == control.CodeNotFound:
		return &cli.Failure{Code: output.CodeNotFound, Message: remote.Message,
			Cause: err}
	case errors.As(err, &remote) && remote.Code == control.CodeNoSuchKey:
		f := &cli.Failure{Code: output.CodeNotFound, Message: remote.Message, Cause: err}
		// The message of a stack folder's key already names the command with its --dir.
		if !strings.Contains(remote.Message, "qhost key generate") {
			f.NextStep = "Create it first with 'qhost key generate'."
		}
		return f
	case errors.As(err, &remote) && remote.Code == control.CodeNoSuchCredential:
		return &cli.Failure{Code: output.CodeNotFound, Message: remote.Message,
			NextStep: "List the credentials with 'qhost credential list'.", Cause: err}
	case errors.As(err, &remote) && remote.Code == control.CodeNoSuchSecret:
		return &cli.Failure{Code: output.CodeNotFound, Message: remote.Message,
			NextStep: "List the secrets with 'qhost stack secret list <stack>'.", Cause: err}
	case errors.As(err, &remote) && remote.Code == control.CodeNoSuchSource:
		return &cli.Failure{Code: output.CodeNotFound, Message: remote.Message,
			NextStep: "List the bound folders with 'qhost source list'.", Cause: err}
	case errors.As(err, &remote) && remote.Code == control.CodeNoRepo:
		return &cli.Failure{Code: output.CodeNotFound, Message: remote.Message,
			NextStep: "Bind the stacks repository with 'qhost repo set <ssh-url>'.", Cause: err}
	case errors.As(err, &remote) && remote.Code == control.CodeRepoBound:
		return &cli.Failure{Code: output.CodeInvalidRequest, Message: remote.Message, Usage: true,
			NextStep: "Run 'qhost repo set' with the URL already bound to that folder, or bind the repository to another folder with --dir.",
			Cause:    err}
	case errors.As(err, &remote) && remote.Code == control.CodeHostKey:
		return &cli.Failure{Code: output.CodeAuth, Message: remote.Message,
			NextStep: "Verify the new host key with the git platform's operator before changing anything; the pinned key is never replaced by this command.",
			Cause:    err}
	case errors.As(err, &remote) && remote.Code == control.CodeRemote:
		return &cli.Failure{Code: output.CodeUnreachable, Message: remote.Message,
			NextStep: "Check the URL, the network and that the deploy key is entered in the git platform ('qhost key show').", Cause: err}
	case errors.As(err, &remote) && remote.Code == control.CodeSecrets:
		f := secretsFailure(remote.Message)
		f.Cause = err
		return f
	case errors.As(err, &remote) && remote.Code == control.CodeRunActive:
		return &cli.Failure{Code: output.CodeRuntime, Message: remote.Message,
			NextStep: "Wait for the active run to end ('qhost stack runs'), then run the command again.", Cause: err}
	case errors.As(err, &remote) && remote.Code == control.CodePaused:
		return &cli.Failure{Code: output.CodeRuntime, Message: remote.Message,
			NextStep: "Find out why the rollback failed ('qhost stack runs <stack>', 'qhost stack log <run>'), fix the stack, then run 'qhost stack resume <stack>'.",
			Cause:    err}
	case errors.As(err, &remote) && remote.Code == control.CodePinned:
		return &cli.Failure{Code: output.CodeRuntime, Message: remote.Message,
			NextStep: "Unpin the stack with 'qhost stack unpin <stack>', then run the command again.", Cause: err}
	case errors.As(err, &remote) && remote.Code == control.CodeStackStopped:
		return &cli.Failure{Code: output.CodeInvalidRequest, Message: remote.Message, Usage: true,
			NextStep: "Run 'qhost stack start <stack>' to start it and clear the mark, then run the command again.", Cause: err}
	case errors.As(err, &remote) && remote.Code == control.CodeStackAction:
		return &cli.Failure{Code: output.CodeRuntime, Message: remote.Message,
			NextStep: "Check the containers with 'qhost container list' and the service log; a start does not create missing containers.", Cause: err}
	case errors.As(err, &remote) && remote.Code == control.CodeNotPaused:
		return &cli.Failure{Code: output.CodeInvalidRequest, Message: remote.Message,
			NextStep: "Check the stack with 'qhost status'; only a paused stack can be resumed.", Cause: err}
	case errors.As(err, &remote) && remote.Code == control.CodeLocalChanges:
		return &cli.Failure{Code: output.CodeRuntime, Message: remote.Message,
			NextStep: "Resolve the local changes in the working copy of the stack; the stack is skipped until then.", Cause: err}
	case errors.As(err, &remote) && remote.Code == control.CodeNoSuchStack:
		return &cli.Failure{Code: output.CodeNotFound, Message: remote.Message,
			NextStep: "List the stacks with 'qhost stack list'.", Cause: err}
	case errors.As(err, &remote) && remote.Code == control.CodeNoSettings:
		return &cli.Failure{Code: output.CodeNotFound, Message: remote.Message,
			NextStep: "Nothing to remove; set update or check with the TUI or 'qhost stack settings set'.", Cause: err}
	case errors.As(err, &remote) && remote.Code == control.CodeNoSuchRun:
		return &cli.Failure{Code: output.CodeNotFound, Message: remote.Message,
			NextStep: "List the runs with 'qhost stack runs'.", Cause: err}
	case errors.As(err, &remote) && remote.Code == control.CodeUnreadable:
		return &cli.Failure{Code: output.CodeConfigInvalid, Message: remote.Message,
			NextStep: "Find the problem with 'qhost stack check', fix it in the stacks repository and run the command again.", Cause: err}
	case errors.As(err, &remote) && remote.Code == control.CodeNoDeployer:
		return &cli.Failure{Code: output.CodeRuntime, Message: remote.Message,
			NextStep: "Stacks are only reported in this version; nothing can be deployed yet.", Cause: err}
	case errors.As(err, &remote) && remote.Code == control.CodeConfirmation:
		return &cli.Failure{Code: output.CodeConfirmationRequired, Message: remote.Message, Usage: true,
			NextStep: "Confirm the action explicitly.", Cause: err}
	case errors.As(err, &remote) && remote.Code == control.CodeNoBackups:
		return &cli.Failure{Code: output.CodeRuntime, Message: remote.Message,
			NextStep: "Update the service to a version that can back up stacks.", Cause: err}
	case errors.As(err, &remote) && remote.Code == control.CodeNoBackupRules:
		return &cli.Failure{Code: output.CodeInvalidRequest, Message: remote.Message, Usage: true,
			NextStep: "Add a backup section to the stack settings in the qhost TUI or with 'qhost stack settings set' (see 'qhost help stack-settings').", Cause: err}
	case errors.As(err, &remote) && remote.Code == control.CodeNoSuchBackup:
		return &cli.Failure{Code: output.CodeNotFound, Message: remote.Message,
			NextStep: "List the backups with 'qhost stack backup list <stack>'.", Cause: err}
	case errors.As(err, &remote) && remote.Code == control.CodeOtherEngine:
		return &cli.Failure{Code: output.CodeInvalidRequest, Message: remote.Message,
			NextStep: "Restore the backup through the service of the engine it was made in ('qhost help backups'); another backup of this engine can be chosen from 'qhost stack backup list <stack>'.", Cause: err}
	case errors.As(err, &remote) && remote.Code == control.CodeBackupFailed:
		return &cli.Failure{Code: output.CodeRuntime, Message: remote.Message,
			NextStep: "Check the service log, then check that the stack runs; a failed backup is removed and a stopped stack is started again.", Cause: err}
	case errors.As(err, &remote) && remote.Code == control.CodeBadVersion:
		return &cli.Failure{Code: output.CodeInvalidRequest, Message: remote.Message, Usage: true,
			NextStep: "Choose a version of the same variant above the current one; 'qhost stack updates <stack> --refresh' shows them.", Cause: err}
	case errors.As(err, &remote) && remote.Code == control.CodeRegistry:
		return &cli.Failure{Code: output.CodeUnreachable, Message: remote.Message,
			NextStep: "Check the network and the registry login (see 'qhost help credentials'), then run the command again.", Cause: err}
	case errors.As(err, &remote) && remote.Code == control.CodeNotUpdatable:
		return &cli.Failure{Code: output.CodeRuntime, Message: remote.Message,
			NextStep: "Resolve the cause named in the message (see 'qhost status' and 'qhost help stack-settings'), then run the command again.", Cause: err}
	case errors.As(err, &remote) && remote.Code == control.CodeBusy:
		return &cli.Failure{Code: output.CodeRuntime, Message: remote.Message,
			NextStep: "Run the command again in a moment.", Cause: err}
	case errors.As(err, &remote) && remote.Code == control.CodeHeadMoved:
		return &cli.Failure{Code: output.Code(control.CodeHeadMoved), Message: remote.Message,
			NextStep: "Read the changes again with 'qhost source changes <path>' and the head with 'qhost source list', then run the command again.",
			Cause:    err}
	case errors.As(err, &remote) && remote.Code == control.CodeNotChanged:
		return &cli.Failure{Code: output.Code(control.CodeNotChanged), Message: remote.Message,
			NextStep: "Read the changes again with 'qhost source changes <path>' and name only files it lists, then run the command again.",
			Cause:    err}
	case errors.As(err, &remote) && remote.Code == control.CodePushRejected:
		return &cli.Failure{Code: output.Code(control.CodePushRejected), Message: remote.Message,
			NextStep: "Nothing was committed or reverted. If the remote branch moved, read the changes again with 'qhost source changes <path>' " +
				"and run the command again. If the message names a refusal such as a protected branch, change the setting at the git platform first.",
			Cause: err}
	case errors.As(err, &remote):
		return &cli.Failure{Code: output.CodeRuntime, Message: "the service refused the request: " + remote.Message,
			NextStep: remoteNextStep(remote.Code), Cause: err}
	}
	return &cli.Failure{Code: output.CodeRuntime, Message: "cannot talk to the service: " + err.Error(),
		NextStep: "Check that the service is healthy and run the command again.", Cause: err}
}
