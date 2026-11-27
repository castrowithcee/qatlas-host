package tui

import (
	"strings"

	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// folderGroups sets the data of the stack folders and their group rows in the Stacks list: every bound folder
// has its row, with or without stacks, and it shows the kind, uncommitted changes and the last poll or why that failed.
func (m *Model) folderGroups() {
	m.set.folderData = map[stackRef]control.SourceStatus{}
	m.fillFolderGroups(m.list, m.snap.status.Engine.Sources, "")
}

// fillFolderGroups sets the group rows of list for the stack folders src, which are the folders of one engine
// when the interface shows several. engine is the engine part of their keys in folderData: empty when the
// interface shows one engine.
func (m *Model) fillFolderGroups(list *kittui.List, src []control.SourceStatus, engine string) {
	s, t := m.set, m.text
	groups := make([]kittui.ListGroup, 0, len(src))
	for _, f := range src {
		s.folderData[stackRef{engine, f.Path}] = f
		kind, polled := t.Lookup(textFolderKindLocal), t.Lookup(textNoValue)
		if f.Kind == control.SourceKindRemote {
			kind = t.Lookup(textFolderKindRemote)
		}
		switch {
		case f.GitUnsafe != "":
			polled = t.Lookup(textFolderGitUnsafe, strings.Join(strings.Fields(f.GitUnsafe), " "))
			m.tones[polled] = kittui.StyleFail
		case m.snap.changes[stackRef{f.Engine, f.Path}] > 0:
			polled = t.Lookup(textFolderUncommitted)
			m.tones[polled] = kittui.StyleWarn
		case f.PollError != "":
			polled = t.Lookup(textFolderPollFailed, strings.Join(strings.Fields(f.PollError), " "))
			m.tones[polled] = kittui.StyleFail
		case !f.LastPoll.IsZero():
			polled = t.Lookup(textFolderPolled, f.LastPoll.In(m.loc).Format(polledLayout))
		}
		cells := make([]string, colGroupPolled+1)
		cells[colGroupKind], cells[colGroupPolled] = kind, polled
		groups = append(groups, kittui.ListGroup{Key: f.Path, Title: f.Path, Cells: cells, Keep: true})
	}
	if err := list.SetGroups(groups); err != nil {
		m.failure = t.Lookup(kittui.TextError, err.Error())
	}
}

// selectedFolder is the stack folder whose group row is marked.
func (m *Model) selectedFolder() (control.SourceStatus, bool) {
	key, ok := m.list.SelectedGroup()
	f, found := m.set.folderData[stackRef{m.listEngine, key}]
	return f, ok && found
}

// folderDetail is the address, the last poll, uncommitted changes and why the poll failed for one folder, in a panel
// of at most rows rows.
func (m *Model) folderDetail(f control.SourceStatus, width, rows int) []string {
	t := m.text
	return m.framed(f.Path, "", width, rows, func(width int) []string {
		var lines []string
		if f.URL != "" {
			lines = append(lines, m.line(detailIndent, kittui.StyleText, t.Lookup(textDetailURL, f.URL), width))
		}
		if !f.LastPoll.IsZero() {
			lines = append(lines, m.line(detailIndent, kittui.StyleText, t.Lookup(textDetailLastPoll, m.stamp(f.LastPoll.In(m.loc))), width))
		}
		if f.GitUnsafe != "" {
			lines = append(lines, m.wrapped(detailIndent, kittui.StyleFail, t.Lookup(textDetailGitUnsafe, f.GitUnsafe), width)...)
			lines = append(lines, m.wrapped(detailIndent, kittui.StyleHint, t.Lookup(textDetailGitSecure), width)...)
		}
		if m.snap.changes[stackRef{f.Engine, f.Path}] > 0 {
			lines = append(lines, m.wrapped(detailIndent, kittui.StyleWarn, t.Lookup(textDetailUncommitted), width)...)
		}
		if f.PollError != "" {
			lines = append(lines, m.wrapped(detailIndent, kittui.StyleFail, t.Lookup(textDetailPollError, f.PollError), width)...)
		}
		if f.Note != "" {
			lines = append(lines, m.wrapped(detailIndent, kittui.StyleHint, t.Lookup(textDetailNote, f.Note), width)...)
		}
		return lines
	})
}
