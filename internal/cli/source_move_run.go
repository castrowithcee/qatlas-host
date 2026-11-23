package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"

	"github.com/castrowithcee/qatlas-kit/cli"
	"github.com/castrowithcee/qatlas-kit/cobracli"
	"github.com/castrowithcee/qatlas-kit/output"

	"github.com/castrowithcee/qatlas-host/internal/client"
	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/engine"
)

// moveRunWait bounds the wait for the run of the moved stack, a little above the time limit of a run; movePoll is
// how often the runs are read meanwhile. Tests shorten both.
var (
	moveRunWait = engine.DefaultRunTimeout + 5*time.Minute
	movePoll    = time.Second
)

// moveComposeLimit bounds one docker compose call of the service, which may pull an image for up.
const moveComposeLimit = 6 * time.Minute

// The results of a move.
const (
	moveMoved      = "moved"
	moveRolledBack = "rolled-back"
	moveIncomplete = "rollback-incomplete"
)

// moveStep is one step of a move or of its way back as the output lists it.
type moveStep struct {
	Step   string `json:"step"`
	Result string `json:"result"`
	Detail string `json:"detail,omitempty"`
}

// pathMode is the group and mode a path of the folder had before the move, relative to the folder.
type pathMode struct {
	rel  string
	dir  bool
	gid  int
	mode fs.FileMode
}

// fileState is a file as it was before the move: absent, or a regular file with its content and mode.
type fileState struct {
	known, exists bool
	data          []byte
	mode          fs.FileMode
	uid, gid      int
}

// moveRun is one move with everything the way back needs: what was reached, and the state before.
type moveRun struct {
	c                                   *cobra.Command
	ctx                                 context.Context
	res                                 control.SourceMoveResult
	path, into, target, folder, project string
	wasRunning                          bool

	ignore  fileState
	perms   []pathMode
	renamed bool
	// head is the head of the source the commit was asked to build on; landed says a commit may hold the
	// folder, commit is it when it is known.
	head        string
	landed      bool
	commit      string
	deployTried bool
	runOpen     bool
	run         control.RunInfo
	revert      string

	steps, back []moveStep
	failedStep  string
	cause       error
	remaining   []string
}

// moveQuestion is what the operator confirms.
func moveQuestion(res control.SourceMoveResult) string {
	return fmt.Sprintf("Move %s to %s, commit and push it to the source, and deploy it there? The project is down meanwhile; "+
		"a failed step takes everything back.", res.Path, res.Target)
}

// runConfirmedMove decides between the preview and the move. Without --yes and without a terminal it is the
// preview only; otherwise the preview is shown, a block stops, and the move runs after the confirmation.
func runConfirmedMove(c *cobra.Command, program *cobracli.Program, gate *cobracli.Gate, res control.SourceMoveResult) error {
	format := program.Format()
	yes, _ := c.Flags().GetBool("yes")
	if !yes && !stdinIsTerminal() {
		return output.Encode(c.OutOrStdout(), format, moveResult(format, res))
	}
	if err := folders.requireRoot(); err != nil {
		return err
	}
	if !structuredFormat(format) {
		if err := output.Encode(c.ErrOrStderr(), format, moveResult(format, res)); err != nil {
			return err
		}
	}
	if len(res.Blocks) > 0 {
		return usageFailure("the move is blocked ("+strings.Join(issueCodesOf(res.Blocks), ", ")+"); nothing was changed",
			"Fix each block as 'qhost source move "+res.Path+" --into "+res.Into+"' names it, then run the command again.")
	}
	return gate.Run(cobracli.Request{Question: moveQuestion(res)}, func() error {
		return executeMove(c, program, res)
	})
}

func issueCodesOf(list []control.SourceMoveIssue) []string {
	out := make([]string, len(list))
	for i, b := range list {
		out[i] = b.Code
	}
	return out
}

// executeMove moves the project, or takes back what it did, and reports either.
func executeMove(c *cobra.Command, program *cobracli.Program, res control.SourceMoveResult) error {
	m := &moveRun{c: c, ctx: c.Context(), res: res, path: res.Path, into: res.Into, target: res.Target,
		folder: filepath.Base(res.Path), project: res.NewProject, wasRunning: len(res.Running) > 0}
	if err := m.check(); err != nil {
		return err
	}
	before, err := m.runIDs()
	if err != nil {
		return folderFailure("cannot read the runs of the stack "+m.project+"; nothing was changed", err)
	}
	m.ignore = readFileState(filepath.Join(m.path, ".gitignore"))
	if err := m.forward(before); err != nil {
		m.cause = err
		// The way back runs to its end even when the command is interrupted.
		m.ctx = context.WithoutCancel(m.ctx)
		m.rollback()
	}
	return m.report(program.Format())
}

// check compares the preview with the folder as it is now. Nothing is changed by then.
func (m *moveRun) check() error {
	if m.project == "" || m.project != m.res.Project || len(m.res.PlannedFiles) == 0 || m.target != filepath.Join(m.into, m.folder) {
		return folderFailure("the preview does not describe a complete move; nothing was changed", nil)
	}
	for _, p := range []string{m.path, m.into} {
		// rename and the changes of group and mode never follow a symbolic link, at no level of the paths.
		if real, err := filepath.EvalSymlinks(p); err != nil || real != p {
			return usageFailure(p+" is or lies below a symbolic link, or is gone; nothing was changed",
				"Pass the real paths of the folder and of the source.")
		}
	}
	if err := requireRealDir(m.path); err != nil {
		return err
	}
	if _, err := os.Lstat(m.target); !errors.Is(err, fs.ErrNotExist) {
		return usageFailure("the target folder "+m.target+" exists now; nothing was changed", "Run the preview again.")
	}
	return nil
}

// forward runs the steps in their order. The folder is renamed only after the containers are down, right before
// the commit, so that it lies in the source untracked for as short a time as possible.
func (m *moveRun) forward(before map[string]bool) error {
	steps := []struct {
		name string
		fn   func() error
	}{
		{"down", func() error { return m.compose(control.SourceProjectDown) }},
		{"rename", m.rename},
		{"permissions", m.setPermissions},
		{"commit", m.commitFolder},
		{"deploy", func() error { return m.deploy(before) }},
	}
	for _, s := range steps {
		if err := s.fn(); err != nil {
			m.failedStep = s.name
			m.steps = append(m.steps, moveStep{Step: s.name, Result: "failed", Detail: err.Error()})
			return err
		}
		m.steps = append(m.steps, moveStep{Step: s.name, Result: "done", Detail: m.doneDetail(s.name)})
	}
	return nil
}

func (m *moveRun) doneDetail(step string) string {
	switch step {
	case "rename":
		return m.path + " -> " + m.target
	case "permissions":
		return fmt.Sprintf("%d paths", len(m.perms))
	case "commit":
		return m.commit
	case "deploy":
		return "run " + m.run.ID + " " + m.run.Result
	}
	return ""
}

func (m *moveRun) call(limit time.Duration, command string, args, res any) error {
	return callFor(m.ctx, limit, targetOf(m.ctx).endpoint(), command, args, res)
}

// callWhenFree is call for a command that needs the working copy to itself, which runs of other stacks may hold:
// a busy answer is asked again until moveRunWait is over.
func (m *moveRun) callWhenFree(command string, args, res any) error {
	deadline := time.Now().Add(moveRunWait)
	for {
		err := m.call(remoteCallLimit, command, args, res)
		var remote *control.Error
		if !errors.As(err, &remote) || remote.Code != control.CodeBusy || time.Now().After(deadline) {
			return err
		}
		select {
		case <-m.ctx.Done():
			return err
		case <-time.After(5 * movePoll):
		}
	}
}

// compose takes the project down or up in its old folder through the deploy service of its engine.
func (m *moveRun) compose(action string) error {
	var res control.SourceProjectResult
	return m.call(moveComposeLimit, control.SourceProjectCommand, control.SourceProjectArgs{Path: m.path, Project: m.project, Action: action}, &res)
}

// rename moves the folder into the source on the same file system; it never replaces what lies at the target.
func (m *moveRun) rename() error {
	return m.renameDir(m.path, m.target, func() { m.renamed = true })
}

func (m *moveRun) renameBack() error {
	return m.renameDir(m.target, m.path, func() { m.renamed = false })
}

func (m *moveRun) renameDir(from, to string, moved func()) error {
	before, err := os.Lstat(from)
	if err != nil || !before.IsDir() {
		return fmt.Errorf("%s is no folder any more", from)
	}
	if err := unix.Renameat2(unix.AT_FDCWD, from, unix.AT_FDCWD, to, unix.RENAME_NOREPLACE); err != nil {
		return fmt.Errorf("rename %s to %s: %w", from, to, err)
	}
	moved()
	if after, err := os.Lstat(to); err != nil || !os.SameFile(before, after) {
		return fmt.Errorf("the folder changed while it was renamed to %s", to)
	}
	return nil
}

// setPermissions gives the service group the folder and the planned files, as source add does for a folder of
// files, and remembers group and mode of every path first.
func (m *moveRun) setPermissions() error {
	_, gid, err := folders.owner(targetOf(m.ctx).account())
	if err != nil {
		return fmt.Errorf("cannot find the service account %s: %w", targetOf(m.ctx).account(), err)
	}
	plan, err := planListedFiles(m.target, gid, m.res.PlannedFiles, m.res.BindMounts)
	if err != nil {
		return err
	}
	for _, set := range []struct {
		paths []string
		dir   bool
	}{{plan.dirs, true}, {plan.files, false}} {
		for _, p := range set.paths {
			info, ok := plainBelow(m.target, p, set.dir)
			if !ok {
				continue
			}
			rel, _ := filepath.Rel(m.target, p)
			m.perms = append(m.perms, pathMode{rel: rel, dir: set.dir, gid: int(info.Sys().(*syscall.Stat_t).Gid),
				mode: info.Mode() & (fs.ModePerm | fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky)})
		}
	}
	return folders.applyPermissions(m.ctx, plan)
}

// restorePermissions puts back group and mode in the old folder; a path that is gone or became a link is left.
func (m *moveRun) restorePermissions() error {
	for _, p := range m.perms {
		path := filepath.Join(m.path, p.rel)
		if _, ok := plainBelow(m.path, path, p.dir); !ok {
			continue
		}
		if err := folders.lchown(path, -1, p.gid); err != nil {
			return fmt.Errorf("cannot set the group of %s back: %w", path, err)
		}
		if _, ok := plainBelow(m.path, path, p.dir); !ok {
			continue
		}
		if err := folders.chmod(path, p.mode); err != nil {
			return fmt.Errorf("cannot set the mode of %s back: %w", path, err)
		}
	}
	return nil
}

// sourceHead is the head of the working copy of the source.
func (m *moveRun) sourceHead() (string, error) {
	var res control.SourceListResult
	if err := m.call(client.CallTimeout, control.SourceListCommand, nil, &res); err != nil {
		return "", err
	}
	for _, s := range res.Sources {
		if s.Path == m.into {
			return s.Head, nil
		}
	}
	return "", errors.New("the source " + m.into + " is not bound any more")
}

// commitFolder commits and pushes the folder through the service.
//
// The head is read after the rename on purpose. As long as the folder lies untracked in the source, the working
// copy counts as changed, and the poll skips its fast-forward instead of attempting it, so no fast-forward can
// trip over the untracked files and the head read here is the one the commit finds; a head read before the
// rename could miss a fast-forward made in between and refuse the commit for nothing. The only fast-forward while
// the folder is untracked is the catch-up of the commit with a remote that moved, which git refuses only when the
// remote added this very folder; then, as for any refusal, nothing is committed and the move is taken back before
// any push. The skipped fast-forwards follow right after the commit, which wakes the poll.
func (m *moveRun) commitFolder() error {
	head, err := m.sourceHead()
	if err != nil {
		return err
	}
	m.head = head
	var res control.SourceFolderCommitResult
	args := control.SourceFolderCommitArgs{Path: m.into, Folder: m.folder, Files: m.res.PlannedFiles, Head: head, SudoUID: uint32(changeEnv.sudoUID())}
	if err := m.callWhenFree(control.SourceFolderCommitCommand, args, &res); err != nil {
		// An answer that was lost, as after a time-out, does not say that nothing landed.
		if now, herr := m.sourceHead(); herr != nil || now != head {
			m.landed, m.commit = true, now
		}
		return err
	}
	m.landed, m.commit = true, res.Commit
	return nil
}

// runIDs are the runs the stack had before the move: a name may have runs of an earlier attempt.
func (m *moveRun) runIDs() (map[string]bool, error) {
	var res control.RunsResult
	if err := m.call(client.CallTimeout, control.RunsCommand, control.RunsArgs{Stack: m.project}, &res); err != nil {
		return nil, err
	}
	ids := map[string]bool{}
	for _, r := range res.Runs {
		ids[r.ID] = true
	}
	return ids, nil
}

// deploy starts the run of the stack at the new place and waits for its result. The commit has woken the poll,
// which may have started the run already; then the new run is the one to wait for.
func (m *moveRun) deploy(before map[string]bool) error {
	m.deployTried = true
	var started control.DeployResult
	err := m.call(client.CallTimeout, control.DeployCommand, control.DeployArgs{Stack: m.project}, &started)
	var remote *control.Error
	if err != nil && (!errors.As(err, &remote) || remote.Code != control.CodeRunActive) {
		return err
	}
	deadline := time.Now().Add(moveRunWait)
	for {
		var runs control.RunsResult
		if err := m.call(client.CallTimeout, control.RunsCommand, control.RunsArgs{Stack: m.project}, &runs); err != nil {
			return err
		}
		for _, r := range runs.Runs {
			if (started.ID != "" && r.ID == started.ID) || (started.ID == "" && !before[r.ID]) {
				m.run = r
				if r.Result != engine.ResultSucceeded {
					return fmt.Errorf("the run %s ended %s: %s", r.ID, r.Result, r.Error)
				}
				return nil
			}
		}
		if time.Now().After(deadline) {
			m.runOpen = true
			return fmt.Errorf("the run of %s did not end within %s", m.project, moveRunWait)
		}
		select {
		case <-m.ctx.Done():
			m.runOpen = true
			return m.ctx.Err()
		case <-time.After(movePoll):
		}
	}
}

// revertCommit takes the move commit back through the service, which has the deploy key.
func (m *moveRun) revertCommit() error {
	commit := m.commit
	if commit == "" {
		now, err := m.sourceHead()
		if err != nil {
			return err
		}
		if now == m.head {
			return nil
		}
		commit = now
	}
	var res control.SourceFolderRevertResult
	args := control.SourceFolderRevertArgs{Path: m.into, Folder: m.folder, Commit: commit, SudoUID: uint32(changeEnv.sudoUID())}
	if err := m.callWhenFree(control.SourceFolderRevertCommand, args, &res); err != nil {
		return err
	}
	m.revert = res.Revert
	return nil
}

// rollback takes back what the move reached, in the order that keeps every file: the folder leaves the source
// before the revert, whose fast-forward would otherwise delete its versioned files. Waiting for an open run and
// renaming back are what every later step needs, so the way back stops when one of them fails; any other step
// that fails is noted and the rest still runs. What failed is left as instructions.
func (m *moveRun) rollback() {
	type backStep struct {
		name     string
		fn       func() error
		todo     string
		required bool
	}
	var plan []backStep
	if m.runOpen {
		plan = append(plan, backStep{"wait", func() error { return errors.New("the run of the stack is still active") },
			fmt.Sprintf("wait until the run of %s ends ('qhost stack runs %s')", m.project, m.project), true})
	}
	if m.renamed {
		plan = append(plan, backStep{"rename-back", m.renameBack, fmt.Sprintf("move the folder back with 'mv %s %s'", m.target, m.path), true})
	}
	if m.deployTried {
		plan = append(plan, backStep{"remove-containers", func() error { return m.compose(control.SourceProjectDown) },
			fmt.Sprintf("remove the containers of the project with 'docker compose down' in %s", m.path), false})
	}
	if m.landed {
		what := "the move commit"
		if m.commit != "" {
			what = "the commit " + m.commit
		}
		plan = append(plan, backStep{"revert", m.revertCommit,
			fmt.Sprintf("take back %s on the branch of %s with git revert and push it, then run 'qhost repo fetch --dir %s'", what, m.into, m.into), false})
	}
	if len(m.perms) > 0 {
		plan = append(plan, backStep{"permissions", m.restorePermissions, m.permsTodo(), false})
	}
	if m.renamed || m.landed {
		plan = append(plan, backStep{"gitignore", m.restoreIgnore, "put back the .gitignore of " + m.path + " as it was before the move", false})
	}
	if m.wasRunning {
		plan = append(plan, backStep{"up", func() error { return m.compose(control.SourceProjectUp) },
			fmt.Sprintf("start the project again with 'docker compose up -d' in %s", m.path), false})
	}
	for i, s := range plan {
		err := s.fn()
		if err == nil {
			m.back = append(m.back, moveStep{Step: s.name, Result: "done", Detail: m.backDetail(s.name)})
			continue
		}
		m.back = append(m.back, moveStep{Step: s.name, Result: "failed", Detail: err.Error()})
		if !s.required {
			m.remaining = append(m.remaining, s.todo)
			continue
		}
		for _, rest := range plan[i:] {
			m.remaining = append(m.remaining, rest.todo)
		}
		return
	}
}

func (m *moveRun) backDetail(step string) string {
	switch step {
	case "rename-back":
		return m.target + " -> " + m.path
	case "revert":
		if m.revert == "" {
			return "nothing on the branch to take back"
		}
		return m.revert
	case "permissions":
		return fmt.Sprintf("%d paths", len(m.perms))
	}
	return ""
}

// permsTodo names the group and mode of each path as they were, limited to the first ones.
func (m *moveRun) permsTodo() string {
	var parts []string
	for i, p := range m.perms {
		if i == listLimit {
			parts = append(parts, fmt.Sprintf("and %d more", len(m.perms)-listLimit))
			break
		}
		parts = append(parts, fmt.Sprintf("%s gid %d mode %04o", filepath.Join(m.path, p.rel), p.gid, p.mode.Perm()|setBits(p.mode)))
	}
	return "set group and mode back: " + strings.Join(parts, ", ")
}

func setBits(mode fs.FileMode) fs.FileMode {
	var bits fs.FileMode
	if mode&fs.ModeSetuid != 0 {
		bits |= 0o4000
	}
	if mode&fs.ModeSetgid != 0 {
		bits |= 0o2000
	}
	if mode&fs.ModeSticky != 0 {
		bits |= 0o1000
	}
	return bits
}

func readFileState(path string) fileState {
	info, err := os.Lstat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return fileState{known: true}
	case err != nil || !info.Mode().IsRegular():
		return fileState{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fileState{}
	}
	st := info.Sys().(*syscall.Stat_t)
	return fileState{known: true, exists: true, data: data, mode: info.Mode().Perm(), uid: int(st.Uid), gid: int(st.Gid)}
}

// restoreIgnore puts the .gitignore of the old folder back as it was before the commit wrote its lines: removed
// when the move created it, the old content when the move changed it. A file that is no regular file is left.
func (m *moveRun) restoreIgnore() error {
	path := filepath.Join(m.path, ".gitignore")
	now := readFileState(path)
	switch {
	case !m.ignore.known || !now.known || !now.exists && !m.ignore.exists:
		return nil
	case !m.ignore.exists:
		return os.Remove(path)
	case !now.exists || !bytes.Equal(now.data, m.ignore.data):
		return replaceFile(path, m.ignore)
	}
	return nil
}

// replaceFile writes a new file next to path, never through a link, and renames it over path.
func replaceFile(path string, want fileState) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".gitignore-qhost-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	_, err = tmp.Write(want.data)
	if err == nil {
		err = tmp.Chmod(want.mode)
	}
	if err == nil {
		err = tmp.Chown(want.uid, want.gid)
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// report prints the outcome, and fails unless the project moved.
func (m *moveRun) report(format output.Format) error {
	result, location := moveMoved, m.target
	switch {
	case m.cause == nil:
	case len(m.remaining) == 0:
		result, location = moveRolledBack, m.path
	default:
		result = moveIncomplete
		if !m.renamed {
			location = m.path
		}
	}
	if err := output.Encode(m.c.OutOrStdout(), format, m.object(format, result, location)); err != nil {
		return err
	}
	switch result {
	case moveRolledBack:
		state := "the project is in " + m.path + " again"
		if m.wasRunning {
			state = "the project runs in " + m.path + " again"
		}
		return &cli.Failure{Code: output.CodeRuntime, Cause: m.cause,
			Message:  fmt.Sprintf("the move failed at the %s step (%v) and was taken back: %s", m.failedStep, m.cause, state),
			NextStep: "Fix the cause, then run the preview and the move again."}
	case moveIncomplete:
		var failed []string
		for _, s := range m.back {
			if s.Result == "failed" {
				failed = append(failed, s.Step+": "+s.Detail)
			}
		}
		return &cli.Failure{Code: output.CodeRuntime, Cause: m.cause,
			Message: fmt.Sprintf("the move failed at the %s step (%v), and the way back did not finish (%s); the folder is in %s",
				m.failedStep, m.cause, strings.Join(failed, "; "), location),
			NextStep: "Finish the way back by hand: " + strings.Join(m.remaining, "; ") + "."}
	}
	return nil
}

func (m *moveRun) object(format output.Format, result, location string) output.Object {
	var f []output.Field
	add := func(name string, value any) { f = append(f, output.Field{Name: name, Value: value}) }
	addText := func(name, value string) {
		if value != "" {
			add(name, value)
		}
	}
	add("path", m.path)
	add("into", m.into)
	add("target", m.target)
	add("project", m.project)
	add("result", result)
	add("location", location)
	addText("failed_step", m.failedStep)
	if m.cause != nil {
		add("error", m.cause.Error())
	}
	addText("commit", m.commit)
	addText("revert", m.revert)
	addText("run", m.run.ID)
	addText("run_result", m.run.Result)
	if structuredFormat(format) {
		add("moved", result == moveMoved)
		add("steps", structured(format, m.steps))
		add("rollback", structured(format, nonNil(m.back)))
		add("remaining", nonNil(m.remaining))
		return output.Object{Fields: f}
	}
	for _, s := range m.steps {
		add("step."+s.Step, strings.TrimSpace(s.Result+" "+s.Detail))
	}
	for _, s := range m.back {
		add("rollback."+s.Step, strings.TrimSpace(s.Result+" "+s.Detail))
	}
	for _, r := range m.remaining {
		add("remaining", r)
	}
	return output.Object{Fields: f}
}

func nonNil[T any](list []T) []T {
	if list == nil {
		return []T{}
	}
	return list
}
