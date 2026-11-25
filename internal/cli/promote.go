package cli

import (
	"fmt"
	"regexp"
	"time"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-kit/cli"
	"github.com/castrowithcee/qatlas-kit/cobracli"
	"github.com/castrowithcee/qatlas-kit/output"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

var promotePreviewColumns = []string{"stack", "variable", "from", "to", "result", "source_commit"}

// sourceCommitForm is the form of a full commit id, SHA-1 or SHA-256.
var sourceCommitForm = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

// The results of the preview rows besides resultCurrent.
const (
	previewSet       = "set"
	previewUnchanged = "unchanged"
	previewKept      = "kept"
)

func newPromoteCommand(program *cobracli.Program, state *notRunning) *cobra.Command {
	var engineName, sourceEngine, sourceCommit string
	var gate *cobracli.Gate
	cmd := &cobra.Command{
		Use:   "promote <source> <target> [--confirm]",
		Short: "Set the image versions of a stack to those another stack runs, as one secured run",
		Long: "promote sets every image version variable of <target> whose image <source> also runs to the version the\n" +
			"source runs. The versions come from the commit the source deployed last, not from its working copy, and\n" +
			"images are matched by repository, not by variable name; images only one of the stacks has stay as they\n" +
			"are. The run is an update of the target: it stops the stack and backs up its data if it has backup rules,\n" +
			"commits all new versions to its .env in one commit, pushes it, then pulls, starts and checks the stack,\n" +
			"and a failing run is rolled back; 'qhost stack rollback' goes back afterwards.\n\n" +
			"The target is refused as for 'qhost stack update <target> --version': no settings, never deployed,\n" +
			"local changes, paused, stopped, pinned, or a run in progress. Each version must be in the registry's tag\n" +
			"list, of the same variant and higher than the target's; a lower version is refused. The same version\n" +
			"is nothing to do and succeeds. The source is refused when its last run did not succeed, when it has a\n" +
			"commit its last run failed at, or when it is not healthy (failure, halted repair, paused or stopped); an\n" +
			"observation time that still runs does not stop the promotion and is shown.\n\n" +
			"Without --confirm the command shows the versions and the source and changes nothing. With --confirm it\n" +
			"runs, bound to the source commit that was shown: when the source deployed another commit since, it\n" +
			"refuses and a new preview is needed. The preview names the full commit and the command to confirm it:\n" +
			"--source-commit <commit> runs with exactly that commit; without it --confirm reads the source commit\n" +
			"when it runs. It exits with 1 when the run did not succeed. Needs sudo.",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) != 2 {
				return usageFailure("promote takes exactly two arguments, the source and the target stack",
					"Run 'qhost stack promote <source> <target>'.")
			}
			if args[0] == args[1] {
				return usageFailure("the source and the target are the same stack", "Name two different stacks.")
			}
			return nil
		},
		RunE: func(c *cobra.Command, args []string) error {
			if sourceCommit != "" && !sourceCommitForm.MatchString(sourceCommit) {
				return usageFailure("--source-commit is not a full commit id: "+sourceCommit,
					"Use the 40 or 64 hex digits of the commit the preview showed.")
			}
			return runPromote(c, program, state, gate, promoteSides{source: args[0], target: args[1], sourceEngine: sourceEngine,
				targetEngine: engineName, commit: sourceCommit})
		},
	}
	addEngineFlag(cmd, &engineName, stackEngineUsage, stackEngineNote+" The source and the target may be in different engines on the same host: "+
		"--engine names the engine of the target and --source-engine that of the source.")
	cmd.Flags().StringVar(&sourceEngine, "source-engine", "", "Docker engine of the source stack: "+engineIDs+
		"; needed only when several engines have a stack of that name")
	cmd.Flags().StringVar(&sourceCommit, "source-commit", "", "with --confirm: run with exactly this source commit, as the preview showed it; "+
		"the run is refused when the source deployed another commit since")
	gate = program.AddGate(cmd, cobracli.Gates{Change: true})
	return cmd
}

// showPromoteSource tells where the versions come from and whether the source is still under observation.
func showPromoteSource(c *cobra.Command, p control.PromoteCheckResult, s promoteSides) {
	_, _ = fmt.Fprintf(c.ErrOrStderr(), "Source %s at commit %s: last run succeeded, healthy.\n", p.Source, p.SourceCommit)
	if o := p.Observe; o != nil {
		_, _ = fmt.Fprintf(c.ErrOrStderr(), "The source is under observation until %s: its update is not settled yet.\n", o.Until.Local().Format(time.DateTime))
	}
	confirm := "sudo qhost stack promote " + s.source + " " + s.target
	if s.sourceEngine != "" {
		confirm += " --source-engine " + s.sourceEngine
	}
	if s.targetEngine != "" {
		confirm += " --engine " + s.targetEngine
	}
	_, _ = fmt.Fprintf(c.ErrOrStderr(), "Confirm with: %s --confirm --source-commit %s\n", confirm, p.SourceCommit)
	_, _ = fmt.Fprintf(c.ErrOrStderr(), "Promote to %s:\n", p.Stack)
}

func promoteRows(p control.PromoteCheckResult) []output.Row {
	commit := p.SourceCommit
	rows := make([]output.Row, 0, len(p.Steps)+len(p.Unchanged)+len(p.Kept))
	for _, s := range p.Steps {
		rows = append(rows, output.Row{"stack": p.Stack, "variable": s.Variable, "from": s.From, "to": s.To, "result": previewSet, "source_commit": commit})
	}
	for _, v := range p.Unchanged {
		rows = append(rows, output.Row{"stack": p.Stack, "variable": v, "result": previewUnchanged, "source_commit": commit})
	}
	for _, v := range p.Kept {
		rows = append(rows, output.Row{"stack": p.Stack, "variable": v, "result": previewKept, "source_commit": commit})
	}
	if len(p.Steps) == 0 {
		return []output.Row{{"stack": p.Stack, "result": resultCurrent}}
	}
	return rows
}

func encodePromoteRows(c *cobra.Command, program *cobracli.Program, rows []output.Row) error {
	return output.Encode(c.OutOrStdout(), program.Format(), output.Collection{Columns: promotePreviewColumns, Rows: rows})
}

// showPromote prints the steps of the run like 'stack update' does.
func showPromote(c *cobra.Command, program *cobracli.Program, res control.PromoteRunResult) error {
	item := control.StackUpdateRun{Name: res.Stack, Steps: make([]control.UpdateStepInfo, 0, len(res.Steps))}
	for _, s := range res.Steps {
		item.Steps = append(item.Steps, control.UpdateStepInfo{Variable: s.Variable, From: s.From, To: s.To, Run: res.Run,
			Commit: res.Commit, Result: res.Result, Error: res.Error})
	}
	rows, failed := updateRunRows(control.UpdateRunResult{Stacks: []control.StackUpdateRun{item}}, program.Format() == output.FormatTable)
	if err := output.Encode(c.OutOrStdout(), program.Format(), output.Collection{Columns: updateRunColumns, Rows: rows}); err != nil {
		return err
	}
	if failed > 0 {
		return &cli.Failure{Code: output.CodeRuntime, Message: "the promotion did not succeed: " + res.Result,
			NextStep: "Find out why with 'qhost stack log " + res.Run + "'."}
	}
	return nil
}

// promoteSides name the source and the target and the engines they are asked in.
type promoteSides struct{ source, target, sourceEngine, targetEngine, commit string }

func runPromote(c *cobra.Command, program *cobracli.Program, state *notRunning, gate *cobracli.Gate, p promoteSides) error {
	// Without --confirm this is the error that ends the command after the preview.
	unconfirmed := gate.Run(cobracli.Request{Changes: true}, func() error { return nil })
	// The source is read at the deploy service that has it, the check and the run reach the one of the
	// target, which applies every rule itself.
	if err := routeStacks(c, state, p.sourceEngine, p.source); err != nil {
		return err
	}
	var src control.PromoteSource
	if err := remoteCall(c, state, remoteCallLimit, control.PromoteSourceCommand, control.PromoteSourceArgs{Stack: p.source}, &src); err != nil {
		return err
	}
	if err := routeStacks(c, state, p.targetEngine, p.target); err != nil {
		return err
	}
	a := control.PromoteArgs{Stack: p.target, Source: src}
	var plan control.PromoteCheckResult
	if err := remoteCall(c, state, updateCallLimit, control.PromoteCheckCommand, a, &plan); err != nil {
		return err
	}
	if len(plan.Steps) == 0 {
		_, _ = fmt.Fprintf(c.ErrOrStderr(), "Nothing to promote: %s already has the versions of %s for the images they share.\n", p.target, p.source)
		return encodePromoteRows(c, program, promoteRows(plan))
	}
	if unconfirmed != nil {
		showPromoteSource(c, plan, p)
		if err := encodePromoteRows(c, program, promoteRows(plan)); err != nil {
			return err
		}
		return unconfirmed
	}
	a.Commit = plan.SourceCommit
	if p.commit != "" {
		a.Commit = p.commit
	}
	var res control.PromoteRunResult
	if err := remoteCall(c, state, updateCallLimit, control.PromoteRunCommand, a, &res); err != nil {
		return err
	}
	return showPromote(c, program, res)
}
