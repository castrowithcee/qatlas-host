package tui

import (
	"context"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// HistoryBackend is what the view of the history of a stack folder needs of a Backend. SourceLog only reads and
// needs no root. SourceShow shows file contents, so only root may call it: the interface calls it as root and runs
// the public command through sudo otherwise.
type HistoryBackend interface {
	SourceLog(ctx context.Context, path string, limit int) (control.SourceLogResult, error)
	SourceShow(ctx context.Context, path, commit string) (control.SourceShowResult, error)
}

var _ HistoryBackend = (*ChannelBackend)(nil)

func (b *ChannelBackend) SourceLog(ctx context.Context, path string, limit int) (res control.SourceLogResult, err error) {
	err = b.call(ctx, control.SourceLogCommand, control.SourceLogArgs{Path: path, Limit: limit}, &res)
	return res, err
}

func (b *ChannelBackend) SourceShow(ctx context.Context, path, commit string) (res control.SourceShowResult, err error) {
	err = b.call(ctx, control.SourceShowCommand, control.SourceShowArgs{Path: path, Commit: commit}, &res)
	return res, err
}
