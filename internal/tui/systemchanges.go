package tui

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/host/hostchange"
)

// System > Changes lists the latest changes of the host from the journal, which only root can read. It is read
// with `host journal` through the same hand-over to sudo as a change: started as root the page reads it on its own,
// otherwise only on r, so that no password prompt appears unasked. enter on an entry asks to undo it with the same
// question as a change: a sentence, the exact commands of the undo dimmed below it, and y or n. What cannot be
// undone says so instead of asking.

// journalEntry is one entry as `host journal` lists it. UndoSentence and UndoCommands describe its undo; both are
// empty where no undo can be offered.
type journalEntry struct {
	ID           string   `json:"id"`
	Time         string   `json:"time"`
	Action       string   `json:"action"`
	Sentence     string   `json:"sentence"`
	Result       string   `json:"result"`
	Reversible   bool     `json:"reversible"`
	SudoUID      int      `json:"sudo_uid"`
	UndoSentence string   `json:"undo_sentence"`
	UndoCommands []string `json:"undo_commands"`
}

// undoable reports whether the entry offers an undo.
func (e journalEntry) undoable() bool { return e.Reversible && e.UndoSentence != "" }

// journalPage is what the changes page holds. read says the journal was read once; stale that the host was
// changed from here since.
type journalPage struct {
	list    *kittui.List
	entries []journalEntry
	byID    map[kittui.ItemID]journalEntry
	read    bool
	reading bool
	stale   bool
}

func newJournalList(keys *kittui.Keymap, text kittui.Translator) (*kittui.List, error) {
	failed, restored := text.Lookup(textJrnFailed), text.Lookup(textJrnRestored)
	tone := func(cell string) kittui.TextStyle {
		if cell == failed || cell == restored {
			return kittui.StyleWarn
		}
		return kittui.StyleText
	}
	l, err := kittui.NewList(kittui.ListSpec{Columns: []kittui.Column{
		{Title: textColTime}, {Title: textColBy, Optional: true}, {Title: textColChange, Flex: true},
		{Title: textColResult, Tone: tone}, {Title: textColUndo}},
		Keys: listKeys(), Search: kittui.SearchByKey, Empty: textJournalEmpty}, keys, text)
	if err != nil {
		return nil, err
	}
	l.SetFocused(true)
	return l, nil
}

// readJournal reads the journal as root; it is the only way the list fills.
func (m *Model) readJournal() tea.Cmd {
	j := &m.sys.jrnl
	if j.reading {
		return nil
	}
	j.reading = true
	return m.runHost(HostRequest{Args: []string{"host", "journal", "--output", "json"}}, func(out []byte, err error) tea.Cmd {
		j.reading = false
		var list []journalEntry
		if err != nil || json.Unmarshal(bytes.TrimSpace(out), &list) != nil {
			m.hostFailed(out, err, false)
			return nil
		}
		j.entries, j.read, j.stale = list, true, false
		m.fillJournal()
		return nil
	})
}

// fillJournal sets the rows of the list from what was read last.
func (m *Model) fillJournal() {
	j, t := &m.sys.jrnl, m.text
	clear(j.byID)
	items := make([]kittui.ListItem, 0, len(j.entries))
	for _, e := range j.entries {
		id := kittui.ItemID(e.ID)
		j.byID[id] = e
		undo := t.Lookup(textNoValue)
		if e.undoable() {
			undo = t.Lookup(textUndoPossible)
		}
		items = append(items, kittui.ListItem{ID: id, Cells: []string{m.journalTime(e.Time), m.uidName(e.SudoUID), e.Sentence,
			m.journalResult(e.Result), undo}, SearchText: e.Action})
	}
	if err := j.list.SetItems(items); err != nil {
		m.failure = t.Lookup(kittui.TextError, err.Error())
	}
}

// journalTime is the time of an entry in the time zone of the interface.
func (m *Model) journalTime(rfc3339 string) string {
	at, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil {
		return rfc3339
	}
	return at.In(m.loc).Format("2006-01-02 15:04")
}

// journalResult is the result of an entry in words.
func (m *Model) journalResult(result string) string {
	switch result {
	case hostchange.ResultApplied:
		return m.text.Lookup(textJrnApplied)
	case hostchange.ResultFailed:
		return m.text.Lookup(textJrnFailed)
	case hostchange.ResultRestored:
		return m.text.Lookup(textJrnRestored)
	case hostchange.ResultUndone:
		return m.text.Lookup(textJrnUndone)
	}
	return result
}

// uidName is the name of the user with the UID as read last, or the number.
func (m *Model) uidName(uid int) string {
	for _, u := range m.sys.users {
		if u.UID == uid {
			return u.Name
		}
	}
	if uid == 0 {
		return "root"
	}
	return strconv.Itoa(uid)
}

// selectedJournal is the entry under the cursor.
func (m *Model) selectedJournal() (journalEntry, bool) {
	j := &m.sys.jrnl
	id, ok := j.list.Selected()
	e, found := j.byID[id]
	return e, ok && found
}

// noUndoReason says why an entry offers no undo.
func (m *Model) noUndoReason(e journalEntry) string {
	switch e.Result {
	case hostchange.ResultUndone:
		return m.text.Lookup(textNoUndoUndone)
	case hostchange.ResultFailed, hostchange.ResultRestored:
		return m.text.Lookup(textNoUndoFailed)
	}
	return m.text.Lookup(textNoUndoFinal)
}

// askUndo asks to undo the marked entry. An undo cannot itself be undone, so the question says so and its yes is
// red, as for every change that cannot be undone.
func (m *Model) askUndo() tea.Cmd {
	e, ok := m.selectedJournal()
	if !ok {
		return nil
	}
	t := m.text
	if m.sys.busy {
		m.warn(t.Lookup(textActionBusy))
		return nil
	}
	if !e.undoable() {
		m.hostNotice(t.Lookup(textNoUndoTitle, e.Sentence), []kittui.DialogLine{{Text: m.noUndoReason(e), Style: kittui.StyleWarn}})
		return nil
	}
	question := t.Lookup(textAskUndo, e.Sentence)
	a := hostAsk{question: question, undo: true}
	details := []kittui.DialogLine{{Text: e.UndoSentence, Style: kittui.StyleText}}
	m.askChange(question, t.Lookup(textUndoNote), true, true, details, e.UndoCommands,
		func() tea.Cmd { return m.runUndo(e.ID, a) }, nil)
	return nil
}

// runUndo runs host undo and shows what it reported. The entry is marked undone at once, so it is not offered
// twice before the journal is read again.
func (m *Model) runUndo(id string, a hostAsk) tea.Cmd {
	return m.runHost(HostRequest{Args: []string{"host", "undo", "--confirm", "--output", "json", "--", id}}, func(out []byte, err error) tea.Cmd {
		report, ok := parseReport(out)
		if !ok {
			m.hostFailed(out, err, true)
			return tea.Batch(m.loadSystem(true), m.afterHostChange())
		}
		m.hostResult(report, a)
		if report.Result == hostchange.ResultRefused {
			return nil
		}
		if report.Result == hostchange.ResultApplied {
			m.markUndone(id)
		}
		return tea.Batch(m.loadSystem(true), m.afterHostChange())
	})
}

// markUndone takes the undone entry out of the offers.
func (m *Model) markUndone(id string) {
	j := &m.sys.jrnl
	for i := range j.entries {
		if j.entries[i].ID == id {
			j.entries[i].Reversible, j.entries[i].Result = false, hostchange.ResultUndone
			j.entries[i].UndoSentence, j.entries[i].UndoCommands = "", nil
		}
	}
	m.fillJournal()
}

// afterHostChange marks the journal and the action log as older than the host. A page that reads without a
// password reads them again at once.
func (m *Model) afterHostChange() tea.Cmd {
	s := m.sys
	s.jrnl.stale, s.hlog.stale, s.prop.stale = true, true, true
	if m.area != areaSystem {
		return nil
	}
	return m.showSystemPage()
}

// changesView draws the changes page in width by height cells: a note above the list where the journal is not read
// or older than the host, the list, and the undo of the marked entry below it.
func (m *Model) changesView(width, height int) string {
	j, t := &m.sys.jrnl, m.text
	if !j.read {
		text := t.Lookup(textJournalHint)
		if j.reading {
			text = t.Lookup(textJournalReading)
		}
		lines := m.wrapped(detailMargin, kittui.StyleHint, text, width)
		return strings.Join(lines[:min(len(lines), height)], "\n")
	}
	var head []string
	if j.stale && m.euid() != 0 {
		head = append(m.wrapped(detailMargin, kittui.StyleWarn, t.Lookup(textJournalStale), width), "")
		head = head[:min(len(head), max(height-minListRows, 0))]
	}
	var detail []string
	if e, ok := m.selectedJournal(); ok {
		if room := height - len(head) - minListRows; room >= 3 {
			detail = m.framed(e.ID, "", width, room, func(w int) []string { return m.journalDetailLines(e, w) })
		}
	}
	view := m.listView(j.list, width, max(height-len(head)-len(detail), 1), len(head))
	return strings.Join(append(append(head, view), detail...), "\n")
}

// journalDetailLines describe the undo of the marked entry: its sentence and its commands dimmed, or why there is none.
func (m *Model) journalDetailLines(e journalEntry, width int) []string {
	if !e.undoable() {
		return m.wrapped(detailIndent, kittui.StyleHint, m.noUndoReason(e), width)
	}
	lines := m.wrapped(detailIndent, kittui.StyleText, m.text.Lookup(textJournalUndo, e.UndoSentence), width)
	for _, c := range e.UndoCommands {
		lines = append(lines, m.line(detailIndent, kittui.StyleHint, "$ "+c, width))
	}
	return lines
}

// changesFooter are the own keys of the changes page.
func (m *Model) changesFooter(own, common, nav []kittui.Hint) []kittui.Hint {
	j := &m.sys.jrnl
	if !j.read {
		return append(own, common...)
	}
	if e, ok := m.selectedJournal(); ok && !e.undoable() {
		// enter explains why there is no undo; the key line offers none.
		if j.list.Searching() {
			return m.listFooter(j.list, len(j.entries), textKeyUndo, append(own, common...), nav)
		}
		return append(append(append(own, kittui.Hint{Action: actSearch, Label: kittui.TextKeySearch, Priority: 1}), common...), nav...)
	}
	return m.listFooter(j.list, len(j.entries), textKeyUndo, append(own, common...), nav)
}
