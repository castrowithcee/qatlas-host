package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-kit/cli"
	"github.com/castrowithcee/qatlas-kit/cobracli"
	"github.com/castrowithcee/qatlas-kit/output"

	"github.com/castrowithcee/qatlas-host/internal/deploysetup"
)

// hookQuestion is the question of the receiver setup; a non-local address says that HTTPS belongs to a reverse
// proxy, because the receiver speaks plain HTTP.
func hookQuestion(listen string) string {
	q := "Set up the webhook receiver on " + listen + " with these steps?"
	if local, err := deploysetup.ValidateHookListen(listen); err == nil && !local {
		q += " The address is not local and the receiver speaks plain HTTP: a reverse proxy should provide HTTPS."
	}
	return q
}

func newHookSetupCommand(program *cobracli.Program) *cobra.Command {
	var gate *cobracli.Gate
	var listen string
	cmd := &cobra.Command{
		Use:   "setup [--listen <host:port>]",
		Short: "Set up the webhook receiver as its own system user, or change its address",
		Long: "setup creates what the webhook receiver needs and starts it: the system user and group qhost-hook, with no\n" +
			"login, no home and no other group, /etc/qhost-hook with the configuration config.json (root:qhost-hook,\n" +
			"mode 0644, no hooks at first), the marker folders /run/qhost-hook and one per deploy service instance\n" +
			"(made at every boot by /etc/tmpfiles.d/qhost-hook.conf) and the unit qhost-hook.service, which it enables\n" +
			"and starts. The user never joins the group docker or qhost. A running deploy service whose marker folder\n" +
			"the setup creates restarts with systemctl try-restart to be able to write it; a stopped one stays stopped.\n" +
			"--listen <host:port> sets the address. Without it the first setup listens on " + deploysetup.DefaultHookListen + ", only\n" +
			"local, and a later setup keeps the stored address. The port must be above 1024. A changed address is\n" +
			"written to the configuration and restarts the receiver; hooks stay as they are. The receiver speaks plain\n" +
			"HTTP: for an address that is not local a reverse proxy should provide HTTPS.\n" +
			"Setting up needs root. It first shows what it will do and the exact commands; without --confirm it stops\n" +
			"there and changes nothing. With --confirm it asks for confirmation on a terminal, and --yes answers the\n" +
			"question. Only the steps still needed run, so a repeated setup changes nothing, and a setup that stopped\n" +
			"at a step continues from there.\n" +
			"See 'qhost help hook-setup'.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if deploySetup.euid() != 0 {
				return &cli.Failure{Code: output.CodePermission, Message: "setting up the webhook receiver needs root; run with sudo",
					NextStep: "Run the same command with sudo."}
			}
			if c.Flags().Changed("listen") {
				if _, err := deploysetup.ValidateHookListen(listen); err != nil {
					return &cli.Failure{Code: output.CodeInvalidRequest, Message: err.Error(), Usage: true,
						NextStep: "Give --listen as <host:port> with a port above 1024."}
				}
			} else {
				listen = ""
			}
			ctx := c.Context()
			plan, err := deploySetup.host.PlanHook(ctx, listen)
			if err != nil {
				return hookSetupFailure(err, false)
			}
			if plan.Empty() {
				return emitHookSetup(c, program, plan, 0)
			}
			_, _ = fmt.Fprint(c.ErrOrStderr(), planText(plan))
			if err := gate.Run(cobracli.Request{Changes: true}, func() error { return nil }); err != nil {
				return err
			}
			return gate.Run(cobracli.Request{Question: hookQuestion(plan.Listen), Changes: true}, func() error {
				if err := plan.Apply(ctx); err != nil {
					return hookSetupFailure(err, true)
				}
				return emitHookSetup(c, program, plan, len(plan.Steps))
			})
		},
	}
	cmd.Flags().StringVar(&listen, "listen", "",
		"address of the receiver as <host:port>, port above 1024 (first setup: "+deploysetup.DefaultHookListen+"; later: the stored address)")
	gate = program.AddGate(cmd, cobracli.Gates{Question: true, Change: true})
	return cmd
}

func hookSetupFailure(err error, started bool) *cli.Failure {
	f := setupFailure(err, started)
	f.Message = "webhook receiver setup: " + err.Error()
	return f
}

func emitHookSetup(c *cobra.Command, program *cobracli.Program, plan deploysetup.Plan, steps int) error {
	return output.Encode(c.OutOrStdout(), program.Format(), output.Object{Fields: []output.Field{
		{Name: "set_up", Value: true},
		{Name: "changed", Value: steps > 0},
		{Name: "steps", Value: steps},
		{Name: "unit", Value: deploysetup.HookService + ".service"},
		{Name: "listen", Value: plan.Listen},
	}})
}

// refreshHookCommand is the internal command 'qhost update' runs in the newly installed program.
const refreshHookCommand = "refresh"

func newHookRefreshCommand(program *cobracli.Program) *cobra.Command {
	return &cobra.Command{
		Use:    refreshHookCommand,
		Short:  "Bring the webhook receiver to the unit of this program",
		Hidden: true,
		Long: "refresh is run by 'qhost update' in the newly installed program, because only that program knows its unit.\n" +
			"If the webhook receiver is set up, it replaces qhost-hook.service where it differs from the one built into this\n" +
			"program, runs systemctl daemon-reload in that case and restarts the receiver; it warns when a unit in\n" +
			"/etc/systemd/system overrides the installed one. Without the setup it changes nothing. Needs root.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if deploySetup.euid() != 0 {
				return &cli.Failure{Code: output.CodePermission, Message: "renewing the webhook receiver needs root; run with sudo",
					NextStep: "Run the same command with sudo."}
			}
			res, err := deploySetup.host.RefreshHook(c.Context())
			if res.Shadowed {
				_, _ = fmt.Fprintf(c.ErrOrStderr(), "warning: a unit in /etc/systemd/system overrides the installed unit of %s\n", deploysetup.HookService)
			}
			if err != nil {
				return &cli.Failure{Code: output.CodeRuntime, Message: "hook-service refresh: " + err.Error(), Cause: err,
					NextStep: "Fix the cause named in the message, then run the command again."}
			}
			return output.Encode(c.OutOrStdout(), program.Format(), output.Object{Fields: []output.Field{
				{Name: "unit_written", Value: res.UnitWritten},
				{Name: "restarted", Value: res.Restarted},
			}})
		},
	}
}
