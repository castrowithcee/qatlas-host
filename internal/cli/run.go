package cli

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/hostservice"
)

// hostEndpoint and hostStateDir name the channel and the state of the host service; tests replace them.
var (
	hostEndpoint = control.HostEndpoint
	hostStateDir = func() string { return hostservice.DefaultStateDir }
)

// protectMemory keeps other processes of the service user from attaching to or dumping the host service; tests
// replace it, so the test process stays as it is.
var protectMemory = func() error { return unix.Prctl(unix.PR_SET_DUMPABLE, 0, 0, 0, 0) }

func newRunCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "run",
		Short: "Run the qhost host service in the foreground",
		Long: "run is what the host service unit starts: it serves root and the administrators of the group qhost on its\n" +
			"channel and keeps the action log ('qhost audit') until it receives SIGTERM or SIGINT, then ends with exit\n" +
			"code 0. It logs to stderr.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if err := protectMemory(); err != nil {
				return fmt.Errorf("cannot protect the memory of the host service: %w", err)
			}
			ctx, stop := signal.NotifyContext(c.Context(), syscall.SIGTERM, syscall.SIGINT)
			defer stop()
			return hostservice.Run(ctx, slog.New(slog.NewTextHandler(c.ErrOrStderr(), nil)).With("pid", os.Getpid()),
				hostservice.Config{Version: version, Endpoint: hostEndpoint(), StateDir: hostStateDir()})
		},
	}
}
