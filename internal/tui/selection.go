package tui

import (
	tea "charm.land/bubbletea/v2"

	kittui "github.com/castrowithcee/qatlas-kit/tui"
)

// selectMouse reads a mouse event for the text selection first. A press on selectable text is held back, so a
// drag never acts as a click; the release without motion hands the click on, as the press would have. The
// copied text goes to the clipboard through the terminal.
func (m *Model) selectMouse(msg tea.MouseMsg) tea.Cmd {
	if text, copied := m.sel.Mouse(m.mouseMap, msg); copied {
		m.result = m.text.Lookup(kittui.TextCopyHandedOver)
		return tea.SetClipboard(text)
	}
	if _, ok := msg.(tea.MouseClickMsg); ok && m.sel.Holds() {
		return nil
	}
	if click, ok := m.sel.Replay(); ok {
		msg = click
	}
	return m.navMouse(msg)
}
