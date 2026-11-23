package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/engine"
	"github.com/castrowithcee/qatlas-host/internal/gitrepo"
	"github.com/castrowithcee/qatlas-host/internal/stacks"
)

// handleFolderCommit answers source.commit-folder: it commits a project folder that lies in a source and is not
// versioned yet. The folder is planned again, by the plan the move preview shows, and committed only when the
// confirmed list is exactly that plan and the head is the expected one. The .gitignore of the plan is written
// first, because the paths left out stay untracked until it covers them; it is put back when the commit fails.
func (s *inspectService) handleFolderCommit(ctx context.Context, raw json.RawMessage) (any, error) {
	var a control.SourceFolderCommitArgs
	if err := decodeArgs(raw, &a); err != nil {
		return nil, err
	}
	if !commitIDRe.MatchString(a.Head) {
		return nil, control.NewError(control.CodeInvalid, "the head must be a full commit id")
	}
	if err := checkFolderName(a.Folder); err != nil {
		return nil, err
	}
	if len(a.Files) == 0 {
		return nil, control.NewError(control.CodeInvalid, "name the files of the folder that the preview listed")
	}
	src := s.repo.set.byPath(a.Path)
	if src == nil || checkDirShape(a.Path) != nil {
		return nil, control.NewError(control.CodeNoSuchSource, "no source is bound to this folder")
	}
	root := filepath.Join(a.Path, a.Folder)
	if err := checkDirShape(root); err != nil {
		return nil, control.NewError(control.CodeInvalid, err.Error())
	}
	if info, err := os.Lstat(root); err != nil || !info.IsDir() {
		return nil, control.NewError(control.CodeInvalid, "the folder is not a folder of the source")
	}
	if list, err := stacks.Load(a.Path); err != nil || slices.ContainsFunc(list, func(st stacks.Stack) bool { return st.Folder == stacks.RootFolder }) {
		return nil, control.NewError(control.CodeInvalid, "the source cannot take a second stack: it is one root stack or cannot be read")
	}
	if list, _ := stacks.Load(root); len(list) != 1 || list[0].Folder != stacks.RootFolder || list[0].Err != nil {
		return nil, control.NewError(control.CodeInvalid, "the folder is not one Compose project that loads, with its compose file at its root")
	}

	plan, _, _, err := planFolder(ctx, root, nil)
	var bad *gitrepo.IgnoreError
	if errors.As(err, &bad) {
		return nil, control.NewError(control.CodeInvalid, printable(bad.Error())+". "+ignoreFix)
	}
	if err != nil {
		return nil, err
	}
	defer plan.close()
	access, err := accessOf(root)
	if err != nil {
		return nil, err
	}
	if err := confirmedFiles(a.Files, plan, access); err != nil {
		return nil, err
	}

	paths := make([]string, 0, len(plan.rawPlanned)+1)
	for _, f := range plan.rawPlanned {
		paths = append(paths, a.Folder+"/"+f)
	}
	if plan.ignore.Write && !slices.Contains(plan.rawPlanned, gitrepo.IgnoreName) {
		paths = append(paths, a.Folder+"/"+gitrepo.IgnoreName)
	}
	if s.headMoved(ctx, src, a.Head) {
		return nil, commitAnswer(gitrepo.ErrHeadMoved)
	}
	if err := plan.ignore.Apply(root); err != nil {
		return nil, control.NewError(control.CodeInvalid, "the .gitignore of the folder could not be written: "+printable(err.Error()))
	}
	commit, err := src.eng.CommitChanges(ctx, a.Head, paths, committer(a.SudoUID))
	if err != nil {
		if !s.headMoved(ctx, src, a.Head) {
			_ = plan.ignore.Revert(root)
		}
		return nil, commitAnswer(err)
	}
	res := control.SourceFolderCommitResult{Path: a.Path, Folder: a.Folder, Commit: commit, Committed: make([]string, len(paths)), Ignore: plan.ignoreResult()}
	for i, p := range paths {
		res.Committed[i] = shown(p, false)
	}
	return res, nil
}

// headMoved reports whether the branch of the source is at another commit than head, or cannot be read. After a
// failed commit it tells a commit that landed, which keeps the written .gitignore, from one that did not.
func (s *inspectService) headMoved(ctx context.Context, src *source, head string) bool {
	now, err := s.headOf(ctx, src)
	return err != nil || now != head
}

func (s *inspectService) headOf(ctx context.Context, src *source) (string, error) {
	wc, err := s.repo.set.git.Open(s.repo.set.gitConfig(src.conf), src.conf.Path)
	if err != nil {
		return "", err
	}
	return wc.Head(ctx)
}

func checkFolderName(folder string) error {
	if folder == "" || strings.ContainsAny(folder, "/\x00") || strings.HasPrefix(folder, ".") {
		return control.NewError(control.CodeInvalid, "the folder must be the name of a folder in the source, without a leading dot")
	}
	return nil
}

// handleFolderRevert answers source.revert-folder: it takes back the commit of source.commit-folder once the client
// moved the folder out again. A folder that is still there is refused, because the working copy following the
// revert would delete its files.
func (s *inspectService) handleFolderRevert(ctx context.Context, raw json.RawMessage) (any, error) {
	var a control.SourceFolderRevertArgs
	if err := decodeArgs(raw, &a); err != nil {
		return nil, err
	}
	if !commitIDRe.MatchString(a.Commit) {
		return nil, control.NewError(control.CodeInvalid, "the commit must be a full commit id")
	}
	if err := checkFolderName(a.Folder); err != nil {
		return nil, err
	}
	src := s.repo.set.byPath(a.Path)
	if src == nil || checkDirShape(a.Path) != nil {
		return nil, control.NewError(control.CodeNoSuchSource, "no source is bound to this folder")
	}
	if _, err := os.Lstat(filepath.Join(a.Path, a.Folder)); !errors.Is(err, fs.ErrNotExist) {
		return nil, control.NewError(control.CodeInvalid, "the folder is still in the source; move it out first, nothing was reverted")
	}
	reason := fmt.Sprintf("the move of %s into the source was taken back by %s", a.Folder, committer(a.SudoUID))
	revert, err := src.eng.RevertFolder(ctx, a.Folder, a.Commit, reason)
	if err != nil {
		return nil, revertAnswer(err)
	}
	head, err := s.headOf(ctx, src)
	if err != nil {
		return nil, errors.New("the working copy cannot be read")
	}
	return control.SourceFolderRevertResult{Path: a.Path, Folder: a.Folder, Revert: revert, Head: head}, nil
}

// revertAnswer maps an error of the revert to a fixed answer, as commitAnswer does.
func revertAnswer(err error) error {
	var refused *gitrepo.RemoteRejectedError
	switch {
	case errors.Is(err, gitrepo.ErrRevertConflict):
		return control.NewError(control.CodeInvalid, "the folder changed on the branch after the commit; nothing was reverted")
	case errors.Is(err, engine.ErrLocalChanges):
		return control.NewError(control.CodeLocalChanges, "the working copy has other local changes and cannot follow a revert; nothing was pushed")
	case errors.Is(err, gitrepo.ErrPushRejected):
		return control.NewError(control.CodePushRejected, "the remote branch moved twice and the revert could not be pushed; nothing was reverted")
	case errors.As(err, &refused):
		return control.NewError(control.CodePushRejected, fmt.Sprintf("the remote refused the push: %s; nothing was reverted", printable(refused.Reason)))
	case strings.HasPrefix(err.Error(), "push failed"):
		return pushFailure(err, "nothing was reverted")
	case errors.Is(err, engine.ErrBusy), errors.Is(err, engine.ErrStopped), errors.Is(err, engine.ErrNoRepo):
		return runsAnswer(err)
	}
	return control.NewError(control.CodeInternal, "the revert failed: "+printable(err.Error()))
}
