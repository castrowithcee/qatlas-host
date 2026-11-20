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
			"version variables of its compose files, their current values and its update mode (none without stack\n" +
			"settings). A stack that cannot be read shows its problem and does not hide the others. Run it\n" +
			"after 'qhost repo set'; it reads the working copy as it is and does not fetch.\n\n" + wrapHelp(overviewEngineNote),
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			replies, err := askOverview[control.StacksResult](c, state, control.StacksListCommand)
			if err != nil {
				return err
			}
			var rows []output.Row
			for _, r := range replies {
				rows = append(rows, stackRows(r.target.id(), r.value.Stacks)...)
			}
			if rows == nil {
				rows = []output.Row{}
			}
			return output.Encode(c.OutOrStdout(), program.Format(),
				output.Collection{Columns: []string{"stack", "engine", "source", "state", "update", "versions", "findings", "error"}, Rows: rows})
		},
	}
}

// stackRows is one row per stack of the deploy service of the engine.
func stackRows(engine string, stacks []control.StackInfo) []output.Row {
	rows := make([]output.Row, 0, len(stacks))
	for _, s := range stacks {
		versions := make([]string, 0, len(s.Images))
		for _, img := range s.Images {
			v := img.Version
			if v == "" {
				v = "(unset)"
			}
			versions = append(versions, img.Var+"="+v)
		}
		state := ""
		if s.Stopped != nil {
			state = "stopped since " + s.Stopped.Since.UTC().Format(time.RFC3339)
		}
		errText := s.Error
		if s.Collision != "" {
			errText = strings.Join(slices.DeleteFunc([]string{s.Error, "name-collision: " + s.Collision},
				func(p string) bool { return p == "" }), "; ")
		}
		rows = append(rows, output.Row{
			"stack": s.Name, "engine": engine, "source": s.Source, "state": state, "update": s.Update, "versions": strings.Join(versions, ", "),
			"findings": int64(s.Findings), "error": errText,
		})
	}
	return rows
}
