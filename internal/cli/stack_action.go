package cli

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-kit/cobracli"
	"github.com/castrowithcee/qatlas-kit/output"

	"github.com/castrowithcee/qatlas-host/internal/client"
	"github.com/castrowithcee/qatlas-host/internal/control"
)

var stackActionCommands = map[string]string{
	"start": control.StackStartCommand, "stop": control.StackStopCommand, "restart": control.StackRestartCommand,
}

func newStackActionCommand(program *cobracli.Program, state *notRunning, verb string) *cobra.Command {
	var gate *cobracli.Gate
	var engine string
	long := map[string]string{
		"start": "start starts the existing, stopped containers of the stack, clears the stopped mark and runs the\n" +
			"stack's check. It never creates containers: a stack that was never deployed or whose containers are\n" +
			"removed fails; deploy it with 'qhost stack deploy'.",
		"stop": "stop stops the containers of the stack without removing them and sets the stopped mark: new commits\n" +
			"of the stacks repository, the maintenance window and updates leave the stack stopped, and 'qhost status'\n" +
			"and 'qhost stack list' show it. Only 'qhost stack start' clears the mark.",
		"restart": "restart restarts the containers of the stack and runs the stack's check. A stack stopped through\n" +
			"qhost is refused: use start.",
	}[verb]
	cmd := &cobra.Command{
		Use:   verb + " <stack> [--yes]",
		Short: map[string]string{"start": "Start a stopped stack", "stop": "Stop a stack and keep it stopped", "restart": "Restart a stack"}[verb],
		Long: long + "\n\nThe command is refused while a run of the stack is active. At a terminal it asks first (default no);\n" +
			"without a terminal it refuses unless --yes is given (with --agent, --yes is always needed). Run it with\n" +
			"sudo; a restart also runs without sudo when a grant for stack.restart covers the stack. See 'qhost help\n" +
			"containers' and 'qhost help grants'.",
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if err := routeStacks(c, state, engine, args[0]); err != nil {
				return err
			}
			question := strings.ToUpper(verb[:1]) + verb[1:] + " stack " + args[0] + "?"
			if verb == "stop" {
				question = "Stop stack " + args[0] + " and keep it stopped until it is started?"
			}
			return gate.Run(cobracli.Request{Question: question}, func() error {
				var res control.StackActionResult
				err := callFor(c.Context(), control.StackActionTimeout, targetOf(c.Context()).endpoint(), stackActionCommands[verb],
					control.StackActionArgs{Stack: args[0]}, &res)
				if errors.Is(err, client.ErrNotRunning) {
					state.seen = true
				}
				if err != nil {
					return targetFailure(c.Context(), err)
				}
				return output.Encode(c.OutOrStdout(), program.Format(), output.Object{Fields: []output.Field{
					{Name: "stack", Value: res.Stack}, {Name: "action", Value: res.Action}, {Name: "stopped", Value: res.Stopped},
				}})
			})
		},
	}
	addEngineFlag(cmd, &engine, stackEngineUsage, stackEngineNote)
	gate = program.AddGate(cmd, cobracli.Gates{Question: true})
	return cmd
}
