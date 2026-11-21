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
			"interval. A stack that cannot be read shows its problem and does not hide the others. Run it\n" +
			"after 'qhost repo set'; it reads the working copy as it is and does not fetch.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			var res control.StacksResult
			if err := credentialCall(c, state, control.StacksListCommand, nil, &res); err != nil {
				return err
			}
			rows := make([]output.Row, 0, len(res.Stacks))
			for _, s := range res.Stacks {
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
				row := output.Row{
					"stack": s.Name, "source": s.Source, "state": state, "update": s.Update, "trigger": s.Trigger,
					"versions": strings.Join(versions, ", "), "findings": int64(s.Findings), "error": errText,
				}
				if s.Interval > 0 {
					row["interval"] = int64(s.Interval)
				}
				rows = append(rows, row)
			}
			return output.Encode(c.OutOrStdout(), program.Format(),
				output.Collection{Columns: []string{"stack", "source", "state", "update", "trigger", "interval", "versions", "findings", "error"}, Rows: rows})
		},
	}
}
