package cli

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-kit/cli"
	"github.com/castrowithcee/qatlas-kit/cobracli"
	"github.com/castrowithcee/qatlas-kit/localipc"
	"github.com/castrowithcee/qatlas-kit/output"

	"github.com/castrowithcee/qatlas-host/internal/backup"
	"github.com/castrowithcee/qatlas-host/internal/compose"
	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/deploysetup"
	"github.com/castrowithcee/qatlas-host/internal/hookservice"
	"github.com/castrowithcee/qatlas-host/internal/service"
)

// stateDir names the credential store of the deploy service; tests replace it.
var stateDir = func() string { return service.DefaultStateDir }

func newDeployServiceCommand(program *cobracli.Program) *cobra.Command {
	run := newDeployRunCommand()
	// The unit starts run; nobody needs it in a listing.
	run.Hidden = true
	cmd := &cobra.Command{
		Use:   "deploy-service",
		Short: "Set up the deploy service",
		Long: "The deploy service rolls out the stacks. It runs as its own system user qhost-deploy beside the host service\n" +
			"and exists only after 'sudo qhost deploy-service setup'. The rootless Docker engine of a user gets its own\n" +
			"instance, which runs as that user, after 'sudo qhost deploy-service setup --rootless <user>'; see\n" +
			"'qhost help deploy-setup'.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error { return c.Help() },
	}
	cmd.AddCommand(run, newDeploySetupCommand(program), newRefreshInstancesCommand(program))
	return cmd
}

// runTarget is what one deploy service serves: its channel, its state and its container engine.
type runTarget struct {
	endpoint localipc.ServiceEndpoint
	stateDir string
	socket   string
	// engine is the id of that engine, as in control.SourceConflict.Engine.
	engine string
	// hookDir is where the webhook receiver leaves the markers for this instance.
	hookDir string
}

// instanceTarget derives channel, state and engine of the instance of user from the instance alone. The instance
// runs as its user, so a process with another uid, and root above all, is refused.
func instanceTarget(user string, uid int) (runTarget, error) {
	inst, err := deploysetup.NewInstance(user)
	if err != nil {
		return runTarget{}, &cli.Failure{Code: output.CodeInvalidRequest, Message: err.Error(), Usage: true,
			NextStep: "Name the user of an instance that 'sudo qhost deploy-service setup --rootless <user>' set up."}
	}
	want, err := deploySetup.host.UserID(user)
	if err != nil {
		return runTarget{}, &cli.Failure{Code: output.CodeConfigInvalid, Message: "deploy service instance: " + err.Error(),
			NextStep: "Name an existing user; the instance runs as that user.", Cause: err}
	}
	if want == 0 || uid < 0 || uint32(uid) != want {
		return runTarget{}, &cli.Failure{Code: output.CodePermission,
			Message:  fmt.Sprintf("the instance of %s runs as that user (uid %d), never as uid %d", user, want, uid),
			NextStep: "Start it through its unit: sudo systemctl start " + inst.Unit()}
	}
	return runTarget{endpoint: inst.Endpoint(), stateDir: inst.StateDir(), socket: deploysetup.EngineSocket(want), engine: rootlessPrefix + user,
		hookDir: filepath.Join(hookservice.DefaultRunDir, user)}, nil
}

func newDeployRunCommand() *cobra.Command {
	var backupImage, dockerSocket, instance string
	cmd := &cobra.Command{
		Use:   "run [--instance <user>]",
		Short: "Run the deploy service in the foreground",
		Long: "run is what the deploy service unit starts: it runs until it receives SIGTERM or SIGINT, then ends with\n" +
			"exit code 0. It logs to stderr. --docker-socket names the one container engine the service uses for\n" +
			"everything: container list and actions, logs, monitoring, Compose, checks, backups and restores.\n" +
			"--instance <user> runs the instance for the rootless engine of that user, as its unit\n" +
			"qhost-deploy@<user>.service does: the channel /run/qhost-deploy-<user>, the state\n" +
			"/var/lib/qhost-deploy-<user> and the engine /run/user/<uid>/docker.sock follow from the user, so it\n" +
			"excludes --docker-socket, and it refuses to run as any other user than that one. An engine that does not\n" +
			"answer yet leaves the service running; it reports the engine as unreachable.",
		RunE: func(c *cobra.Command, _ []string) error {
			target := runTarget{endpoint: serviceEndpoint(), stateDir: stateDir(), socket: dockerSocket, engine: control.EngineSystem,
				hookDir: filepath.Join(hookservice.DefaultRunDir, hookservice.SystemInstance)}
			if c.Flags().Changed("instance") {
				if c.Flags().Changed("docker-socket") {
					return &cli.Failure{Code: output.CodeInvalidRequest, Usage: true,
						Message:  "--instance and --docker-socket exclude each other: an instance uses the engine of its user",
						NextStep: "Drop --docker-socket."}
				}
				var err error
				if target, err = instanceTarget(instance, os.Getuid()); err != nil {
					return err
				}
			}
			ctx, stop := signal.NotifyContext(c.Context(), syscall.SIGTERM, syscall.SIGINT)
			defer stop()
			backups := backup.NewManager(filepath.Join(target.stateDir, "backups"), target.stateDir, target.socket)
			backups.Image = backupImage
			return service.Run(ctx, slog.New(slog.NewTextHandler(c.ErrOrStderr(), nil)).With("pid", os.Getpid()),
				service.Config{Version: version, Endpoint: target.endpoint, StateDir: target.stateDir,
					Deployer: compose.New(backups.ForRuns(), target.stateDir, target.socket), Backups: backups, DockerSocket: target.socket, Engine: target.engine,
					HookDir: target.hookDir})
		},
	}
	cmd.Flags().StringVar(&instance, "instance", "", "run the instance for the rootless engine of this user")
	cmd.Flags().StringVar(&backupImage, "backup-image", backup.DefaultImage,
		"helper image of backups and restores; it needs a POSIX shell, find, rm and a tar that knows --numeric-owner")
	cmd.Flags().StringVar(&dockerSocket, "docker-socket", service.DefaultDockerSocket,
		"socket of the container engine for every Docker access of the service")
	return cmd
}

// refreshInstancesCommand is the internal command 'qhost update' runs in the newly installed program.
const refreshInstancesCommand = "refresh-instances"

func newRefreshInstancesCommand(program *cobracli.Program) *cobra.Command {
	cmd := &cobra.Command{
		Use:    refreshInstancesCommand,
		Short:  "Bring the deploy service instances to the template of this program",
		Hidden: true,
		Long: "refresh-instances is run by 'qhost update' in the newly installed program, because only that program knows its\n" +
			"template unit. If deploy service instances for rootless engines are set up, it replaces\n" +
			"qhost-deploy@.service where it differs from the one built into this program, runs systemctl daemon-reload and\n" +
			"restarts every instance; it warns about an instance whose unit or template an administrator overrides in\n" +
			"/etc/systemd/system. Before the restart it makes a window the system deploy service holds the host window if the host\n" +
			"has none, so that the instances follow it. Without an instance it changes nothing. A failed restart does not stop the others and\n" +
			"makes the command fail. Needs root.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if deploySetup.euid() != 0 {
				return &cli.Failure{Code: output.CodePermission, Message: "renewing the deploy service instances needs root; run with sudo",
					NextStep: "Run the same command with sudo."}
			}
			if instances, _ := deploysetup.Instances(deploySetup.host.Root); len(instances) > 0 {
				adoptHostWindow(c.ErrOrStderr())
			}
			res, err := deploySetup.host.RefreshInstances(c.Context())
			for _, unit := range res.Shadowed {
				_, _ = fmt.Fprintf(c.ErrOrStderr(), "warning: a unit in /etc/systemd/system overrides the installed unit of %s\n", unit)
			}
			if err != nil {
				return &cli.Failure{Code: output.CodeRuntime, Message: "refresh-instances: " + err.Error(), Cause: err,
					NextStep: "Fix the cause named in the message, then run the command again."}
			}
			// A list cannot be rendered as a table, the default format the update runs this command with.
			return output.Encode(c.OutOrStdout(), program.Format(), output.Object{Fields: []output.Field{
				{Name: "template_written", Value: res.TemplateWritten},
				{Name: "restarted", Value: strings.Join(res.Restarted, ", ")},
			}})
		},
	}
	return cmd
}
