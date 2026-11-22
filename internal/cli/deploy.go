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

var runIDShape = regexp.MustCompile(`^[0-9]{8}T[0-9]{6}Z-[0-9a-f]{8}$`)

func newDeployCommand(program *cobracli.Program, state *notRunning) *cobra.Command {
	var engine string
	cmd := &cobra.Command{
		Use:   "deploy <stack>",
		Short: "Start a manual run of a stack",
		Long: "deploy starts a run of the stack at the commit the working copy has now; it does not fetch. It returns as\n" +
			"soon as the run is started and prints its id; follow it with 'qhost stack runs' and 'qhost stack log <run>'. A\n" +
			"stack with a run in progress, with local changes, that cannot be read or that is paused is refused.\n\n" +
			"A run of a stack that is deployed and has backup rules stops the stack and backs up its data first; any\n" +
			"other run leaves it running. Then it pulls, starts and checks the new version, and starting recreates\n" +
			"only the containers whose configuration or image changed. When a later step fails, the run is rolled\n" +
			"back: the data is restored from the run's backup if there is one, a commit that puts the stack folder back\n" +
			"to its deployed state is pushed to the branch, and that state is started and checked.\n" +
			"A stack whose rollback fails is left stopped and paused until 'qhost stack resume' is run.",
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if err := routeStacks(c, state, engine, args[0]); err != nil {
				return err
			}
			var res control.DeployResult
			if err := credentialCall(c, state, control.DeployCommand, control.DeployArgs{Stack: args[0]}, &res); err != nil {
				return err
			}
			return output.Encode(c.OutOrStdout(), program.Format(), output.Object{Fields: []output.Field{
				{Name: "run", Value: res.ID}, {Name: "stack", Value: res.Stack}, {Name: "commit", Value: res.Commit},
			}})
		},
	}
	addEngineFlag(cmd, &engine, stackEngineUsage, stackEngineNote)
	return cmd
}

func newResumeCommand(program *cobracli.Program, state *notRunning) *cobra.Command {
	var engine string
	cmd := &cobra.Command{
		Use:   "resume <stack>",
		Short: "Clear the pause after a failed rollback or the halt of the automatic repair",
		Long: "resume releases a halted automatic repair of the stack, so it tries again on the next failure, and clears the\n" +
			"pause a failed rollback left on the stack, so polls and manual runs reach it again. It\n" +
			"starts nothing and leaves the stack as it is: the failed commit stays marked, so the stack runs again on\n" +
			"a new commit or with 'qhost stack deploy <stack>'. Find out first why the rollback failed with\n" +
			"'qhost stack runs <stack>' and 'qhost stack log <run>'.",
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if err := routeStacks(c, state, engine, args[0]); err != nil {
				return err
			}
			var res control.ResumeResult
			if err := credentialCall(c, state, control.ResumeCommand, control.ResumeArgs{Stack: args[0]}, &res); err != nil {
				return err
			}
			if res.Pause == nil {
				return output.Encode(c.OutOrStdout(), program.Format(), output.Object{Fields: []output.Field{
					{Name: "stack", Value: res.Stack}, {Name: "repair", Value: "released"},
				}})
			}
			return output.Encode(c.OutOrStdout(), program.Format(), output.Object{Fields: []output.Field{
				{Name: "stack", Value: res.Stack}, {Name: "paused_since", Value: res.Pause.Since.UTC().Format(time.RFC3339)},
				{Name: "run", Value: res.Pause.Run}, {Name: "reason", Value: res.Pause.Reason},
			}})
		},
	}
	addEngineFlag(cmd, &engine, stackEngineUsage, stackEngineNote)
	return cmd
}

func newRunsCommand(program *cobracli.Program, state *notRunning) *cobra.Command {
	var engine string
	runs := &cobra.Command{
		Use:   "runs [stack]",
		Short: "List the recorded runs, newest first",
		Long: "runs lists the recorded runs of all stacks, or of one stack, newest first, with trigger, result, duration and the Git tag the run set on its commit.\n" +
			"The runs are those of one deploy service: without a stack all stacks of that service.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			var a control.RunsArgs
			if len(args) == 1 {
				a.Stack = args[0]
				if err := routeStacks(c, state, engine, a.Stack); err != nil {
					return err
				}
			} else if err := routeInstance(c, engine); err != nil {
				return err
			}
			var res control.RunsResult
			if err := credentialCall(c, state, control.RunsCommand, a, &res); err != nil {
				return err
			}
			rows := make([]output.Row, 0, len(res.Runs))
			for _, r := range res.Runs {
				if r.LogUnreadable != "" {
					_, _ = fmt.Fprintf(c.ErrOrStderr(), "warning: the log of run %s cannot be read: %s\n", r.ID, r.LogUnreadable)
				}
				commit := r.Commit
				if len(commit) > 12 && program.Format() == output.FormatTable {
					commit = commit[:12]
				}
				rows = append(rows, output.Row{
					"run": r.ID, "stack": r.Stack, "commit": commit, "trigger": r.Trigger,
					"started":  r.Started.UTC().Format(time.RFC3339),
					"duration": (time.Duration(r.DurationMS) * time.Millisecond).String(),
					"result":   r.Result, "tag": r.Tag, "error": r.Error,
				})
			}
			return output.Encode(c.OutOrStdout(), program.Format(), output.Collection{
				Columns: []string{"run", "stack", "commit", "trigger", "started", "duration", "result", "tag", "error"}, Rows: rows})
		},
	}
	addEngineFlag(runs, &engine, optionalStackEngineUsage, stackEngineNote+" "+wholeEngineNote)
	return runs
}

func newLogCommand(_ *cobracli.Program, state *notRunning) *cobra.Command {
	var engine string
	logCmd := &cobra.Command{
		Use:   "log <run>",
		Short: "Print the log of one run",
		Long:  "log prints the log of the run with the given id, as listed by 'qhost stack runs'.",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if !runIDShape.MatchString(args[0]) {
				return &cli.Failure{Code: output.CodeNotFound, Message: "no such run",
					NextStep: "List the runs with 'qhost stack runs'.", Cause: fmt.Errorf("malformed run id")}
			}
			res, err := runLog(c, state, engine, args[0])
			if err != nil {
				return err
			}
			_, err = fmt.Fprint(c.OutOrStdout(), res.Text)
			return err
		},
	}
	addEngineFlag(logCmd, &engine, runEngineUsage, runEngineNote)
	return logCmd
}

// runLog reads the log of a run from the deploy service that knows the run: the one of the named engine, the
// only one of the host, or, with several, the one that answers for the id.
func runLog(c *cobra.Command, state *notRunning, engine, id string) (control.LogResult, error) {
	var res control.LogResult
	targets := []engineTarget{}
	if engine == "" {
		var err error
		if targets, err = folders.engineTargets(); err != nil {
			return res, err
		}
	}
	if len(targets) > 1 {
		t, res, err := askRun(c, state, targets, id)
		if err == nil {
			c.SetContext(withEngine(c.Context(), t))
		}
		return res, err
	}
	if engine != "" {
		t, err := folders.namedEngine(engine)
		if err != nil {
			return res, err
		}
		targets = []engineTarget{t}
	}
	if len(targets) == 1 {
		c.SetContext(withEngine(c.Context(), targets[0]))
	}
	err := credentialCall(c, state, control.LogCommand, control.LogArgs{ID: id}, &res)
	return res, err
}
