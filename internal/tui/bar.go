package tui

import (
	"slices"

	tea "charm.land/bubbletea/v2"

	kittui "github.com/castrowithcee/qatlas-kit/tui"
)

// barActions are the actions of the shown screen for the action bar of the frame. An area whose screen has
// actions of its own adds them here; the footer leaves out the hints of the actions that stand in the bar.
func (m *Model) barActions() []kittui.BarAction {
	if b := m.shownBrowser(); b != nil {
		return b.BarActions()
	}
	return nil
}

// shownFiles is the place whose browser the workspace draws, nil while another page covers it.
func (m *Model) shownFiles() *fileView {
	var v *fileView
	switch {
	case m.modal != nil && m.modal.sheet(), m.clogShown(), m.formShown():
		return nil
	case m.area == areaSystem:
		s := m.sys
		if !s.loaded || s.changes != nil || s.page != pageFolders || s.fold.asked != "" {
			return nil
		}
		v = m.hostFiles()
	case m.area == areaStacks && m.loaded && !m.setupShown():
		v = m.shownStackFiles()
	}
	if v == nil || v.preview != nil {
		return nil
	}
	return v
}

// shownBrowser is the file browser the workspace draws, nil where there is none.
func (m *Model) shownBrowser() *kittui.FileBrowser {
	if v := m.shownFiles(); v != nil {
		return v.browser
	}
	return nil
}

// actionClick is a click on a button or hint that stands for an action: the shown file browser runs its own
// actions, any other takes the way of the key.
func (m *Model) actionClick(id kittui.ActionID) tea.Cmd {
	if cmd, ok := m.barClick(id); ok {
		return cmd
	}
	if key, ok := m.keyOf(id); ok {
		return m.press(key)
	}
	return nil
}

// barClick runs a click on a button of the action bar through the file browser that offers the action, as its
// key does; false where no shown browser offers it.
func (m *Model) barClick(id kittui.ActionID) (tea.Cmd, bool) {
	v := m.shownFiles()
	if v == nil || v.browser == nil || !slices.ContainsFunc(v.browser.BarActions(), func(a kittui.BarAction) bool { return a.Action == id }) {
		return nil, false
	}
	r, ok := v.browser.Act(id)
	if !ok {
		return nil, false
	}
	m.setZone(kittui.ZoneRows)
	return m.fileResult(v, r), true
}
