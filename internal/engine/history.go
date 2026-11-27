package engine

import (
	"context"

	"github.com/castrowithcee/qatlas-host/internal/gitrepo"
	"github.com/castrowithcee/qatlas-host/internal/stacks"
)

// LocalChanges is the part of a working copy that shows and changes local changes and reads the history.
type LocalChanges interface {
	Diff(ctx context.Context, path string) (text string, truncated bool, err error)
	Discard(ctx context.Context, paths []string) error
	CommitPaths(ctx context.Context, expectHead string, paths []string, list []stacks.Stack, message string) (string, error)
	Log(ctx context.Context, limit int) (commits []gitrepo.Commit, truncated bool, err error)
	Show(ctx context.Context, commit string) (text string, truncated bool, err error)
}

// History returns at most limit commits of the first-parent history of the working copy (see gitrepo.Repo.Log).
func (e *Engine) History(ctx context.Context, limit int) (commits []gitrepo.Commit, truncated bool, err error) {
	b, err := e.cfg.Source()
	if err != nil {
		return nil, false, err
	}
	if err := e.lockCommand(ctx); err != nil {
		return nil, false, err
	}
	defer e.lock.Unlock()
	return b.Copy.Log(ctx, limit)
}

// ShowCommit returns the diff of one commit of that history, cut where gitrepo.Repo.Show cuts it.
func (e *Engine) ShowCommit(ctx context.Context, commit string) (text string, truncated bool, err error) {
	b, err := e.cfg.Source()
	if err != nil {
		return "", false, err
	}
	if err := e.lockCommand(ctx); err != nil {
		return "", false, err
	}
	defer e.lock.Unlock()
	return b.Copy.Show(ctx, commit)
}
