package cli

import (
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-kit/cobracli"
	"github.com/castrowithcee/qatlas-kit/output"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

func newStacksCommand(program *cobracli.Program, state *notRunning) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the stacks of the working copy with image versions and update mode",
		Long: "list reads the working copy of the bound stacks repository and lists each stack with the image\n" +
			"version variables of its compose files, their current values, its update mode (none without stack\n" +
			"settings) and its update trigger (window, interval or webhook), with the interval in seconds for\n" +
			"interval. A stack with a promote source (update.promote_source) shows it and, beside its own versions, the\n" +
			"versions the source runs, or that the source is missing or not proven (see 'qhost help promote'). Each\n" +
			"variable shows once, as VAR=a (web) / b (worker) when services give it different values. A\n" +
			"stack that cannot be read shows its problem and does not hide the others. Run it\n" +
			"after 'qhost repo set'; it reads the working copy as it is and does not fetch.\n\n" + wrapHelp(overviewEngineNote),
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			replies, err := askOverview[control.StacksResult](c, state, control.StacksListCommand)
			if err != nil {
				return err
			}
			var rows []output.Row
			promoted := promoteVersions(c.Context(), replies)
			for _, r := range replies {
				rows = append(rows, stackRows(r.target.id(), r.value.Stacks, promoted)...)
			}
			if rows == nil {
				rows = []output.Row{}
			}
			return output.Encode(c.OutOrStdout(), program.Format(),
				output.Collection{Columns: []string{"stack", "engine", "source", "state", "update", "trigger", "interval", "versions", "promote_source", "promote_versions", "findings", "error"}, Rows: rows})
		},
	}
}

// stackRows is one row per stack of the deploy service of the engine.
func stackRows(engine string, stacks []control.StackInfo, promoted map[engineStack]string) []output.Row {
	rows := make([]output.Row, 0, len(stacks))
	for _, s := range stacks {
		vals := make([]control.VarValue, len(s.Images))
		for i, img := range s.Images {
			vals[i] = control.VarValue{Var: img.Var, Service: img.Service, Value: img.Version}
		}
		versions := control.DistinctVars(vals)
		state := ""
		if s.Stopped != nil {
			state = "stopped since " + s.Stopped.Since.UTC().Format(time.RFC3339)
		}
		errText := s.Error
		if s.Collision != "" {
			errText = strings.Join(slices.DeleteFunc([]string{s.Error, "name-collision: " + s.Collision},
				func(p string) bool { return p == "" }), "; ")
		}
		row := output.Row{
			"stack": s.Name, "engine": engine, "source": s.Source, "state": state, "update": s.Update, "trigger": s.Trigger,
			"versions": strings.Join(versions, ", "), "findings": int64(s.Findings), "error": errText,
		}
		if s.PromoteSource != "" {
			row["promote_source"], row["promote_versions"] = s.PromoteSource, promoted[engineStack{engine, s.Name}]
		}
		if s.Interval > 0 {
			row["interval"] = int64(s.Interval)
		}
		rows = append(rows, row)
	}
	return rows
}
