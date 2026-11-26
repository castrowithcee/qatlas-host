package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// The view Changes of a stack folder in the Stacks area: the uncommitted changes as source.changes lists them, the
// diff of one, a commit of all of them and the discard of one. It opens on the row of a folder with changes and in
// the browser over it, and esc leaves it for where it was opened. The list is what the service names, so
// secrets.env, data folders and new files below a bind mount never appear in it.

// actChanges opens the view; actCommit commits every file it lists. Every letter is the key of an action already,
// so both take a sign: = as "compared with the commit", > as "passed on".
const (
	actChanges kittui.ActionID = "changes"
	actCommit  kittui.ActionID = "commit"
)

func changesKeys() []kittui.Action {
	return []kittui.Action{act(actChanges, kittui.LevelLetter, textKeyChanges, "="), act(actCommit, kittui.LevelLetter, textKeyCommit, ">")}
}

// opChanges labels the writes of the view; no branch reads it, so it shares the one of the return.
const opChanges = opReturn

// stackChanges is the view while open is set: the folder, the list as read last with the head of the folder it was
// read at, and the diff shown, nil while the list is shown. gen and diffGen separate a read from an older one.
type stackChanges struct {
	open    bool
	folder  control.SourceStatus
	list    *kittui.List
	read    part
	res     control.SourceChangesResult
	head    string
	gen     int
	diff    *changeDiff
	diffGen int
}

// changeDiff is the diff of one file: read, failed with note, or shown in text.
type changeDiff struct {
	file      string
	reading   bool
	note      string
	text      *kittui.TextView
	truncated bool
}

// shownChanges is the view while the Stacks area shows it, nil otherwise.
func (m *Model) shownChanges() *stackChanges {
	if m.area != areaStacks || !m.sf.changes.open || m.set.keyOpen {
		return nil
	}
	return &m.sf.changes
}

// folderChanged reports whether the last read counted uncommitted changes in the folder f.
func (m *Model) folderChanged(f control.SourceStatus) bool {
	return m.snap.changes[stackRef{f.Engine, f.Path}] > 0
}

// openFolderChanges opens the view for the folder of the browser v, or of the marked row where v is nil. A folder
// without changes says so.
func (m *Model) openFolderChanges(v *fileView) (tea.Cmd, bool) {
	f, ok := m.sf.folder, v != nil
	if !ok {
		f, ok = m.selectedFolder()
	}
	switch {
	case !ok:
		return nil, false
	case !m.folderChanged(f):
		m.warn(m.text.Lookup(textChangesFolderClean, f.Path))
		return nil, true
	}
	c := &m.sf.changes
	if c.list == nil {
		l, err := kittui.NewList(kittui.ListSpec{Columns: []kittui.Column{{Title: textColFile}, {Title: textColKind},
			{Title: textColStack, Flex: true}}, Keys: listKeys(), Search: kittui.SearchByKey, Empty: textChangesNone}, m.keys, m.text)
		if err != nil {
			m.failure = m.text.Lookup(kittui.TextError, err.Error())
			return nil, true
		}
		l.SetFocused(true)
		c.list = l
	}
	if c.folder.Path != f.Path || c.folder.Engine != f.Engine {
		c.res, c.head = control.SourceChangesResult{}, ""
		m.fillChanges()
	}
	c.open, c.folder, c.diff = true, f, nil
	return m.readChanges(), true
}

// changesMsg carries the changes of the folder the view shows, with the head of the folder read before them.
type changesMsg struct {
	gen  int
	head string
	res  control.SourceChangesResult
	err  error
}

func (msg changesMsg) applyTo(m *Model) {
	c := &m.sf.changes
	if msg.gen != c.gen {
		return
	}
	c.read = part{loaded: msg.err == nil, err: msg.err}
	if msg.err == nil {
		c.res, c.head = msg.res, msg.head
		m.fillChanges()
	}
}

// readChanges reads the changes of the folder of the view again. The head is read first: a commit that moves it in
// between makes the commit of the list stop with head-moved rather than take a change it was not read at.
func (m *Model) readChanges() tea.Cmd {
	c := &m.sf.changes
	c.gen++
	c.read.loading = true
	gen, f, ctx, b, head := c.gen, c.folder, m.engineCtx(c.folder.Engine), m.backend, headOf(m.snap.status.Engine.Sources, c.folder)
	return func() tea.Msg {
		if list, err := b.SourceList(ctx); err == nil {
			head = headOf(list.Sources, f)
		}
		res, err := b.SourceChanges(ctx, f.Path)
		return changesMsg{gen: gen, head: head, res: res, err: err}
	}
}

// headOf is the head of the folder f among the folders src, else the one f names.
func headOf(src []control.SourceStatus, f control.SourceStatus) string {
	for _, s := range src {
		if s.Path == f.Path && inEngine(s.Engine, f.Engine) {
			return s.Head
		}
	}
	return f.Head
}

// fillChanges sets the rows of the list from what was read last.
func (m *Model) fillChanges() {
	c, t := &m.sf.changes, m.text
	items := make([]kittui.ListItem, len(c.res.Changes))
	for i, ch := range c.res.Changes {
		kind := ch.Kind
		switch ch.Kind {
		case control.ChangeModified:
			kind = t.Lookup(textChangeModified)
		case control.ChangeNew:
			kind = t.Lookup(textChangeNew)
		case control.ChangeDeleted:
			kind = t.Lookup(textChangeDeleted)
		}
		items[i] = kittui.ListItem{ID: kittui.ItemID(ch.Path), Cells: []string{ch.Path, kind, ch.Stack}}
	}
	if err := c.list.SetItems(items); err != nil {
		m.failure = t.Lookup(kittui.TextError, err.Error())
	}
}

// selectedChange is the change under the cursor of the list.
func (m *Model) selectedChange() (control.SourceChange, bool) {
	c := &m.sf.changes
	id, ok := c.list.Selected()
	for _, ch := range c.res.Changes {
		if ok && kittui.ItemID(ch.Path) == id {
			return ch, true
		}
	}
	return control.SourceChange{}, false
}

// changesInput is the input focus of the view.
func (m *Model) changesInput(c *stackChanges) kittui.InputFocus {
	if c.diff != nil {
		if c.diff.text != nil {
			return c.diff.text.Input()
		}
		return kittui.InputNavigation
	}
	return c.list.Input()
}

// changesTyped gives a key to the diff or to the list, where enter asks for the diff of the marked file.
func (m *Model) changesTyped(c *stackChanges, msg tea.KeyPressMsg) (tea.Cmd, bool) {
	if c.diff != nil {
		return nil, c.diff.text != nil && c.diff.text.Update(msg)
	}
	open, handled := c.list.Update(msg)
	if open != "" {
		return m.askDiff(), true
	}
	return nil, handled
}

// changesAction acts on a letter in the list: discard the marked file, commit all.
func (m *Model) changesAction(c *stackChanges, id kittui.ActionID) (tea.Cmd, bool) {
	if c.diff != nil {
		return nil, false
	}
	_, marked := m.selectedChange()
	switch {
	case id == actRemove && marked:
		return m.askDiscard(), true
	case id == actCommit && len(c.res.Changes) > 0:
		return m.askCommit(), true
	}
	return nil, false
}

// changesBack is esc in the view: it clears a search, closes the diff, and leaves the view from the list.
func (m *Model) changesBack(c *stackChanges, msg tea.KeyPressMsg) {
	if c.diff != nil {
		if c.diff.text == nil || !c.diff.text.Update(msg) {
			c.diff = nil
			c.diffGen++
		}
		return
	}
	if _, handled := c.list.Update(msg); !handled {
		c.open = false
	}
}

// stackChangesFooter are the own keys of the view.
func (m *Model) stackChangesFooter(c *stackChanges) []kittui.Hint {
	back := kittui.Hint{Action: actBack, Label: kittui.TextKeyBack, Priority: 1}
	if d := c.diff; d != nil {
		if d.text != nil {
			return append(d.text.Hints(), back)
		}
		return []kittui.Hint{back}
	}
	var own []kittui.Hint
	for _, b := range m.changesButtons(c) {
		if b.Action != actEnter {
			own = append(own, kittui.Hint{Action: b.Action, Label: b.Label, Priority: 2})
		}
	}
	return m.listFooter(c.list, len(c.res.Changes), textKeyDiff, append(own, back), nil)
}

// changesButtons are the actions of the list that its button row offers to a click.
func (m *Model) changesButtons(c *stackChanges) []kittui.Hint {
	var out []kittui.Hint
	if _, ok := m.selectedChange(); ok {
		out = append(out, kittui.Hint{Action: actEnter, Label: textKeyDiff}, kittui.Hint{Action: actRemove, Label: textKeyDiscard})
	}
	if len(c.res.Changes) > 0 {
		out = append(out, kittui.Hint{Action: actCommit, Label: textKeyCommit})
	}
	return out
}

// stackChangesView draws the view in width by height cells: the folder, a row of buttons, the list and what a commit
// takes, or the diff.
func (m *Model) stackChangesView(c *stackChanges, width, height int) string {
	t := m.text
	if c.diff != nil {
		return m.diffView(c.diff, width, height)
	}
	head := []string{m.line(detailMargin, kittui.StyleActive, t.Lookup(textChangesOf, c.folder.Path), width)}
	if !c.read.loaded {
		head = append(head, m.partLines(c.read, width)...)
	}
	if c.res.Truncated {
		head = append(head, m.wrapped(detailMargin, kittui.StyleWarn, t.Lookup(textChangesCut), width)...)
	}
	if buttons := m.changesButtons(c); len(buttons) > 0 {
		head = append(head, m.buttonRow(buttons, len(head)), "")
	}
	foot := append([]string{""}, m.wrapped(detailMargin, kittui.StyleHint, t.Lookup(textChangesNote), width)...)
	view := m.listView(c.list, width, max(height-len(head)-len(foot), 1), len(head))
	out := strings.Split(strings.Join(append(append(head, view), foot...), "\n"), "\n")
	return strings.Join(out[:min(len(out), height)], "\n")
}

// buttonRow draws buttons on one line at row y of the workspace; a click on one takes the way of its key.
func (m *Model) buttonRow(buttons []kittui.Hint, y int) string {
	var line string
	var targets []kittui.Target
	x := detailMargin
	for _, b := range buttons {
		label := m.text.Lookup(b.Label)
		cell := m.styles.Button(kittui.ButtonNormal, label, false)
		w := len([]rune("[ " + label + " ]"))
		targets = append(targets, kittui.Target{Rect: kittui.Rect{X: x, Width: w, Height: 1}, Action: b.Action})
		line += "  " + cell
		x += w + 2
	}
	m.parts = append(m.parts, mousePart{rect: kittui.Rect{Y: y, Width: x, Height: 1}, targets: targets})
	return line
}

// diffView draws the diff: the file, a note where it is cut, then the text or why there is none.
func (m *Model) diffView(d *changeDiff, width, height int) string {
	lines := []string{m.line(detailMargin, kittui.StyleActive, d.file, width)}
	if d.truncated {
		lines = append(lines, m.line(detailMargin, kittui.StyleWarn, m.text.Lookup(textDiffCut), width))
	}
	switch {
	case d.reading:
		lines = append(lines, m.line(detailMargin, kittui.StyleHint, m.text.Lookup(textDetailLoading), width))
	case d.text == nil:
		lines = append(append(lines, ""), m.wrapped(detailMargin, kittui.StyleFail, d.note, width)...)
	default:
		y, rows := len(lines), max(height-len(lines), 1)
		view, targets := d.text.View(m.styles, width, rows)
		m.parts = append(m.parts, mousePart{rect: kittui.Rect{Y: y, Width: width, Height: rows}, targets: targets,
			scroll: d.text.Wheel, click: func(res kittui.MouseResult) tea.Cmd { d.text.Click(res); return nil }})
		lines = append(lines, view)
	}
	out := strings.Split(strings.Join(lines, "\n"), "\n")
	return strings.Join(out[:min(len(out), height)], "\n")
}

// changesHint is the key of the view where a stack folder with changes is shown, as its row or its browser.
func (m *Model) changesHint(f control.SourceStatus) []kittui.Hint {
	if !m.folderChanged(f) {
		return nil
	}
	return []kittui.Hint{{Action: actChanges, Label: textKeyChanges, Priority: 2}}
}

// changesTarget makes rows rows from row y of the workspace a click that opens the view.
func (m *Model) changesTarget(y, width, rows int) {
	r := kittui.Rect{Width: width, Height: rows}
	m.parts = append(m.parts, mousePart{rect: kittui.Rect{Y: y, Width: width, Height: rows},
		targets: []kittui.Target{{Rect: r, Zone: kittui.ZoneRows, Action: actChanges}}})
}
