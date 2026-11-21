package tui

import (
	"github.com/castrowithcee/qatlas-host/internal/control"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	kittui "github.com/castrowithcee/qatlas-kit/tui"
)

// With more than one engine the Stacks area has one section per engine: a panel with the engine in its heading
// and, inside, the list of the stacks and stack folders of that engine. A section is a list of its own; the
// list the keys act on (Model.list) is the marked section's, and the cursor passes from the last row of a section to
// the first row of the next. An engine whose deploy service could not be asked is a section with the reason.

// stackSection is one engine in the Stacks area.
type stackSection struct {
	engine string
	// list is nil for an engine that could not be asked; reason says why.
	list   *kittui.List
	reason string
	// ids are the rows of the list; shown is how many rows it draws, with the stack folders.
	ids   map[kittui.ItemID]bool
	shown int
}

// navigable reports whether the cursor can stand in the section.
func (s *stackSection) navigable() bool { return s.list != nil && s.shown > 0 }

// stackLists are the lists of the Stacks area: the one list, or the list of every section.
func (m *Model) stackLists() []*kittui.List {
	if len(m.secs) == 0 {
		return []*kittui.List{m.list}
	}
	var out []*kittui.List
	for _, s := range m.secs {
		if s.list != nil {
			out = append(out, s.list)
		}
	}
	return out
}

// tickedStacks are the ticked rows of the Stacks area, section after section.
func (m *Model) tickedStacks() []kittui.ItemID {
	var out []kittui.ItemID
	for _, l := range m.stackLists() {
		out = append(out, l.Ticked()...)
	}
	return out
}

// setTickedStacks ticks exactly the rows ids, each in the list that has it.
func (m *Model) setTickedStacks(ids []kittui.ItemID) {
	if len(m.secs) == 0 {
		_ = m.list.SetTicked(ids)
		return
	}
	for _, s := range m.secs {
		if s.list == nil {
			continue
		}
		var own []kittui.ItemID
		for _, id := range ids {
			if s.ids[id] {
				own = append(own, id)
			}
		}
		_ = s.list.SetTicked(own)
	}
}

// fillSections sets the lists of the sections from the rows and folders read last. The marked section stays
// marked when it still exists.
func (m *Model) fillSections(rows []stackRow, ids []kittui.ItemID) {
	engines := m.engines()
	if m.baseList == nil {
		m.baseList = m.list
	}
	if m.secLists == nil {
		m.secLists = map[string]*kittui.List{}
	}
	reasons := map[string]string{}
	for _, n := range m.notChecked() {
		reasons[n.Engine] = n.Reason
	}
	m.set.folderData = map[stackRef]control.SourceStatus{}
	m.secs = m.secs[:0]
	for _, e := range engines {
		sec := &stackSection{engine: e, ids: map[kittui.ItemID]bool{}}
		m.secs = append(m.secs, sec)
		if reason, down := reasons[e]; down && !m.engineHasData(e) {
			sec.reason = reason
			continue
		}
		list := m.secLists[e]
		if list == nil {
			l, err := m.newStackList(m.text)
			if err != nil {
				m.failure = m.text.Lookup(kittui.TextError, err.Error())
				continue
			}
			list, m.secLists[e] = l, l
		}
		sec.list = list
		var src []control.SourceStatus
		for _, f := range m.snap.status.Engine.Sources {
			if f.Engine == e {
				src = append(src, f)
			}
		}
		grouped := len(src) > 0
		list.SetGrouped(grouped)
		var items []kittui.ListItem
		for i, r := range rows {
			if r.engine != e {
				continue
			}
			it := kittui.ListItem{ID: ids[i], Cells: cells(r, m.text, m.loc, m.tones)}
			if grouped {
				it.Group = r.folder
			}
			items = append(items, it)
			sec.ids[ids[i]] = true
		}
		if err := list.SetItems(items); err != nil {
			m.failure = m.text.Lookup(kittui.TextError, m.text.Lookup(textErrUnshownStacks, err.Error()))
		}
		m.fillFolderGroups(list, src, e)
		sec.shown = len(items) + len(src)
		if reason, down := reasons[e]; down {
			sec.reason = reason
		}
	}
	// Lists of engines that are gone are dropped.
	for e := range m.secLists {
		if !slices.Contains(engines, e) {
			delete(m.secLists, e)
		}
	}
	m.markSection(m.listEngine)
}

// engineHasData reports whether a read named the engine's stacks or folders, so that the section can show them
// beside a note that a later read failed.
func (m *Model) engineHasData(engine string) bool {
	for _, s := range m.snap.stacks.Stacks {
		if s.Engine == engine {
			return true
		}
	}
	for _, f := range m.snap.status.Engine.Sources {
		if f.Engine == engine {
			return true
		}
	}
	return false
}

// markSection makes the section of engine the marked one; when it has no rows to stand on, the first section
// that has.
func (m *Model) markSection(engine string) {
	at := slices.IndexFunc(m.secs, func(s *stackSection) bool { return s.engine == engine && s.navigable() })
	if at < 0 {
		at = slices.IndexFunc(m.secs, func(s *stackSection) bool { return s.navigable() })
	}
	if at < 0 {
		// Nothing to mark: keep a list that draws, the first one, so the keys have one to act on.
		at = slices.IndexFunc(m.secs, func(s *stackSection) bool { return s.list != nil })
	}
	if at < 0 {
		m.list, m.listEngine = m.baseList, ""
		return
	}
	m.list, m.listEngine = m.secs[at].list, m.secs[at].engine
	m.setZone(m.zone)
}

// leaveSections goes back to the one list of a host that shows one engine.
func (m *Model) leaveSections() {
	if len(m.secs) == 0 {
		return
	}
	m.secs, m.secLists = nil, nil
	if m.baseList != nil {
		m.list = m.baseList
	}
	m.listEngine = ""
	m.setZone(m.zone)
}

// activateList marks the section that holds list, as a click on one of its rows does.
func (m *Model) activateList(list *kittui.List) {
	for _, s := range m.secs {
		if s.list == list {
			m.list, m.listEngine = list, s.engine
			m.setZone(m.zone)
			return
		}
	}
}

// stackUpdate gives a key to the marked list. With sections, up from the first row goes to the last row of the
// section above and down from the last row to the first row of the section below.
func (m *Model) stackUpdate(msg tea.KeyPressMsg) (open kittui.ItemID, handled bool) {
	if len(m.secs) < 2 || m.list.Input() == kittui.InputText {
		return m.list.Update(msg)
	}
	id, ok := m.keys.Match(msg, kittui.InputNavigation)
	before := m.cursorKey()
	open, handled = m.list.Update(msg)
	if !ok || (id != actUp && id != actDown) || m.cursorKey() != before {
		return open, handled
	}
	at := slices.IndexFunc(m.secs, func(s *stackSection) bool { return s.list == m.list })
	step := 1
	if id == actUp {
		step = -1
	}
	for i := at + step; i >= 0 && i < len(m.secs); i += step {
		if !m.secs[i].navigable() {
			continue
		}
		m.list, m.listEngine = m.secs[i].list, m.secs[i].engine
		if step > 0 {
			m.list.Act(m.firstKey())
		} else {
			m.list.Act(m.lastKey())
		}
		m.setZone(m.zone)
		return "", true
	}
	return open, handled
}

// cursorKey identifies the row the marked list stands on.
func (m *Model) cursorKey() string {
	if it, ok := m.list.Current(); ok {
		return "i" + string(it.ID)
	}
	g, _ := m.list.SelectedGroup()
	return "g" + g
}

func (m *Model) firstKey() kittui.ActionID { return stackListKeys().First }
func (m *Model) lastKey() kittui.ActionID  { return stackListKeys().Last }

// minSection is the least height of a section: its borders, the header and one row.
const minSection = 5

// stackSectionsView draws the sections one below the other in height rows. A section takes the rows its content
// needs; when they do not suffice, each keeps a few and the marked one gets the rest.
func (m *Model) stackSectionsView(width, height int) string {
	t := m.text
	inner := max(width-panelChrome, 1)
	need := make([]int, len(m.secs))
	notes := make([][]string, len(m.secs))
	for i, s := range m.secs {
		if s.reason != "" {
			notes[i] = m.wrapped(0, kittui.StyleWarn, t.Lookup(textEngineNotChecked, s.engine, s.reason), inner)
		}
		switch {
		case s.list == nil:
			need[i] = len(notes[i]) + 2
		case s.shown == 0:
			need[i] = 4
		default:
			need[i] = s.shown + 3
		}
	}
	heights := make([]int, len(m.secs))
	left := height
	for i := range heights {
		heights[i] = min(need[i], minSection)
	}
	sum := 0
	for _, h := range heights {
		sum += h
	}
	if sum > height {
		// Too small for every section: the marked one and those after it that still fit.
		for i := range heights {
			heights[i] = 0
		}
		at := slices.IndexFunc(m.secs, func(s *stackSection) bool { return s.list == m.list })
		for _, i := range append([]int{max(at, 0)}, otherThan(len(m.secs), max(at, 0))...) {
			if h := min(min(need[i], minSection), left); h >= 3 {
				heights[i], left = h, left-h
			}
		}
	} else {
		left = height - sum
	}
	// The rest goes first to the marked section, then to the others in order.
	at := slices.IndexFunc(m.secs, func(s *stackSection) bool { return s.list == m.list })
	for _, i := range append([]int{max(at, 0)}, otherThan(len(m.secs), max(at, 0))...) {
		if heights[i] == 0 {
			continue
		}
		extra := min(need[i]-heights[i], left)
		heights[i], left = heights[i]+extra, left-extra
	}
	focused := m.focus() != kittui.ZoneTabs && m.focus() != kittui.ZoneSubtabs
	var out []string
	y := 0
	for i, s := range m.secs {
		h := heights[i]
		if h == 0 {
			continue
		}
		var body []string
		switch {
		case s.list == nil:
			body = notes[i]
		default:
			// A service that failed after an earlier read showed data: the note stays above that data.
			if s.reason != "" {
				body = notes[i][:1]
			}
			rows := max(h-2-len(body), 1)
			view, targets := s.list.View(m.styles, inner, rows)
			body = append(body, strings.Split(view, "\n")...)
			m.parts = append(m.parts, mousePart{rect: kittui.Rect{X: 2, Y: y + 1 + len(body) - rows, Width: inner, Height: rows}, list: s.list,
				targets: kittui.OffsetTargets(targets, 2, 0)})
		}
		out = append(out, m.styles.Panel(t.Lookup(textEngineHeading, s.engine), "", body, width, h, focused && s.list == m.list)...)
		y += h
	}
	return strings.Join(out, "\n")
}

// otherThan lists 0..n-1 without skip.
func otherThan(n, skip int) []int {
	var out []int
	for i := range n {
		if i != skip {
			out = append(out, i)
		}
	}
	return out
}
