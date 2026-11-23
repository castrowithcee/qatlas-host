package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/castrowithcee/qatlas-host/internal/control"
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
	if a.Folder == "" || strings.ContainsAny(a.Folder, "/\x00") || strings.HasPrefix(a.Folder, ".") {
		return nil, control.NewError(control.CodeInvalid, "the folder must be the name of a folder in the source, without a leading dot")
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
	wc, err := s.repo.set.git.Open(s.repo.set.gitConfig(src.conf), src.conf.Path)
	if err != nil {
		return true
	}
	now, err := wc.Head(ctx)
	return err != nil || now != head
}
