package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-kit/cobracli"
	"github.com/castrowithcee/qatlas-kit/output"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

func newSourceSecureCommand(program *cobracli.Program, state *notRunning) *cobra.Command {
	var engine string
	var gate *cobracli.Gate
	secure := &cobra.Command{
		Use:   "secure <path> [--yes]",
		Short: "Make the .git of a stack folder of the system engine root-only, so that git runs in it again",
		Long: "secure makes the .git of a stack folder of the system engine one that only root can change: every entry in it\n" +
			"gets the owner and group root and loses write for group and others (and setgid on folders), and\n" +
			"core.sharedRepository is removed from the git configuration. The deploy service of that engine runs as root\n" +
			"and runs git only in such a .git; any other .git gives the folder the finding git-unsafe, and no fetch or\n" +
			"run happens until it is repaired. secure shows what it changes and asks first: at a terminal by answering\n" +
			"the question, without a terminal with --yes. The work tree keeps its owners and modes. Nothing is followed:\n" +
			"a .git that is a link or no folder, a link or special file in it, and a file with further hard links are\n" +
			"refused with the reason, and nothing changes. The next fetch of the folder then starts at once. Only for\n" +
			"folders of the system engine; a rootless engine is refused. Run it with sudo.",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) != 1 {
				return usageFailure("source secure takes exactly one argument, the path of the folder",
					"Run 'qhost source secure <path>'.")
			}
			return nil
		},
		RunE: func(c *cobra.Command, args []string) error {
			return runSourceSecure(c, program, gate, state, args[0], engine)
		},
	}
	secure.Flags().StringVar(&engine, "engine", "", "Docker engine of the stack folder: "+control.EngineSystem+"; a rootless engine is refused")
	gate = program.AddGate(secure, cobracli.Gates{Question: true})
	return secure
}

func runSourceSecure(c *cobra.Command, program *cobracli.Program, gate *cobracli.Gate, state *notRunning, path, engine string) error {
	if err := folders.requireRoot(); err != nil {
		return err
	}
	if err := folderArg(path); err != nil {
		return err
	}
	if err := routeFolder(c, state, engine, path); err != nil {
		return err
	}
	if targetOf(c.Context()).inst != nil {
		return usageFailure("source secure is only for stack folders of the system engine; "+path+" belongs to "+targetOf(c.Context()).id(),
			"The folders of a rootless engine are owned by its user and need no change.")
	}
	if _, err := os.Lstat(filepath.Join(path, ".git")); err != nil {
		return usageFailure(path+" has no .git", "Take the folder over with 'qhost source add "+path+"'.")
	}
	s, err := planGitSecure(c.Context(), path)
	if err != nil {
		return err
	}
	done := output.Object{Fields: []output.Field{{Name: "path", Value: path}, {Name: "changed", Value: s.count()}}}
	if s.count() == 0 {
		_, _ = fmt.Fprintln(c.ErrOrStderr(), ".git of "+path+" is root-only already.")
		return output.Encode(c.OutOrStdout(), program.Format(), done)
	}
	plan := &permPlan{dir: path, secure: s}
	w := c.ErrOrStderr()
	_, _ = fmt.Fprintf(w, "Make .git of %s root-only:\n", path)
	for _, l := range plan.summary() {
		_, _ = fmt.Fprintln(w, l)
	}
	return gate.Run(cobracli.Request{Question: fmt.Sprintf("Make .git of %s root-only?", path)}, func() error {
		if err := folders.applyPermissions(c.Context(), plan); err != nil {
			return err
		}
		// The finding goes at the next fetch; do not wait for the poll. A fetch that fails changes nothing here.
		var res control.RepoFetchResult
		if err := repoCall(c, state, control.RepoFetchCommand, control.RepoFetchArgs{Dir: path}, &res); err != nil {
			_, _ = fmt.Fprintln(w, "warning: the folder was secured, but the service could not fetch it now: "+err.Error())
		}
		return output.Encode(c.OutOrStdout(), program.Format(), done)
	})
}
