package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-kit/cobracli"
	"github.com/castrowithcee/qatlas-kit/output"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

func newSourceLogCommand(program *cobracli.Program, state *notRunning) *cobra.Command {
	var engine string
	var limit int
	cmd := &cobra.Command{
		Use:   "log <path> [--limit <n>]",
		Short: "List the latest commits of a stack folder with their changed paths",
		Long: "log lists the latest commits of the stack folder that its head reaches, newest first along the first parent,\n" +
			"each with its short id, author date, author, subject and the paths it changed with their kind (new, modified\n" +
			"or deleted), without file contents. --limit is the number of commits (default 20, at most 200). At most 100\n" +
			"paths are listed per commit and the list stops at a size limit; the table says when paths or commits are left\n" +
			"out, and --output json gives the full ids and the flags truncated and paths_truncated. Author, subject and\n" +
			"paths are text of other people: characters a terminal would act on are replaced. It changes nothing. Root and\n" +
			"administrators in the group qhost may read; run it without sudo. This is the way to see the history of a\n" +
			"stack folder: git in the folder is not meant for it.\n\n" + wrapHelp(changesNote),
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) != 1 {
				return usageFailure("source log takes exactly one argument, the path of the stack folder", "Run 'qhost source log <path>'.")
			}
			return nil
		},
		RunE: func(c *cobra.Command, args []string) error {
			if err := folderArg(args[0]); err != nil {
				return err
			}
			if limit < 0 || limit > control.MaxLogLimit {
				return usageFailure(fmt.Sprintf("--limit must be between 1 and %d", control.MaxLogLimit), "Pass a smaller number of commits.")
			}
			if err := routeFolder(c, state, engine, args[0]); err != nil {
				return err
			}
			var res control.SourceLogResult
			if err := credentialCall(c, state, control.SourceLogCommand, control.SourceLogArgs{Path: args[0], Limit: limit}, &res); err != nil {
				return err
			}
			if structuredFormat(program.Format()) {
				if res.Commits == nil {
					res.Commits = []control.SourceLogCommit{}
				}
				fields := []output.Field{{Name: "path", Value: res.Path}, {Name: "commits", Value: structured(program.Format(), res.Commits)}}
				if res.Truncated {
					fields = append(fields, output.Field{Name: "truncated", Value: true})
				}
				return output.Encode(c.OutOrStdout(), program.Format(), output.Object{Fields: fields})
			}
			rows := make([]output.Row, 0, len(res.Commits))
			for _, cm := range res.Commits {
				rows = append(rows, output.Row{"commit": shortCommit(cm.ID), "date": cm.Date, "author": cm.Author, "subject": cm.Subject, "paths": pathsText(cm)})
			}
			if res.Truncated {
				_, _ = fmt.Fprintln(c.ErrOrStderr(), "note: more commits exist than are listed; the list was cut by its size limit or --limit")
			}
			return output.Encode(c.OutOrStdout(), program.Format(), output.Collection{
				Columns: []string{"commit", "date", "author", "subject", "paths"}, Rows: rows})
		},
	}
	addEngineFlag(cmd, &engine, folderEngineUsage, folderEngineNote)
	cmd.Flags().IntVar(&limit, "limit", 0, fmt.Sprintf("the number of commits to list (default %d, at most %d)", control.DefaultLogLimit, control.MaxLogLimit))
	return cmd
}

// pathsText lists the paths of a commit as "kind path", and says when the service cut the list.
func pathsText(cm control.SourceLogCommit) string {
	parts := make([]string, 0, len(cm.Paths)+1)
	for _, p := range cm.Paths {
		parts = append(parts, p.Kind+" "+p.Path)
	}
	if cm.PathsTruncated {
		parts = append(parts, "(more paths not shown)")
	}
	return strings.Join(parts, "; ")
}

func newSourceShowCommand(program *cobracli.Program, state *notRunning) *cobra.Command {
	var engine string
	cmd := &cobra.Command{
		Use:   "show <path> <commit>",
		Short: "Show the diff of one commit of a stack folder",
		Long: "show prints what one commit of the stack folder changed against its first parent as a unified diff. <commit>\n" +
			"is an id of at least 7 hexadecimal digits that 'qhost source log' lists; a commit the head does not reach is\n" +
			"refused. The diff holds file contents the administrator may not be allowed to read, so only root may ask for\n" +
			"it: run it with sudo. The text is cut after 256 KiB, and truncated says so; characters a terminal would act\n" +
			"on are replaced.\n\n" + wrapHelp(changesNote),
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) != 2 {
				return usageFailure("source show takes the stack folder and one commit", "Run 'qhost source show <path> <commit>'.")
			}
			return nil
		},
		RunE: func(c *cobra.Command, args []string) error {
			if err := folderArg(args[0]); err != nil {
				return err
			}
			if err := routeFolder(c, state, engine, args[0]); err != nil {
				return err
			}
			var res control.SourceShowResult
			if err := credentialCall(c, state, control.SourceShowCommand, control.SourceShowArgs{Path: args[0], Commit: args[1]}, &res); err != nil {
				return err
			}
			fields := []output.Field{{Name: "path", Value: res.Path}, {Name: "commit", Value: res.Commit}, {Name: "diff", Value: res.Diff}}
			if res.Truncated {
				fields = append(fields, output.Field{Name: "truncated", Value: true})
			}
			return output.Encode(c.OutOrStdout(), program.Format(), output.Object{Fields: fields})
		},
	}
	addEngineFlag(cmd, &engine, folderEngineUsage, folderEngineNote)
	return cmd
}
