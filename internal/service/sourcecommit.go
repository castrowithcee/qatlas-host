package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/user"
	"strconv"
	"strings"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/engine"
	"github.com/castrowithcee/qatlas-host/internal/gitrepo"
)

// committer names the person behind the sudo call for the commit message.
func committer(uid uint32) string {
	if uid == 0 {
		return "root"
	}
	id := strconv.FormatUint(uint64(uid), 10)
	if u, err := user.LookupId(id); err == nil {
		return u.Username + " (" + id + ")"
	}
	return "uid " + id
}

// commitAnswer maps an error of the commit to a fixed answer; the text of git or of a path is never passed on,
// except the reason of a remote that refused and the reason a stack does not load, both of them bounded and
// cleaned of what a terminal would act on.
func commitAnswer(err error) error {
	var secrets *gitrepo.SecretsError
	var refused *gitrepo.RemoteRejectedError
	switch {
	case errors.Is(err, gitrepo.ErrHeadMoved):
		return control.NewError(control.CodeHeadMoved, "the branch moved since the changes were read; nothing was committed, read the changes again")
	case errors.Is(err, engine.ErrNotChanged), errors.Is(err, gitrepo.ErrNotChanged), errors.Is(err, gitrepo.ErrUnchanged):
		return control.NewError(control.CodeNotChanged, "a path is not a local change of the stack any more; nothing was committed, read the changes again")
	case errors.Is(err, gitrepo.ErrUnmerged), errors.Is(err, gitrepo.ErrExcluded), errors.Is(err, gitrepo.ErrNotRegular):
		return control.NewError(control.CodeInvalid, "a path has an unresolved merge, is the host's own or is not a regular file; nothing was committed")
	case errors.Is(err, engine.ErrUnreadable):
		return control.NewError(control.CodeUnreadable, "a stack of the files does not load, so nothing was committed: "+
			strings.TrimPrefix(printable(err.Error()), "the stack cannot be read: "))
	case errors.As(err, &secrets):
		return secretsRefusal(secrets.Scan, "nothing was committed")
	case errors.Is(err, gitrepo.ErrPushRejected):
		return control.NewError(control.CodePushRejected, "the remote branch moved and the changes could not be pushed on top of it; nothing was committed, read the changes again")
	case errors.As(err, &refused):
		return control.NewError(control.CodePushRejected, fmt.Sprintf("the remote refused the push: %s; nothing was committed", printable(refused.Reason)))
	case strings.HasPrefix(err.Error(), "push failed"):
		return pushFailure(err, "nothing was committed")
	}
	return changesAnswer(err)
}

// commitHandler commits the confirmed paths of a bound stack folder. Only root reaches it, so a.SudoUID is as
// trustworthy as the caller.
func commitHandler(eng *sourceSet) control.Command {
	return changing(func(ctx context.Context, raw json.RawMessage) (any, error) {
		var a control.SourceCommitArgs
		if err := decodeArgs(raw, &a); err != nil {
			return nil, err
		}
		if len(a.Files) == 0 || len(a.Files) > control.MaxDiscardPaths {
			return nil, control.NewError(control.CodeInvalid, "name between one and 200 files to commit")
		}
		if !commitIDRe.MatchString(a.Head) {
			return nil, control.NewError(control.CodeInvalid, "the head must be a full commit id")
		}
		e, err := eng.folderEngine(a.Path)
		if err != nil {
			return nil, err
		}
		commit, err := e.CommitChanges(ctx, a.Head, a.Files, committer(a.SudoUID))
		if err != nil {
			return nil, commitAnswer(err)
		}
		res := control.SourceCommitResult{Path: a.Path, Commit: commit, Committed: make([]string, len(a.Files))}
		for i, p := range a.Files {
			res.Committed[i] = shown(p, false)
		}
		return res, nil
	})
}
