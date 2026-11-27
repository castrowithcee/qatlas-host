package servicetest

import (
	"context"

	"github.com/castrowithcee/qatlas-host/internal/gitrepo"
)

// The fake has no history.
func (c *copyOf) Log(context.Context, int) ([]gitrepo.Commit, bool, error) { return nil, false, nil }

func (c *copyOf) Show(context.Context, string) (string, bool, error) {
	return "", false, gitrepo.ErrCommitNotFound
}
