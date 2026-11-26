package tui

import (
	"context"
	"slices"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

func (f *fakeBackend) SourceInspect(ctx context.Context, path, url string, include []string) (control.SourceInspectResult, error) {
	f.count("sourceinspect")
	f.mu.Lock()
	f.inspectEngines = append(f.inspectEngines, EngineOf(ctx))
	f.mu.Unlock()
	if err := f.fail("sourceinspect", path); err != nil {
		return control.SourceInspectResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inspectIncludes = append(f.inspectIncludes, slices.Clone(include))
	if res, ok := f.inspects[path]; ok {
		return planWithInclude(res, include)
	}
	return control.SourceInspectResult{Path: path, Case: control.SourceCaseReject, Reason: "no such folder in the fake",
		Stacks: []control.SourceStack{}}, nil
}

func (f *fakeBackend) SourceCompare(_ context.Context, args control.SourceCompareArgs) (control.SourceCompareResult, error) {
	f.count("sourcecompare")
	if err := f.fail("sourcecompare", args.Path); err != nil {
		return control.SourceCompareResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.compares = append(f.compares, args)
	return f.compare, nil
}

func (f *fakeBackend) SourceRemote(ctx context.Context, args control.SourceRemoteArgs) (control.SourceRemoteResult, error) {
	if err := f.actCtx(ctx, "sourceremote", args.Path); err != nil {
		return control.SourceRemoteResult{}, err
	}
	f.mu.Lock()
	f.remotes = append(f.remotes, args)
	f.mu.Unlock()
	return control.SourceRemoteResult{ID: "demo-id", Path: args.Path, URL: args.URL, State: args.ExpectState, Branch: "main",
		Head: commitA, Pushed: args.ExpectState != control.SourceRemoteEqual}, nil
}

func (f *fakeBackend) SourceAdd(ctx context.Context, args control.SourceAddArgs) (control.SourceAddResult, error) {
	if err := f.actCtx(ctx, "sourceadd", args.Path); err != nil {
		return control.SourceAddResult{}, err
	}
	f.mu.Lock()
	f.sourceAdds = append(f.sourceAdds, args)
	f.mu.Unlock()
	return control.SourceAddResult{ID: "demo-id", Path: args.Path, Kind: "local", Head: commitA}, nil
}

func (f *fakeBackend) SourceList(context.Context) (control.SourceListResult, error) {
	f.count("sourcelist")
	if err := f.fail("sourcelist", ""); err != nil {
		return control.SourceListResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.folderList, nil
}

func (f *fakeBackend) SourceRemove(ctx context.Context, path string) (control.SourceRemoveResult, error) {
	if err := f.actCtx(ctx, "sourceremove", path); err != nil {
		return control.SourceRemoveResult{}, err
	}
	f.mu.Lock()
	f.removes = append(f.removes, path)
	f.mu.Unlock()
	return control.SourceRemoveResult{ID: "demo-id", Path: path}, nil
}

// SourceChanges answers from changes by path; a folder without an entry has none.
func (f *fakeBackend) SourceChanges(_ context.Context, path string) (control.SourceChangesResult, error) {
	if err := f.fail("sourcechanges", path); err != nil {
		return control.SourceChangesResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return control.SourceChangesResult{Path: path, Changes: slices.Clone(f.changes[path])}, nil
}
