package tui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// The view History of a stack folder in the Stacks area: the latest commits as source.log lists them, the files of
// one with their kind, and its diff. It covers the list of folders or the file browser it was opened from, and esc
// leads back one level at a time and from the list to there. Author, subject and paths are shown as the service
// masked them.

// actHistory opens the view; actShowCommit shows the diff of the marked commit, as a sign since every letter is
// the key of an action already.
const (
	actHistory    kittui.ActionID = "history"
	actShowCommit kittui.ActionID = "showcommit"
)

func historyKeys() []kittui.Action {
	return []kittui.Action{act(actHistory, kittui.LevelLetter, textKeyHistory, "@"),
		act(actShowCommit, kittui.LevelLetter, textKeyDiff, "<")}
}

const historyDateLayout = "2006-01-02 15:04"

// stackHistory is the view while open is set: the folder, the commits as read last, the list of the files of one
// commit while files is set, and the diff shown, nil otherwise. gen and diffGen separate a read from an older one.
type stackHistory struct {
	open    bool
	folder  control.SourceStatus
	list    *kittui.List
	read    part
	res     control.SourceLogResult
	gen     int
	files   *kittui.List
	commit  control.SourceLogCommit
	diff    *changeDiff
	diffGen int
}

// shownHistory is the view while the Stacks area shows it, nil otherwise.
func (m *Model) shownHistory() *stackHistory {
	if m.area != areaStacks || !m.sf.history.open || m.set.keyOpen {
		return nil
	}
	return &m.sf.history
}

// openFolderHistory opens the view for the folder of the browser v, or of the marked row where v is nil. This is
// the entry of the view: the folder with its engine, and esc returns to the browser or the row it covers.
func (m *Model) openFolderHistory(v *fileView) (tea.Cmd, bool) {
	f, ok := m.sf.folder, v != nil
	if !ok {
		f, ok = m.selectedFolder()
	}
	if !ok {
		return nil, false
	}
	return m.openHistory(f), true
}

func (m *Model) openHistory(f control.SourceStatus) tea.Cmd {
	h := &m.sf.history
	if h.list == nil {
		l, err := newHistoryList([]kittui.Column{{Title: textColDate}, {Title: textColAuthor}, {Title: textColSubject, Flex: true}},
			textHistoryNone, m)
		if err != nil {
			m.failure = m.text.Lookup(kittui.TextError, err.Error())
			return nil
		}
		h.list = l
	}
	if h.folder.Path != f.Path || h.folder.Engine != f.Engine {
		h.res = control.SourceLogResult{}
		m.fillHistory()
	}
	h.open, h.folder, h.files, h.diff = true, f, nil, nil
	return m.readHistory()
}

func newHistoryList(cols []kittui.Column, empty kittui.TextID, m *Model) (*kittui.List, error) {
	l, err := kittui.NewList(kittui.ListSpec{Columns: cols, Keys: listKeys(), Search: kittui.SearchByKey, Empty: empty}, m.keys, m.text)
	if err == nil {
		l.SetFocused(true)
	}
	return l, err
}

// historyMsg carries the commits of the folder the view shows.
type historyMsg struct {
	gen int
	res control.SourceLogResult
	err error
}

func (msg historyMsg) applyTo(m *Model) {
	h := &m.sf.history
	if msg.gen != h.gen {
		return
	}
	h.read = part{loaded: msg.err == nil, err: msg.err}
	if msg.err == nil {
		h.res = msg.res
		m.fillHistory()
	}
}

// readHistory reads the commits of the folder of the view again.
func (m *Model) readHistory() tea.Cmd {
	h := &m.sf.history
	hb, ok := m.backend.(HistoryBackend)
	if !ok {
		return nil
	}
	h.gen++
	h.read.loading = true
	gen, f, ctx := h.gen, h.folder, m.engineCtx(h.folder.Engine)
	return func() tea.Msg {
		res, err := hb.SourceLog(ctx, f.Path, 0)
		return historyMsg{gen: gen, res: res, err: err}
	}
}

// fillHistory sets the rows of the commit list from what was read last.
func (m *Model) fillHistory() {
	h := &m.sf.history
	items := make([]kittui.ListItem, len(h.res.Commits))
	for i, c := range h.res.Commits {
		when := c.Date
		if d, err := time.Parse(time.RFC3339, c.Date); err == nil {
			when = d.In(m.loc).Format(historyDateLayout)
		}
		items[i] = kittui.ListItem{ID: kittui.ItemID(c.ID), Cells: []string{when, c.Author, c.Subject}}
	}
	if err := h.list.SetItems(items); err != nil {
		m.failure = m.text.Lookup(kittui.TextError, err.Error())
	}
}

// selectedCommit is the commit whose files or diff the view shows, else the one under the cursor of the list.
func (m *Model) selectedCommit() (control.SourceLogCommit, bool) {
	h := &m.sf.history
	if h.files != nil {
		return h.commit, true
	}
	id, ok := h.list.Selected()
	for _, c := range h.res.Commits {
		if ok && kittui.ItemID(c.ID) == id {
			return c, true
		}
	}
	return control.SourceLogCommit{}, false
}

// openCommitFiles lists the files of the marked commit with their kind.
func (m *Model) openCommitFiles() {
	c, ok := m.selectedCommit()
	if !ok {
		return
	}
	h, t := &m.sf.history, m.text
	l, err := newHistoryList([]kittui.Column{{Title: textColFile}, {Title: textColKind, Flex: true}}, textHistoryFilesNone, m)
	if err != nil {
		m.failure = t.Lookup(kittui.TextError, err.Error())
		return
	}
	items := make([]kittui.ListItem, len(c.Paths))
	for i, p := range c.Paths {
		kind := p.Kind
		switch p.Kind {
		case control.ChangeModified:
			kind = t.Lookup(textChangeModified)
		case control.ChangeNew:
			kind = t.Lookup(textChangeNew)
		case control.ChangeDeleted:
			kind = t.Lookup(textChangeDeleted)
		}
		items[i] = kittui.ListItem{ID: kittui.ItemID(p.Path), Cells: []string{p.Path, kind}}
	}
	if err := l.SetItems(items); err != nil {
		m.failure = t.Lookup(kittui.TextError, err.Error())
		return
	}
	h.files, h.commit = l, c
}

// historyInput is the input focus of the view.
func (m *Model) historyInput(h *stackHistory) kittui.InputFocus {
	switch {
	case h.diff != nil && h.diff.text != nil:
		return h.diff.text.Input()
	case h.diff != nil:
		return kittui.InputNavigation
	case h.files != nil:
		return h.files.Input()
	}
	return h.list.Input()
}

// historyTyped gives a key to the diff, the files or the commit list, where enter opens the files of the marked commit.
func (m *Model) historyTyped(h *stackHistory, msg tea.KeyPressMsg) (tea.Cmd, bool) {
	switch {
	case h.diff != nil:
		return nil, h.diff.text != nil && h.diff.text.Update(msg)
	case h.files != nil:
		_, handled := h.files.Update(msg)
		return nil, handled
	}
	open, handled := h.list.Update(msg)
	if open != "" {
		m.openCommitFiles()
		return nil, true
	}
	return nil, handled
}

// historyAction acts on the key of the diff, in the commit list and in the files.
func (m *Model) historyAction(h *stackHistory, id kittui.ActionID) (tea.Cmd, bool) {
	if id != actShowCommit || h.diff != nil {
		return nil, false
	}
	if _, ok := m.selectedCommit(); !ok {
		return nil, false
	}
	return m.askShow(), true
}

// historyBack is esc in the view: it clears a search, closes the diff, then the files, and leaves the view from the list.
func (m *Model) historyBack(h *stackHistory, msg tea.KeyPressMsg) {
	switch {
	case h.diff != nil:
		if h.diff.text == nil || !h.diff.text.Update(msg) {
			h.diff = nil
			h.diffGen++
		}
	case h.files != nil:
		if _, handled := h.files.Update(msg); !handled {
			h.files = nil
		}
	default:
		if _, handled := h.list.Update(msg); !handled {
			h.open = false
		}
	}
}

// historyHint is the key of the view on the row of a stack folder and in its browser.
var historyHint = kittui.Hint{Action: actHistory, Label: textKeyHistory, Priority: 3}

// stackHistoryFooter are the own keys of the view.
func (m *Model) stackHistoryFooter(h *stackHistory) []kittui.Hint {
	back := kittui.Hint{Action: actBack, Label: kittui.TextKeyBack, Priority: 1}
	show := kittui.Hint{Action: actShowCommit, Label: textKeyDiff, Priority: 2}
	switch {
	case h.diff != nil && h.diff.text != nil:
		return append(h.diff.text.Hints(), back)
	case h.diff != nil:
		return []kittui.Hint{back}
	case h.files != nil && h.files.Searching():
		return m.listFooter(h.files, 1, textKeyDiff, nil, nil)
	case h.files != nil:
		return []kittui.Hint{show, {Action: actSearch, Label: kittui.TextKeySearch, Priority: 1}, back}
	}
	return m.listFooter(h.list, len(h.res.Commits), textKeyFiles, []kittui.Hint{show, back}, nil)
}

// stackHistoryView draws the view in width by height cells: the diff, or a heading with the cut notes over the list
// of the files of a commit or of the commits.
func (m *Model) stackHistoryView(h *stackHistory, width, height int) string {
	if h.diff != nil {
		return m.diffView(h.diff, width, height)
	}
	t := m.text
	list, cut, cutText := h.list, h.res.Truncated, textHistoryCut
	head := []string{m.line(detailMargin, kittui.StyleActive, t.Lookup(textHistoryOf, h.folder.Path), width)}
	if h.files != nil {
		list, cut, cutText = h.files, h.commit.PathsTruncated, textHistoryFilesCut
		head = []string{m.line(detailMargin, kittui.StyleActive, t.Lookup(textHistoryFilesOf, short(h.commit.ID), h.commit.Subject), width)}
	} else if !h.read.loaded {
		head = append(head, m.partLines(h.read, width)...)
	}
	if cut {
		head = append(head, m.wrapped(detailMargin, kittui.StyleWarn, t.Lookup(cutText), width)...)
	}
	head = append(head, "")
	view := m.listView(list, width, max(height-len(head), 1), len(head))
	out := strings.Split(strings.Join(append(head, view), "\n"), "\n")
	return strings.Join(out[:min(len(out), height)], "\n")
}
