package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/host/hostchange"
)

// fileView is one place of the file browser: the folder root, which the browser never leaves, the browser with its
// preview, the edit and the change under way there, and the rules of the place. readPath and hooks reach the file
// system and the editor; tests replace them.
type fileView struct {
	root     string
	rules    fileRules
	browser  *kittui.FileBrowser
	started  bool
	preview  *filePreview
	text     *kittui.TextView
	fileGen  int
	edit     *fileEdit
	hooks    editHooks
	chg      fileChanges
	readPath func(path string) (hostchange.PathContent, error)
}

// fileRules are what a place decides itself; a field left empty keeps what every place does.
type fileRules struct {
	// rootLabel is the text of the path line for the root; empty takes the one of the kit.
	rootLabel string
	// readOnly is the text, with the path as its argument, that says why nothing at path is changed here; empty
	// where it may be changed.
	readOnly func(path string) kittui.TextID
	// rights opens the permissions of path; without it the browser offers no Rights.
	rights func(path string) tea.Cmd
	// covered reports a page of the place drawn over the browser, which a reread after a change leaves alone.
	covered func() bool
	// spans colour the text of path in the preview and may mask parts of it; without it the text is coloured by
	// the format its name names.
	spans func(path, text string) [][]kittui.TextSpan
	// beforeWrite may add to the question of a change of paths, as a warning that does not refuse it.
	beforeWrite func(a *hostAsk, paths ...string)
	// afterWrite runs after a change of paths was applied, as to say what follows from it.
	afterWrite func(paths ...string) tea.Cmd
}

// fileViews are the places of the file browser; at most one of them has an edit under way.
func (m *Model) fileViews() []*fileView { return []*fileView{m.hostFiles(), &m.sf.view} }

// path is the absolute path of the parts below the root.
func (v *fileView) path(parts ...string) string {
	if len(parts) == 0 {
		return v.root
	}
	return strings.TrimSuffix(v.root, "/") + "/" + strings.Join(parts, "/")
}

// readOnlyText is the text of readOnly for path, empty where the place may change it.
func (v *fileView) readOnlyText(path string) kittui.TextID {
	if v.rules.readOnly == nil {
		return ""
	}
	return v.rules.readOnly(path)
}

// beforeWrite lets the place add to the question of a change of paths.
func (v *fileView) beforeWrite(a *hostAsk, paths ...string) {
	if v.rules.beforeWrite != nil {
		v.rules.beforeWrite(a, paths...)
	}
}

// wrote is what the place runs after a change of paths.
func (v *fileView) wrote(paths ...string) tea.Cmd {
	if v.rules.afterWrite == nil {
		return nil
	}
	return v.rules.afterWrite(paths...)
}

// activeEdit is the edit under way in any place, nil if none.
func (m *Model) activeEdit() *fileEdit {
	for _, v := range m.fileViews() {
		if v.edit != nil {
			return v.edit
		}
	}
	return nil
}

// formFiles is the place the open file form f belongs to, nil if none.
func (m *Model) formFiles(f *settingsForm) *fileView {
	for _, v := range m.fileViews() {
		if v.chg.form == f {
			return v
		}
	}
	return nil
}
