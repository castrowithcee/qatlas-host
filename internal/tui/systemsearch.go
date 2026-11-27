package tui

import (
	"context"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/castrowithcee/qatlas-kit/projectpath"
	kittui "github.com/castrowithcee/qatlas-kit/tui"
)

// The search of the browser below the shown folder reads the file system with the rights of the person and never
// asks for sudo: a folder it may not read is skipped by projectpath.

// fileSearchMsg carries the end of a search for the place view.
type fileSearchMsg struct {
	view   *fileView
	search kittui.FileSearch
	res    projectpath.SearchResult
	err    error
}

func (msg fileSearchMsg) applyTo(m *Model) { m.applyFileSearch(msg) }

// searchFiles starts the search of the browser, off the event loop, and stops the one running before.
func (m *Model) searchFiles(v *fileView, s kittui.FileSearch) tea.Cmd {
	v.stopSearch()
	ctx, cancel := context.WithCancel(context.Background())
	v.cancel = cancel
	start := v.path(s.Dir...)
	return func() tea.Msg {
		res, err := projectpath.Search(ctx, start, s.Query, projectpath.SearchOptions{})
		return fileSearchMsg{view: v, search: s, res: res, err: err}
	}
}

// stopSearch ends the running search, if any.
func (v *fileView) stopSearch() {
	if v.cancel != nil {
		v.cancel()
		v.cancel = nil
	}
}

// applyFileSearch hands the end of a search to the browser, which drops an answer to a request it left.
func (m *Model) applyFileSearch(msg fileSearchMsg) {
	b := m.fileBrowser(msg.view)
	if b == nil {
		return
	}
	if msg.err != nil {
		b.SetSearchError(msg.search, msg.err)
		return
	}
	found := kittui.FileFound{}
	// The search reports canonical paths, so the root is compared in the same form.
	root := projectpath.Canonical(msg.view.root)
	for _, mt := range msg.res.Matches {
		rel, err := filepath.Rel(root, mt.Path)
		if err != nil || !filepath.IsLocal(rel) {
			continue
		}
		kind := kittui.FileRegular
		switch {
		case mt.Dir:
			kind = kittui.FileDir
		case mt.Link:
			kind = kittui.FileLink
		}
		found.Matches = append(found.Matches, kittui.FileMatch{Path: strings.Split(rel, string(filepath.Separator)), Kind: kind})
	}
	switch msg.res.End {
	case projectpath.EntryLimit:
		found.End, found.Limit = kittui.FileSearchEntryLimit, projectpath.DefaultMaxEntries
	case projectpath.TimeLimit:
		found.End, found.Limit = kittui.FileSearchTimeLimit, int(projectpath.DefaultTimeout.Seconds())
	case projectpath.MatchLimit:
		found.End, found.Limit = kittui.FileSearchMatchLimit, projectpath.DefaultMaxMatches
	case projectpath.Canceled:
		found.End = kittui.FileSearchCanceled
	}
	b.SetMatches(msg.search, found)
}
