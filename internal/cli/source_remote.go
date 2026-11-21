package cli

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-kit/cli"
	"github.com/castrowithcee/qatlas-kit/cobracli"
	"github.com/castrowithcee/qatlas-kit/output"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/gitrepo"
)

func newSourceRemoteCommand(program *cobracli.Program, state *notRunning) *cobra.Command {
	var fingerprint, expectState, expectCommit, engine string
	var gate *cobracli.Gate
	remote := &cobra.Command{
		Use:   "remote <path> <ssh-url>",
		Short: "Give a local source a remote repository, after comparing the folder with it",
		Long: "remote compares the history of a local source with the repository at the address and shows how they\n" +
			"relate: the remote has no commit (the local history is pushed to it), it holds the same files (the\n" +
			"service follows its history; the local history stays under " + gitrepo.LocalHistoryRef + "), or it\n" +
			"differs (the paths are listed; the state of the host becomes one commit on the remote branch and is\n" +
			"pushed, and the host never overwrites the remote). Nothing is changed before you confirm: at a\n" +
			"terminal by answering the question, without a terminal with --yes, which does not confirm a server's\n" +
			"host key (that takes the terminal or --fingerprint). If the commits that would be pushed hold secret-like\n" +
			"files (secrets files, env files with values, private keys), remote stops, names commit, path and kind\n" +
			"and never a value; there is no way around it, clean the history yourself. The source is then a remote\n" +
			"source. The deploy key of the folder needs write access to the repository. Run it with sudo.\n" +
			"--expect-state and --expect-commit take the state (empty, equal or different) and the commit of the\n" +
			"comparison that 'source compare' showed; give both or neither. The commit is empty exactly when the state\n" +
			"is empty. remote compares again and, when state or commit differ now, stops with the code state-changed\n" +
			"before it changes anything. They do not replace the question or the confirmation of a host key.",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) != 2 {
				return usageFailure("source remote takes two arguments, the path of the folder and the SSH address",
					"Run 'qhost source remote <path> <ssh-url>'.")
			}
			return nil
		},
		RunE: func(c *cobra.Command, args []string) error {
			return runSourceRemote(c, program, gate, state, args[0], args[1], fingerprint, expectState, expectCommit, engine)
		},
	}
	remote.Flags().StringVar(&fingerprint, "fingerprint", "",
		"expected SHA256 fingerprint of the server's host key; required without a terminal")
	remoteFlags(remote, &expectState, &expectCommit)
	addEngineFlag(remote, &engine, folderEngineUsage, folderEngineNote)
	gate = program.AddGate(remote, cobracli.Gates{Question: true})
	return remote
}

func newSourceCompareCommand(program *cobracli.Program, state *notRunning) *cobra.Command {
	var fingerprint, engine string
	compare := &cobra.Command{
		Use:   "compare <path> <ssh-url>",
		Short: "Compare a folder with a remote repository and show how they relate; changes nothing in the folder",
		Long: "compare shows how the history of the repository at the address relates to the folder, as 'source remote'\n" +
			"and 'source add --url' compare before they ask: the state (empty: the remote has no commit; equal: the\n" +
			"same files; different: the paths that differ, the first 50 and their number), the branch and commit of\n" +
			"the remote and the secret-like files in the commits a push would publish (commit, path and kind, never a\n" +
			"value). Finding such files is an answer here, not an error. The host key of the server is confirmed as\n" +
			"for 'source remote': it is pinned already, or it matches --fingerprint, or you confirm it at a terminal;\n" +
			"the confirmed key is pinned, and that is all compare stores. Nothing in the folder or the repository is\n" +
			"changed. Pass the state and the commit to --expect-state and --expect-commit of 'source remote' or\n" +
			"'source add' to apply exactly this comparison. Run it with sudo.\n" +
			"With --output json the fields are case, state, branch, commit, paths, path_count, secrets (total and\n" +
			"findings) and, for a folder of files, planned_files, excluded and remote_files: the plan the folder\n" +
			"joins the remote with, where remote_files are the planned files no stack refers to that the remote\n" +
			"versions.",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) != 2 {
				return usageFailure("source compare takes two arguments, the path of the folder and the SSH address",
					"Run 'qhost source compare <path> <ssh-url>'.")
			}
			return nil
		},
		RunE: func(c *cobra.Command, args []string) error {
			return runSourceCompare(c, program, state, args[0], args[1], fingerprint, engine)
		},
	}
	compare.Flags().StringVar(&fingerprint, "fingerprint", "",
		"expected SHA256 fingerprint of the server's host key; required without a terminal")
	addEngineFlag(compare, &engine, compareEngineUsage, compareEngineNote)
	return compare
}

// checkRemoteInput validates what source remote and source compare take: the folder, the address and the
// fingerprint of the host key.
func checkRemoteInput(path, url, fingerprint string) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return usageFailure("the path must be absolute and clean", "Pass the absolute path of the folder.")
	}
	if err := gitrepo.ValidateURL(url); err != nil {
		return usageFailure(err.Error(), "Pass the SSH address of the repository.")
	}
	if fingerprint != "" && !strings.HasPrefix(fingerprint, "SHA256:") {
		return usageFailure("--fingerprint must look like SHA256:..., as ssh-keygen -l prints it",
			"Copy the fingerprint of the server's host key.")
	}
	return nil
}

func runSourceCompare(c *cobra.Command, program *cobracli.Program, state *notRunning, path, url, fingerprint, engine string) error {
	if err := folders.requireRoot(); err != nil {
		return err
	}
	if err := checkRemoteInput(path, url, fingerprint); err != nil {
		return err
	}
	if err := routeFolderKey(c, state, engine, path); err != nil {
		return err
	}
	probe, err := trustHostKey(c, state, url, fingerprint)
	if err != nil {
		return err
	}
	cmp, err := compareRemote(c, state, path, url, probe.Fingerprint)
	if err != nil {
		return err
	}
	return output.Encode(c.OutOrStdout(), program.Format(), compareFields(program.Format(), cmp))
}

// compareFields is the comparison as the fields of its JSON form; for the text formats the fields are flat.
func compareFields(format output.Format, cmp control.SourceCompareResult) output.Object {
	fields := []output.Field{{Name: "case", Value: cmp.Case}, {Name: "state", Value: cmp.State}, {Name: "branch", Value: cmp.Branch}}
	add := func(name string, value any) { fields = append(fields, output.Field{Name: name, Value: value}) }
	if cmp.Commit != "" {
		add("commit", cmp.Commit)
	}
	if structuredFormat(format) {
		if len(cmp.Paths) > 0 {
			add("paths", cmp.Paths)
		}
		add("path_count", cmp.PathCount)
		if len(cmp.PlannedFiles) > 0 {
			add("planned_files", cmp.PlannedFiles)
		}
		if len(cmp.Excluded) > 0 {
			add("excluded", structured(format, cmp.Excluded))
		}
		if len(cmp.RemoteFiles) > 0 {
			add("remote_files", cmp.RemoteFiles)
		}
		if cmp.Secrets != nil {
			add("secrets", structured(format, cmp.Secrets))
		}
		return output.Object{Fields: fields}
	}
	add("path_count", cmp.PathCount)
	for i, p := range cmp.Paths {
		if i == listLimit {
			break
		}
		add("path", p)
	}
	for i, p := range cmp.PlannedFiles {
		if i == listLimit {
			break
		}
		add("planned_file", p)
	}
	for i, p := range cmp.RemoteFiles {
		if i == listLimit {
			break
		}
		add("remote_file", p)
	}
	for i, e := range cmp.Excluded {
		if i == listLimit {
			break
		}
		value := e.Path + ": " + e.Reason
		if e.Stack != "" {
			value += " (stack " + e.Stack + ")"
		}
		add("excluded", value)
	}
	if s := cmp.Secrets; s != nil {
		add("secrets_total", s.Total)
		for i, f := range s.Findings {
			if i == listLimit {
				break
			}
			add("secret", fmt.Sprintf("%s %s (%s)", shortCommit(f.Commit), f.Path, f.Kind))
		}
	}
	return output.Object{Fields: fields}
}

func runSourceRemote(c *cobra.Command, program *cobracli.Program, gate *cobracli.Gate, state *notRunning, path, url, fingerprint,
	expectState, expectCommit, engine string) error {
	if err := folders.requireRoot(); err != nil {
		return err
	}
	if err := checkRemoteInput(path, url, fingerprint); err != nil {
		return err
	}
	expect, err := expectedRemoteOf(expectState, expectCommit)
	if err != nil {
		return err
	}
	if err := routeFolder(c, state, engine, path); err != nil {
		return err
	}
	probe, err := trustHostKey(c, state, url, fingerprint)
	if err != nil {
		return err
	}
	cmp, err := compareRemote(c, state, path, url, probe.Fingerprint)
	if err != nil {
		return err
	}
	if cmp.Case != control.SourceCaseLocal {
		return usageFailure("source remote works on a local source; this folder is case "+cmp.Case,
			"Run 'qhost source add "+path+" --url <ssh-url>' for a folder of files.")
	}
	w := c.ErrOrStderr()
	_, _ = fmt.Fprintf(w, "Compare %s with %s:\n", path, url)
	printLines(w, compareLines(cmp))
	if err := expect.check(cmp, path, url); err != nil {
		return err
	}
	if err := refuseSecrets(cmp); err != nil {
		return err
	}
	question, err := remoteQuestion(cmp, url)
	if err != nil {
		return err
	}
	return gate.Run(cobracli.Request{Question: question}, func() error {
		var res control.SourceRemoteResult
		args := control.SourceRemoteArgs{Path: path, URL: url, Fingerprint: probe.Fingerprint,
			ExpectState: cmp.State, ExpectCommit: cmp.Commit}
		if err := repoCall(c, state, control.SourceRemoteCommand, args, &res); err != nil {
			return err
		}
		return output.Encode(c.OutOrStdout(), program.Format(), output.Object{Fields: []output.Field{
			{Name: "id", Value: res.ID},
			{Name: "path", Value: res.Path},
			{Name: "url", Value: res.URL},
			{Name: "state", Value: res.State},
			{Name: "branch", Value: res.Branch},
			{Name: "head", Value: res.Head},
			{Name: "pushed", Value: res.Pushed},
			{Name: "history_ref", Value: res.HistoryRef},
		}})
	})
}

func compareRemote(c *cobra.Command, state *notRunning, path, url, fingerprint string) (control.SourceCompareResult, error) {
	var cmp control.SourceCompareResult
	err := repoCall(c, state, control.SourceCompareCommand,
		control.SourceCompareArgs{Path: path, URL: url, Fingerprint: fingerprint}, &cmp)
	return cmp, err
}

func printLines(w io.Writer, lines []string) {
	for _, l := range lines {
		_, _ = fmt.Fprintln(w, "  "+l)
	}
}

// shortCommit is the part of a commit id the output shows.
func shortCommit(id string) string { return id[:min(len(id), 12)] }

// compareLines describes how the remote relates to the folder: the state, the paths that differ and the
// secret-like files a push would publish. Paths and kinds only; no value is ever shown.
func compareLines(cmp control.SourceCompareResult) []string {
	remote := "branch " + cmp.Branch
	if cmp.Commit != "" {
		remote += " at " + shortCommit(cmp.Commit)
	}
	lines := []string{fmt.Sprintf("State %s (remote %s).", cmp.State, remote)}
	if cmp.PathCount > 0 {
		lines = append(lines, fmt.Sprintf("%d paths differ between the remote and the folder:", cmp.PathCount))
		for i, p := range cmp.Paths {
			if i == listLimit {
				break
			}
			lines = append(lines, "  "+p)
		}
		if shown := min(len(cmp.Paths), listLimit); cmp.PathCount > shown {
			lines = append(lines, fmt.Sprintf("  ... and %d more", cmp.PathCount-shown))
		}
	}
	if s := cmp.Secrets; s != nil && s.Total > 0 {
		lines = append(lines, fmt.Sprintf("The commits to push hold %d secret-like files:", s.Total))
		for i, f := range s.Findings {
			if i == listLimit {
				break
			}
			lines = append(lines, fmt.Sprintf("  %s %s (%s)", shortCommit(f.Commit), f.Path, f.Kind))
		}
		if shown := min(len(s.Findings), listLimit); s.Total > shown {
			lines = append(lines, fmt.Sprintf("  ... and %d more", s.Total-shown))
		}
	}
	return lines
}

// refuseSecrets stops before any change when the push would publish secret-like files. There is no override.
func refuseSecrets(cmp control.SourceCompareResult) error {
	if cmp.Secrets == nil || cmp.Secrets.Total == 0 {
		return nil
	}
	return secretsFailure(fmt.Sprintf("the commits to push hold %d secret-like files and are not pushed; nothing was changed",
		cmp.Secrets.Total))
}

func secretsFailure(message string) *cli.Failure {
	return &cli.Failure{Code: output.Code(control.CodeSecrets), Message: message,
		NextStep: "qhost cannot push these files and has no override. Remove them from the history yourself (rewrite " +
			"it, or start a new history without them), then run the command again."}
}

// remoteQuestion is what the operator confirms for a local source and the state of its remote.
func remoteQuestion(cmp control.SourceCompareResult, url string) (string, error) {
	switch cmp.State {
	case control.SourceRemoteEmpty:
		return fmt.Sprintf("Push the local history to %s (branch %s)?", url, cmp.Branch), nil
	case control.SourceRemoteEqual:
		return fmt.Sprintf("Take over the history of the remote; the local history stays under %s?", gitrepo.LocalHistoryRef), nil
	case control.SourceRemoteDifferent:
		return fmt.Sprintf("Put the state of the host as a commit on %s and push it?", cmp.Branch), nil
	}
	return "", folderFailure("the service answered with the unknown state "+cmp.State, nil)
}
