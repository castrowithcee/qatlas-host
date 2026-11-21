package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-kit/cli"
	"github.com/castrowithcee/qatlas-kit/cobracli"
	"github.com/castrowithcee/qatlas-kit/output"
	"github.com/castrowithcee/qatlas-kit/redact"

	"github.com/castrowithcee/qatlas-host/internal/client"
	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/host/hostchange"
)

// maxSecretInput bounds the secret input apply reads from standard input; file.write reads its content up to
// hostchange.MaxFileContent instead.
const maxSecretInput = 64 << 10

// codeChangeRefused marks a change that a protection rule or the state of the host refused before anything ran.
const codeChangeRefused output.Code = "change-refused"

// changeEnv is everything apply, undo and journal take from the host. Production code never changes it; tests
// replace it so that no real change is made.
var changeEnv = changeEnvironment{
	registry: hostchange.Default, exec: hostchange.SystemExecutor{},
	journalDir: hostchange.DefaultJournalDir, now: time.Now, sudoUID: sudoUID, record: recordChange,
}

type changeEnvironment struct {
	registry func() *hostchange.Registry
	exec     hostchange.Executor
	// journalDir and ownerUID name the journal and the user that must own it; root owns it.
	journalDir string
	ownerUID   int
	now        func() time.Time
	// sudoUID is the human behind the sudo call, 0 when unknown.
	sudoUID func() int
	// record hands a finished change to the action log of the host service.
	record hostchange.Recorder
	// shell runs the free commands of 'proposal run'; nil means the system shell.
	shell hostchange.ShellExecutor
}

// sudoUID reads SUDO_UID strictly: plain decimal digits without sign, space or leading zero. Anything else,
// and a missing variable, counts as 0, the narrow reading that root itself made the change; the journal
// and the audit entry then name root rather than a guessed user.
func sudoUID() int {
	text := os.Getenv("SUDO_UID")
	if text == "" || (len(text) > 1 && text[0] == '0') {
		return 0
	}
	uid, err := strconv.ParseUint(text, 10, 31)
	if err != nil {
		return 0
	}
	return int(uid)
}

// recordChange hands a finished change to the action log of the host service. Only a final result is
// recorded; the sentence and the commands stay in the journal.
func recordChange(a hostchange.AuditEntry) error {
	args := control.HostRecordArgs{Action: a.Action, Result: a.Result, SudoUID: uint32(a.SudoUID), JournalID: a.ID, UndoOf: a.UndoOf, ProposalID: a.ProposalID}
	err := call(context.Background(), hostEndpoint(), control.HostRecordCommand, args, nil)
	if errors.Is(err, client.ErrNotRunning) {
		return errors.New("the host service is not running")
	}
	return err
}

func (e changeEnvironment) runner(j *hostchange.Journal) *hostchange.Runner {
	return &hostchange.Runner{
		Registry: e.registry(), Journal: j, Exec: e.exec, Now: e.now, SudoUID: e.sudoUID(), Record: e.record,
	}
}

func (e changeEnvironment) openJournal() (*hostchange.Journal, error) {
	return hostchange.OpenJournal(e.journalDir, e.ownerUID)
}

func requireRoot(action string) error {
	if hostEnv.euid() != 0 {
		return &cli.Failure{Code: output.CodePermission, Message: action + " must run as root",
			NextStep:      "Repeat the command with sudo.",
			AgentNextStep: "Do not run this yourself; ask the user to run it with sudo."}
	}
	return nil
}

// changeFailed is a change that did not stand, or an error around it.
func changeFailed(err error) error {
	return &cli.Failure{Code: output.CodeRuntime, Message: err.Error(), Cause: err,
		NextStep:      "Read the report and the journal entry for what ran, then fix the cause before repeating the change.",
		AgentNextStep: "The change did not stand. Do not repeat it; tell the user what failed."}
}

func newHostApplyCommand(program *cobracli.Program, state *notRunning, secrets *redact.Redactor) *cobra.Command {
	var gate *cobracli.Gate
	var proposal, fingerprint string
	cmd := &cobra.Command{
		Use:   "apply <action-id> [args...] | apply --proposal <id> [--fingerprint <fingerprint>]",
		Short: "Change the host with a registered action or an approved proposal (needs root)",
		Long: "Runs one registered action as root: records the previous state in the journal, runs the commands,\n" +
			"checks the result and restores the previous state when the change or its check fails, then reports it\n" +
			"to the action log of the host service. A secret input (a password, an SSH key) is read from standard\n" +
			"input only, never from an argument; file.write takes its content, up to 1 MiB, the same way. A protection\n" +
			"rule refuses a change before anything runs. It first shows the sentence and the commands on standard\n" +
			"error; without --confirm it stops there with exit code 2 and changes nothing, and a secret input is not\n" +
			"read. With --confirm there is no question. See 'qhost help host-admin' for the actions.\n\n" +
			"With --proposal it applies an open proposal of the host service instead (kind host-change): it reads the\n" +
			"proposal, checks it again with the rules of proposals, shows the sentence and the commands on standard\n" +
			"error, reads the proposal once more right before the change and applies nothing if it differs from what\n" +
			"was shown, then takes the approval at the host service, runs the change the same way and keeps its result\n" +
			"on the proposal. The proposed SSH keys come from the proposal; the initial password of user.create comes\n" +
			"on standard input as {\"password\": \"...\"}. --fingerprint binds the run to the proposal as you reviewed it\n" +
			"with 'qhost proposal show'; without it you are asked at the terminal, which a user.create proposal does\n" +
			"not allow because standard input holds the password. Without --confirm the command shows the proposal\n" +
			"and stops with exit code 2, without a question and without taking the approval. Only a person at a\n" +
			"terminal may apply a proposal: --agent and a process without a terminal are refused.",
		Args: func(_ *cobra.Command, args []string) error {
			switch {
			case proposal != "" && len(args) > 0:
				return usageFailure("--proposal takes no action and no arguments; they come from the proposal",
					"Run 'sudo qhost host apply --proposal <id> --confirm'.")
			case proposal == "" && fingerprint != "":
				return usageFailure("--fingerprint belongs to --proposal", "Run 'sudo qhost host apply --proposal <id> --fingerprint <fingerprint> --confirm'.")
			case proposal == "" && len(args) == 0:
				return errNoAction
			}
			return nil
		},
		RunE: func(c *cobra.Command, args []string) error {
			if err := requireRoot("host apply"); err != nil {
				return err
			}
			// Without --confirm the plan is shown and this error then ends the command; with it the change runs.
			unconfirmed := gate.Run(cobracli.Request{Changes: true}, func() error { return nil })
			if proposal != "" {
				return applyProposal(c, program, state, secrets, proposal, fingerprint, unconfirmed)
			}
			return applyChange(c, program, secrets, args, unconfirmed)
		},
	}
	cmd.Flags().StringVar(&proposal, "proposal", "", "id of an open proposal of the host to apply")
	cmd.Flags().StringVar(&fingerprint, "fingerprint", "", "fingerprint of the proposal as you reviewed it")
	gate = program.AddGate(cmd, cobracli.Gates{Change: true})
	return cmd
}

// showChangePlan prints what a change does: the sentence and the exact commands, on standard error. Where the
// action reads a secret input, the commands show a placeholder for it.
func showChangePlan(w io.Writer, plan hostchange.Plan, secretInput bool) {
	_, _ = fmt.Fprintf(w, "%s\n", plainText(plan.Sentence))
	_, _ = fmt.Fprint(w, "These commands run as root:\n")
	for _, cmd := range plan.Commands {
		_, _ = fmt.Fprintf(w, "  %s\n", plainText(cmd.Display()))
	}
	if secretInput {
		_, _ = fmt.Fprint(w, "The secret input is read from standard input when the change runs; it is shown as a placeholder.\n")
	}
}

// previewChange shows the plan of an action and ends with the error of the missing confirmation. The plan is built
// with a placeholder for a secret input, so standard input is not read and the state of the host is not captured;
// the journal is not opened and nothing runs.
func previewChange(c *cobra.Command, program *cobracli.Program, def hostchange.Definition, args []string, unconfirmed error) error {
	plan, err := changeEnv.registry().Preview(args[0], args[1:])
	if err != nil {
		if refused, ok := refusedChange(c, program, args[0], err); ok {
			return refused
		}
		var arg *hostchange.ArgumentError
		if errors.As(err, &arg) {
			return usageFailure(err.Error(), "Correct the arguments of the action.")
		}
		return changeFailed(fmt.Errorf("host apply: %w", err))
	}
	showChangePlan(c.ErrOrStderr(), plan, def.WantsSecret)
	return unconfirmed
}

func applyChange(c *cobra.Command, program *cobracli.Program, secrets *redact.Redactor, args []string, unconfirmed error) error {
	def, ok := changeEnv.registry().Lookup(args[0])
	if !ok {
		return usageFailure(fmt.Sprintf("unknown action %q", args[0]), "Use an action id that 'qhost help host-admin' lists.")
	}
	if unconfirmed != nil {
		return previewChange(c, program, def, args, unconfirmed)
	}
	var secret []byte
	if def.WantsSecret {
		read := readChangeSecret
		if def.ID == hostchange.ActionFileWrite {
			read = readFileContent
		}
		var err error
		if secret, err = read(stdinStream()); err != nil {
			return err
		}
		secrets.Add(string(secret))
		var in hostchange.CreateInput
		if json.Unmarshal(secret, &in) == nil && in.Password != "" {
			secrets.Add(in.Password)
		}
	}
	journal, err := changeEnv.openJournal()
	if err != nil {
		return changeFailed(fmt.Errorf("host apply: %w", err))
	}
	res, err := changeEnv.runner(journal).Apply(c.Context(), args[0], args[1:], secret)
	if err != nil {
		if refused, ok := refusedChange(c, program, args[0], err); ok {
			return refused
		}
		var arg *hostchange.ArgumentError
		if errors.As(err, &arg) {
			return usageFailure(err.Error(), "Correct the arguments of the action.")
		}
		return changeFailed(fmt.Errorf("host apply: %w", err))
	}
	return finishChange(c, program, res)
}

func newHostUndoCommand(program *cobracli.Program) *cobra.Command {
	var gate *cobracli.Gate
	cmd := &cobra.Command{
		Use:   "undo <journal-id>",
		Short: "Undo a journaled change of the host (needs root)",
		Long: "Runs the inverse of a reversible change as root, with the same protection rules, check and restore\n" +
			"as 'host apply', records the undo as a new journal entry and reports it to the action log. It first\n" +
			"shows the sentence and the commands of the undo on standard error; without --confirm it stops there\n" +
			"with exit code 2 and changes nothing. With --confirm there is no question.",
		Args: exactlyOne("journal id"),
		RunE: func(c *cobra.Command, args []string) error {
			if err := requireRoot("host undo"); err != nil {
				return err
			}
			unconfirmed := gate.Run(cobracli.Request{Changes: true}, func() error { return nil })
			if unconfirmed != nil {
				return previewUndo(c, program, args[0], unconfirmed)
			}
			return gate.Run(cobracli.Request{Changes: true}, func() error {
				journal, err := changeEnv.openJournal()
				if err != nil {
					return changeFailed(fmt.Errorf("host undo: %w", err))
				}
				res, err := changeEnv.runner(journal).Undo(c.Context(), args[0])
				if err != nil {
					if refused, ok := refusedChange(c, program, "", err); ok {
						return refused
					}
					return changeFailed(fmt.Errorf("host undo: %w", err))
				}
				return finishChange(c, program, res)
			})
		},
	}
	gate = program.AddGate(cmd, cobracli.Gates{Change: true})
	return cmd
}

// previewUndo shows the plan of the undo of a journal entry and ends with the error of the missing confirmation.
// It reads the entry and changes nothing.
func previewUndo(c *cobra.Command, program *cobracli.Program, id string, unconfirmed error) error {
	journal, err := changeEnv.openJournal()
	if err != nil {
		return changeFailed(fmt.Errorf("host undo: %w", err))
	}
	plan, err := changeEnv.runner(journal).PreviewUndo(id)
	if err != nil {
		if refused, ok := refusedChange(c, program, "", err); ok {
			return refused
		}
		return changeFailed(fmt.Errorf("host undo: %w", err))
	}
	showChangePlan(c.ErrOrStderr(), plan, false)
	return unconfirmed
}

func newHostJournalCommand(program *cobracli.Program) *cobra.Command {
	return &cobra.Command{
		Use:   "journal",
		Short: "List the latest changes of the host (needs root)",
		Long: fmt.Sprintf("Lists the latest %d changes, newest first, with what an undo of each would do. The journal is readable by\n"+
			"root only.", hostchange.KeepEntries),
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if err := requireRoot("host journal"); err != nil {
				return err
			}
			journal, err := changeEnv.openJournal()
			if err != nil {
				return hostError("host journal", err)
			}
			entries, err := journal.List()
			if err != nil {
				return hostError("host journal", err)
			}
			registry := changeEnv.registry()
			result := output.Collection{Columns: []string{"id", "time", "action", "sentence", "result", "reversible",
				"sudo_uid", "undo_sentence", "undo_commands"}}
			for _, e := range entries {
				reversible := e.Reversible && e.Result == hostchange.ResultApplied && e.UndoneBy == ""
				undoSentence, undoCommands := undoPlan(registry, e, reversible)
				result.Rows = append(result.Rows, output.Row{
					"id": e.ID, "time": e.Time.UTC().Format(time.RFC3339), "action": e.Action, "sentence": e.Sentence,
					"result": e.Result, "reversible": reversible, "sudo_uid": int64(e.SudoUID),
					"undo_sentence": undoSentence, "undo_commands": listValueSep(program.Format(), undoCommands, "; "),
				})
			}
			return output.Encode(c.OutOrStdout(), program.Format(), result)
		},
	}
}

// undoPlan is the text of the undo of a reversible entry: the sentence and the displayed commands of its
// inverse, secrets as placeholders. An inverse that cannot be built leaves both empty, so no undo is offered.
func undoPlan(registry *hostchange.Registry, e hostchange.Entry, reversible bool) (string, []string) {
	def, ok := registry.Lookup(e.Action)
	if !reversible || !ok || def.Inverse == nil {
		return "", []string{}
	}
	plan, err := def.Inverse(e.Previous)
	if err != nil {
		return "", []string{}
	}
	commands := make([]string, len(plan.Commands))
	for i, c := range plan.Commands {
		commands[i] = c.Display()
	}
	return plan.Sentence, commands
}

// refusedChange reports a change that a rule refused before anything ran: the report object with the result
// "refused" and the reason, then an error for the exit code. No journal entry exists for it.
func refusedChange(c *cobra.Command, program *cobracli.Program, action string, err error, extra ...output.Field) (error, bool) {
	var refusal *hostchange.RefusedError
	if !errors.As(err, &refusal) {
		return nil, false
	}
	if emitErr := output.Encode(c.OutOrStdout(), program.Format(), output.Object{Fields: append([]output.Field{
		{Name: "journal_id", Value: ""},
		{Name: "action", Value: action},
		{Name: "sentence", Value: ""},
		{Name: "commands", Value: listValue(program.Format(), nil)},
		{Name: "result", Value: hostchange.ResultRefused},
		{Name: "check", Value: ""},
		{Name: "check_error", Value: ""},
		{Name: "restored", Value: false},
		{Name: "reversible", Value: false},
		{Name: "error", Value: refusal.Msg},
		{Name: "warning", Value: ""},
	}, extra...)}); emitErr != nil {
		return emitErr, true
	}
	return &cli.Failure{Code: codeChangeRefused, Message: "refused: " + refusal.Msg, Cause: err,
		NextStep:      "Nothing was changed. Choose another target or action, or fix what the reason names.",
		AgentNextStep: "Nothing ran. Do not try to work around the rule; tell the user the reason."}, true
}

// finishChange prints the report. A change that did not stand is an error after the report, so that the exit
// code tells the caller and the report still arrives on standard output. A change that stands but has no entry
// in the action log says so on standard error and in the report.
func finishChange(c *cobra.Command, program *cobracli.Program, res hostchange.Result, extra ...output.Field) error {
	e := res.Entry
	if err := output.Encode(c.OutOrStdout(), program.Format(), output.Object{Fields: append([]output.Field{
		{Name: "journal_id", Value: e.ID},
		{Name: "action", Value: e.Action},
		{Name: "sentence", Value: e.Sentence},
		{Name: "commands", Value: listValueSep(program.Format(), e.Commands, "; ")},
		{Name: "result", Value: e.Result},
		{Name: "check", Value: e.Check},
		{Name: "check_error", Value: e.CheckError},
		{Name: "restored", Value: e.Restored},
		{Name: "reversible", Value: e.Reversible},
		{Name: "error", Value: e.Error},
		{Name: "warning", Value: res.Warning},
	}, extra...)}); err != nil {
		return err
	}
	if res.Warning != "" {
		fmt.Fprintf(c.ErrOrStderr(), "warning: %s\n", res.Warning)
	}
	switch e.Result {
	case hostchange.ResultApplied:
		return nil
	case hostchange.ResultRestored:
		return changeFailed(fmt.Errorf("the change did not succeed and the previous state is back: %s", e.Error))
	}
	return changeFailed(fmt.Errorf("the change did not succeed: %s", e.Error))
}

func readChangeSecret(in io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(in, maxSecretInput+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxSecretInput {
		return nil, usageFailure("the secret input on standard input is too long", "Provide a shorter secret on standard input.")
	}
	data = bytes.TrimSuffix(data, []byte("\n"))
	if len(data) == 0 {
		return nil, usageFailure("this action reads its secret from standard input, which is empty", "Provide the secret on standard input.")
	}
	return data, nil
}

// readFileContent reads the content of file.write exactly as given: no newline is trimmed, and an empty input
// writes an empty file.
func readFileContent(in io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(in, hostchange.MaxFileContent+1))
	if err != nil {
		return nil, err
	}
	if len(data) > hostchange.MaxFileContent {
		return nil, usageFailure(fmt.Sprintf("the content on standard input is larger than %d bytes", hostchange.MaxFileContent),
			"Write a text file of at most 1 MiB.")
	}
	return data, nil
}
