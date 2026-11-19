package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-kit/cli"
	"github.com/castrowithcee/qatlas-kit/cobracli"
	"github.com/castrowithcee/qatlas-kit/output"

	"github.com/castrowithcee/qatlas-host/internal/deploysetup"
	"github.com/castrowithcee/qatlas-host/internal/tui"
)

// deploySetupHost is everything the setup of the deploy service takes from the host. Production code never
// changes it; tests replace it so that no real /etc, systemctl or user database is involved.
type deploySetupHost struct {
	host deploysetup.Host
	euid func() int
	// wait returns once the deploy service answers on its channel; nil does not wait.
	wait func(ctx context.Context) error
}

var deploySetup = deploySetupHost{euid: os.Geteuid, wait: waitForService}

// deployInstalled reports whether the setup installed the unit of the deploy service.
func deployInstalled() bool { return deploysetup.Installed(deploySetup.host.Root) }

// notSetUp is the diagnostic of a command that needs the deploy service before it exists. It ends with the
// exit code of a service that is not running, because for a caller waiting on the service both mean that
// nothing answers, and only the next step differs.
func notSetUp(cause error) *cli.Failure {
	return &cli.Failure{Code: output.CodeUnreachable, Message: "the deploy service is not set up; run sudo qhost deploy-service setup",
		NextStep: "Run 'sudo qhost deploy-service setup', then run the command again.", Cause: cause}
}

func (h deploySetupHost) requireRoot() error {
	if h.euid() != 0 {
		return &cli.Failure{Code: output.CodePermission, Message: "setting up the deploy service needs root; run with sudo",
			NextStep: "Run the same command with sudo."}
	}
	return nil
}

// setupFailure is a setup that cannot start or stopped at a step; a repeated setup continues from there.
func setupFailure(err error, started bool) *cli.Failure {
	next := "Fix the cause named in the message; nothing was changed. Then run the command again."
	if started {
		next = "Fix the cause named in the message, then run the command again; steps already done are not repeated."
	}
	return &cli.Failure{Code: output.CodeRuntime, Message: "deploy service setup: " + err.Error(), NextStep: next, Cause: err}
}

// planText is the question of the setup: the sentence and, below it, the exact commands.
func planText(p deploysetup.Plan) string {
	var b strings.Builder
	b.WriteString(p.Sentence())
	b.WriteString("\nThese steps run as root:\n")
	for _, c := range p.Commands() {
		b.WriteString("  " + c + "\n")
	}
	return b.String()
}

func newDeploySetupCommand(program *cobracli.Program) *cobra.Command {
	var gate *cobracli.Gate
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Set up the deploy service as its own system user",
		Long: "setup creates what the deploy service needs and starts it: the system user and group qhost-deploy, with the\n" +
			"group docker as its only extra group and no login, its channel directory (made at every boot by\n" +
			"/etc/tmpfiles.d/qhost-deploy.conf) and the unit qhost-deploy.service, which it enables and starts. The\n" +
			"user qhost-deploy never joins the group qhost and cannot reach the host service. Setting up needs root.\n" +
			"It first shows what it will do and the exact commands and asks; it needs --confirm and asks for\n" +
			"confirmation on a terminal, and --yes answers the question. Only the steps still needed run, so a\n" +
			"repeated setup changes nothing, and a setup that stopped at a step continues from there.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if err := deploySetup.requireRoot(); err != nil {
				return err
			}
			ctx := c.Context()
			plan, err := deploySetup.host.Plan(ctx)
			if err != nil {
				return setupFailure(err, false)
			}
			if plan.Empty() {
				return emitDeploySetup(c, program, 0)
			}
			// Without --confirm the request fails before anything is shown or changed.
			if err := gate.Run(cobracli.Request{Changes: true}, func() error { return nil }); err != nil {
				return err
			}
			_, _ = fmt.Fprint(c.ErrOrStderr(), planText(plan))
			return gate.Run(cobracli.Request{Question: "Set up the deploy service with these steps?", Changes: true}, func() error {
				if err := plan.Apply(ctx); err != nil {
					return setupFailure(err, true)
				}
				if deploySetup.wait != nil {
					if err := deploySetup.wait(ctx); err != nil {
						_, _ = fmt.Fprintf(c.ErrOrStderr(), "warning: %v\n", err)
					}
				}
				return emitDeploySetup(c, program, len(plan.Steps))
			})
		},
	}
	gate = program.AddGate(cmd, cobracli.Gates{Question: true, Change: true})
	return cmd
}

func emitDeploySetup(c *cobra.Command, program *cobracli.Program, steps int) error {
	return output.Encode(c.OutOrStdout(), program.Format(), output.Object{Fields: []output.Field{
		{Name: "set_up", Value: true},
		{Name: "changed", Value: steps > 0},
		{Name: "steps", Value: steps},
	}})
}

// tuiDeploySetup is the setup as the interface offers it: it lists the steps still needed and runs them.
type tuiDeploySetup struct{}

var _ tui.DeploySetup = tuiDeploySetup{}

func (tuiDeploySetup) Installed() bool { return deployInstalled() }

func (tuiDeploySetup) Plan(ctx context.Context) ([]string, error) {
	plan, err := deploySetup.host.Plan(ctx)
	if err != nil {
		return nil, err
	}
	return plan.Commands(), nil
}

func (tuiDeploySetup) Run(ctx context.Context) error {
	if err := deploySetup.requireRoot(); err != nil {
		return err
	}
	plan, err := deploySetup.host.Plan(ctx)
	if err != nil {
		return err
	}
	if err := plan.Apply(ctx); err != nil {
		return err
	}
	if deploySetup.wait != nil {
		return deploySetup.wait(ctx)
	}
	return nil
}
