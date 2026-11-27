package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-kit/cli"
	"github.com/castrowithcee/qatlas-kit/cobracli"
	"github.com/castrowithcee/qatlas-kit/output"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

func newSourceAddCommand(program *cobracli.Program, state *notRunning) *cobra.Command {
	var url, fingerprint, engine, expectEffect, expectState, expectCommit string
	var include []string
	var gate *cobracli.Gate
	add := &cobra.Command{
		Use:   "add <path>",
		Short: "Take over a folder as a stack folder and give the service the access it needs",
		Long: "add inspects the folder like 'source inspect', shows the case, the answer and the changes it would make,\n" +
			"and applies them only after you confirm: at a terminal by answering the question, without a terminal\n" +
			"with --yes. It never overwrites silently. A folder that is missing or empty is cloned from --url like\n" +
			"'repo set' (and created with the owner and group of the service account and mode 0755 when missing); a clone with an origin\n" +
			"remote is taken over with that address; a repository without a remote is bound as a local source; a\n" +
			"folder with files and no repository gets a repository with a first commit of the files its stacks refer to (compose files, .env, env_file, configs, build contexts, include and extends, read-only mounts; also\n" +
			".gitignore, .gitattributes and the stack settings), branch main, as qhost; everything else is left out\n" +
			"with its reason. Each path left out becomes a line of a block marked by qhost in the .gitignore of the\n" +
			"folder, so every clone ignores it too: a missing .gitignore is created, an existing one keeps its own\n" +
			"lines and gets the block at its end (a block of an earlier takeover is extended), and it is committed\n" +
			"with the files. The preview shows these lines; with nothing to add, no file of the folder is written. A\n" +
			".gitignore that is a symbolic link, cannot be read or is larger than 1 MiB is refused. The folder is\n" +
			"bound as a local source. With --url, a folder of files is instead compared, the .gitignore included,\n" +
			"with the repository at that address, which must hold commits: a file of the folder that the remote versions is also taken when no stack refers to\n" +
			"it, marked as versioned in the remote, so joining removes nothing the remote has (a file left out for\n" +
			"another reason, such as a writable mount, a secrets file, data or an unreadable stack, stays out); add\n" +
			"shows how they relate like 'source remote' does, stops at secret-like files in what a push would\n" +
			"publish, and after you confirm starts the repository on the history of the remote (the state of the\n" +
			"host becomes one commit and is pushed when the files differ) and binds it as a remote source. If the\n" +
			"list changed since it was shown, add stops before it changes anything.\n" +
			"A file that may be data or a secret is unclear: a file a stack mounts writable (writable-file) and the\n" +
			"file of a Compose secret (secrets-file). The preview lists these apart from the other left-out files\n" +
			"with their reason, and they stay out unless you choose them. At a terminal without --yes add asks for\n" +
			"each one (default no). --include <path> (repeatable, a path below the folder) chooses a file in advance,\n" +
			"also with --yes and without a terminal; a path that is not unclear is refused and nothing changes. A\n" +
			"chosen file with a secret in it is taken like any other, and with --url the check of what a push would\n" +
			"publish still applies to it. --expect-effect covers the choice: take the effect from 'source inspect'\n" +
			"with the same --include, or add stops with state-changed. Only a folder of files has unclear files.\n" +
			"A folder that is rejected is refused and nothing changes.\n" +
			"Like 'source inspect', add checks the Docker engine of the service and the rootless engines of the users for containers of the stack folders and names each conflict with its engine, and the engines it\n" +
			"could not check. Conflicts only warn: the question says deploying would create duplicate containers\n" +
			"and take their ports twice, and the folder can still be taken over.\nFor the system engine add changes only .git, and only for root: an existing one gets the owner and group root, no write for group and others and loses core.sharedRepository (see 'source secure'); a new repository or a clone is made that way. The service inspects no other .git, so add first shows that change, asks and makes .git root-only before it inspects; --expect-effect refuses instead and names 'source secure'. For a rootless engine, when its service cannot write the folder or its .git, add gives them to the group\n" +
			"of the user of the instance as root: the group of the folder, of the folders of versioned files and of .git, group write and setgid\n" +
			"on folders, group read and write on versioned files (for a folder of files, the listed files) and on\n" +
			"files in .git, and core.sharedRepository group. The owner never changes. Bind-mount sources of the stacks and everything below, unversioned\n" +
			"files and folders, symbolic links and paths outside the folder stay as they are. The folder is then added\n" +
			"to the systemd drop-in and the service restarts once. --yes does not confirm a server's host key: that\n" +
			"takes the terminal or --fingerprint. Run it with sudo.\n" +
			"--engine names the Docker engine the folder belongs to: system (the default) or rootless:<user>, for a\n" +
			"user whose deploy service instance is set up ('qhost deploy-service setup --rootless <user>'). Everything\n" +
			"above then concerns that instance: it inspects and binds the folder, the group of the folder is the\n" +
			"primary group of its user, a created folder gets that user as owner, and the drop-in and the restart are\n" +
			"those of its unit. The system engine does not start the stacks. Before it changes anything, add asks\n" +
			"the deploy services of all other engines and refuses a folder that overlaps one of their stack folders and\n" +
			"a stack name that one of them already has. A service that cannot be asked is named in the question,\n" +
			"which you confirm like the rest. When containers of the folder run in another engine, add suggests it\n" +
			"with the option; the engine never changes silently.",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) != 1 {
				return usageFailure("source add takes exactly one argument, the path of the folder",
					"Run 'qhost source add <path> [--url <ssh-url>]'.")
			}
			return nil
		},
		RunE: func(c *cobra.Command, args []string) error {
			return runSourceAdd(c, program, gate, state, args[0], url, fingerprint, engine, expectEffect, expectState, expectCommit, include)
		},
	}
	add.Flags().StringVar(&url, "url", "", "SSH address of the repository to clone, to expect as origin, or to join a folder of files with")
	add.Flags().StringVar(&fingerprint, "fingerprint", "",
		"expected SHA256 fingerprint of the server's host key; required without a terminal")
	add.Flags().StringVar(&engine, "engine", "", "Docker engine of the stack folder: system (default) or rootless:<user>")
	add.Flags().StringVar(&expectEffect, "expect-effect", "",
		"effect fingerprint from 'source inspect'; refuses with state-changed, changing nothing, when the effect differs now")
	add.Flags().StringArrayVar(&include, "include", nil, includeUsage)
	remoteFlags(add, &expectState, &expectCommit)
	gate = program.AddGate(add, cobracli.Gates{Question: true})
	return add
}

func runSourceAdd(c *cobra.Command, program *cobracli.Program, gate *cobracli.Gate, state *notRunning, path, url, fingerprint, engine,
	expectEffect, expectState, expectCommit string, include []string) error {
	if err := folders.requireRoot(); err != nil {
		return err
	}
	expect, err := expectedRemoteOf(expectState, expectCommit)
	if err != nil {
		return err
	}
	if _, err := folders.selectEngine(c, engine); err != nil {
		return err
	}
	if err := secureGitFirst(c, gate, path, expectEffect); err != nil {
		return err
	}
	// At a terminal without --yes the operator is asked about each unclear file.
	yes, _ := c.Flags().GetBool("yes")
	plan, err := prepareSourceAdd(c, program, state, c.ErrOrStderr(), path, url, fingerprint,
		"Run 'qhost source add "+path+" --url <ssh-url>'.", expect, include, !yes && stdinIsTerminal())
	if err != nil {
		return err
	}
	if err := checkExpectedEffect(c.Context(), expectEffect, plan); err != nil {
		return err
	}
	return gate.Run(cobracli.Request{Question: plan.question}, func() error {
		return executeSourceAdd(c, program, state, plan.res, plan.repoURL, fingerprint, plan.perms, plan.remote, plan.include)
	})
}

// sourceAdd is everything that is known about adding a folder before anything is changed: the inspection, what
// the service needs, the comparison with the remote and the question to confirm.
type sourceAdd struct {
	res control.SourceInspectResult
	// inspected is the inspection; it differs from res only for a folder of files that joins a remote, whose
	// plan is the one of the comparison.
	inspected control.SourceInspectResult
	repoURL   string
	perms     *permPlan
	remote    *filesRemote
	question  string
	// include is the choice of unclear files the plan was made with: --include and the answers at the terminal.
	include []string
}

// includeUsage is the help of --include.
const includeUsage = "unclear file (a writable mount or a secrets file) of a folder of files to take into the first commit; repeatable"

// prepareSourceAdd inspects the folder, shows the case and the changes on notice and returns what applying
// would do. It changes nothing. needURL is the advice when the case needs an address that was not given.
//
// include is the choice of unclear files. With ask the person is asked about each unclear file that is not
// chosen yet, and the answers join the choice, which the result carries.
func prepareSourceAdd(c *cobra.Command, program *cobracli.Program, state *notRunning, notice io.Writer,
	path, url, fingerprint, needURL string, expect *expectedRemote, include []string, ask bool) (*sourceAdd, error) {
	if err := folders.requireRoot(); err != nil {
		return nil, err
	}
	if fingerprint != "" && !strings.HasPrefix(fingerprint, "SHA256:") {
		return nil, usageFailure("--fingerprint must look like SHA256:..., as ssh-keygen -l prints it",
			"Copy the fingerprint of the server's host key.")
	}
	peers, err := folders.askPeers(c.Context(), targetOf(c.Context()))
	if err != nil {
		return nil, err
	}
	if err := checkPath(peers, path); err != nil {
		return nil, err
	}
	res, err := inspectSource(c, state, path, url, include)
	if err != nil {
		return nil, err
	}
	if len(include) > 0 && res.Case != control.SourceCaseFiles && res.Case != control.SourceCaseReject {
		return nil, usageFailure("--include applies to a folder of files without a repository; this folder is case "+res.Case,
			"Leave out --include.")
	}
	if ask && res.Case == control.SourceCaseFiles {
		more, err := askUnclear(c, res)
		if err != nil {
			return nil, err
		}
		if len(more) > 0 {
			include = append(slices.Clone(include), more...)
			if res, err = inspectSource(c, state, path, url, include); err != nil {
				return nil, err
			}
		}
	}
	if !structuredFormat(program.Format()) {
		shown := res
		if res.Case == control.SourceCaseFiles {
			// The preview lists the files, what is left out and the lines for .gitignore, limited to the first ones.
			shown.PlannedFiles, shown.Excluded, shown.Ignore = nil, nil, nil
		}
		if err := output.Encode(notice, program.Format(), sourceResult(program.Format(), shown)); err != nil {
			return nil, err
		}
	}
	switch res.Case {
	case control.SourceCaseReject:
		return nil, usageFailure("the folder cannot be taken over: "+res.Reason, cmpFix(res.Fix))
	case control.SourceCaseClone, control.SourceCaseAdopt, control.SourceCaseLocal, control.SourceCaseFiles:
	default:
		return nil, folderFailure("the service answered with the unknown case "+res.Case, nil)
	}

	if res.Case == control.SourceCaseFiles && len(res.PlannedFiles) == 0 {
		return nil, usageFailure("the folder has no file a first commit could take; nothing was changed",
			"Put the stack files into the folder, or look at what is left out with 'qhost source inspect "+path+"'.")
	}
	repoURL := res.URL
	if repoURL == "" {
		repoURL = url
	}
	if res.Case != control.SourceCaseLocal && res.Case != control.SourceCaseFiles && repoURL == "" {
		return nil, usageFailure("this folder needs the SSH address of its repository; nothing was changed", needURL)
	}

	if expect != nil && (res.Case != control.SourceCaseFiles || url == "") {
		return nil, usageFailure("--expect-state and --expect-commit apply only to a folder of files with --url; this folder is case "+res.Case,
			"Leave out --expect-state and --expect-commit.")
	}
	inspected := res
	var remote *filesRemote
	if res.Case == control.SourceCaseFiles && url != "" {
		var err error
		if remote, err = compareFiles(c, state, notice, path, url, fingerprint, expect, include); err != nil {
			return nil, err
		}
		// The folder joins the remote with the plan of the comparison, which keeps what the remote versions.
		res = remote.plan(res)
	}

	if err := checkStacks(peers, res.Stacks); err != nil {
		return nil, err
	}
	unchecked := uncheckedPeers(peers)
	perms, err := planAccess(c.Context(), res)
	if err != nil {
		return nil, err
	}
	question := previewSourceAdd(notice, targetOf(c.Context()), res, repoURL, perms, remote, unchecked)
	return &sourceAdd{res: res, inspected: inspected, repoURL: repoURL, perms: perms, remote: remote, question: question, include: include}, nil
}

// planFolderAccess works out what must change so that the service can write the folder of res: nil when nothing
// needs to. It changes nothing.
func planFolderAccess(ctx context.Context, res control.SourceInspectResult) (*permPlan, error) {
	path, a := res.Path, res.Access
	t := targetOf(ctx)
	if res.Case == control.SourceCaseClone {
		if a == nil || !a.SetsPermissions || a.ModePath != path {
			return nil, nil
		}
		_, gid, err := folders.owner(t.account())
		if err != nil {
			return nil, folderFailure("cannot find the service account "+t.account(), err)
		}
		return planFolder(path, gid)
	}
	if _, err := folders.missing(path); err != nil {
		return nil, err
	}
	switch {
	case a == nil:
		return nil, nil
	case res.Case == control.SourceCaseFiles:
		if !a.SetsPermissions {
			return nil, nil
		}
		_, gid, err := folders.owner(t.account())
		if err != nil {
			return nil, folderFailure("cannot find the service account "+t.account(), err)
		}
		return planListedFiles(path, gid, res.PlannedFiles, bindMounts(res))
	case a.SetsPermissions || (a.GitWritable != nil && !*a.GitWritable):
		_, gid, err := folders.owner(t.account())
		if err != nil {
			return nil, folderFailure("cannot find the service account "+t.account(), err)
		}
		return planPermissions(ctx, path, gid, bindMounts(res))
	}
	return nil, nil
}

func executeSourceAdd(c *cobra.Command, program *cobracli.Program, state *notRunning, res control.SourceInspectResult,
	repoURL, fingerprint string, plan *permPlan, remote *filesRemote, include []string) error {
	path, notice := res.Path, c.ErrOrStderr()
	changed := 0
	if plan != nil {
		changed = plan.changed()
	}
	switch res.Case {
	case control.SourceCaseFiles:
		return executeSourceFiles(c, program, state, res, plan, remote, include)
	case control.SourceCaseClone, control.SourceCaseAdopt:
		probe, err := trustHostKey(c, state, repoURL, fingerprint)
		if err != nil {
			return err
		}
		if err := applyPlan(c, plan); err != nil {
			return err
		}
		set, err := bindRepo(c, state, repoURL, path, probe, res.Case == control.SourceCaseClone)
		if err != nil {
			return err
		}
		fields := append(repoSetFields(set), output.Field{Name: "changed_paths", Value: changed})
		return output.Encode(c.OutOrStdout(), program.Format(), output.Object{Fields: fields})
	}

	if err := applyPlan(c, plan); err != nil {
		return err
	}
	if err := folders.grant(c.Context(), path, false, notice); err != nil {
		return err
	}
	var added control.SourceAddResult
	if err := credentialCall(c, state, control.SourceAddCommand, control.SourceAddArgs{Path: path}, &added); err != nil {
		return err
	}
	return output.Encode(c.OutOrStdout(), program.Format(), output.Object{Fields: []output.Field{
		{Name: "id", Value: added.ID},
		{Name: "path", Value: added.Path},
		{Name: "kind", Value: added.Kind},
		{Name: "head", Value: added.Head},
		{Name: "changed_paths", Value: changed},
	}})
}

// executeSourceFiles starts a repository in a folder of files. It looks at the folder once more first: what
// was added or left out since the preview is not taken along silently, and nothing has changed by then.
func executeSourceFiles(c *cobra.Command, program *cobracli.Program, state *notRunning, shown control.SourceInspectResult,
	plan *permPlan, remote *filesRemote, include []string) error {
	path, notice := shown.Path, c.ErrOrStderr()
	var now control.SourceInspectResult
	var err error
	if remote != nil {
		var cmp control.SourceCompareResult
		if cmp, err = compareRemote(c, state, path, remote.address, remote.fingerprint, include); err == nil {
			now = (&filesRemote{cmp: cmp}).plan(control.SourceInspectResult{Case: cmp.Case})
		}
	} else {
		now, err = inspectSource(c, state, path, "", include)
	}
	if err != nil {
		return err
	}
	if diff := listDifference(shown, now); diff != "" {
		return usageFailure("the folder changed since the list was shown ("+diff+"); nothing was changed",
			"Run 'qhost source add "+path+"' again and review the new list.")
	}
	changed := 0
	if plan != nil {
		changed = plan.changed()
	}
	if err := applyPlan(c, plan); err != nil {
		return err
	}
	if err := folders.grant(c.Context(), path, false, notice); err != nil {
		return err
	}
	var added control.SourceAddResult
	args := control.SourceAddArgs{Path: path, Files: shown.PlannedFiles, Include: include}
	callService := credentialCall
	if remote != nil {
		args.URL, args.Fingerprint, args.ExpectState, args.ExpectCommit = remote.address, remote.fingerprint, remote.cmp.State, remote.cmp.Commit
		callService = repoCall
	}
	if err := callService(c, state, control.SourceAddCommand, args, &added); err != nil {
		var f *cli.Failure
		if changed > 0 && errors.As(err, &f) {
			kept := *f
			kept.Message += fmt.Sprintf("; the group and the mode of %d paths of the listed files stay changed", changed)
			return &kept
		}
		return err
	}
	fields := []output.Field{
		{Name: "id", Value: added.ID},
		{Name: "path", Value: added.Path},
		{Name: "kind", Value: added.Kind},
		{Name: "head", Value: added.Head},
		{Name: "files", Value: added.Files},
	}
	if remote != nil {
		fields = append(fields, output.Field{Name: "url", Value: remote.address},
			output.Field{Name: "state", Value: remote.cmp.State}, output.Field{Name: "branch", Value: remote.cmp.Branch},
			output.Field{Name: "pushed", Value: added.Pushed})
	}
	fields = append(fields, output.Field{Name: "changed_paths", Value: changed})
	return output.Encode(c.OutOrStdout(), program.Format(), output.Object{Fields: fields})
}

// filesRemote is the comparison of a folder of files with the repository it is to join, as the operator saw
// it. Only the state and commit shown are acted on.
type filesRemote struct {
	address, fingerprint string
	cmp                  control.SourceCompareResult
}

// plan is res with the plan of the comparison: the files the folder joins the remote with, including those only
// the remote versions, and what is left out.
func (r *filesRemote) plan(res control.SourceInspectResult) control.SourceInspectResult {
	res.PlannedFiles, res.Excluded, res.Included, res.Ignore = r.cmp.PlannedFiles, r.cmp.Excluded, r.cmp.Included, r.cmp.Ignore
	return res
}

// compareFiles confirms the host key, compares the folder of files with the remote and shows the result. It
// stops at secret-like files in what a push would publish, and at a remote without commit. Nothing in the
// folder has changed by then.
func compareFiles(c *cobra.Command, state *notRunning, w io.Writer, path, url, fingerprint string, expect *expectedRemote, include []string) (*filesRemote, error) {
	probe, err := trustHostKey(c, state, url, fingerprint)
	if err != nil {
		return nil, err
	}
	cmp, err := compareRemote(c, state, path, url, probe.Fingerprint, include)
	if err != nil {
		return nil, err
	}
	if cmp.Case != control.SourceCaseFiles {
		return nil, folderFailure("the service answered with the case "+cmp.Case+" for a folder of files", nil)
	}
	_, _ = fmt.Fprintf(w, "Compare %s with %s:\n", path, url)
	printLines(w, compareLines(cmp))
	if err := expect.check(cmp, path, url); err != nil {
		return nil, err
	}
	if err := refuseSecrets(cmp); err != nil {
		return nil, err
	}
	switch cmp.State {
	case control.SourceRemoteEqual, control.SourceRemoteDifferent:
	case control.SourceRemoteEmpty:
		return nil, usageFailure("the repository at the address has no commit; nothing was changed",
			"Take the folder over without --url, then run 'qhost source remote "+path+" "+url+"'.")
	default:
		return nil, folderFailure("the service answered with the unknown state "+cmp.State, nil)
	}
	return &filesRemote{address: url, fingerprint: probe.Fingerprint, cmp: cmp}, nil
}

// listDifference names what differs between two answers for a folder of files: the case, and the paths that
// were added or dropped from the planned files or from the exclusions (path and reason).
func listDifference(shown, now control.SourceInspectResult) string {
	if now.Case != shown.Case {
		return "the folder is now case " + now.Case
	}
	excluded := func(r control.SourceInspectResult) []string {
		var out []string
		for _, e := range r.Excluded {
			out = append(out, e.Path+": "+e.Reason)
		}
		return out
	}
	var parts []string
	for _, set := range []struct {
		name      string
		old, next []string
	}{
		{"files", shown.PlannedFiles, now.PlannedFiles},
		{"left out", excluded(shown), excluded(now)},
	} {
		added, dropped := setDiff(set.old, set.next)
		if len(added) > 0 {
			parts = append(parts, "new in "+set.name+": "+limitedList(added))
		}
		if len(dropped) > 0 {
			parts = append(parts, "no longer in "+set.name+": "+limitedList(dropped))
		}
	}
	return strings.Join(parts, "; ")
}

// setDiff returns the entries of next that old lacks and the entries of old that next lacks, sorted.
func setDiff(old, next []string) (added, dropped []string) {
	in := func(list []string) map[string]bool {
		m := make(map[string]bool, len(list))
		for _, v := range list {
			m[v] = true
		}
		return m
	}
	oldSet, nextSet := in(old), in(next)
	for v := range nextSet {
		if !oldSet[v] {
			added = append(added, v)
		}
	}
	for v := range oldSet {
		if !nextSet[v] {
			dropped = append(dropped, v)
		}
	}
	sort.Strings(added)
	sort.Strings(dropped)
	return added, dropped
}

// listLimit is how many paths a list shows before it only counts the rest.
const listLimit = 50

func limitedList(items []string) string {
	if len(items) <= listLimit {
		return strings.Join(items, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(items[:listLimit], ", "), len(items)-listLimit)
}

func cmpFix(fix string) string {
	if fix == "" {
		return "Choose another folder."
	}
	return fix
}

func applyPlan(c *cobra.Command, plan *permPlan) error {
	if plan == nil {
		return nil
	}
	if err := folders.applyPermissions(c.Context(), plan); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(c.ErrOrStderr(), "Changed the permissions of %d paths in %s.\n", plan.changed(), plan.dir)
	return nil
}

// bindMounts is every bind-mount source of the stacks the folder brings.
func bindMounts(res control.SourceInspectResult) []string {
	var out []string
	for _, st := range res.Stacks {
		out = append(out, st.BindMounts...)
	}
	return out
}

// previewSourceAdd shows what will change and returns the question for the confirmation. Without a terminal
// the confirmation needs --yes; --yes never confirms a host key. Nothing has been changed by then.
func previewSourceAdd(w io.Writer, t engineTarget, res control.SourceInspectResult, url string, plan *permPlan, remote *filesRemote, unchecked []string) string {
	var lines []string
	switch res.Case {
	case control.SourceCaseClone:
		lines = append(lines, fmt.Sprintf("Clone %s into %s; a missing folder is created with owner %s:%s and mode %04o.",
			url, res.Path, t.account(), t.account(), folderMode))
	case control.SourceCaseAdopt:
		lines = append(lines, fmt.Sprintf("Take over the clone %s with the address %s.", res.Path, url))
	case control.SourceCaseFiles:
		if remote != nil {
			lines = append(lines, filesRemotePreview(res, remote)...)
		} else {
			lines = append(lines, filesPreview(res)...)
		}
	default:
		lines = append(lines, fmt.Sprintf("Take over the repository %s as a local source.", res.Path))
	}
	if plan != nil {
		lines = append(lines, plan.summary()...)
	}
	lines = append(lines, "The first run deploys every stack: containers whose configuration and image are unchanged keep running, and a stack stopped by hand is started.")
	lines = append(lines, fmt.Sprintf("The folder belongs to the %s engine, is added to %s and the %s service restarts.",
		t.id(), folders.dropIn(t), t.unit()))
	_, _ = fmt.Fprintf(w, "Case %s, folder %s:\n", res.Case, res.Path)
	for _, l := range lines {
		_, _ = fmt.Fprintln(w, "  "+l)
	}
	warn := conflictConsequence(res)
	if warn != "" {
		_, _ = fmt.Fprintln(w, "  "+warn)
		warn += " "
	}
	for _, u := range unchecked {
		_, _ = fmt.Fprintln(w, "  warning: "+u)
	}
	if len(unchecked) > 0 {
		warn = uncheckedQuestion + warn
	}
	if remote != nil {
		return warn + filesRemoteQuestion(remote)
	}
	return warn + fmt.Sprintf("Apply these changes (case %s, folder %s)?", res.Case, res.Path)
}

// filesPreview describes the repository a folder of files gets: the files of the first commit, what is left
// out with the reason, and where each part is recorded.
func filesPreview(res control.SourceInspectResult) []string {
	lines := []string{
		fmt.Sprintf("Start a git repository in %s (branch main) and bind it as a local source.", res.Path),
		"The first commit is made as " + commitAuthor + ".",
	}
	lines = append(lines, fileListLines(res, "The first commit takes", nil)...)
	return append(lines, ignoreLines(res, "the first commit")...)
}

// filesRemotePreview describes the repository a folder of files gets on the history of a remote.
func filesRemotePreview(res control.SourceInspectResult, remote *filesRemote) []string {
	branch := remote.cmp.Branch
	lines := []string{
		fmt.Sprintf("Start a git repository in %s (branch %s) on the history of %s and bind it as a remote source.",
			res.Path, branch, remote.address),
	}
	if remote.cmp.State == control.SourceRemoteDifferent {
		lines = append(lines, fmt.Sprintf("The state of the host becomes one commit as %s on %s and is pushed to the remote.", commitAuthor, branch))
	} else {
		lines = append(lines, "The files equal the remote: no commit is made and nothing is pushed.")
	}
	lines = append(lines, fileListLines(res, "The repository takes", remote.cmp.RemoteFiles)...)
	return append(lines, ignoreLines(res, "the repository")...)
}

// remoteMark follows a listed file that is taken only because the remote versions it.
const remoteMark = " (versioned in the remote)"

// fileListLines lists the files of the repository and what is left out with the reason. remote are the listed
// files that no stack refers to and that are taken because the remote versions them.
func fileListLines(res control.SourceInspectResult, takes string, remote []string) []string {
	lines := []string{fmt.Sprintf("%s %d files:", takes, len(res.PlannedFiles))}
	if len(remote) > 0 {
		lines[0] = fmt.Sprintf("%s %d files, %d of them only because the remote versions them:", takes, len(res.PlannedFiles), len(remote))
	}
	byRemote := make(map[string]bool, len(remote))
	for _, f := range remote {
		byRemote[f] = true
	}
	chosen := make(map[string]string, len(res.Included))
	for _, e := range res.Included {
		chosen[e.Path] = e.Reason
	}
	for i, f := range res.PlannedFiles {
		if i == listLimit {
			lines = append(lines, fmt.Sprintf("  ... and %d more", len(res.PlannedFiles)-listLimit))
			break
		}
		line := "  " + f
		if byRemote[f] {
			line += remoteMark
		}
		if reason, ok := chosen[f]; ok {
			line += " (chosen, " + reason + ")"
		}
		lines = append(lines, line)
	}
	lines = append(lines, unresolvedLines(res)...)
	var unclear, others []control.SourceExcluded
	for _, e := range res.Excluded {
		if control.SourceUnclear(e.Reason) {
			unclear = append(unclear, e)
		} else {
			others = append(others, e)
		}
	}
	if len(unclear) > 0 {
		lines = append(lines, fmt.Sprintf("Unclear (%d), may hold data or a secret, left out unless chosen with --include <path>:", len(unclear)))
		lines = append(lines, excludedLines(unclear)...)
	}
	if len(others) == 0 {
		if len(unclear) == 0 {
			lines = append(lines, "Nothing is left out.")
		}
		return lines
	}
	lines = append(lines, fmt.Sprintf("Left out (%d), not needed by the stacks, kept out of git by .gitignore:", len(others)))
	return append(lines, excludedLines(others)...)
}

// excludedLines lists left-out entries with reason and stack, limited to the first ones.
func excludedLines(list []control.SourceExcluded) []string {
	var lines []string
	for i, e := range list {
		if i == listLimit {
			lines = append(lines, fmt.Sprintf("  ... and %d more", len(list)-listLimit))
			break
		}
		line := "  " + e.Path + ": " + e.Reason
		if e.Stack != "" {
			line += " (stack " + e.Stack + ")"
		}
		lines = append(lines, line)
	}
	return lines
}

// askUnclear asks, for each unclear file the plan leaves out, whether the first commit takes it. Anything but y or
// yes is no, so an unreadable answer leaves the file out. It returns the paths answered with yes. Standard input
// is read byte by byte so that the answers after it still reach the next question.
func askUnclear(c *cobra.Command, res control.SourceInspectResult) ([]string, error) {
	var chosen []string
	for _, e := range res.Excluded {
		if !control.SourceUnclear(e.Reason) {
			continue
		}
		what := "a file mounted writable, which may hold data"
		if e.Reason == control.SourceExcludedSecretsFile {
			what = "the file of a secret"
		}
		_, _ = fmt.Fprintf(c.ErrOrStderr(), "%s is %s (%s, stack %s). Take it into the first commit? [y/N] ", e.Path, what, e.Reason, e.Stack)
		answer := strings.ToLower(strings.TrimSpace(readLine(stdinStream())))
		if answer == "y" || answer == "yes" {
			chosen = append(chosen, e.Path)
		}
	}
	return chosen, nil
}

// readLine reads up to the next newline without reading beyond it.
func readLine(r io.Reader) string {
	var sb strings.Builder
	buf := make([]byte, 1)
	for {
		n, err := r.Read(buf)
		if n == 1 {
			if buf[0] == '\n' {
				break
			}
			sb.WriteByte(buf[0])
		}
		if err != nil {
			break
		}
	}
	return sb.String()
}

// ignoreLines says what is written to the .gitignore of the folder, the only file of the folder that can change,
// with the lines added; takes names what holds the file.
func ignoreLines(res control.SourceInspectResult, takes string) []string {
	ig := res.Ignore
	if ig == nil {
		return []string{"No file in the folder is written."}
	}
	head := fmt.Sprintf("Create .gitignore with a block marked by qhost of %d lines; %s holds it:", len(ig.Lines), takes)
	if ig.Action == control.SourceIgnoreExtend {
		head = fmt.Sprintf("Add %d lines to the block marked by qhost at the end of .gitignore, whose other lines stay as they are; %s holds it:",
			len(ig.Lines), takes)
	}
	lines := []string{head}
	for i, l := range ig.Lines {
		if i == listLimit {
			lines = append(lines, fmt.Sprintf("  ... and %d more", len(ig.Lines)-listLimit))
			break
		}
		lines = append(lines, "  "+l)
	}
	return append(lines, "No other file in the folder is written.")
}

// unresolvedLines names the references of the stacks whose path the service could not resolve, so the operator
// knows the list may miss a file.
func unresolvedLines(res control.SourceInspectResult) []string {
	var lines []string
	for _, st := range res.Stacks {
		for _, u := range st.Unresolved {
			lines = append(lines, fmt.Sprintf("Not resolvable in stack %s, so the file it names is not listed: %s", st.Name, u))
		}
	}
	return lines
}

// filesRemoteQuestion is what the operator confirms for a folder of files that joins a remote.
func filesRemoteQuestion(remote *filesRemote) string {
	if remote.cmp.State == control.SourceRemoteEqual {
		return "Take over the folder with the history of the remote?"
	}
	return fmt.Sprintf("Start the repository on the history of the remote, put the state of the host as a commit on %s and push it?",
		remote.cmp.Branch)
}
