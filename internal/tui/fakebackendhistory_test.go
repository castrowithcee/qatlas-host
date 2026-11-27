package tui

import (
	"context"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// fakeHistory is what the fake keeps of the history: the answer of source.log by folder and the diff of source.show
// by commit.
type fakeHistory struct {
	commitLogs map[string]control.SourceLogResult
	shows      map[string]string
}

func (f *fakeBackend) SourceLog(_ context.Context, path string, _ int) (control.SourceLogResult, error) {
	f.count("sourcelog")
	if err := f.fail("sourcelog", path); err != nil {
		return control.SourceLogResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	res := f.commitLogs[path]
	res.Path = path
	return res, nil
}

func (f *fakeBackend) SourceShow(_ context.Context, path, commit string) (control.SourceShowResult, error) {
	if err := f.act("sourceshow", commit); err != nil {
		return control.SourceShowResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return control.SourceShowResult{Path: path, Commit: commit, Diff: f.shows[commit]}, nil
}
