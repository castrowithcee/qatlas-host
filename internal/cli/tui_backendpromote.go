package cli

import (
	"context"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/tui"
)

// The source is read at the service that has it, the check and the run reach the one of the target.

func (b *engineBackend) PromoteSource(ctx context.Context, stack string) (control.PromoteSource, error) {
	return onStack(b, ctx, stack, func(c *tui.ChannelBackend) (control.PromoteSource, error) { return c.PromoteSource(ctx, stack) })
}

func (b *engineBackend) PromoteCheck(ctx context.Context, args control.PromoteArgs) (control.PromoteCheckResult, error) {
	return onStack(b, ctx, args.Stack, func(c *tui.ChannelBackend) (control.PromoteCheckResult, error) { return c.PromoteCheck(ctx, args) })
}

func (b *engineBackend) Promote(ctx context.Context, args control.PromoteArgs) (control.PromoteRunResult, error) {
	return onStack(b, ctx, args.Stack, func(c *tui.ChannelBackend) (control.PromoteRunResult, error) { return c.Promote(ctx, args) })
}

var _ tui.PromoteBackend = (*engineBackend)(nil)
