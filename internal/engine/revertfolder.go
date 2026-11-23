package engine

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/castrowithcee/qatlas-host/internal/gitrepo"
	"github.com/castrowithcee/qatlas-host/internal/stacks"
)

// RevertFolder takes back commit, the commit that added the stack folder folder, once the folder was moved out of
// the working copy again: a revert commit on the freshly fetched branch that puts the folder back to its state in
// the parent of commit, pushed as a fast-forward (once more after a rejection), which the working copy then
// follows. The branch must have the folder exactly as commit left it (gitrepo.ErrRevertConflict otherwise).
//
// The only changes the working copy may have are the deletions of the files of folder, which the revert makes as
// well, so following it deletes no file; any other change refuses it with ErrLocalChanges before anything is
// pushed. It waits up to LockWait until no run relies on the working copy (ErrBusy) and keeps everyone off it
// meanwhile. It returns the revert commit, empty when the branch no longer holds the folder.
func (e *Engine) RevertFolder(ctx context.Context, folder, commit, reason string) (string, error) {
	b, err := e.cfg.Source()
	if err != nil {
		return "", err
	}
	wctx, cancel := context.WithTimeout(ctx, e.cfg.LockWait)
	defer cancel()
	if err := e.exclusive(wctx); err != nil {
		if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
			return "", ErrBusy
		}
		return "", err
	}
	defer e.lock.Unlock()
	e.refs.Lock()
	defer e.refs.Unlock()
	wc := b.Copy

	if err := wc.Fetch(ctx); err != nil {
		return "", fmt.Errorf("fetch: %w", err)
	}
	changes, err := wc.Status(ctx)
	if err != nil {
		return "", err
	}
	rest := slices.DeleteFunc(changes, func(c gitrepo.Change) bool {
		return c.Code == " D" && strings.HasPrefix(c.Path, folder+"/")
	})
	list, _ := stacks.Load(b.Dir)
	if _, dirty := classify(rest, list); dirty {
		return "", fmt.Errorf("%w: the working copy cannot follow a revert; nothing was pushed", ErrLocalChanges)
	}
	parent, err := wc.ParentOf(ctx, commit)
	if err != nil {
		return "", err
	}
	var at string
	for attempt := 1; at == ""; attempt++ {
		c, err := wc.RevertStack(ctx, folder, parent, commit, reason)
		if errors.Is(err, gitrepo.ErrNothingToRevert) {
			return "", nil
		}
		if err != nil {
			return "", err
		}
		switch err := wc.Push(ctx, c); {
		case err == nil:
			at = c
		case errors.Is(err, gitrepo.ErrPushRejected) && attempt == 1:
			if err := wc.Fetch(ctx); err != nil {
				return "", fmt.Errorf("fetch: %w", err)
			}
		default:
			return "", err
		}
	}
	if err := wc.FastForward(ctx, at); err != nil {
		return "", fmt.Errorf("the revert %s is on the branch, but the working copy did not follow it: %w", at, err)
	}
	e.cfg.Logger.Info("stack folder reverted", "commit", at)
	e.Wake()
	return at, nil
}
