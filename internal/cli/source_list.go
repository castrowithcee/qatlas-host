package cli

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-kit/cobracli"
	"github.com/castrowithcee/qatlas-kit/output"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

func newSourceListCommand(program *cobracli.Program, state *notRunning) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the bound stack folders with kind, address, head and state of the last poll",
		Long: "list shows every bound stack folder in the order it was bound: its path, id, kind (remote or local), the\n" +
			"repository address (none for a local folder), the head commit, the time of the last poll, a poll error\n" +
			"and a note about why the working copy was not brought up to date, and its engine. It changes nothing. Root and administrators in the group qhost may read; run it without sudo.\n\n" +
			wrapHelp(overviewEngineNote),
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			replies, err := askOverview[control.SourceListResult](c, state, control.SourceListCommand)
			if err != nil {
				return err
			}
			var sources []engineSource
			for _, r := range replies {
				for _, src := range r.value.Sources {
					sources = append(sources, engineSource{SourceStatus: src, Engine: r.target.id()})
				}
			}
			return output.Encode(c.OutOrStdout(), program.Format(), sourceListResult(program.Format(), sources))
		},
	}
}

// engineSource is a bound stack folder with the engine whose deploy service holds it.
type engineSource struct {
	control.SourceStatus
	Engine string `json:"engine"`
}

func sourceListResult(format output.Format, sources []engineSource) output.Object {
	if structuredFormat(format) {
		if sources == nil {
			sources = []engineSource{}
		}
		return output.Object{Fields: []output.Field{{Name: "sources", Value: structured(format, sources)}}}
	}
	var f []output.Field
	for _, src := range sources {
		p := "source." + src.ID + "."
		f = append(f, output.Field{Name: p + "path", Value: src.Path}, output.Field{Name: p + "engine", Value: src.Engine},
			output.Field{Name: p + "kind", Value: src.Kind})
		if !src.LastPoll.IsZero() {
			f = append(f, output.Field{Name: p + "last_poll", Value: src.LastPoll.UTC().Format(time.RFC3339)})
		}
		for _, kv := range [][2]string{{"url", src.URL}, {"head", src.Head}, {"poll_error", src.PollError}, {"note", src.Note}, {"git_unsafe", src.GitUnsafe}} {
			if kv[1] != "" {
				f = append(f, output.Field{Name: p + kv[0], Value: kv[1]})
			}
		}
	}
	return output.Object{Fields: f}
}

func newSourceRemoveCommand(program *cobracli.Program, state *notRunning) *cobra.Command {
	var gate *cobracli.Gate
	var engine string
	remove := &cobra.Command{
		Use:   "remove <path>",
		Short: "Release a bound stack folder; no file is deleted and no container is stopped",
		Long: "remove releases the binding of the folder and drops the engine state of the source. No file in the folder\n" +
			"is deleted or changed, no container is stopped, and the permissions, the systemd drop-in entry and the\n" +
			"deploy key stay, so the folder can be bound again with 'source add'. It asks first: at a terminal by\n" +
			"answering the question, without a terminal with --yes. A source with an active run is refused. Run it\n" +
			"with sudo.",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) != 1 {
				return usageFailure("source remove takes exactly one argument, the path of the folder",
					"Run 'qhost source remove <path>'.")
			}
			return nil
		},
		RunE: func(c *cobra.Command, args []string) error {
			path := args[0]
			if !filepath.IsAbs(path) || filepath.Clean(path) != path {
				return usageFailure("the path must be absolute and clean", "Pass the absolute path of the folder.")
			}
			if err := routeFolder(c, state, engine, path); err != nil {
				return err
			}
			w := c.ErrOrStderr()
			_, _ = fmt.Fprintf(w, "Release the stack folder %s:\n", path)
			for _, l := range []string{
				"Only the binding is released and the engine state of the source is removed.",
				"No file in the folder is deleted, no container is stopped.",
				"The permissions, the systemd drop-in entry and the deploy key stay, so the folder can be bound again.",
			} {
				_, _ = fmt.Fprintln(w, "  "+l)
			}
			question := fmt.Sprintf("Release the stack folder %s?", path)
			return gate.Run(cobracli.Request{Question: question}, func() error {
				var res control.SourceRemoveResult
				if err := credentialCall(c, state, control.SourceRemoveCommand, control.SourceRemoveArgs{Path: path}, &res); err != nil {
					return err
				}
				return output.Encode(c.OutOrStdout(), program.Format(), output.Object{Fields: []output.Field{
					{Name: "id", Value: res.ID}, {Name: "path", Value: res.Path},
				}})
			})
		},
	}
	addEngineFlag(remove, &engine, folderEngineUsage, folderEngineNote)
	gate = program.AddGate(remove, cobracli.Gates{Question: true})
	return remove
}
