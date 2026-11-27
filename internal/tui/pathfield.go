package tui

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"github.com/castrowithcee/qatlas-kit/projectpath"
	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// pathState is what a form with path fields keeps: the priority entries of its folder field and the way to
// stop the search the last request started.
type pathState struct {
	prio   []string
	cancel context.CancelFunc
}

// Update takes one message and adds the answer to the request a path field made on the way, so typing never
// waits for the file system.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	_, cmd := m.dispatch(msg)
	return m, tea.Batch(cmd, m.pathCmd())
}

func (m *Model) folderForm() *settingsForm {
	prio := m.folderPriority()
	f := m.newSettingsForm(formFolder, textTitleFolderAdd, []kittui.FormField{
		pathField("path", textFieldFolderPath, textHintFolderPath, kittui.PathDirs, prio),
	})
	if f != nil {
		for _, p := range prio {
			f.prio = append(f.prio, p.Text)
		}
	}
	return f
}

// pathMsg is the answer to a request of a path field, tagged with the form that asked so that the answer
// to a form that was closed or rebuilt is dropped.
type pathMsg struct {
	form *kittui.Form
	res  kittui.PathResult
}

func (p pathMsg) applyTo(m *Model) {
	if f := m.set.form; f != nil && f.form == p.form {
		f.form.SetPath(p.res)
	}
}

// pathCmd starts the answer to the request the last input left in the open form, if any. A new request
// cancels the search the last one started; its answer is stale by then and the form drops it.
func (m *Model) pathCmd() tea.Cmd {
	f := m.set.form
	if f == nil {
		return nil
	}
	load, ok := f.form.TakePathLoad()
	if !ok {
		return nil
	}
	if f.cancel != nil {
		f.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	f.cancel = cancel
	form := f.form
	return func() tea.Msg { return pathMsg{form: form, res: answerPath(ctx, load)} }
}

// answerPath answers a request with the rights of the person who runs the interface: the entries of a level,
// or the matches of a search. Nothing here asks for more rights, so what the person may not read is not shown.
// A Suggester is not safe for concurrent use, so every request gets its own.
func answerPath(ctx context.Context, load kittui.PathLoad) kittui.PathResult {
	res := kittui.PathResult{Token: load.Token}
	if load.Search {
		return searchPath(ctx, load, res)
	}
	mode := projectpath.Directories
	if load.Kind == kittui.PathFiles {
		mode = projectpath.Files
	}
	wd, _ := os.Getwd()
	for _, s := range projectpath.NewSuggester(mode, "", wd).Suggest(load.Typed) {
		res.Entries = append(res.Entries, kittui.Suggestion{Text: s, Continues: s != "" && os.IsPathSeparator(s[len(s)-1])})
	}
	return res
}

func searchPath(ctx context.Context, load kittui.PathLoad, res kittui.PathResult) kittui.PathResult {
	start, text, ok := projectpath.SplitSearch(load.Typed)
	if !ok {
		return res
	}
	opt := projectpath.SearchOptions{Kind: projectpath.OnlyDirectories}
	if load.Kind == kittui.PathFiles {
		opt.Kind = projectpath.Both
	}
	found, err := projectpath.Search(ctx, start, text, opt)
	if err != nil {
		return res
	}
	for _, match := range found.Matches {
		res.Entries = append(res.Entries, kittui.Suggestion{Text: match.Path, Continues: match.Dir})
	}
	switch found.End {
	case projectpath.EntryLimit:
		res.End, res.Limit = kittui.PathEndEntries, projectpath.DefaultMaxEntries
	case projectpath.TimeLimit:
		res.End, res.Limit = kittui.PathEndTime, int(projectpath.DefaultTimeout/time.Second)
	case projectpath.MatchLimit:
		res.End, res.Limit = kittui.PathEndMatches, projectpath.DefaultMaxMatches
	case projectpath.Canceled:
		res.End = kittui.PathEndCanceled
	}
	return res
}

// folderPriority are the working folders of the foreign projects that can be taken over, without the folders
// already bound; they stand first in the folder field. Nothing is read from the file system here.
func (m *Model) folderPriority() []kittui.Suggestion {
	bound := map[string]bool{}
	for _, s := range m.snap.status.Engine.Sources {
		bound[filepath.Clean(s.Path)] = true
	}
	var out []kittui.Suggestion
	for _, g := range m.ct.groups {
		if g.Kind != control.ContainerKindForeign || g.ReadOnly || g.WorkingDir == "" {
			continue
		}
		dir := filepath.Clean(g.WorkingDir)
		// The kit refuses a form whose priority entry holds a control character; the name is foreign text.
		if !filepath.IsAbs(dir) || bound[dir] || strings.IndexFunc(dir, unicode.IsControl) >= 0 {
			continue
		}
		bound[dir] = true
		out = append(out, kittui.Suggestion{Text: dir + string(filepath.Separator), Continues: true})
	}
	return out
}

// openFolderForm shows the form of a folder to add and, where the containers were not read yet, reads them once:
// the answer rebuilds the form with the folders of foreign projects on top.
func (m *Model) openFolderForm() tea.Cmd {
	m.openForm(m.folderForm())
	if m.ct.loaded {
		return nil
	}
	return m.readContainers()
}

// foldPriority puts the folders of foreign projects into the open folder form once the containers are read. The
// typed text stays; a form whose priority entries did not change is left alone, as the list is read again
// while the Containers area is shown.
func (m *Model) foldPriority() {
	f := m.set.form
	if f == nil || f.kind != formFolder {
		return
	}
	prio := m.folderPriority()
	texts := make([]string, len(prio))
	for i, p := range prio {
		texts[i] = p.Text
	}
	if slices.Equal(texts, f.prio) {
		return
	}
	next := m.folderForm()
	if next == nil {
		return
	}
	if err := next.form.SetValue("path", f.form.Values()["path"]); err != nil {
		return
	}
	if f.cancel != nil {
		f.cancel()
	}
	m.set.form = next
}
