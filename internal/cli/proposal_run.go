package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-kit/cli"
	"github.com/castrowithcee/qatlas-kit/cobracli"
	"github.com/castrowithcee/qatlas-kit/output"
	"github.com/castrowithcee/qatlas-kit/redact"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/host/hostchange"
)

// maxRunTimeout bounds --timeout so a typing mistake does not leave a command running for days.
const maxRunTimeout = time.Hour

func newProposalRunCommand(program *cobracli.Program, state *notRunning, secrets *redact.Redactor) *cobra.Command {
	var gate *cobracli.Gate
	var fingerprint string
	timeout := hostchange.DefaultShellTimeout
	c := &cobra.Command{
		Use:   "run <id> [--fingerprint <fingerprint>] [--confirm] [--timeout <duration>]",
		Short: "Run the commands of an open proposal of free commands as root (needs root)",
		Long: "run is the only way the commands of a proposal of the kind command run, and only while the switch of\n" +
			"'qhost proposal commands' is on. It shows the commands and the reason as text of the submitter, with control\n" +
			"characters replaced, and the notice: " + commandProposalNotice + ". Without --confirm it stops there and runs\n" +
			"nothing. With --confirm, and the fingerprint of the proposal as you reviewed it (or an answer at the terminal\n" +
			"when you pass none), it takes the approval for that fingerprint and runs the commands one after the other\n" +
			"with /bin/sh -c as root: in a process group of its own, with the environment PATH, LANG=C and HOME=/root\n" +
			"only, standard input closed, and a time limit for each command (--timeout, 5 minutes by default, at most 1 hour;\n" +
			"the whole group is killed when it runs out). The first command that does not exit with 0 ends the run, and\n" +
			"the commands after it do not run. The output of a command is cut at 64 KiB and known secrets are removed from\n" +
			"it. The proposal ends as applied or failed, and the commands, their exit codes and the human behind sudo go\n" +
			"to the journal, marked as not to be undone ('host undo' refuses it). The action log names the proposal and\n" +
			"the human, never the commands. Needs sudo and is refused for --agent. See 'qhost help proposals'.",
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if err := requireRoot("proposal run"); err != nil {
				return err
			}
			// Without --confirm the plan is shown and this error then ends the command; with it the commands run.
			unconfirmed := gate.Run(cobracli.Request{Changes: true}, func() error { return nil })
			return runCommandProposal(c, program, state, secrets, args[0], fingerprint, timeout, unconfirmed)
		},
	}
	c.Flags().StringVar(&fingerprint, "fingerprint", "", "fingerprint of the proposal as you reviewed it")
	c.Flags().DurationVar(&timeout, "timeout", hostchange.DefaultShellTimeout, "time limit for each command, at most 1h")
	gate = program.AddGate(c, cobracli.Gates{Change: true})
	return c
}

// loadCommandProposal reads an open proposal of the kind command from the host service and decodes its content.
func loadCommandProposal(c *cobra.Command, state *notRunning, id string) (control.ProposalShowResult, control.ProposalCommand, error) {
	var res control.ProposalShowResult
	var content control.ProposalCommand
	if err := proposalCallTo(c, state, true, control.ProposalShowCommand, control.ProposalIDArgs{ID: id}, &res); err != nil {
		return res, content, err
	}
	if res.State != control.ProposalOpen {
		return res, content, usageFailure("the proposal is "+plainText(res.State)+", not open", "List the proposals with 'qhost proposal list'.")
	}
	if res.Kind != control.ProposalKindCommand {
		return res, content, usageFailure("the proposal is of the kind "+shortText(res.Kind)+", not "+control.ProposalKindCommand,
			"Apply a proposal of the kind "+control.ProposalKindHostChange+" with 'sudo qhost host apply --proposal "+plainText(id)+"'.")
	}
	dec := json.NewDecoder(bytes.NewReader(res.Content))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&content); err != nil || len(content.Commands) == 0 || len(content.Commands) > control.ProposalCommandMax {
		return res, content, usageFailure("the content of the proposal cannot be read",
			"Reject it with 'sudo qhost proposal reject "+plainText(id)+"'.")
	}
	return res, content, nil
}

func runCommandProposal(c *cobra.Command, program *cobracli.Program, state *notRunning, secrets *redact.Redactor,
	id, reviewed string, timeout time.Duration, unconfirmed error) error {
	if program.Agent() {
		return &cli.Failure{Code: output.CodePermission, Message: "running the commands of a proposal is for a person, not for an agent",
			NextStep: "Ask the person who runs the host to review the proposal and run 'sudo qhost proposal run " + plainText(id) + "'."}
	}
	if !hostProposalID(id) {
		return usageFailure("proposal run takes a proposal of the host service, whose id starts with "+control.HostProposalIDPrefix,
			"List the proposals with 'qhost proposal list'.")
	}
	if timeout <= 0 || timeout > maxRunTimeout {
		return usageFailure("--timeout must be longer than 0 and at most "+maxRunTimeout.String(), "Pass a duration such as 30s or 10m.")
	}
	var sw control.HostCommandsResult
	if err := hostCall(c, state, control.HostCommandsStatusCommand, nil, &sw); err != nil {
		return err
	}
	if !sw.Enabled {
		return &cli.Failure{Code: output.CodePermission, Message: "proposals of commands are switched off, so nothing runs",
			NextStep: "Turn the switch on with 'sudo qhost proposal commands enable', then run the command again."}
	}
	again := "Review it with 'qhost proposal show " + plainText(id) + "' and run the command again."
	changed := func() error {
		return &cli.Failure{Code: output.CodeInvalidRequest, Usage: true, NextStep: again,
			Message: "the proposal is not the one that was shown; nothing ran"}
	}
	first, content, err := loadCommandProposal(c, state, id)
	if err != nil {
		return err
	}
	if reviewed != "" && reviewed != first.Fingerprint {
		return changed()
	}
	showCommandPlan(c.ErrOrStderr(), first.ProposalInfo, content, timeout)
	if unconfirmed != nil {
		return unconfirmed
	}
	if reviewed == "" {
		if !stdinIsTerminal() {
			return usageFailure("there is no terminal to ask; pass --fingerprint of the proposal you reviewed",
				"Review the proposal with 'qhost proposal show "+plainText(id)+"', then pass its fingerprint with --fingerprint.")
		}
		if err := askApproval(c, "Run these commands of proposal "+plainText(id)+" as root?"); err != nil {
			return err
		}
	}
	journal, err := changeEnv.openJournal()
	if err != nil {
		return changeFailed(fmt.Errorf("proposal run: %w", err))
	}

	// Last look before anything runs: the proposal must be the one that was shown.
	now, _, err := loadCommandProposal(c, state, id)
	if err != nil {
		return err
	}
	if now.Fingerprint != first.Fingerprint {
		return changed()
	}
	// The host service takes the approval first, and only for the fingerprint that was shown and while the switch is on.
	if err := proposalCallTo(c, state, true, control.ProposalApproveCommand,
		control.ProposalApproveArgs{ID: id, Fingerprint: first.Fingerprint}, &control.ProposalInfo{}); err != nil {
		return err
	}
	var sh hostchange.ShellExecutor = hostchange.SystemShell{}
	if changeEnv.shell != nil {
		sh = changeEnv.shell
	}
	res, results, runErr := changeEnv.runner(journal).RunCommands(c.Context(), sh, hostchange.CommandRun{
		ProposalID: id, Commands: content.Commands, Timeout: timeout, SudoUID: changeEnv.sudoUID(),
		SubmitterUID: int(first.SubmitterUID), SubmitterName: shortName(first.SubmitterName),
	}, secrets.Apply)
	result := control.ProposalResultArgs{ID: id, Fingerprint: first.Fingerprint, Applied: runErr == nil && res.Entry.Result == hostchange.ResultApplied}
	switch {
	case runErr != nil:
		result.Message = "nothing ran: " + secrets.Apply(runErr.Error())
	case !result.Applied:
		result.Message = res.Entry.Error
	}
	resultErr := proposalCallTo(c, state, true, control.ProposalResultCommand, result, &control.ProposalInfo{})
	if runErr != nil {
		return changeFailed(fmt.Errorf("proposal run: %s", secrets.Apply(runErr.Error())))
	}
	if res.Warning != "" {
		_, _ = fmt.Fprintf(c.ErrOrStderr(), "warning: %s\n", plainText(res.Warning))
	}
	reportErr := finishCommandRun(c, program, id, res, results)
	if resultErr != nil {
		note := "the commands ran, but the result could not be kept on the proposal"
		_, _ = fmt.Fprintf(c.ErrOrStderr(), "warning: %s\n", note)
		if reportErr == nil {
			return &cli.Failure{Code: output.CodeRuntime, Message: note, Cause: resultErr,
				NextStep: "Run 'qhost proposal show " + plainText(id) + "' to see its state."}
		}
	}
	return reportErr
}

// showCommandPlan prints what running the proposal does. Everything of the submitter is shown with its control
// characters replaced.
func showCommandPlan(w io.Writer, p control.ProposalInfo, content control.ProposalCommand, timeout time.Duration) {
	_, _ = fmt.Fprintf(w, "Proposal %s from %s, submitted %s, expires %s: %s.\n",
		plainText(p.ID), shortText(p.SubmitterName), proposalTime(p.Submitted), proposalTime(p.Expires), commandProposalNotice)
	_, _ = fmt.Fprintf(w, "These commands of the submitter run one after the other as root with /bin/sh -c, %s at most each:\n", timeout)
	for i, line := range content.Commands {
		_, _ = fmt.Fprintf(w, "  %d. %s\n", i+1, plainText(line))
	}
	_, _ = fmt.Fprintf(w, "  Reason of the submitter: %s\n", plainText(content.Reason))
	_, _ = fmt.Fprintf(w, "  Fingerprint of the proposal: %s\n", plainText(p.Fingerprint))
}

// finishCommandRun prints the report. A run that did not end with every command at exit code 0 is an error after
// the report, so that the exit code tells the caller and the report still arrives on standard output.
func finishCommandRun(c *cobra.Command, program *cobracli.Program, id string, res hostchange.Result, results []hostchange.ShellResult) error {
	format := program.Format()
	fields := []output.Field{
		{Name: "proposal_id", Value: id},
		{Name: "journal_id", Value: res.Entry.ID},
		{Name: "result", Value: res.Entry.Result},
		{Name: "notice", Value: commandProposalNotice},
	}
	if structuredFormat(format) {
		list := make([]map[string]any, len(results))
		for i, r := range results {
			list[i] = map[string]any{"command": r.Command, "ran": r.Ran, "exit_code": r.ExitCode, "output": r.Output,
				"truncated": r.Truncated, "timed_out": r.TimedOut}
		}
		fields = append(fields, output.Field{Name: "commands", Value: list})
	} else {
		for i, r := range results {
			n := strconv.Itoa(i + 1)
			fields = append(fields, output.Field{Name: "command." + n, Value: plainText(r.Command)})
			if !r.Ran {
				fields = append(fields, output.Field{Name: "status." + n, Value: "not run"})
				continue
			}
			status := "exit code " + strconv.Itoa(r.ExitCode)
			if r.TimedOut {
				status = "timed out"
			}
			if r.Truncated {
				status += ", output cut at " + strconv.Itoa(hostchange.MaxShellOutput) + " bytes"
			}
			fields = append(fields, output.Field{Name: "status." + n, Value: status},
				output.Field{Name: "output." + n, Value: outputText(r.Output)})
		}
	}
	if err := output.Encode(c.OutOrStdout(), format, output.Object{Fields: fields}); err != nil {
		return err
	}
	if res.Entry.Result != hostchange.ResultApplied {
		return &cli.Failure{Code: output.CodeRuntime, Message: "the run ended early: " + res.Entry.Error,
			NextStep:      "Read the report; nothing was undone, and the commands after the failed one did not run.",
			AgentNextStep: "The commands did not all succeed. Do not repeat them; tell the user what failed."}
	}
	return nil
}

// outputText makes the output of a command safe for a terminal: every control character but the line end, every
// bidirectional control and every invalid byte becomes U+FFFD, and a trailing line end is dropped.
func outputText(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == '\n':
			b.WriteRune(r)
		case r == utf8.RuneError && n <= 1, unicode.IsControl(r), unicode.Is(unicode.Zl, r), unicode.Is(unicode.Zp, r), isBidi(r):
			b.WriteRune(utf8.RuneError)
		default:
			b.WriteRune(r)
		}
		i += n
	}
	return strings.TrimRight(b.String(), "\n")
}

// runInstead refuses the approval of a proposal of free commands elsewhere: only 'proposal run' runs it.
func runInstead(id string) error {
	return usageFailure("a proposal of the kind "+control.ProposalKindCommand+" is run with 'sudo qhost proposal run "+plainText(id)+"', not approved here",
		"Review it with 'qhost proposal show "+plainText(id)+"', then run 'sudo qhost proposal run "+plainText(id)+" --fingerprint <fingerprint> --confirm'.")
}

// shortName is the name of the submitter as the journal keeps it: control characters replaced, at most 64 characters.
func shortName(s string) string {
	r := []rune(plainText(s))
	if len(r) > 64 {
		r = r[:64]
	}
	return string(r)
}
