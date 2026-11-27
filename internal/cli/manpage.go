package cli

import (
	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"

	"github.com/castrowithcee/qatlas-kit/cobracli"
)

// writeManpages writes the manpages of the program into dir, as the release build does.
func writeManpages(program *cobracli.Program, dir string) error {
	return program.Manpage(dir, `The same text is printed by \fBqhost help\fP.`,
		func(root *cobra.Command, dir string) error {
			return doc.GenManTree(root, &doc.GenManHeader{Title: "QHOST", Section: "1"}, dir)
		})
}

// newManpageCommand writes the manpages for the release build, which calls it with the output directory.
func newManpageCommand(program *cobracli.Program) *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use:    "manpage",
		Short:  "Write the manpages into a directory",
		Hidden: true,
		RunE: func(*cobra.Command, []string) error {
			return writeManpages(program, dir)
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "directory the manpages are written to")
	_ = cmd.MarkFlagRequired("dir")
	return cmd
}
