package cli

import (
	"errors"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-kit/cli"
	"github.com/castrowithcee/qatlas-kit/cobracli"
	"github.com/castrowithcee/qatlas-kit/output"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/window"
)

func newWindowCommand(program *cobracli.Program, state *notRunning) *cobra.Command {
	group := &cobra.Command{
		Use:   "window",
		Short: "Show or change the maintenance window of the host or of one deploy service",
		Long: "In the maintenance window the service asks the registries for the versions of every stack and stores\n" +
			"the result, as 'qhost stack updates --refresh' does. Then it updates the stacks in automatic mode one after\n" +
			"another, as 'qhost stack update <stack>' would, within the ceiling of each stack. A stack in manual mode is\n" +
			"only reported in the service log. A stack is skipped, with a log line that says why, when it is paused or\n" +
			"pinned ('qhost stack pause', 'qhost stack pin'), paused after a failed rollback, removed, has local changes, a run\n" +
			"in progress, was never deployed or has nothing to update.\n\n" +
			"Nothing new begins once the window has closed, neither a stack nor a further version step of a stepwise\n" +
			"update; a step that has begun runs to its end. What did not begin waits for the next window. The work runs\n" +
			"once per opening, also across a restart of the service. 'qhost stack update', 'qhost stack updates --refresh' and\n" +
			"'qhost stack deploy' work at any time.\n\n" +
			"The window is one for the whole host, shared by every deploy service, unless a service has an own one. 'sudo qhost\n" +
			"window set' without --engine changes the host window; with --engine it sets the own window of that service, and\n" +
			"'sudo qhost window reset --engine <id>' lets it follow the host again. show lists the window of every deploy\n" +
			"service with its origin, host or own. show needs no sudo for an administrator in the group qhost; set and\n" +
			"reset need sudo. See 'qhost help window'.",
	}
	show := &cobra.Command{
		Use:   "show",
		Short: "Show the maintenance window of every deploy service and where it comes from",
		Long: "show prints one line per deploy service: its engine, the origin of its window (host or own), the weekdays,\n" +
			"the start time, the duration and the time zone, whether the window is open now (with open_until), when it\n" +
			"opens next and when the work of an opening last began. The times are given in the window's time zone. A\n" +
			"warning names a host window file that cannot be used, in which case the default window applies.\n\n" +
			overviewEngineNote,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			replies, err := askOverview[control.WindowResult](c, state, control.WindowShowCommand)
			if err != nil {
				return err
			}
			rows := make([]output.Row, 0, len(replies))
			for _, r := range replies {
				rows = append(rows, windowRow(r.target.id(), r.value))
			}
			return output.Encode(c.OutOrStdout(), program.Format(), output.Collection{Columns: windowColumns, Rows: rows})
		},
	}
	var days, start, duration, zone, engine string
	set := &cobra.Command{
		Use:   "set [--days <days>] [--start HH:MM] [--duration <d>] [--zone <zone>] [--engine <id>]",
		Short: "Change the maintenance window of the host, or the own window of one deploy service",
		Long: "set replaces the parts of the window given as flags and keeps the others. Without --engine it changes the\n" +
			"host window, which every deploy service without an own window follows; the change is written to " + window.HostFilePath + "\n" +
			"and applies in each running service within seconds, without a restart. With --engine it sets the own window\n" +
			"of that deploy service, starting from the window that applies to it now. The running service follows\n" +
			"at once: a window that opens earlier or later is waited for anew, and a window shortened while open\n" +
			"begins nothing after its new end.\n\n" +
			"--days takes weekday names separated by commas (mon,tue or monday,tuesday) or daily. --start takes the\n" +
			"start time HH:MM on the 24-hour clock. --duration takes whole minutes such as 2h, 90m or 1h30m, from 1m\n" +
			"to 12h. --zone takes local (the host's time zone) or an IANA time zone name such as Europe/Berlin or\n" +
			"UTC. The duration is elapsed time, also on a day the clocks change. The default window is daily at 03:00\n" +
			"for 2h, local time.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			var a control.WindowSetArgs
			flags := c.Flags()
			if flags.Changed("days") {
				a.Days = strings.Split(days, ",")
				for i := range a.Days {
					a.Days[i] = strings.TrimSpace(a.Days[i])
				}
			}
			a.Start, a.Duration, a.Zone = start, duration, zone
			for _, f := range []struct {
				name, value string
			}{{"start", start}, {"duration", duration}, {"zone", zone}} {
				if flags.Changed(f.name) && f.value == "" {
					return usageFailure("--"+f.name+" needs a value", "Run 'qhost window set --help' for the accepted values.")
				}
			}
			if a.Days == nil && a.Start == "" && a.Duration == "" && a.Zone == "" {
				return usageFailure("window set needs at least one of --days, --start, --duration and --zone",
					"Run 'qhost window set --start 02:30', for example; 'qhost window show' prints the window.")
			}
			if engine == "" {
				fields, err := setHostWindow(a)
				if err != nil {
					return err
				}
				return output.Encode(c.OutOrStdout(), program.Format(), output.Object{Fields: fields})
			}
			if err := routeInstance(c, engine); err != nil {
				return err
			}
			var res control.WindowResult
			if err := credentialCall(c, state, control.WindowSetCommand, a, &res); err != nil {
				return err
			}
			return output.Encode(c.OutOrStdout(), program.Format(),
				output.Object{Fields: append([]output.Field{{Name: "engine", Value: targetOf(c.Context()).id()}}, windowFields(res)...)})
		},
	}
	set.Flags().StringVar(&days, "days", "", "weekdays the window opens on, such as mon,wed,fri, or daily")
	set.Flags().StringVar(&start, "start", "", "start time HH:MM on the 24-hour clock")
	set.Flags().StringVar(&duration, "duration", "", "how long the window stays open, such as 2h or 1h30m")
	set.Flags().StringVar(&zone, "zone", "", "local or an IANA time zone name such as Europe/Berlin")
	addEngineFlag(set, &engine, "Docker engine whose deploy service gets an own window: "+engineIDs+"; without it the host window changes",
		"The command reaches the deploy service of the engine --engine names and never another one.")
	var resetEngine string
	reset := &cobra.Command{
		Use:   "reset --engine <id>",
		Short: "Remove the own window of a deploy service, so it follows the host window again",
		Long: "reset deletes the own window of the deploy service of the engine and lets it follow the host window again; if the\n" +
			"host has none, the default window applies. The host window itself is not touched.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if resetEngine == "" {
				return usageFailure("window reset needs --engine", "Run 'sudo qhost window reset --engine "+control.EngineSystem+"', for example.")
			}
			if err := routeInstance(c, resetEngine); err != nil {
				return err
			}
			var res control.WindowResult
			if err := credentialCall(c, state, control.WindowResetCommand, nil, &res); err != nil {
				return err
			}
			return output.Encode(c.OutOrStdout(), program.Format(),
				output.Object{Fields: append([]output.Field{{Name: "engine", Value: targetOf(c.Context()).id()}}, windowFields(res)...)})
		},
	}
	addEngineFlag(reset, &resetEngine, "Docker engine whose deploy service follows the host window again: "+engineIDs,
		"The command reaches the deploy service of the engine --engine names and never another one.")
	group.AddCommand(show, set, reset)
	return group
}

// windowEuid and windowHostFile are replaced by tests: who runs the command and where the host window lives.
var (
	windowEuid     = os.Geteuid
	windowHostFile = func() string { return window.HostFilePath }
)

// setHostWindow changes the host window file; only root writes it. Before the file exists, the window the system
// deploy service kept before the host window existed becomes its first content.
func setHostWindow(a control.WindowSetArgs) ([]output.Field, error) {
	if windowEuid() != 0 {
		return nil, &cli.Failure{Code: output.CodePermission, Message: "changing the host window needs root; run with sudo",
			NextStep: "Run the same command with sudo, or name a deploy service with --engine to set its own window."}
	}
	c, err := windowChange(a)
	var w window.Window
	if err == nil {
		w, err = window.SetHost(windowHostFile(), stateDir(), c)
	}
	var invalid *window.InvalidError
	switch {
	case errors.As(err, &invalid):
		return nil, usageFailure(invalid.Text, "Run 'qhost window set --help' for the accepted values.")
	case err != nil:
		return nil, &cli.Failure{Code: output.CodeRuntime, Message: "the host window was not changed: " + err.Error(),
			NextStep: "Fix or move aside " + windowHostFile() + " if it is damaged, then run the command again.", Cause: err}
	}
	fields := []output.Field{{Name: "origin", Value: control.WindowOriginHost},
		{Name: "days", Value: strings.Join(w.Days.Names(), ",")}, {Name: "start", Value: window.FormatStart(w.Start)},
		{Name: "duration", Value: window.FormatDuration(w.Duration)}, {Name: "zone", Value: w.Zone}}
	if next, err := w.Next(time.Now()); err == nil {
		fields = append(fields, output.Field{Name: "next_opening", Value: next.Start.Format(time.RFC3339)})
	}
	return fields, nil
}

// windowChange parses the parts a request names.
func windowChange(a control.WindowSetArgs) (window.Change, error) {
	return window.ParseChange(a.Days, a.Start, a.Duration, a.Zone)
}

// windowColumns are the columns of 'window show'.
var windowColumns = []string{"engine", "origin", "days", "start", "duration", "zone", "open", "open_until", "next_opening", "last_run", "running", "warning"}

func windowRow(engine string, w control.WindowResult) output.Row {
	row := output.Row{"engine": engine, "origin": w.Origin, "days": strings.Join(w.Days, ","), "start": w.Start,
		"duration": w.Duration, "zone": w.Zone, "open": w.Open, "next_opening": w.NextOpening.Format(time.RFC3339)}
	if w.Running {
		row["running"] = true
	}
	if w.Warning != "" {
		row["warning"] = w.Warning
	}
	if w.Open {
		row["open_until"] = w.OpenUntil.Format(time.RFC3339)
	}
	if !w.LastRun.IsZero() {
		row["last_run"] = w.LastRun.Format(time.RFC3339)
	}
	return row
}

// windowFields lists the window as scalar fields, which every output format can carry.
func windowFields(w control.WindowResult) []output.Field {
	fields := []output.Field{
		{Name: "days", Value: strings.Join(w.Days, ",")},
		{Name: "start", Value: w.Start},
		{Name: "duration", Value: w.Duration},
		{Name: "zone", Value: w.Zone},
		{Name: "origin", Value: w.Origin},
		{Name: "open", Value: w.Open},
	}
	if w.Open {
		fields = append(fields, output.Field{Name: "open_until", Value: w.OpenUntil.Format(time.RFC3339)})
	}
	fields = append(fields, output.Field{Name: "next_opening", Value: w.NextOpening.Format(time.RFC3339)})
	if !w.LastRun.IsZero() {
		fields = append(fields, output.Field{Name: "last_run", Value: w.LastRun.Format(time.RFC3339)})
	}
	if w.Running {
		fields = append(fields, output.Field{Name: "running", Value: true})
	}
	if w.Warning != "" {
		fields = append(fields, output.Field{Name: "warning", Value: w.Warning})
	}
	return fields
}
