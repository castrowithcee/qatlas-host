package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"unicode"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-kit/cobracli"
	"github.com/castrowithcee/qatlas-kit/output"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

func newSourceMoveCommand(program *cobracli.Program, state *notRunning) *cobra.Command {
	var into string
	var gate *cobracli.Gate
	move := &cobra.Command{
		Use:   "move <folder> --into <source> [--yes]",
		Short: "Move a Compose project into an existing source after a preview, with the way back when a step fails",
		Long: "move shows what moving the project in <folder> into the source <source> would do, and does it after you\n" +
			"confirm: at a terminal by answering the question, without a terminal with --yes. Without --yes and without a\n" +
			"terminal it is the preview only and changes nothing. The folder must lie outside every source and hold one\n" +
			"Compose project with its compose file at its root; <source> is the path of a source that exists. The\n" +
			"project gets the folder <source>/<name of the folder>, by renaming the folder on the same file system: the\n" +
			"data stays untouched, and the way back is renaming it back. The preview shows that target folder, the files\n" +
			"a commit into the source takes, as source inspect lists them for a folder of files, what is left out and\n" +
			"the lines for the .gitignore of the folder. It changes nothing: no file is written, no container is\n" +
			"touched, nothing is fetched or committed, and no value of an env file is shown.\n" +
			"A block means the move cannot be done as it is; each block names its code, the reason and what to do.\n" +
			"Blocks: root-stack (the source is one root stack), source-unreadable, target-exists, target-overlaps,\n" +
			"target-name (a leading dot, or other characters than letters, digits, ., _ and -), other-filesystem,\n" +
			"engine-differs (the project runs in another Docker engine than the source), name-taken (a stack of that\n" +
			"name exists in any engine), project-name-changes (the Compose project name would change, which would\n" +
			"create new, empty volumes), absolute-bind-mount (a bind mount names a path inside the old folder),\n" +
			"secrets-in-env (values of secret-like keys in .env, for a source with a remote; the keys are named, never\n" +
			"the values), and folder-rejected, not-a-project, project-is-repository and stack-unreadable for a folder\n" +
			"that cannot be looked at as a project. Warnings do not block: secrets-in-env for a local source,\n" +
			"fixed-tags (images with a fixed tag, which qhost cannot update; the stack counts as deploy only), downtime\n" +
			"(the containers are recreated) and not-checked (an engine or a deploy service could not be asked, or the\n" +
			"containers could not be read). qhost never rewrites compose files or .env: what does not fit is named, and\n" +
			"you change it yourself.\n" +
			"The deploy service of the engine that has <source> bound answers. The Docker engines of the host are\n" +
			"read like source inspect does, only with sudo; the system and rootless engines, and the deploy services of\n" +
			"the other engines, are asked for containers of the folder, stack folders and stack names. The preview\n" +
			"exits 0 also for a rejection or a block; read the movable field of --output json. With --yes a block stops\n" +
			"the command with exit code 2 and changes nothing.\n" +
			"The move takes these steps in this order: down (docker compose down in the old folder, without -v, so\n" +
			"named volumes stay), rename (the folder into the source; never over something at the target and never\n" +
			"through a symbolic link), permissions (the group of the service account with group write on the folder,\n" +
			"the folders of the listed files and the listed files, as source add does; the owner, bind-mount sources\n" +
			"and unlisted files stay), commit (the listed files and the .gitignore lines in one commit as the person\n" +
			"behind sudo, pushed when the source has a remote) and deploy (the normal run of the new stack with its\n" +
			"check; the command waits for its result). The folder lies in the source uncommitted only between rename\n" +
			"and commit; meanwhile the source skips its fast-forwards and catches up right after the commit.\n" +
			"When a step fails, everything is taken back: the folder is renamed back, the containers of the project\n" +
			"are removed once the deploy was tried, a commit that reached the branch is taken back by a revert commit\n" +
			"that is pushed, group and mode of each path and the .gitignore are put back as they were, and a project\n" +
			"that ran is started in its old folder with docker compose up -d. The output lists every step and every\n" +
			"step of the way back with its result; result is moved, rolled-back or rollback-incomplete, location says\n" +
			"where the folder is, and for rollback-incomplete remaining says what is left to do by hand. A move that was\n" +
			"taken back or not finished exits 1. Data is never copied or deleted. Run it with sudo.",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) != 1 {
				return usageFailure("source move takes exactly one argument, the path of the project folder",
					"Run 'qhost source move <folder> --into <source>'.")
			}
			return nil
		},
		RunE: func(c *cobra.Command, args []string) error {
			return runSourceMove(c, program, gate, state, args[0], into)
		},
	}
	move.Flags().StringVar(&into, "into", "", "path of the source the project moves into")
	_ = move.MarkFlagRequired("into")
	gate = program.AddGate(move, cobracli.Gates{Question: true})
	return move
}

func runSourceMove(c *cobra.Command, program *cobracli.Program, gate *cobracli.Gate, state *notRunning, path, into string) error {
	for _, p := range []string{path, into} {
		if !filepath.IsAbs(p) || filepath.Clean(p) != p {
			return usageFailure("the paths must be absolute and clean", "Pass the absolute paths of the folder and of the source.")
		}
	}
	if err := routeFolder(c, state, "", into); err != nil {
		return err
	}
	var res control.SourceMoveResult
	if err := credentialCall(c, state, control.SourceMoveCommand, control.SourceMoveArgs{Path: path, Into: into}, &res); err != nil {
		return err
	}
	res, err := addMoveHost(c.Context(), res, path, into)
	if err != nil {
		return err
	}
	return runConfirmedMove(c, program, gate, res)
}

// addMoveHost adds to the answer of the service what only a client on the host can tell: the file system, the
// engines of the containers of the folder, and the stack folders and names of the other engines. It reads and
// changes nothing.
func addMoveHost(ctx context.Context, res control.SourceMoveResult, path, into string) (control.SourceMoveResult, error) {
	own := targetOf(ctx)
	res.Engine = own.id()
	shown := maskedText(path)
	target := filepath.Join(into, filepath.Base(path))

	if res.Target != "" {
		same, err := folders.sameFS(path, into)
		var gone *fs.PathError
		switch {
		case errors.As(err, &gone) && errors.Is(err, fs.ErrNotExist):
		case err != nil:
			return res, folderFailure("cannot compare the file systems of "+shown+" and "+res.Into, err)
		case !same:
			res.Blocks = append(res.Blocks, control.SourceMoveIssue{Code: control.MoveOtherFilesystem,
				Reason: fmt.Sprintf("the folder %s and the source %s are on different file systems, so the folder cannot be moved by renaming it and its data would have to be copied", shown, res.Into),
				Fix:    "Choose a source on the same file system as the project folder."})
		}
	}

	engines := map[string]bool{}
	if len(res.Running) > 0 {
		engines[own.id()] = true
	}
	var notChecked []control.EngineNotChecked
	scan := folders.scanEngines(ctx)
	for _, cf := range scan.Conflicts(path) {
		if cf.Engine != own.id() {
			engines[cf.Engine] = true
		}
	}
	for _, nc := range scan.NotChecked {
		notChecked = append(notChecked, control.EngineNotChecked{Engine: nc.Engine, Reason: nc.Reason})
	}
	if own.inst != nil {
		// The rootless service does not see the system engine.
		switch {
		case !deployInstalled():
			notChecked = append(notChecked, control.EngineNotChecked{Engine: control.EngineSystem,
				Reason: "the deploy service of the system engine is not set up, so its containers cannot be read"})
		default:
			var list control.ContainerListResult
			if err := callFor(ctx, peerLimit, engineTarget{}.endpoint(), control.ContainerListCommand, control.ContainerListArgs{}, &list); err != nil {
				notChecked = append(notChecked, control.EngineNotChecked{Engine: control.EngineSystem,
					Reason: "the deploy service of the system engine could not be asked (" + briefReason(err) + ")"})
			}
			for _, g := range list.Groups {
				if g.WorkingDir != "" && filepath.Clean(g.WorkingDir) == maskedText(path) {
					engines[control.EngineSystem] = true
				}
			}
		}
	}
	res.EnginesNotChecked = notChecked
	for e := range engines {
		res.ProjectEngines = append(res.ProjectEngines, e)
	}
	sort.Strings(res.ProjectEngines)
	if other := slices.DeleteFunc(slices.Clone(res.ProjectEngines), func(e string) bool { return e == own.id() }); len(other) > 0 {
		res.Blocks = append(res.Blocks, control.SourceMoveIssue{Code: control.MoveEngineDiffers,
			Reason: fmt.Sprintf("containers of the project run in the engine %s, and the source is in the engine %s; qhost does not move a project between engines",
				strings.Join(other, ", "), own.id()),
			Fix: "Take the folder over into the engine it runs in with 'qhost source add --engine', or choose a source of that engine.", Items: other})
	}

	peers, err := folders.askPeers(ctx, own)
	if err != nil {
		return res, err
	}
	var peerNotes []string
	for _, p := range peers {
		if p.err != nil {
			peerNotes = append(peerNotes, uncheckedText(p))
			continue
		}
		for _, src := range p.status.Engine.Sources {
			switch {
			case overlapping(path, src.Path):
				res.Blocks = append(res.Blocks, control.SourceMoveIssue{Code: control.MoveFolderRejected,
					Reason: fmt.Sprintf("the project folder overlaps the stack folder %s of the %s engine, which is a source", maskedText(src.Path), p.engine()),
					Fix:    "Choose a folder that does not overlap a stack folder that is in use."})
			case overlapping(target, src.Path):
				res.Blocks = append(res.Blocks, control.SourceMoveIssue{Code: control.MoveTargetOverlaps,
					Reason: fmt.Sprintf("the target folder %s overlaps the stack folder %s of the %s engine", res.Target, maskedText(src.Path), p.engine()),
					Fix:    "Choose a source whose folders do not overlap a stack folder of another engine."})
			}
		}
		for _, st := range p.status.Engine.Stacks {
			if res.NewProject != "" && st.Name == res.NewProject {
				res.Blocks = append(res.Blocks, control.SourceMoveIssue{Code: control.MoveNameTaken,
					Reason: fmt.Sprintf("the %s engine already has a stack named %s", p.engine(), maskedText(st.Name)),
					Fix:    "Rename or remove that stack first."})
				break
			}
		}
	}

	if len(res.Running) > 0 || len(engines) > 0 {
		res.Warnings = append(res.Warnings, control.SourceMoveIssue{Code: control.MoveDowntime,
			Reason: "the containers of the project are recreated from the new folder, so the project is down for a short time",
			Fix:    "Move in a quiet period."})
	}
	var items []string
	if res.ContainersError != "" {
		items = append(items, "containers of the engine "+own.id()+" were not read: "+res.ContainersError)
	}
	for _, nc := range res.EnginesNotChecked {
		items = append(items, notCheckedText(nc))
	}
	items = append(items, peerNotes...)
	if len(items) > 0 {
		res.Warnings = append(res.Warnings, control.SourceMoveIssue{Code: control.MoveNotChecked,
			Reason: "not every engine could be asked, so containers, stack folders or stack names there are unknown",
			Fix:    "Start the engine or its deploy service, run the command with sudo, then run the preview again.", Items: items})
	}
	sortIssues(res.Blocks)
	sortIssues(res.Warnings)
	return res, nil
}

// issueOrder is the order of blocks and warnings in the answer.
var issueOrder = []string{
	control.MoveFolderRejected, control.MoveNotAProject, control.MoveIsRepository, control.MoveStackUnreadable,
	control.MoveSourceUnreadable, control.MoveRootStack, control.MoveTargetName, control.MoveTargetExists,
	control.MoveTargetOverlaps, control.MoveOtherFilesystem, control.MoveEngineDiffers, control.MoveNameTaken,
	control.MoveProjectName, control.MoveAbsoluteBind, control.MoveSecretsInEnv, control.MoveFixedTags,
	control.MoveDowntime, control.MoveNotChecked,
}

func sortIssues(list []control.SourceMoveIssue) {
	sort.SliceStable(list, func(i, j int) bool {
		return slices.Index(issueOrder, list[i].Code) < slices.Index(issueOrder, list[j].Code)
	})
}

// maskedText replaces what is not printable, as the service does for the paths it reads.
func maskedText(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) {
			return r
		}
		return unicode.ReplacementChar
	}, s)
}

func issueText(i control.SourceMoveIssue) string {
	out := i.Reason + " Fix: " + i.Fix
	if len(i.Items) > 0 {
		out += " (" + strings.Join(i.Items, ", ") + ")"
	}
	return out
}

// moveResult is the nested result for JSON and TOON and one flat field list for the text formats.
func moveResult(format output.Format, res control.SourceMoveResult) output.Object {
	var f []output.Field
	add := func(name string, value any) { f = append(f, output.Field{Name: name, Value: value}) }
	addText := func(name, value string) {
		if value != "" {
			add(name, value)
		}
	}
	movable := len(res.Blocks) == 0
	if structuredFormat(format) {
		add("path", res.Path)
		add("into", res.Into)
		addText("target", res.Target)
		addText("engine", res.Engine)
		addText("source_kind", res.SourceKind)
		addText("project", res.Project)
		addText("new_project", res.NewProject)
		add("movable", movable)
		add("blocks", structured(format, res.Blocks))
		add("warnings", structured(format, res.Warnings))
		add("planned_files", res.PlannedFiles)
		add("excluded", structured(format, res.Excluded))
		if res.Ignore != nil {
			add("ignore", structured(format, res.Ignore))
		}
		add("running", structured(format, res.Running))
		add("project_engines", res.ProjectEngines)
		add("containers_checked", res.ContainersChecked)
		addText("containers_error", res.ContainersError)
		if len(res.EnginesNotChecked) > 0 {
			add("engines_not_checked", structured(format, res.EnginesNotChecked))
		}
		return output.Object{Fields: f}
	}
	add("path", res.Path)
	add("into", res.Into)
	addText("target", res.Target)
	addText("engine", res.Engine)
	addText("project", res.Project)
	addText("new_project", res.NewProject)
	if movable {
		add("movable", "yes")
	} else {
		add("movable", "no")
	}
	for _, b := range res.Blocks {
		add("block."+b.Code, issueText(b))
	}
	for _, w := range res.Warnings {
		add("warning."+w.Code, issueText(w))
	}
	for _, r := range res.Running {
		add("running", fmt.Sprintf("container %s of project %s", r.Container, r.Project))
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
	if ig := res.Ignore; ig != nil {
		add("gitignore", ig.Action)
		for _, l := range ig.Lines {
			add("gitignore_line", l)
		}
	}
	return output.Object{Fields: f}
}
