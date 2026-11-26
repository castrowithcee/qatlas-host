package cli

import (
	"context"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/tui"
)

func (b *engineBackend) SourceChanges(ctx context.Context, path string) (control.SourceChangesResult, error) {
	return onFolder(b, ctx, path, false, func(c *tui.ChannelBackend) (control.SourceChangesResult, error) { return c.SourceChanges(ctx, path) })
}
