package tui

import (
	"context"

	"github.com/castrowithcee/qatlas-host/internal/client"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// PromoteBackend is what the take-over of the versions of another stack needs of a Backend. PromoteSource
// reads, at the service that has the stack, what a promotion takes from it. PromoteCheck, at the service of the
// target, changes nothing: it refuses as Promote would and names the versions it sets. Promote sets them as a
// secured run bound to args.Commit, the source commit of the check. Every Backend of the interface has it.
type PromoteBackend interface {
	PromoteSource(ctx context.Context, stack string) (control.PromoteSource, error)
	PromoteCheck(ctx context.Context, args control.PromoteArgs) (control.PromoteCheckResult, error)
	Promote(ctx context.Context, args control.PromoteArgs) (control.PromoteRunResult, error)
}

var _ PromoteBackend = (*ChannelBackend)(nil)

func (b *ChannelBackend) PromoteSource(ctx context.Context, stack string) (res control.PromoteSource, err error) {
	err = b.call(ctx, control.PromoteSourceCommand, control.PromoteSourceArgs{Stack: stack}, &res)
	return res, err
}

func (b *ChannelBackend) PromoteCheck(ctx context.Context, args control.PromoteArgs) (res control.PromoteCheckResult, err error) {
	err = client.Do(ctx, b.endpoint, updateCallLimit, control.PromoteCheckCommand, args, &res)
	return res, err
}

func (b *ChannelBackend) Promote(ctx context.Context, args control.PromoteArgs) (res control.PromoteRunResult, err error) {
	err = client.Do(ctx, b.endpoint, updateCallLimit, control.PromoteRunCommand, args, &res)
	return res, err
}
