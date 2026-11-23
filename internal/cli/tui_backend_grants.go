package cli

import (
	"context"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/tui"
)

func (b *engineBackend) GrantList(ctx context.Context, stack string) (control.GrantListResult, error) {
	return onStack(b, ctx, stack, func(c *tui.ChannelBackend) (control.GrantListResult, error) { return c.GrantList(ctx, stack) })
}
