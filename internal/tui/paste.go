package tui

import tea "charm.land/bubbletea/v2"

// paste hands pasted text to the open dialog, or to the field of the open settings form.
func (m *Model) paste(msg tea.PasteMsg) {
	if m.modal != nil {
		if m.modal.dialog != nil {
			m.modal.dialog.Paste(msg.Content)
		}
	} else if m.set.form != nil {
		if target := m.set.form.pasteTo(); target != nil {
			target.Paste(msg.Content)
		}
	}
}
