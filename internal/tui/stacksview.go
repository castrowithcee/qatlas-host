package tui

import (
	"strings"

	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// stacksView draws the Stacks area: the list with a row for every stack folder and, below it, the detail of
// the marked folder, which a click opens on its changes where it has any, or the file browser over a stack folder, or
// the changes of one. Without a folder it points to adding one.
func (m *Model) stacksView(width, height int) string {
	if m.set.keyOpen {
		return m.keyView(width, height)
	}
	if c := m.shownChanges(); c != nil {
		return m.stackChangesView(c, width, height)
	}
	if v := m.shownStackFiles(); v != nil {
		return m.stackFilesView(v, width, height)
	}
	if len(m.snap.status.Engine.Sources) == 0 && len(m.rows) == 0 {
		return m.stacksEmpty(width, height)
	}
	var detail []string
	if f, ok := m.selectedFolder(); ok {
		// The list keeps at least its header, the marked row and a few more.
		if room := height - minListRows; room >= 3 {
			detail = m.folderDetail(f, width, room)
		}
		if m.folderChanged(f) && len(detail) > 0 {
			m.changesTarget(height-len(detail), width, len(detail))
		}
	}
	var view string
	if len(m.secs) > 0 {
		view = m.stackSectionsView(width, height-len(detail))
	} else {
		view = m.listView(m.list, width, height-len(detail), 0)
	}
	return strings.Join(append([]string{view}, detail...), "\n")
}

// minListRows is what the list keeps of the workspace when the detail of a folder is long.
const minListRows = 5

// stacksEmpty says that there is no stack folder and how to add one; a click on it is the key.
func (m *Model) stacksEmpty(width, height int) string {
	lines := kittui.Wrap(kittui.Escape(m.text.Lookup(textStacksEmpty)), max(width-2, 1))
	lines = lines[:min(len(lines), max(height, 1))]
	for i, l := range lines {
		lines[i] = "  " + m.styles.Render(kittui.StyleHint, l)
	}
	m.parts = append(m.parts, mousePart{rect: kittui.Rect{Width: width, Height: len(lines)},
		targets: []kittui.Target{{Rect: kittui.Rect{Width: width, Height: len(lines)}, Zone: kittui.ZoneRows, Action: actAdd}}})
	return strings.Join(lines, "\n")
}

// keyView draws the deploy key of the folder the key view was opened for.
func (m *Model) keyView(width, height int) string {
	return strings.Join(m.framed(m.text.Lookup(textTitleKey), "", width, height, m.keyLines), "\n")
}

// stacksFooter is the key line of the Stacks area: the keys of the marked row, and adding a folder in every case.
func (m *Model) stacksFooter(common, nav []kittui.Hint) []kittui.Hint {
	if c := m.shownChanges(); c != nil {
		return append(m.stackChangesFooter(c), common...)
	}
	if v := m.shownStackFiles(); v != nil {
		return append(append(m.filesFooter(v), m.changesHint(m.sf.folder)...), common...)
	}
	add := kittui.Hint{Action: actAdd, Label: textKeyAdd, Priority: 3}
	f, onFolder := m.selectedFolder()
	switch {
	case onFolder:
		var own []kittui.Hint
		// Fetch and the deploy key exist only for a folder with a remote.
		if f.Kind == control.SourceKindRemote {
			own = append(own, kittui.Hint{Action: actFetch, Label: textKeyFetch, Priority: 3},
				kittui.Hint{Action: actEdit, Label: textKeyDeployKey, Priority: 3})
		}
		if f.Kind != control.SourceKindRemote {
			own = append(own, kittui.Hint{Action: actRemote, Label: textKeyRemote, Priority: 3})
		}
		own = append(own, m.changesHint(f)...)
		own = append(own, kittui.Hint{Action: actFiles, Label: textKeyFiles, Priority: 3},
			kittui.Hint{Action: actRemove, Label: textKeyRelease, Priority: 3}, add)
		common = append(own, common...)
	case len(m.rows) > 0:
		common = append(append(m.actionHints(), add), common...)
	default:
		common = append([]kittui.Hint{add}, common...)
	}
	if m.setupOffered() {
		common = append(common, kittui.Hint{Action: actSetupDeploy, Label: textKeySetupDeploy, Priority: 3})
	}
	return m.listFooter(m.list, len(m.rows)+len(m.set.folderData), textKeyDetails, common, nav)
}

// note is a dimmed sentence wrapped into width cells and indented like the rows of a list.
func (m *Model) note(text string, width int) string {
	lines := kittui.Wrap(kittui.Escape(text), max(width-2, 1))
	for i, l := range lines {
		lines[i] = "  " + m.styles.Render(kittui.StyleHint, l)
	}
	return strings.Join(lines, "\n")
}
