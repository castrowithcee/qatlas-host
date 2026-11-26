package cli

import (
	"context"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/tui"
)

// The changes of a stack folder reach the service that has the folder bound.

func (b *engineBackend) SourceChanges(ctx context.Context, path string) (control.SourceChangesResult, error) {
	return onFolder(b, ctx, path, false, func(c *tui.ChannelBackend) (control.SourceChangesResult, error) { return c.SourceChanges(ctx, path) })
}

func (b *engineBackend) SourceDiff(ctx context.Context, path, file string) (control.SourceDiffResult, error) {
	return onFolder(b, ctx, path, false, func(c *tui.ChannelBackend) (control.SourceDiffResult, error) { return c.SourceDiff(ctx, path, file) })
}

func (b *engineBackend) SourceDiscard(ctx context.Context, path string, files []string) (control.SourceDiscardResult, error) {
	return onFolder(b, ctx, path, false, func(c *tui.ChannelBackend) (control.SourceDiscardResult, error) {
		return c.SourceDiscard(ctx, path, files)
	})
}

func (b *engineBackend) SourceCommit(ctx context.Context, args control.SourceCommitArgs) (control.SourceCommitResult, error) {
	return onFolder(b, ctx, args.Path, false, func(c *tui.ChannelBackend) (control.SourceCommitResult, error) { return c.SourceCommit(ctx, args) })
}

var _ tui.ChangesBackend = (*engineBackend)(nil)
