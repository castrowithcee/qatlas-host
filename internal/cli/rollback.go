package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-kit/cli"
	"github.com/castrowithcee/qatlas-kit/cobracli"
	"github.com/castrowithcee/qatlas-kit/output"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/engine"
)

func newRollbackCommand(program *cobracli.Program, state *notRunning) *cobra.Command {
	var restore, keep bool
	var engineName string
	var gate *cobracli.Gate
	cmd := &cobra.Command{
		Use:   "rollback <stack> <run> --restore-data|--keep-data [--yes]",
		Short: "Set a stack back to the image versions before an update, with or without its data",
		Long: "rollback sets the image version variables that the update run <run> changed back to the values they had\n" +
			"before it; run ids come from 'qhost stack runs <stack>'. The run must be a successful update step by qhost,\n" +
			"an older one is fine. Variables the update did not change stay as they are.\n\n" +
			"One of --restore-data and --keep-data is required, with no default, so no data is lost by accident.\n" +
			"--restore-data puts back the backup that update made: everything written to the data directories and\n" +
			"volumes since is lost, and it needs that backup and backup rules for the current data. --keep-data leaves\n" +
			"the data as it is, but the older version may not understand data a newer one migrated.\n\n" +
			"The return is a secured run of its own: it stops the stack and backs up its current data when the stack has\n" +
			"backup rules, commits the old versions to the stack's .env and pushes the commit, restores the backup with\n" +
			"--restore-data, then pulls, starts and checks the stack. A failing run is rolled back like any run, with\n" +
			"the backup of the current data. After a success the stack is pinned, so the maintenance window, the\n" +
			"update check and webhooks leave it on those versions until 'qhost stack unpin'. The stack is refused as\n" +
			"for 'qhost stack update' (never deployed, local changes, run in progress, paused or stopped), but a pinned\n" +
			"stack is allowed. At a terminal the command shows the versions and asks first (default no); without a\n" +
			"terminal it refuses unless --yes is given (with --agent, --yes is always needed). It exits with 1 when the\n" +
			"return did not succeed.",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) != 2 {
				return usageFailure("rollback takes exactly two arguments, the stack and the update run",
					"Find the run with 'qhost stack runs <stack>', then run 'qhost stack rollback <stack> <run> --restore-data|--keep-data'.")
			}
			if restore == keep {
				return usageFailure("rollback needs exactly one of --restore-data and --keep-data",
					"--restore-data puts back the data from before the update; --keep-data leaves the data as it is.")
			}
			return nil
		},
		RunE: func(c *cobra.Command, args []string) error {
			stack, runID := args[0], args[1]
			if !runIDShape.MatchString(runID) {
				return &cli.Failure{Code: output.CodeNotFound, Message: "no such run",
					NextStep: "List the runs with 'qhost stack runs " + stack + "'.", Cause: fmt.Errorf("malformed run id")}
			}
			if err := routeStacks(c, state, engineName, stack); err != nil {
				return err
			}
			a := control.ReturnArgs{Stack: stack, Run: runID, Data: control.ReturnKeepData}
			if restore {
				a.Data = control.ReturnRestoreData
			}
			var plan control.ReturnCheckResult
			if err := remoteCall(c, state, updateCallLimit, control.ReturnCheckCommand, a, &plan); err != nil {
				return err
			}
			return gate.Run(cobracli.Request{Question: returnQuestion(plan)}, func() error {
				var res control.ReturnRunResult
				if err := remoteCall(c, state, updateCallLimit, control.ReturnRunCommand, a, &res); err != nil {
					return err
				}
				return showReturn(c, program, res)
			})
		},
	}
	cmd.Flags().BoolVar(&restore, "restore-data", false, "also restore the backup that update made; the data written since is lost")
	cmd.Flags().BoolVar(&keep, "keep-data", false, "leave the data as it is")
	addEngineFlag(cmd, &engineName, stackEngineUsage, stackEngineNote)
	gate = program.AddGate(cmd, cobracli.Gates{Question: true})
	return cmd
}

// returnQuestion names the versions the return sets and what the data choice means for the data.
func returnQuestion(p control.ReturnCheckResult) string {
	steps := make([]string, 0, len(p.Steps))
	for _, s := range p.Steps {
		steps = append(steps, fmt.Sprintf("%s %s -> %s", s.Variable, s.From, s.To))
	}
	data := "The data is left as it is; the older version may not understand data a newer one migrated."
	if p.Data == control.ReturnRestoreData {
		data = "The data is restored from backup " + p.Backup + ": everything written since that update is lost."
	}
	return fmt.Sprintf("Return stack %s to the versions before run %s (%s)? %s The stack is pinned afterwards.",
		p.Stack, p.Run, strings.Join(steps, ", "), data)
}

func showReturn(c *cobra.Command, program *cobracli.Program, res control.ReturnRunResult) error {
	commit := res.Commit
	if program.Format() == output.FormatTable && len(commit) > maxShownCommit {
		commit = commit[:maxShownCommit]
	}
	steps := make([]string, 0, len(res.Steps))
	for _, s := range res.Steps {
		steps = append(steps, fmt.Sprintf("%s %s -> %s", s.Variable, s.From, s.To))
	}
	ok := res.Result == engine.ResultSucceeded
	fields := []output.Field{
		{Name: "stack", Value: res.Stack}, {Name: "update_run", Value: res.Run}, {Name: "data", Value: res.Data},
		{Name: "versions", Value: strings.Join(steps, ", ")}, {Name: "run", Value: res.ReturnRun},
		{Name: "commit", Value: commit}, {Name: "result", Value: res.Result}, {Name: "pinned", Value: ok},
	}
	if res.Error != "" {
		fields = append(fields, output.Field{Name: "error", Value: res.Error})
	}
	if ok {
		fields = append(fields, output.Field{Name: "note",
			Value: "the stack is pinned on these versions; release it with 'qhost stack unpin " + res.Stack + "'"})
	}
	if err := output.Encode(c.OutOrStdout(), program.Format(), output.Object{Fields: fields}); err != nil {
		return err
	}
	if !ok {
		return &cli.Failure{Code: output.CodeRuntime, Message: "the return did not succeed: " + res.Result,
			NextStep: "Find out why with 'qhost stack log " + res.ReturnRun + "'."}
	}
	return nil
}
