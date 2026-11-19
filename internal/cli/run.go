package cli

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/hostservice"
)

// hostEndpoint and hostStateDir name the channel and the state of the host service; tests replace them.
var (
	hostEndpoint = control.HostEndpoint
	hostStateDir = func() string { return hostservice.DefaultStateDir }
)

func newRunCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "run",
		Short: "Run the qhost host service in the foreground",
		Long: "run is what the host service unit starts: it runs until it receives SIGTERM or SIGINT, then ends with\n" +
			"exit code 0. It logs to stderr.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			ctx, stop := signal.NotifyContext(c.Context(), syscall.SIGTERM, syscall.SIGINT)
			defer stop()
			return hostservice.Run(ctx, slog.New(slog.NewTextHandler(c.ErrOrStderr(), nil)).With("pid", os.Getpid()),
				hostservice.Config{Version: version, Endpoint: hostEndpoint(), StateDir: hostStateDir()})
		},
	}
}
