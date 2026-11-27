package tui

import (
	kittui "github.com/castrowithcee/qatlas-kit/tui"
)

// The help screen IDs name a screen for the pages of tuidocs, the most specific first: a dialog or form
// "title:<TextID>", the confirmation of a stack action "action:<action ID>", a state "<area>[/<tab>]/<state>",
// a tab "<area>/<tab>" and an area "<area>".

// titleScreen is the screen ID of a dialog or form under the title text id.
func titleScreen(id kittui.TextID) string { return "title:" + string(id) }

// screen is the screen ID of the confirmation, preview and result of a stack or container action.
func (k actionKind) screen() string {
	for id, kind := range actionKinds {
		if kind == k {
			return "action:" + string(id)
		}
	}
	return ""
}

// helpScreens are the screen IDs of what the interface shows, the most specific first. It mirrors the branches
// of footer, so the help opened from a screen shows the page of the keys the footer names.
func (m *Model) helpScreens() []string {
	var ids []string
	if m.modal != nil && m.modal.screen != "" {
		ids = append(ids, m.modal.screen)
	}
	// The General form is the page of its tab, not a form of its own.
	if m.formShown() && m.set.form.kind != formGeneral {
		ids = append(ids, titleScreen(m.set.form.title))
	}
	area, base := string(m.area), string(m.area)
	if m.area == areaSettings || m.area == areaSystem {
		base += "/" + string(m.activeTab())
	}
	if state := m.helpState(); state != "" {
		ids = append(ids, base+"/"+state)
	}
	if base != area {
		ids = append(ids, base)
	}
	return append(ids, area)
}

// helpState is the state of the area or tab that covers its list or page; empty where there is none.
func (m *Model) helpState() string {
	switch {
	case m.setupShown() && !m.clogShown():
		return "setup"
	case m.clogShown():
		return "clog"
	}
	switch {
	case m.area == areaStacks:
		return m.stacksState()
	case m.area == areaSystem:
		return m.systemState()
	case m.area == areaDetail && m.pick != nil:
		return "pick"
	case m.area == areaContainers && m.ctv.open:
		return "detail"
	case m.area == areaProposals && m.pv.open:
		return "preview"
	case m.area == areaRuns && m.log.open:
		return "log"
	}
	return ""
}

func (m *Model) stacksState() string {
	switch {
	case m.set.keyOpen:
		return "key"
	case m.shownChanges() != nil:
		return "changes"
	case m.shownStackFiles() != nil:
		return filesState(m.shownStackFiles())
	}
	return ""
}

// filesState is "preview" while a file is open and empty while the browser lists a folder.
func filesState(v *fileView) string {
	if v.preview != nil {
		return "preview"
	}
	return ""
}

func (m *Model) systemState() string {
	s := m.sys
	switch {
	case s.changes != nil:
		return "pick"
	case s.page == pageFolders && s.loaded && s.fold.asked != "":
		return "rights"
	case s.page == pageFolders && s.loaded:
		return filesState(m.hostFiles())
	}
	return ""
}
