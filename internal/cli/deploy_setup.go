package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-kit/cli"
	"github.com/castrowithcee/qatlas-kit/cobracli"
	"github.com/castrowithcee/qatlas-kit/localipc"
	"github.com/castrowithcee/qatlas-kit/output"

	"github.com/castrowithcee/qatlas-host/internal/deploysetup"
	"github.com/castrowithcee/qatlas-host/internal/tui"
	"github.com/castrowithcee/qatlas-host/internal/window"
)

// deploySetupHost is everything the setup of the deploy service takes from the host. Production code never
// changes it; tests replace it so that no real /etc, systemctl or user database is involved.
type deploySetupHost struct {
	host deploysetup.Host
	euid func() int
	// wait returns once a deploy service answers on its channel; nil does not wait.
	wait func(ctx context.Context, ep localipc.ServiceEndpoint) error
}

var deploySetup = deploySetupHost{euid: os.Geteuid, wait: waitForEndpoint}

// setupTarget is the deploy service a setup is for: the one of the system engine, or with user the instance for
// the rootless engine of that user. The command line and the terminal interface share it.
type setupTarget struct {
	user string
}

func (t setupTarget) plan(ctx context.Context) (deploysetup.Plan, error) {
	if t.user != "" {
		return deploySetup.host.PlanInstance(ctx, t.user)
	}
	return deploySetup.host.Plan(ctx)
}

// endpoint is the channel the service answers on once it runs; plan has checked the user name before.
func (t setupTarget) endpoint() localipc.ServiceEndpoint {
	if t.user != "" {
		if inst, err := deploysetup.NewInstance(t.user); err == nil {
			return inst.Endpoint()
		}
	}
	return serviceEndpoint()
}

func (t setupTarget) question() string {
	if t.user != "" {
		return "Set up the deploy service for the rootless engine of " + t.user + " with these steps?"
	}
	return "Set up the deploy service with these steps?"
}

// apply carries the plan out and waits for the service to answer.
func (t setupTarget) apply(ctx context.Context, plan deploysetup.Plan) error {
	if err := plan.Apply(ctx); err != nil {
		return err
	}
	if deploySetup.wait != nil {
		return deploySetup.wait(ctx, t.endpoint())
	}
	return nil
}

// adoptHostWindow makes the window the system deploy service kept the host window when the host has none, before
// an instance for a rootless engine starts and follows the host window. The deploy service cannot write /etc, so
// root does it here. A failure is a warning: the window can still be set with 'sudo qhost window set'.
func adoptHostWindow(stderr io.Writer) {
	if _, err := window.AdoptState(windowHostFile(), stateDir()); err != nil {
		_, _ = fmt.Fprintf(stderr, "warning: the maintenance window of the system deploy service was not taken over as the host window: %v\n"+
			"Run 'sudo qhost window set' to set the host window.\n", err)
	}
}

// deployInstalled reports whether the setup installed the unit of the deploy service.
func deployInstalled() bool { return deploysetup.Installed(deploySetup.host.Root) }

// deployPresent reports whether any deploy service is set up: the one of the system engine or the instance of a
// rootless engine. A host with only instances is set up; the service of the system engine can still be added.
func deployPresent() bool {
	if deployInstalled() {
		return true
	}
	list, err := folders.instances()
	return err == nil && len(list) > 0
}

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
	for _, w := range p.Warnings {
		b.WriteString("\nwarning: " + w)
	}
	b.WriteString("\nThese steps run as root:\n")
	for _, c := range p.Commands() {
		b.WriteString("  " + c + "\n")
	}
	return b.String()
}

func newDeploySetupCommand(program *cobracli.Program) *cobra.Command {
	var gate *cobracli.Gate
	var target setupTarget
	cmd := &cobra.Command{
		Use:   "setup [--rootless <user>]",
		Short: "Set up the deploy service as root in a sandbox, or for the rootless engine of a user",
		Long: "setup creates what the deploy service needs and starts it: its channel directory (made at every boot by\n" +
			"/etc/tmpfiles.d/qhost-deploy.conf) and the unit qhost-deploy.service, which it enables and starts. The\n" +
			"service runs as root in a systemd sandbox, creates no account and cannot reach the host service.\n" +
			"With --rootless <user> it sets up an instance for the rootless Docker engine of an existing user instead:\n" +
			"the unit qhost-deploy@<user>.service from the template qhost-deploy@.service runs as that user with its\n" +
			"own state /var/lib/qhost-deploy-<user>, its channel /run/qhost-deploy-<user> (made at every boot by\n" +
			"/etc/tmpfiles.d/qhost-deploy-<user>.conf) and the engine at /run/user/<uid>/docker.sock, and lingering\n" +
			"is enabled for the user when it is off. The user needs a rootless engine and entries in /etc/subuid and\n" +
			"/etc/subgid, and setup refuses a user in the group qhost or docker; it warns about an administrator. It\n" +
			"never creates a user or changes a group. Setting up needs root.\n" +
			"It first shows what it will do and the exact commands; without --confirm it stops there and changes\n" +
			"nothing. With --confirm it asks for confirmation on a terminal, and --yes answers the question. Only the\n" +
			"steps still needed run, so a repeated setup changes nothing, and a setup that stopped at a step continues\n" +
			"from there.\n" +
			"See 'qhost help deploy-setup'.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if err := deploySetup.requireRoot(); err != nil {
				return err
			}
			if c.Flags().Changed("rootless") {
				if _, err := deploysetup.NewInstance(target.user); err != nil {
					return &cli.Failure{Code: output.CodeInvalidRequest, Message: err.Error(), Usage: true,
						NextStep: "Name an existing user with a rootless Docker engine."}
				}
			}
			ctx := c.Context()
			plan, err := target.plan(ctx)
			if err != nil {
				return setupFailure(err, false)
			}
			if plan.Empty() {
				return emitDeploySetup(c, program, target, 0)
			}
			// The plan is shown first; without --confirm the request then fails before anything is changed.
			_, _ = fmt.Fprint(c.ErrOrStderr(), planText(plan))
			if err := gate.Run(cobracli.Request{Changes: true}, func() error { return nil }); err != nil {
				return err
			}
			return gate.Run(cobracli.Request{Question: target.question(), Changes: true}, func() error {
				if target.user != "" {
					adoptHostWindow(c.ErrOrStderr())
				}
				if err := plan.Apply(ctx); err != nil {
					return setupFailure(err, true)
				}
				if deploySetup.wait != nil {
					if err := deploySetup.wait(ctx, target.endpoint()); err != nil {
						_, _ = fmt.Fprintf(c.ErrOrStderr(), "warning: %v\n", err)
					}
				}
				return emitDeploySetup(c, program, target, len(plan.Steps))
			})
		},
	}
	cmd.Flags().StringVar(&target.user, "rootless", "",
		"set up an instance for the rootless Docker engine of this existing user instead of the system engine's service")
	gate = program.AddGate(cmd, cobracli.Gates{Question: true, Change: true})
	return cmd
}

func emitDeploySetup(c *cobra.Command, program *cobracli.Program, target setupTarget, steps int) error {
	fields := []output.Field{
		{Name: "set_up", Value: true},
		{Name: "changed", Value: steps > 0},
		{Name: "steps", Value: steps},
	}
	if inst, err := deploysetup.NewInstance(target.user); err == nil {
		fields = append(fields, output.Field{Name: "unit", Value: inst.Unit()})
	}
	return output.Encode(c.OutOrStdout(), program.Format(), output.Object{Fields: fields})
}

// tuiDeploySetup is the setup as the interface offers it: it lists the steps still needed and runs them.
type tuiDeploySetup struct{}

var _ tui.DeploySetup = tuiDeploySetup{}

func (tuiDeploySetup) Installed() bool { return deployPresent() }

func (tuiDeploySetup) Users() []string {
	users, err := deploySetup.host.RootlessUsers()
	if err != nil {
		return nil
	}
	return users
}

func (tuiDeploySetup) Plan(ctx context.Context, user string) (tui.SetupPlan, error) {
	plan, err := setupTarget{user: user}.plan(ctx)
	if err != nil {
		return tui.SetupPlan{}, err
	}
	return tui.SetupPlan{Commands: plan.Commands(), Warnings: plan.Warnings}, nil
}

func (tuiDeploySetup) Run(ctx context.Context, user string) error {
	if err := deploySetup.requireRoot(); err != nil {
		return err
	}
	target := setupTarget{user: user}
	plan, err := target.plan(ctx)
	if err != nil {
		return err
	}
	if user != "" {
		adoptHostWindow(io.Discard)
	}
	return target.apply(ctx, plan)
}
