package cli

import (
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-kit/cobracli"

	"github.com/castrowithcee/qatlas-host/internal/backup"
	"github.com/castrowithcee/qatlas-host/internal/compose"
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
			"and exists only after 'sudo qhost deploy-service setup'; see 'qhost help deploy-setup'.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error { return c.Help() },
	}
	cmd.AddCommand(run, newDeploySetupCommand(program))
	return cmd
}

func newDeployRunCommand() *cobra.Command {
	var backupImage, dockerSocket string
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the deploy service in the foreground",
		Long: "run is what the deploy service unit starts: it runs until it receives SIGTERM or SIGINT, then ends with\n" +
			"exit code 0. It logs to stderr. --docker-socket names the one container engine the service uses for\n" +
			"everything: container list and actions, logs, monitoring, Compose, checks, backups and restores.",
		RunE: func(c *cobra.Command, _ []string) error {
			ctx, stop := signal.NotifyContext(c.Context(), syscall.SIGTERM, syscall.SIGINT)
			defer stop()
			backups := backup.NewManager(filepath.Join(stateDir(), "backups"), dockerSocket)
			backups.Image = backupImage
			return service.Run(ctx, slog.New(slog.NewTextHandler(c.ErrOrStderr(), nil)).With("pid", os.Getpid()),
				service.Config{Version: version, Endpoint: serviceEndpoint(), StateDir: stateDir(),
					Deployer: compose.New(backups.ForRuns(), dockerSocket), Backups: backups, DockerSocket: dockerSocket})
		},
	}
	cmd.Flags().StringVar(&backupImage, "backup-image", backup.DefaultImage,
		"helper image of backups and restores; it needs a POSIX shell, find, rm and a tar that knows --numeric-owner")
	cmd.Flags().StringVar(&dockerSocket, "docker-socket", service.DefaultDockerSocket,
		"socket of the container engine for every Docker access of the service")
	return cmd
}
