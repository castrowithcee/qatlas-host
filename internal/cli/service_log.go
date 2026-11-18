package cli

import (
	"context"
	"fmt"
	"io"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-kit/cli"
	"github.com/castrowithcee/qatlas-kit/cobracli"
	"github.com/castrowithcee/qatlas-kit/output"

	"github.com/castrowithcee/qatlas-host/internal/client"
	"github.com/castrowithcee/qatlas-host/internal/control"
)

func newServiceCommand(program *cobracli.Program, state *notRunning) *cobra.Command {
	group := &cobra.Command{
		Use:   "service",
		Short: "Read the log of the qhost service",
		Long: "The service commands read what the qhost service itself reports. log shows its log, including start and\n" +
			"stop, maintenance windows, the calls of the sources, warnings and errors. It changes nothing and needs\n" +
			"no sudo for an administrator in the group qhost.",
	}
	group.AddCommand(newServiceLog(program, state))
	return group
}

func newServiceLog(program *cobracli.Program, state *notRunning) *cobra.Command {
	var source, since, level string
	var tail int
	var follow bool
	cmd := &cobra.Command{
		Use:   "log [--since <time>] [--level <level>] [--tail <n>] [--follow] [--source <name>]",
		Short: "Show the log of the qhost service, also as it comes in",
		Long: "log shows the log the qhost service keeps of itself: start and stop, maintenance windows, the calls of\n" +
			"the sources, warnings and errors. It is not the output of a container ('qhost container logs') and not\n" +
			"the protocol of a run ('qhost stack log <run>'). The service keeps a bounded copy, so the oldest\n" +
			"entries fall away. Secret values are replaced by a mask and control characters by U+FFFD.\n\n" +
			"--since takes a duration back from now (90s, 15m, 2h) or a time (2026-01-02T15:04:05Z). --level shows\n" +
			"entries of at least that level: debug, info, warn or error. --tail shows the last n entries (default\n" +
			"100, 0 for all). --source names the service whose log is read; without it all are shown, and today\n" +
			"the only one is qhost. --follow keeps running, asks about once a second for new entries and ends\n" +
			"with Ctrl-C. With --output json every entry is an object.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			a := control.ServiceLogArgs{Source: source, Level: level, Tail: tail}
			var err error
			if a.Since, err = parseLogTime("--since", since, time.Now()); err != nil {
				return err
			}
			switch level {
			case "", control.LogLevelDebug, control.LogLevelInfo, control.LogLevelWarn, control.LogLevelError:
			default:
				return &cli.Failure{Code: output.CodeInvalidRequest, Usage: true,
					Message:  "--level must be debug, info, warn or error",
					NextStep: "Fix the value and run the command again."}
			}
			if tail < 0 || tail > control.ServiceLogMaxTail {
				return &cli.Failure{Code: output.CodeInvalidRequest, Usage: true,
					Message:  fmt.Sprintf("--tail must be between 0 and %d", control.ServiceLogMaxTail),
					NextStep: "Use a smaller --tail, or narrow the entries with --since."}
			}
			ctx := c.Context()
			if follow {
				var stop context.CancelFunc
				ctx, stop = signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
				defer stop()
				c.SetContext(ctx)
			}
			return readServiceLog(ctx, c, state, program.Format(), a, follow)
		},
	}
	cmd.Flags().StringVar(&source, "source", "", "read the log of this service only (default: all; today only qhost)")
	cmd.Flags().StringVar(&since, "since", "", "only entries from this time on: a duration back from now or an RFC 3339 time")
	cmd.Flags().StringVar(&level, "level", "", "only entries of at least this level: debug, info, warn or error")
	cmd.Flags().IntVar(&tail, "tail", defaultLogTail, "show only the last n entries (0 for all)")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "keep running and show new entries until interrupted")
	return cmd
}

// readServiceLog asks for the log page by page. Every later request continues at the cursor of the answer
// before, so no entry is repeated or missed; with follow it keeps asking until the context ends, which is a
// normal end.
func readServiceLog(ctx context.Context, c *cobra.Command, state *notRunning, format output.Format, a control.ServiceLogArgs, follow bool) error {
	var cursor string
	for {
		req := a
		if cursor != "" {
			req.Cursor, req.Tail, req.Since = cursor, 0, time.Time{}
		}
		var res control.ServiceLogResult
		if err := containerCall(c, state, client.CallTimeout, control.ServiceLogCommand, req, &res); err != nil {
			if follow && ctx.Err() != nil {
				return nil
			}
			return err
		}
		if err := encodeServiceLog(c.OutOrStdout(), format, res.Entries); err != nil {
			return err
		}
		if res.Cursor != "" {
			cursor = res.Cursor
		}
		if res.More {
			continue
		}
		if !follow {
			return nil
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(logsPollInterval):
		}
	}
}

func encodeServiceLog(w io.Writer, format output.Format, entries []control.ServiceLogEntry) error {
	for _, e := range entries {
		if format == output.FormatJSON || format == output.FormatTOON {
			fields := []output.Field{
				{Name: "time", Value: e.Time.Format(time.RFC3339Nano)}, {Name: "level", Value: e.Level},
				{Name: "source", Value: e.Source}, {Name: "message", Value: e.Message},
			}
			if len(e.Attrs) > 0 {
				fields = append(fields, output.Field{Name: "attrs", Value: structured(format, e.Attrs)})
			}
			if err := output.Encode(w, format, output.Object{Fields: fields}); err != nil {
				return err
			}
			continue
		}
		var b strings.Builder
		fmt.Fprintf(&b, "%s %-5s %s", e.Time.UTC().Format("2006-01-02T15:04:05.000Z"), strings.ToUpper(e.Level), e.Message)
		for _, at := range e.Attrs {
			v := at.Value
			if v == "" || strings.ContainsAny(v, " \t\"=") {
				v = strconv.Quote(v)
			}
			b.WriteString(" " + at.Key + "=" + v)
		}
		if _, err := fmt.Fprintln(w, b.String()); err != nil {
			return err
		}
	}
	return nil
}
