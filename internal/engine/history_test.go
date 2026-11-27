package engine

import (
	"context"

	"github.com/castrowithcee/qatlas-host/internal/gitrepo"
)

// The fake has no history; the history commands are tested through the service with a real repository.
func (f *fakeCopy) Log(context.Context, int) ([]gitrepo.Commit, bool, error) { return nil, false, nil }

func (f *fakeCopy) Show(context.Context, string) (string, bool, error) {
	return "", false, gitrepo.ErrCommitNotFound
}
