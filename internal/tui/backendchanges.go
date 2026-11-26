package tui

import (
	"context"

	"github.com/castrowithcee/qatlas-host/internal/client"
	"github.com/castrowithcee/qatlas-host/internal/control"
)

// ChangesBackend is what the view of the uncommitted changes of a stack folder needs of a Backend beyond
// SourceChanges. All three need root, so the interface calls them only as root and after a question; without root
// it runs the public commands through sudo instead. SourceDiff only reads. Every Backend of the interface has it.
type ChangesBackend interface {
	SourceDiff(ctx context.Context, path, file string) (control.SourceDiffResult, error)
	SourceDiscard(ctx context.Context, path string, files []string) (control.SourceDiscardResult, error)
	SourceCommit(ctx context.Context, args control.SourceCommitArgs) (control.SourceCommitResult, error)
}

var _ ChangesBackend = (*ChannelBackend)(nil)

func (b *ChannelBackend) SourceDiff(ctx context.Context, path, file string) (res control.SourceDiffResult, err error) {
	err = b.call(ctx, control.SourceDiffCommand, control.SourceDiffArgs{Path: path, File: file}, &res)
	return res, err
}

func (b *ChannelBackend) SourceDiscard(ctx context.Context, path string, files []string) (res control.SourceDiscardResult, err error) {
	err = b.call(ctx, control.SourceDiscardCommand, control.SourceDiscardArgs{Path: path, Files: files}, &res)
	return res, err
}

// SourceCommit may push to the remote, so it has the limit of a call that reaches one.
func (b *ChannelBackend) SourceCommit(ctx context.Context, args control.SourceCommitArgs) (res control.SourceCommitResult, err error) {
	err = client.Do(ctx, b.endpoint, remoteCallLimit, control.SourceCommitCommand, args, &res)
	return res, err
}
