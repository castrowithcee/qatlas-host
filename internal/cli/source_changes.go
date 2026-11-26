package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-kit/cobracli"
	"github.com/castrowithcee/qatlas-kit/output"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

const changesNote = "<path> is the path of a bound stack folder, as 'qhost source list' shows it; the files are paths relative to " +
	"that folder, as 'qhost source changes' lists them. Only the files that hold a stack back count, as the finding " +
	"local-changes names them: secrets.env, the data folder of a stack and new files below a bind mount of a stack are " +
	"the host's own and never appear."

// folderArg checks the stack folder of a source command: absolute and clean.
func folderArg(path string) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return usageFailure("the path must be absolute and clean", "Pass the absolute path of the bound stack folder.")
	}
	return nil
}

func newSourceChangesCommand(program *cobracli.Program, state *notRunning) *cobra.Command {
	var engine string
	cmd := &cobra.Command{
		Use:   "changes <path>",
		Short: "List the uncommitted changes of a stack folder with their kind",
		Long: "changes lists the files of the stack folder that differ from the last commit, for all its stacks in one list, each\n" +
			"with its path below the folder, its kind (modified, new, which is not versioned yet or added, or deleted) and\n" +
			"the stack it lies in. They are the changes that keep a stack out of a run and hold back the fast-forward of the\n" +
			"whole folder until they are committed or discarded with 'qhost source discard'. It changes nothing. Root and\n" +
			"administrators in the group qhost may read; run it without sudo. --output json gives the list as data.\n\n" +
			wrapHelp(changesNote),
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) != 1 {
				return usageFailure("source changes takes exactly one argument, the path of the stack folder",
					"Run 'qhost source changes <path>'.")
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
			var res control.SourceChangesResult
			if err := credentialCall(c, state, control.SourceChangesCommand, control.SourceChangesArgs{Path: args[0]}, &res); err != nil {
				return err
			}
			return output.Encode(c.OutOrStdout(), program.Format(), changesResult(program.Format(), res))
		},
	}
	addEngineFlag(cmd, &engine, folderEngineUsage, folderEngineNote)
	return cmd
}

func changesResult(format output.Format, res control.SourceChangesResult) output.Object {
	if structuredFormat(format) {
		if res.Changes == nil {
			res.Changes = []control.SourceChange{}
		}
		fields := []output.Field{{Name: "path", Value: res.Path}, {Name: "changes", Value: structured(format, res.Changes)}}
		if res.Truncated {
			fields = append(fields, output.Field{Name: "truncated", Value: true})
		}
		return output.Object{Fields: fields}
	}
	fields := []output.Field{{Name: "path", Value: res.Path}, {Name: "count", Value: len(res.Changes)}}
	for i, c := range res.Changes {
		p := fmt.Sprintf("change.%d.", i+1)
		fields = append(fields, output.Field{Name: p + "path", Value: c.Path}, output.Field{Name: p + "kind", Value: c.Kind})
		if c.Stack != "" {
			fields = append(fields, output.Field{Name: p + "stack", Value: c.Stack})
		}
	}
	if res.Truncated {
		fields = append(fields, output.Field{Name: "truncated", Value: true})
	}
	return output.Object{Fields: fields}
}

func newSourceDiffCommand(program *cobracli.Program, state *notRunning) *cobra.Command {
	var engine string
	cmd := &cobra.Command{
		Use:   "diff <path> <file>",
		Short: "Show the diff of one uncommitted change of a stack folder",
		Long: "diff shows what one file of the stack folder changed against the last commit as a unified diff; a new file\n" +
			"is shown against nothing. <file> is one the list of 'qhost source changes' shows. The diff holds file\n" +
			"contents the administrator may not be allowed to read, so only root may ask for it: run it with sudo. The\n" +
			"text is cut after 256 KiB, and truncated says so; characters a terminal would act on are replaced.\n\n" +
			wrapHelp(changesNote),
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) != 2 {
				return usageFailure("source diff takes the stack folder and one file", "Run 'qhost source diff <path> <file>'.")
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
			var res control.SourceDiffResult
			if err := credentialCall(c, state, control.SourceDiffCommand, control.SourceDiffArgs{Path: args[0], File: args[1]}, &res); err != nil {
				return err
			}
			fields := []output.Field{{Name: "path", Value: res.Path}, {Name: "file", Value: res.File}, {Name: "diff", Value: res.Diff}}
			if res.Truncated {
				fields = append(fields, output.Field{Name: "truncated", Value: true})
			}
			return output.Encode(c.OutOrStdout(), program.Format(), output.Object{Fields: fields})
		},
	}
	addEngineFlag(cmd, &engine, folderEngineUsage, folderEngineNote)
	return cmd
}

func newSourceDiscardCommand(program *cobracli.Program, state *notRunning) *cobra.Command {
	var engine string
	var gate *cobracli.Gate
	cmd := &cobra.Command{
		Use:   "discard <path> <file>... [--yes]",
		Short: "Take uncommitted changes of a stack folder back, for the named files only",
		Long: "discard takes the changes at the named files back. A modified or deleted file is restored to the last\n" +
			"commit. A new file is removed, because you named it; nothing else is ever removed, and a folder, a pattern,\n" +
			"a path outside the stack folder or a file the host owns is refused. Each file must be one 'qhost source\n" +
			"changes' lists, and when one is not, nothing is changed. The edit is gone afterwards: there is no way back\n" +
			"except a copy you made. A stack of the files with an active run is refused. When nothing else holds a stack\n" +
			"back, its finding local-changes goes at the next poll, which starts at once. At a terminal it names the files\n" +
			"and asks first (default no); without a terminal it refuses unless --yes is given (with --agent, --yes is\n" +
			"always needed). Run it with sudo; --output json gives the discarded files as data.\n\n" + wrapHelp(changesNote),
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) < 2 || len(args) > control.MaxDiscardPaths+1 {
				return usageFailure("source discard takes the stack folder and between one and 200 files",
					"Run 'qhost source discard <path> <file>...'.")
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
			question := fmt.Sprintf("Discard the changes at %s in %s? The edits are lost and cannot be brought back.",
				strings.Join(args[1:], ", "), args[0])
			return gate.Run(cobracli.Request{Question: question}, func() error {
				var res control.SourceDiscardResult
				if err := credentialCall(c, state, control.SourceDiscardCommand, control.SourceDiscardArgs{Path: args[0], Files: args[1:]}, &res); err != nil {
					return err
				}
				if res.Discarded == nil {
					res.Discarded = []string{}
				}
				fields := []output.Field{{Name: "path", Value: res.Path}}
				if structuredFormat(program.Format()) {
					fields = append(fields, output.Field{Name: "discarded", Value: structured(program.Format(), res.Discarded)})
				} else {
					for i, p := range res.Discarded {
						fields = append(fields, output.Field{Name: fmt.Sprintf("discarded.%d", i+1), Value: p})
					}
				}
				return output.Encode(c.OutOrStdout(), program.Format(), output.Object{Fields: fields})
			})
		},
	}
	addEngineFlag(cmd, &engine, folderEngineUsage, folderEngineNote)
	gate = program.AddGate(cmd, cobracli.Gates{Question: true})
	return cmd
}

func newSourceCommitCommand(program *cobracli.Program, state *notRunning) *cobra.Command {
	var engine, head string
	cmd := &cobra.Command{
		Use:   "commit <path> <file>... --head <commit>",
		Short: "Commit the named uncommitted changes of a stack folder",
		Long: "commit makes one commit of exactly the named files, deletions included, as the stack folder has them now, and\n" +
			"pushes it when the source has a remote. Nothing else of the folder is committed. --head is the full commit id\n" +
			"that 'qhost source changes' was read at (the head field of 'qhost source list'); when the branch moved since,\n" +
			"nothing is committed. Each file must be one 'qhost source changes' lists, and every stack of the files must\n" +
			"still load. Secret-like files in the commit stop it, and a remote that refuses the push, such as a protected\n" +
			"branch, is reported with its reason. The commit names the files and the person behind sudo. A stack of the\n" +
			"files with an active run is refused. Afterwards the normal run starts at once, and a stack that fails is\n" +
			"rolled back as always. Run it with sudo; --output json gives the commit and the files as data.\n\n" + wrapHelp(changesNote),
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) < 2 || len(args) > control.MaxDiscardPaths+1 {
				return usageFailure("source commit takes the stack folder and between one and 200 files",
					"Run 'qhost source commit <path> <file>... --head <commit>'.")
			}
			return nil
		},
		RunE: func(c *cobra.Command, args []string) error {
			if err := folderArg(args[0]); err != nil {
				return err
			}
			if head == "" {
				return usageFailure("--head is required", "Pass the commit id the changes were read at.")
			}
			if err := routeFolder(c, state, engine, args[0]); err != nil {
				return err
			}
			var res control.SourceCommitResult
			a := control.SourceCommitArgs{Path: args[0], Files: args[1:], Head: head, SudoUID: uint32(changeEnv.sudoUID())}
			if err := credentialCall(c, state, control.SourceCommitCommand, a, &res); err != nil {
				return err
			}
			if res.Committed == nil {
				res.Committed = []string{}
			}
			fields := []output.Field{{Name: "path", Value: res.Path}, {Name: "commit", Value: res.Commit}}
			if structuredFormat(program.Format()) {
				fields = append(fields, output.Field{Name: "committed", Value: structured(program.Format(), res.Committed)})
			} else {
				for i, p := range res.Committed {
					fields = append(fields, output.Field{Name: fmt.Sprintf("committed.%d", i+1), Value: p})
				}
			}
			return output.Encode(c.OutOrStdout(), program.Format(), output.Object{Fields: fields})
		},
	}
	cmd.Flags().StringVar(&head, "head", "", "the full commit id the changes were read at (required)")
	addEngineFlag(cmd, &engine, folderEngineUsage, folderEngineNote)
	return cmd
}
