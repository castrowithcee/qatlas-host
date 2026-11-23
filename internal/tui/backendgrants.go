package tui

import (
	"context"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

func (b *ChannelBackend) GrantList(ctx context.Context, stack string) (res control.GrantListResult, err error) {
	err = b.call(ctx, control.GrantListCommand, control.GrantListArgs{Stack: stack}, &res)
	return res, err
}
