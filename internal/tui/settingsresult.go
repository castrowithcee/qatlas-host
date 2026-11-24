package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// settingsFormFor opens the form the stack settings were read for.
func (m *Model) settingsFormFor(op settingOp, res control.StackSettingsResult) tea.Cmd {
	switch op {
	case opStackSettingsBackup:
		return m.backupFormFor(res)
	case opStackSettingsCompose:
		return m.composeFormFor(res)
	case opStackSettingsCheck:
		return m.checkFormFor(res)
	case opStackSettingsGrant:
		return m.grantFormFor(res)
	}
	return m.updateFormFor(res)
}
