package tui

import (
	tea "charm.land/bubbletea/v2"

	kittui "github.com/castrowithcee/qatlas-kit/tui"
)

// submitHostForm submits the forms of System: users, groups, folders and the password of a proposal.
func (m *Model) submitHostForm(f *settingsForm, v map[kittui.FieldID]kittui.FormValue) tea.Cmd {
	switch f.kind {
	case formUserKeyAdd:
		return m.submitUserKeyAdd(f, v)
	case formUserKeyRemove:
		return m.submitUserKeyRemove(f, v)
	case formUserDelete:
		return m.submitUserDelete(f, v)
	case formGroupCreate:
		return m.submitGroupCreate(f, v)
	case formGroupMembers:
		return m.submitGroupMembers(f, v)
	case formGroupRename:
		return m.submitGroupRename(f, v)
	case formHostFolder:
		return m.submitFolderChange(f, v)
	case formFileNew, formFileRename, formFileCopy:
		return m.submitFileForm(f, v)
	case formProposalPassword:
		return m.submitProposalPassword(f, v)
	}
	return nil
}
