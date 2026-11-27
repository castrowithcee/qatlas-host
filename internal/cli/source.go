package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-kit/cobracli"
	"github.com/castrowithcee/qatlas-kit/output"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/gitrepo"
)

// stRdonly is ST_RDONLY, the statfs flag of a file system mounted read-only.
const stRdonly = 1

const readOnlyReason = "the folder lies on a read-only file system"

func newSourceCommand(program *cobracli.Program, state *notRunning) *cobra.Command {
	group := &cobra.Command{
		Use:   "source",
		Short: "Inspect, take over, list and release stack folders, show their history, and show or discard their uncommitted changes",
		Long: "The source commands work on the stack folders of the host. list needs no sudo for an administrator in\n" +
			"the group qhost, and so do changes and log; inspect, add, move, compare, remote, remove, diff, show and discard need sudo.\n" +
			"See 'qhost help sources'.",
	}
	var url, engine string
	var include []string
	inspect := &cobra.Command{
		Use:   "inspect <path>",
		Short: "Classify a folder and show its permissions, files, stacks and container conflicts; changes nothing",
		Long: "inspect tells which case applies to the folder: clone (missing or empty, gets a clone of the URL),\n" +
			"adopt-clone (a clone with an origin remote), local (a repository without a remote), files (files\n" +
			"without a repository; the first commit would take the listed files and leave out the listed ones with\n" +
			"the reason) or reject (with the reason and what to do). It shows the owner, group and mode of the\n" +
			"folder, whether the service can write there and whether source add would have to set permissions\n" +
			"(only for a rootless engine: group of the instance user, group write, setgid), and per stack the functions, the findings and the containers\n" +
			"that run in the stack's folder: those of other Compose projects in the Docker engine of the service,\n" +
			"and every container of the folder, also under the stack's own project name, in the rootless Docker\n" +
			"engines of the host's users. Each conflict names its engine, system or rootless:<user>. A rootless\n" +
			"engine that is set up but cannot be asked is listed as not checked, with the reason. Deploying would\n" +
			"create duplicate containers and take their ports twice; qhost warns and does not block. Without sudo\n" +
			"no rootless engine is asked and all are listed as not checked.\n" +
			"--url is the SSH address to clone or to expect as origin; for a folder of files the list does not look\n" +
			"at it, and the files the remote versions join the plan only when 'source add --url' compares. A\n" +
			"folder on a read-only file system is rejected. A rejection is a complete answer and exits 0; read the\n" +
			"case. Nothing is created, changed or fetched, and no value of an env file is shown. Run it with sudo.\n" +
			"As root, inspect adds what 'source add' would do to the folder: missing (the folder does not exist yet,\n" +
			"which matters for clone), permissions (changed paths and a summary, only when source add would change\n" +
			"permissions) and effect, a fingerprint of the case, missing, the listed and the left-out files and the\n" +
			"permission changes. Pass the effect to 'source add --expect-effect' to apply exactly what was shown:\n" +
			"add stops with the code state-changed when the effect differs by then. The address, the stacks and the\n" +
			"containers are not part of the effect. A rejected folder has none of these fields.\n" +
			"--include <path> (repeatable) plans a folder of files as if you had chosen that unclear file for the first\n" +
			"commit, as 'source add --include' does: a file a stack mounts writable or names as a secrets file. The\n" +
			"choice is part of the effect, so pass the same --include to 'source add' together with the effect; an\n" +
			"effect worked out with another choice makes add stop with state-changed. A path that is not unclear is\n" +
			"refused.\n" +
			"--engine inspects the folder as it would be taken over into that Docker engine: system (the default) or\n" +
			"rootless:<user>. The deploy service and the service account of that engine decide the permissions and the\n" +
			"effect, and containers that run in that engine are not conflicts. Pass the same --engine to 'source add'\n" +
			"together with the effect.",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) != 1 {
				return usageFailure("source inspect takes exactly one argument, the path of the folder",
					"Run 'qhost source inspect <path> [--url <ssh-url>]'.")
			}
			return nil
		},
		RunE: func(c *cobra.Command, args []string) error {
			if _, err := folders.selectEngine(c, engine); err != nil {
				return err
			}
			return runSourceInspect(c, program, state, args[0], url, include)
		},
	}
	inspect.Flags().StringVar(&url, "url", "", "SSH address of the repository to clone, or to expect as origin")
	inspect.Flags().StringArrayVar(&include, "include", nil, includeUsage)
	inspect.Flags().StringVar(&engine, "engine", "", "Docker engine the folder would be taken over into: system (default) or rootless:<user>")
	group.AddCommand(inspect, newSourceAddCommand(program, state), newSourceRemoteCommand(program, state), newSourceCompareCommand(program, state), newSourceListCommand(program, state),
		newSourceRemoveCommand(program, state), newSourceMoveCommand(program, state), newSourceChangesCommand(program, state),
		newSourceDiffCommand(program, state), newSourceLogCommand(program, state), newSourceShowCommand(program, state), newSourceDiscardCommand(program, state), newSourceCommitCommand(program, state), newSourceSecureCommand(program, state))
	return group
}

func runSourceInspect(c *cobra.Command, program *cobracli.Program, state *notRunning, path, url string, include []string) error {
	res, err := inspectSource(c, state, path, url, include)
	if err != nil {
		return err
	}
	if !structuredFormat(program.Format()) && targetOf(c.Context()).inst != nil && res.Access != nil && res.Access.GitWritable != nil && !*res.Access.GitWritable {
		_, _ = fmt.Fprintln(c.ErrOrStderr(), gitNotWritableWarning)
	}
	obj := sourceResult(program.Format(), res)
	// Only root can plan the permissions and look at the folder as source add does.
	if folders.euid() == 0 {
		effect, err := inspectEffect(c.Context(), res)
		if err != nil {
			return err
		}
		if effect != nil {
			obj.Fields = append(obj.Fields, effectFields(program.Format(), effect)...)
		}
	}
	return output.Encode(c.OutOrStdout(), program.Format(), obj)
}

// gitNotWritableWarning is for a rootless engine; the service of the system engine is root and needs no access.
const gitNotWritableWarning = "warning: the service cannot write .git; source add sets the group permissions"

func structuredFormat(f output.Format) bool { return f == output.FormatJSON || f == output.FormatTOON }

// inspectSource validates the arguments, asks the service and adds what only this client can tell.
func inspectSource(c *cobra.Command, state *notRunning, path, url string, include []string) (control.SourceInspectResult, error) {
	var res control.SourceInspectResult
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return res, usageFailure("the path must be absolute and clean", "Pass the absolute path of the folder.")
	}
	if url != "" {
		if err := gitrepo.ValidateURL(url); err != nil {
			return res, usageFailure(err.Error(), "Pass the SSH address of the repository.")
		}
	}
	if err := credentialCall(c, state, control.SourceInspectCommand, control.SourceInspectArgs{Path: path, URL: url, Include: include}, &res); err != nil {
		return res, err
	}
	if res.Case == control.SourceCaseReject && gitUnsafeFor(c, path) {
		res.Fix = secureCommandHint(path)
	}
	// The service is sandboxed and sees every path that is not granted as read-only, so only this client can tell.
	if res.Case != control.SourceCaseReject {
		readOnly, err := folders.readOnly(path)
		if err != nil {
			return res, folderFailure("cannot inspect the file system of "+path, err)
		}
		if readOnly {
			res = control.SourceInspectResult{Path: res.Path, Case: control.SourceCaseReject, Reason: readOnlyReason,
				Fix:    "Choose a folder on a writable file system, or remount the file system read-write.",
				Stacks: []control.SourceStack{}}
		}
	}
	return addEngines(c.Context(), res), nil
}

// pathOnReadOnlyFS reports whether the file system of path, or of its nearest existing parent while it is
// missing, is mounted read-only.
func pathOnReadOnlyFS(path string) (bool, error) {
	target := path
	for {
		_, err := os.Lstat(target)
		if err == nil {
			break
		}
		if !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, syscall.ENOTDIR) {
			return false, err
		}
		parent := filepath.Dir(target)
		if parent == target {
			break
		}
		target = parent
	}
	var st syscall.Statfs_t
	if err := syscall.Statfs(target, &st); err != nil {
		return false, err
	}
	return st.Flags&stRdonly != 0, nil
}

// pathsOnSameFS reports whether two existing paths lie on the same file system, without following links.
func pathsOnSameFS(a, b string) (bool, error) {
	var sa, sb syscall.Stat_t
	if err := syscall.Lstat(a, &sa); err != nil {
		return false, &fs.PathError{Op: "lstat", Path: a, Err: err}
	}
	if err := syscall.Lstat(b, &sb); err != nil {
		return false, &fs.PathError{Op: "lstat", Path: b, Err: err}
	}
	return sa.Dev == sb.Dev, nil
}

// sourceResult is the nested result for JSON and TOON and one flat field list for the text formats.
func sourceResult(format output.Format, res control.SourceInspectResult) output.Object {
	if structuredFormat(format) {
		fields := []output.Field{{Name: "path", Value: res.Path}, {Name: "case", Value: res.Case}}
		// An empty string would stay in the output, so a field without a value is left out here.
		add := func(name string, value any) { fields = append(fields, output.Field{Name: name, Value: value}) }
		addText := func(name, value string) {
			if value != "" {
				add(name, value)
			}
		}
		addText("url", res.URL)
		if res.NeedsURL {
			add("needs_url", true)
		}
		addText("reason", res.Reason)
		addText("fix", res.Fix)
		add("access", structured(format, res.Access))
		add("planned_files", res.PlannedFiles)
		add("excluded", structured(format, res.Excluded))
		if len(res.Included) > 0 {
			add("included", structured(format, res.Included))
		}
		if res.Ignore != nil {
			add("ignore", structured(format, res.Ignore))
		}
		add("stacks", structured(format, res.Stacks))
		addText("stacks_error", res.StacksError)
		add("containers_checked", res.ContainersChecked)
		addText("containers_error", res.ContainersError)
		if len(res.EnginesNotChecked) > 0 {
			add("engines_not_checked", structured(format, res.EnginesNotChecked))
		}
		if e := suggestedEngines(res); len(e) > 0 {
			add("suggested_engines", e)
		}
		return output.Object{Fields: fields}
	}
	f := []output.Field{{Name: "path", Value: res.Path}, {Name: "case", Value: res.Case}}
	add := func(name string, value any) { f = append(f, output.Field{Name: name, Value: value}) }
	if res.Reason != "" {
		add("reason", res.Reason)
		add("fix", res.Fix)
	}
	switch {
	case res.NeedsURL:
		add("url", "none; pass --url <ssh-url> to clone")
	case res.URL != "":
		add("url", res.URL)
	}
	if a := res.Access; a != nil {
		add("owner", nameOrID(a.Owner, a.OwnerID))
		add("group", nameOrID(a.Group, a.GroupID))
		add("mode", a.Mode)
		if a.ModePath != res.Path {
			add("mode_path", a.ModePath)
		}
		add("service_can_write", a.ServiceCanWrite)
		add("source_add_sets_permissions", a.SetsPermissions)
		if a.GitWritable != nil {
			add("git_writable", *a.GitWritable)
		}
	}
	for _, p := range res.PlannedFiles {
		add("planned_file", p)
	}
	for _, e := range res.Excluded {
		value := e.Path + ": " + e.Reason
		if e.Stack != "" {
			value += " (stack " + e.Stack + ")"
		}
		add("excluded", value)
	}
	for _, e := range res.Included {
		add("included", e.Path+": "+e.Reason+" (chosen)")
	}
	if ig := res.Ignore; ig != nil {
		add("gitignore", ig.Action)
		for _, l := range ig.Lines {
			add("gitignore_line", l)
		}
	}
	for _, st := range res.Stacks {
		p := "stack." + st.Name + "."
		add(p+"folder", st.Folder)
		for _, fn := range st.Functions {
			value := "no"
			if fn.Works {
				value = "yes"
			}
			if fn.Mode != "" {
				value += ", " + fn.Mode
			}
			value += ": " + fn.Reason
			if fn.Action != "" {
				value += " Action: " + fn.Action
			}
			add(p+fn.Function, value)
		}
		for _, fd := range st.Findings {
			add(p+"finding."+fd.Rule, fd.Severity+": "+fd.Cause+" Fix: "+fd.Fix)
		}
		for _, u := range st.Unresolved {
			add(p+"unresolved", u)
		}
		for _, cf := range st.Conflicts {
			add(p+"conflict", conflictText(cf))
		}
	}
	for _, nc := range res.EnginesNotChecked {
		add("engine_not_checked", notCheckedText(nc))
	}
	if e := suggestedEngines(res); len(e) > 0 {
		add("suggested_engine", suggestionText(res.Path, e))
	}
	if res.StacksError != "" {
		add("stacks_error", res.StacksError)
	}
	if res.ContainersError != "" {
		add("containers_error", res.ContainersError)
	}
	return output.Object{Fields: f}
}

func nameOrID(name string, id uint32) string {
	if name != "" {
		return name
	}
	return strconv.FormatUint(uint64(id), 10)
}
