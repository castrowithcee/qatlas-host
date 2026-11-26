package tui

import (
	"context"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// readFolderChanges counts the uncommitted changes of every stack folder src names. A folder whose changes the service
// does not give, as to a user outside the group qhost, is left out, and its row shows none.
func readFolderChanges(ctx context.Context, b Backend, src []control.SourceStatus) map[stackRef]int {
	out := map[stackRef]int{}
	for _, f := range src {
		c := ctx
		if f.Engine != "" {
			c = WithEngine(ctx, f.Engine)
		}
		if res, err := b.SourceChanges(c, f.Path); err == nil {
			out[stackRef{f.Engine, f.Path}] = len(res.Changes)
		}
	}
	return out
}
