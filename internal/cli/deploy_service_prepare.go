package cli

import (
	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-kit/cli"
	"github.com/castrowithcee/qatlas-kit/output"
)

func newDeployPrepareCommand() *cobra.Command {
	return &cobra.Command{
		Use:    "prepare",
		Short:  "Run the transitions of the deploy service before it starts",
		Hidden: true,
		Long: "prepare is the start step of the deploy service unit (ExecStartPre=+), which systemd runs as root before the\n" +
			"service opens its channel. It runs the transitions of the deploy service that the host does not have yet,\n" +
			"in order, and reports each on standard error, which is the service log. A transition changes only what\n" +
			"belongs to it and follows no links; the step touches no stack folder and neither reloads systemd nor\n" +
			"restarts the unit. A repeated run changes nothing more. If a transition fails, the command fails, the\n" +
			"service does not start, and a new start or 'sudo qhost update migrate' continues. Needs root.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if deploySetup.euid() != 0 {
				return &cli.Failure{Code: output.CodePermission, Message: "preparing the deploy service needs root; run with sudo",
					NextStep: "Start the service through its unit: sudo systemctl start qhost-deploy"}
			}
			if _, err := runTransitions(c.Context(), deploySetup.host.Root, atStart(deployServiceTransitions), c.ErrOrStderr()); err != nil {
				return &cli.Failure{Code: output.CodeRuntime, Message: "deploy-service prepare: " + err.Error(), Cause: err,
					NextStep: "Fix the cause named in the message, then start the service again or run 'sudo qhost update " + migrateCommand + "'."}
			}
			return nil
		},
	}
}
