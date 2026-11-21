package cli

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-kit/cobracli"

	"github.com/castrowithcee/qatlas-host/internal/hookservice"
)

// hookServiceOptions names the configuration, the marker folder and the expected owner of the webhook
// receiver; tests replace it.
var hookServiceOptions = func() hookservice.Options {
	return hookservice.Options{ConfigPath: hookservice.DefaultConfigPath, RunDir: hookservice.DefaultRunDir, OwnerUID: 0}
}

func newHookServiceCommand(program *cobracli.Program) *cobra.Command {
	run := &cobra.Command{
		Use:   "run",
		Short: "Run the webhook receiver in the foreground",
		Long: "run is what the webhook receiver unit starts: it accepts POST /hook/<secret> on the address of its\n" +
			"configuration, discards the body and drops an empty marker for the stack of the hook. It runs until it\n" +
			"receives SIGTERM or SIGINT, then ends with exit code 0. It logs to stderr, never the secret.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			ctx, stop := signal.NotifyContext(c.Context(), syscall.SIGTERM, syscall.SIGINT)
			defer stop()
			opts := hookServiceOptions()
			opts.Logger = slog.New(slog.NewTextHandler(c.ErrOrStderr(), nil)).With("pid", os.Getpid())
			return hookservice.Run(ctx, opts)
		},
	}
	// The unit starts run, so nobody needs it in a listing; setup is the visible part.
	run.Hidden = true
	cmd := &cobra.Command{
		Use:   "hook-service",
		Short: "Set up the webhook receiver",
		Long: "The webhook receiver is its own service, set up on request, that turns a call of a secret address into an\n" +
			"empty marker for a stack. See 'qhost help hook-setup'.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error { return c.Help() },
	}
	cmd.AddCommand(run, newHookSetupCommand(program), newHookRefreshCommand(program))
	return cmd
}
