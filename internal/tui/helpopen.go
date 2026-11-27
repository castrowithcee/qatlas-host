package tui

import (
	tea "charm.land/bubbletea/v2"

	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/tuidocs"
)

func (m *Model) openHelp() {
	help, err := kittui.NewHelp(kittui.HelpSpec{Pages: tuidocs.FS, Lang: m.lang, Screen: m.helpScreen(),
		Global: []kittui.ActionID{actHelp, actArea, actReload, actBack, actQuit}, Keys: helpKeys(), CloseOutside: true}, m.keys, m.text)
	if err != nil {
		m.failure = m.text.Lookup(kittui.TextError, err.Error())
		return
	}
	help.OpenScreen(m.helpScreens()...)
	m.help, m.helpOpen = help, true
}

// helpKey routes the keys while the help is open and opens it on f9. f9 comes before the dialog and the form,
// which do not handle it, so the help opens over them and closing it returns to them with their input.
func (m *Model) helpKey(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	if m.helpOpen {
		if act, handled := m.help.Update(msg); handled {
			if act != "" {
				m.helpOpen = false
			}
			return nil, true
		}
		if id, ok := m.keys.Match(msg, kittui.InputNavigation); ok && (id == actQuit || id == actForceQuit) {
			return tea.Quit, true
		}
		return nil, true
	}
	if id, ok := m.keys.Match(msg, kittui.InputNavigation); ok && id == actHelpF9 {
		m.openHelp()
		return nil, true
	}
	return nil, false
}

// helpScreen are the keys the help lists for the screen it was opened from.
func (m *Model) helpScreen() []kittui.Hint {
	hints := m.footer()
	if m.focus() != kittui.ZoneTabs && m.zones()[0] != kittui.ZoneForm {
		hints = append(hints, kittui.Hint{Action: actNext, Label: textKeyZone, Priority: 3})
	}
	if m.stackRowsShown() && len(m.rows) > 0 {
		hints = append(hints, m.moreActionHints()...)
	}
	return hints
}
