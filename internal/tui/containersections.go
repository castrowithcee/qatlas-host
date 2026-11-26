package tui

import (
	"cmp"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// With more than one engine the Containers area has one section per engine, drawn as the sections of the Stacks
// area: a panel with the engine in its heading and, inside, the groups of that engine. Each section is a list of
// its own; the list the keys act on (Model.ctList) is the marked section's. An engine without a deploy service
// is a read-only section: it shows what root read, and no key acts on its rows.

// ctRef is what a row of the Containers list stands for. The engine is empty while one engine is shown.
type ctRef struct {
	name, engine string
	readOnly     bool
}

// ctSection is one engine in the Containers area.
type ctSection struct {
	engine string
	// list is nil for an engine that could not be asked and has no rows to show; reason says why.
	list     *kittui.List
	reason   string
	readOnly bool
	// shown is how many rows the list draws, with the group rows.
	shown int
}

func (s *ctSection) navigable() bool { return s.list != nil && s.shown > 0 }

// ctGroupEngine is the engine of a group; a Backend over one channel leaves it empty for the system engine.
func ctGroupEngine(g control.ContainerGroup) string { return cmp.Or(g.Engine, control.EngineSystem) }

// engineCompare puts the system engine first and the others by id.
func engineCompare(a, b string) int {
	switch {
	case a == b:
		return 0
	case a == control.EngineSystem:
		return -1
	case b == control.EngineSystem:
		return 1
	}
	return strings.Compare(a, b)
}

// ctEngines are the engines the Containers area has a section for: those with a deploy service, those with
// groups and those that could not be read.
func (m *Model) ctEngines() []string {
	var out []string
	add := func(e string) {
		if e != "" && !slices.Contains(out, e) {
			out = append(out, e)
		}
	}
	for _, e := range m.snap.engines {
		add(e)
	}
	for _, g := range m.ct.groups {
		add(ctGroupEngine(g))
	}
	for _, n := range m.ct.down {
		add(n.Engine)
	}
	slices.SortStableFunc(out, engineCompare)
	return out
}

// ctEntry is a container row with the engine it came from.
type ctEntry struct {
	ref  ctRef
	item kittui.ListItem
}

// fillCtSections sets the lists of the sections from the rows read last. The marked section stays marked when it
// still exists.
func (m *Model) fillCtSections(engines []string, entries []ctEntry) {
	ct := &m.ct
	if ct.base == nil {
		ct.base = m.ctList
	}
	if ct.lists == nil {
		ct.lists = map[string]*kittui.List{}
	}
	reasons := map[string]string{}
	for _, n := range ct.down {
		reasons[n.Engine] = n.Reason
	}
	ct.secs = ct.secs[:0]
	for _, e := range engines {
		sec := &ctSection{engine: e, reason: reasons[e]}
		ct.secs = append(ct.secs, sec)
		var items []kittui.ListItem
		groups := map[string]bool{}
		for _, en := range entries {
			if en.ref.engine == e {
				items = append(items, en.item)
				groups[en.item.Group] = true
				sec.readOnly = sec.readOnly || en.ref.readOnly
			}
		}
		if _, down := reasons[e]; down && len(items) == 0 {
			continue
		}
		list := ct.lists[e]
		if list == nil {
			l, err := m.newContainerList(m.text)
			if err != nil {
				ct.err = err
				continue
			}
			list, ct.lists[e] = l, l
		}
		sec.list = list
		sec.shown = len(items) + len(groups)
		if err := list.SetItems(items); err != nil {
			ct.err = err
		}
	}
	for e := range ct.lists {
		if !slices.Contains(engines, e) {
			delete(ct.lists, e)
		}
	}
	m.markCtSection(ct.engine)
}

// markCtSection makes the section of engine the marked one; when it has no rows to stand on, the first section
// that has.
func (m *Model) markCtSection(engine string) {
	secs := m.ct.secs
	at := slices.IndexFunc(secs, func(s *ctSection) bool { return s.engine == engine && s.navigable() })
	if at < 0 {
		at = slices.IndexFunc(secs, func(s *ctSection) bool { return s.navigable() })
	}
	if at < 0 {
		at = slices.IndexFunc(secs, func(s *ctSection) bool { return s.list != nil })
	}
	if at < 0 {
		m.ctList, m.ct.engine = m.ct.base, ""
		return
	}
	m.ctList, m.ct.engine = secs[at].list, secs[at].engine
	m.setZone(m.zone)
}

// leaveCtSections goes back to the one list of a host that shows one engine.
func (m *Model) leaveCtSections() {
	if len(m.ct.secs) == 0 {
		return
	}
	m.ct.secs, m.ct.lists, m.ct.engine = nil, nil, ""
	if m.ct.base != nil {
		m.ctList = m.ct.base
	}
	m.setZone(m.zone)
}

// activateCtList marks the section that holds list, as a click on one of its rows does.
func (m *Model) activateCtList(list *kittui.List) {
	for _, s := range m.ct.secs {
		if s.list == list {
			m.ctList, m.ct.engine = list, s.engine
			m.setZone(m.zone)
			return
		}
	}
}

// ctInMarked reports whether a group belongs to the marked section, or to the only list.
func (m *Model) ctInMarked(g control.ContainerGroup) bool {
	return len(m.ct.secs) == 0 || ctGroupEngine(g) == m.ct.engine
}

// ctCursor identifies the row the marked list stands on.
func (m *Model) ctCursor() string {
	if it, ok := m.ctList.Current(); ok {
		return "i" + string(it.ID)
	}
	g, _ := m.ctList.SelectedGroup()
	return "g" + g
}

// ctUpdate gives a key to the marked list. With sections, up from the first row goes to the last row of the
// section above and down from the last row to the first row of the section below; Home and End reach the first
// row of the first section and the last row of the last one.
func (m *Model) ctUpdate(msg tea.KeyPressMsg) (open kittui.ItemID, handled bool) {
	secs := m.ct.secs
	if len(secs) < 2 || m.ctList.Input() == kittui.InputText {
		return m.ctList.Update(msg)
	}
	mark := func(i int) {
		m.ctList, m.ct.engine = secs[i].list, secs[i].engine
		m.setZone(m.zone)
	}
	id, ok := m.keys.Match(msg, kittui.InputNavigation)
	if ok && (id == actFirst || id == actLast) {
		at := slices.IndexFunc(secs, func(s *ctSection) bool { return s.navigable() })
		if id == actLast {
			for i := len(secs) - 1; i >= 0; i-- {
				if secs[i].navigable() {
					at = i
					break
				}
			}
		}
		if at >= 0 {
			mark(at)
		}
		return m.ctList.Update(msg)
	}
	before := m.ctCursor()
	open, handled = m.ctList.Update(msg)
	if !ok || (id != actUp && id != actDown) || m.ctCursor() != before {
		return open, handled
	}
	at := slices.IndexFunc(secs, func(s *ctSection) bool { return s.list == m.ctList })
	step := 1
	if id == actUp {
		step = -1
	}
	for i := at + step; i >= 0 && i < len(secs); i += step {
		if !secs[i].navigable() {
			continue
		}
		mark(i)
		if step > 0 {
			m.ctList.Act(actFirst)
		} else {
			m.ctList.Act(actLast)
		}
		return "", true
	}
	return open, handled
}

// ctSectionsView draws the sections one below the other in height rows.
func (m *Model) ctSectionsView(width, height int) string {
	t := m.text
	inner := max(width-panelChrome, 1)
	secs := m.ct.secs
	need, keep := make([]int, len(secs)), make([]int, len(secs))
	notes := make([][]string, len(secs))
	for i, s := range secs {
		if s.reason != "" {
			notes[i] = m.wrapped(0, kittui.StyleWarn, t.Lookup(textEngineNotChecked, s.engine, s.reason), inner)
		}
		if s.readOnly {
			notes[i] = append(notes[i], m.wrapped(0, kittui.StyleHint, t.Lookup(textCtReadOnlyNote), inner)...)
		}
		keep[i] = minSection + len(notes[i])
		switch {
		case s.list == nil:
			need[i] = len(notes[i]) + 2
		case s.shown == 0:
			need[i] = len(notes[i]) + 3
		default:
			need[i] = len(notes[i]) + s.shown + 2 + listHead
		}
	}
	heights := sectionHeights(need, keep, slices.IndexFunc(secs, func(s *ctSection) bool { return s.list == m.ctList }), height)
	focused := m.focus() != kittui.ZoneTabs && m.focus() != kittui.ZoneSubtabs
	var out []string
	y := 0
	for i, s := range secs {
		h := heights[i]
		if h == 0 {
			continue
		}
		body := notes[i]
		switch {
		case s.list == nil:
		case s.shown == 0:
			body = append(body, m.line(0, kittui.StyleHint, t.Lookup(textCtEngineEmpty), inner))
		default:
			rows := max(h-2-len(body), 1)
			view, targets := s.list.View(m.styles, inner, rows)
			body = append(body, strings.Split(view, "\n")...)
			m.parts = append(m.parts, mousePart{rect: kittui.Rect{X: 2, Y: y + 1 + len(body) - rows, Width: inner, Height: rows}, list: s.list,
				targets: kittui.OffsetTargets(targets, 2, 0), click: m.listClick(s.list)})
		}
		heading := textEngineHeading
		if s.readOnly {
			heading = textCtEngineReadOnly
		}
		out = append(out, m.styles.Panel(t.Lookup(heading, s.engine), "", body, width, h, focused && s.list == m.ctList)...)
		y += h
	}
	return strings.Join(out, "\n")
}

// ctSelected is the container the marked row stands for; false for a group row and for a row of an engine
// without a deploy service, which no key acts on.
func (m *Model) ctSelected() (ctRef, bool) {
	id, found := m.ctList.Selected()
	ref, known := m.ctRows[id]
	return ref, found && known && !ref.readOnly
}
