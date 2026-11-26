package tui

import (
	"context"

	"github.com/castrowithcee/qatlas-host/internal/client"
	"github.com/castrowithcee/qatlas-host/internal/control"
)

func (b *ChannelBackend) SourceInspect(ctx context.Context, path, url string, include []string) (res control.SourceInspectResult, err error) {
	err = b.call(ctx, control.SourceInspectCommand, control.SourceInspectArgs{Path: path, URL: url, Include: include}, &res)
	return res, err
}

func (b *ChannelBackend) SourceCompare(ctx context.Context, args control.SourceCompareArgs) (res control.SourceCompareResult, err error) {
	err = client.Do(ctx, b.endpoint, remoteCallLimit, control.SourceCompareCommand, args, &res)
	return res, err
}

func (b *ChannelBackend) SourceRemote(ctx context.Context, args control.SourceRemoteArgs) (res control.SourceRemoteResult, err error) {
	err = client.Do(ctx, b.endpoint, remoteCallLimit, control.SourceRemoteCommand, args, &res)
	return res, err
}

func (b *ChannelBackend) SourceAdd(ctx context.Context, args control.SourceAddArgs) (res control.SourceAddResult, err error) {
	err = client.Do(ctx, b.endpoint, remoteCallLimit, control.SourceAddCommand, args, &res)
	return res, err
}

func (b *ChannelBackend) SourceList(ctx context.Context) (res control.SourceListResult, err error) {
	err = b.call(ctx, control.SourceListCommand, nil, &res)
	return res, err
}

func (b *ChannelBackend) SourceRemove(ctx context.Context, path string) (res control.SourceRemoveResult, err error) {
	err = b.call(ctx, control.SourceRemoveCommand, control.SourceRemoveArgs{Path: path}, &res)
	return res, err
}

func (b *ChannelBackend) SourceChanges(ctx context.Context, path string) (res control.SourceChangesResult, err error) {
	err = b.call(ctx, control.SourceChangesCommand, control.SourceChangesArgs{Path: path}, &res)
	return res, err
}
