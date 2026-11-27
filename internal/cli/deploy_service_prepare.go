package cli

import (
	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-kit/cli"
	"github.com/castrowithcee/qatlas-kit/cobracli"
	"github.com/castrowithcee/qatlas-kit/output"
)

func newDeployPrepareCommand(program *cobracli.Program) *cobra.Command {
	return &cobra.Command{
		Use:    "prepare",
		Short:  "Run the transitions of the deploy service before it starts",
		Hidden: true,
		Long: "prepare is the start step of the deploy service unit (ExecStartPre=+), which systemd runs as root before the\n" +
			"service opens its channel. It runs the transitions of the deploy service that the host does not have yet,\n" +
			"in order, and reports each on standard error, which is the service log. A transition changes only what\n" +
			"belongs to it and follows no links. Of the stack folders the step touches only the .git of a bound folder of\n" +
			"the system engine that an earlier release shared with the group of its service account\n" +
			"(core.sharedRepository=group): it makes it root-only as 'qhost source secure' does and reports each folder. A\n" +
			"folder whose .git differs from that setup in anything else, or cannot be secured, is reported with the next\n" +
			"step, keeps the finding git-unsafe and never stops the start. The step neither reloads systemd nor\n" +
			"restarts the unit. A repeated run changes nothing more. If a transition fails, the command fails, the\n" +
			"service does not start, and a new start or 'sudo qhost update migrate' continues. Afterwards it renews the\n" +
			"installed manpages that differ from those of this program, because the self-update of an older release\n" +
			"replaces only the main page and the host service runs without root; a failure there is a warning on\n" +
			"standard error and does not stop the start. Needs root.",
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
			reportManpages(program, deploySetup.host.Root, c.ErrOrStderr())
			return nil
		},
	}
}
