package tui

import (
	"slices"

	tea "charm.land/bubbletea/v2"

	kittui "github.com/castrowithcee/qatlas-kit/tui"
	"github.com/castrowithcee/qatlas-kit/uiprefs"
)

// navigationTexts are the navigation forms the interface offers, in the order the field shows them.
var navigationTexts = []struct {
	id    string
	label kittui.TextID
}{
	{uiprefs.NavigationTabs, textNavTabs},
	{uiprefs.NavigationPathLine, textNavPathLine},
	{uiprefs.NavigationPathHeader, textNavPathHeader},
	{uiprefs.NavigationHeaderChoice, textNavHeaderChoice},
}

// navigations lists the form names for the preferences schema.
func navigations() []string {
	var out []string
	for _, n := range navigationTexts {
		out = append(out, n.id)
	}
	return out
}

// navigationField is the Navigation choice of the General page; an empty value shows the default form.
func navigationField(value string) kittui.FormField {
	if value == "" {
		value = uiprefs.NavigationTabs
	}
	var choices []kittui.Choice
	for _, n := range navigationTexts {
		choices = append(choices, choice(n.id, n.label))
	}
	return kittui.FormField{ID: "navigation", Kind: kittui.FieldChoice, Label: textFieldNavigation,
		Hint: textHintNavigation, Choices: choices, Value: kittui.FormValue{Choice: kittui.ChoiceID(value)}}
}

// navForm maps the stored form name to the frame's form; an empty or unknown name is the tab line.
func navForm(name string) kittui.NavForm {
	switch name {
	case uiprefs.NavigationPathLine:
		return kittui.NavPathLine
	case uiprefs.NavigationPathHeader:
		return kittui.NavPathHeader
	case uiprefs.NavigationHeaderChoice:
		return kittui.NavHeaderChoice
	}
	return kittui.NavTabs
}

// actMenu opens the overview of the path forms; the navigation forms without a path ignore it.
const actMenu = kittui.ActionMenu

// navActions are the actions the navigation forms add to the key table.
func navActions() []kittui.Action { return []kittui.Action{std(actMenu, kittui.TextKeyMenu)} }

func pathNavKeys() kittui.PathNavKeys {
	return kittui.PathNavKeys{Up: actUp, Down: actDown, PageUp: actPageUp, PageDown: actPageDown, First: actFirst,
		Last: actLast, Left: actLeft, Right: actRight, Enter: actEnter, Back: actBack, CloseOutside: true}
}

// pathForm reports whether the navigation names the place as a path, in its own line or in the header.
func (m *Model) pathForm() bool {
	return m.prefs.Navigation == uiprefs.NavigationPathLine || m.prefs.Navigation == uiprefs.NavigationPathHeader
}

// choiceForm reports whether the areas are a choice field in the header.
func (m *Model) choiceForm() bool { return m.prefs.Navigation == uiprefs.NavigationHeaderChoice }

// onNavLine reports whether the focus lies on the navigation, not in the work area. A path form keeps the
// zones of the tab line inside: ZoneTabs is its area section and ZoneSubtabs its tab section; the frame
// sees both as ZonePath.
func (m *Model) onNavLine() bool {
	return m.focus() == kittui.ZoneTabs || m.focus() == kittui.ZoneSubtabs
}

// stepsPages reports whether left and right step the sub-tab pages: they do on the sub-tab line, not on a path.
func (m *Model) stepsPages() bool { return m.focus() == kittui.ZoneSubtabs && !m.pathForm() }

// frameFocus is the focus zone as the frame names it.
func (m *Model) frameFocus() kittui.FocusZone {
	if m.pathForm() && m.onNavLine() {
		return kittui.ZonePath
	}
	return m.focus()
}

// pathFocusArea reports whether the focus of a path lies on its area section.
func (m *Model) pathFocusArea() bool { return m.pathForm() && m.focus() == kittui.ZoneTabs }

// navOpen reports whether an overview or choice of the path navigation is open.
func (m *Model) navOpen() bool { return m.pnav != nil && m.pnav.Open() }

// openNav builds the overlay anew, so it always speaks the language in force, and opens it with open.
func (m *Model) openNav(open func(*kittui.PathNav, kittui.FrameSpec) bool) {
	nav, err := kittui.NewPathNav(pathNavKeys(), m.keys, m.text)
	if err != nil {
		m.failure = m.text.Lookup(kittui.TextError, err.Error())
		return
	}
	if open(nav, m.frameSpec()) {
		m.pnav = nav
	}
}

// pathZones are the zones a path form walks: its sections are one line.
func (m *Model) pathZones() []kittui.FocusZone {
	return slices.DeleteFunc(slices.Clone(m.zones()), func(z kittui.FocusZone) bool { return z == kittui.ZoneSubtabs })
}

// zoneKey handles the keys that move the focus between the lines of the navigation and the work area.
func (m *Model) zoneKey(id kittui.ActionID) (tea.Cmd, bool) {
	switch {
	case id == actMenu:
		if !m.pathForm() && !m.choiceForm() {
			return nil, false
		}
		m.openNav((*kittui.PathNav).OpenOverview)
		return nil, true
	case m.pathForm() && (m.onNavLine() || id == actNext || id == actPrev):
		return m.pathLineKey(id)
	case m.choiceForm() && m.focus() == kittui.ZoneTabs && id == actEnter:
		m.openNav((*kittui.PathNav).OpenAreas)
		return nil, true
	}
	return m.tabKey(id)
}

// pathLineKey is a key on the path: left and right move between its sections, enter opens the overlay of the
// focused one, and down leads to the work area.
func (m *Model) pathLineKey(id kittui.ActionID) (tea.Cmd, bool) {
	zones, cur := m.pathZones(), m.focus()
	if cur == kittui.ZoneSubtabs {
		cur = kittui.ZoneTabs
	}
	if !m.onNavLine() && id != actNext && id != actPrev {
		return nil, false
	}
	switch id {
	case actEnter:
		m.openNav((*kittui.PathNav).OpenFocused)
	case actDown:
		m.setZone(kittui.StepZoneLevel(zones, kittui.ZoneTabs, 1))
	case actLeft, actRight:
		if slices.Contains(m.zones(), kittui.ZoneSubtabs) {
			m.setZone(kittui.ZoneSubtabs)
			if id == actLeft {
				m.setZone(kittui.ZoneTabs)
			}
		}
	case actNext:
		m.setZone(kittui.CycleZone(zones, cur, 1))
	case actPrev:
		m.setZone(kittui.CycleZone(zones, cur, -1))
	case actUp:
		// nothing lies above the path
	default:
		return nil, false
	}
	return nil, true
}

// tabKey is a key on the tab line and the sub-tab line.
func (m *Model) tabKey(id kittui.ActionID) (tea.Cmd, bool) {
	zones, cur := m.zones(), m.focus()
	tabs, sub := cur == kittui.ZoneTabs, cur == kittui.ZoneSubtabs
	switch {
	case id == actNext, (tabs || sub) && (id == actDown || id == actEnter):
		m.setZone(kittui.CycleZone(zones, cur, 1))
	case id == actPrev:
		m.setZone(kittui.CycleZone(zones, cur, -1))
	case tabs && id == actUp:
		// nothing lies above the tab line; the unfocused content must not move under it
	case sub && id == actUp:
		m.setZone(kittui.ZoneTabs)
	case (tabs || sub) && (id == actLeft || id == actRight):
		return m.stepLine(id, sub), true
	default:
		return nil, false
	}
	return nil, true
}

// stepLine steps the sub-tab line, or the tab line with the focus staying on it so the arrows can go on.
func (m *Model) stepLine(id kittui.ActionID, sub bool) tea.Cmd {
	step := 1
	if id == actLeft {
		step = -1
	}
	if sub {
		return m.stepTab(step)
	}
	cmd := m.enter(kittui.StepArea(areas(), m.area, step))
	m.setZone(kittui.ZoneTabs)
	return cmd
}

// navCommon puts the hints of the focus on the navigation before the common ones.
func (m *Model) navCommon(common []kittui.Hint) []kittui.Hint {
	var nav []kittui.Hint
	switch {
	case m.pathForm() && m.onNavLine():
		nav = append(nav, kittui.Hint{Action: actEnter, Label: kittui.TextKeyEnter, Kind: kittui.HintNavigation},
			kittui.Hint{Action: actNext, Label: textKeyZone, Priority: 3})
	case m.focus() == kittui.ZoneTabs:
		nav = append(nav, kittui.Hint{Action: actLeft, Label: textKeyPrevArea, Priority: 3},
			kittui.Hint{Action: actRight, Label: textKeyNextArea, Priority: 3},
			kittui.Hint{Action: actNext, Label: textKeyZone, Priority: 3})
	}
	if m.pathForm() || m.choiceForm() {
		nav = append(nav, kittui.Hint{Action: actMenu, Label: kittui.TextKeyMenu, Priority: 3})
	}
	return append(nav, common...)
}

// navKeyOpen gives a key to the open overlay; no key reaches the screen behind it.
func (m *Model) navKeyOpen(msg tea.KeyPressMsg) tea.Cmd {
	if id, ok := m.keys.Match(msg, kittui.InputNavigation); ok && id == actForceQuit {
		return tea.Quit
	}
	choice, _ := m.pnav.Update(msg)
	return m.choose(choice)
}

// choose runs a choice of the overlay as the click on that tab: the area as its digit, then the tab of it.
func (m *Model) choose(c kittui.PathChoice) tea.Cmd {
	if c.Area == "" {
		return nil
	}
	var cmds []tea.Cmd
	if c.Area != m.area {
		cmds = append(cmds, m.areaClick(c.Area))
	}
	if c.Tab != "" && c.Area == m.area && m.modal == nil {
		cmds = append(cmds, m.selectTab(c.Tab))
	}
	if m.modal == nil {
		m.setZone(kittui.ZoneTabs)
		if c.Tab != "" {
			m.setZone(kittui.ZoneSubtabs)
		}
	}
	return tea.Batch(cmds...)
}

// navMouse is the mouse of the navigation forms: the open overlay takes every click, and a click on the
// path or on the choice field opens one. Everything else takes the way of the screen.
func (m *Model) navMouse(msg tea.MouseMsg) tea.Cmd {
	res := m.mouseMap.Handle(msg)
	switch {
	case m.navOpen():
		if res.WheelSteps != 0 {
			m.pnav.Wheel(res.WheelSteps)
			return nil
		}
		choice, _ := m.pnav.Click(res)
		return m.choose(choice)
	case m.navTarget(res):
		if m.pathForm() {
			m.openNav(func(p *kittui.PathNav, spec kittui.FrameSpec) bool { return p.OpenClick(spec, res) })
		} else {
			m.openNav((*kittui.PathNav).OpenAreas)
		}
		return nil
	}
	return m.mouse(msg)
}

// navTarget reports whether res is a click on the path or on the choice field while the screen takes clicks.
func (m *Model) navTarget(res kittui.MouseResult) bool {
	if !res.Handled || res.Area == "" || res.Item != "" || m.modal != nil || m.helpOpen || m.overlayOpen() {
		return false
	}
	return m.pathForm() && res.Zone == kittui.ZonePath || m.choiceForm() && res.Zone == kittui.ZoneTabs
}

// navOverlay draws the open overlay over the frame, or in the workspace where the screen is too small.
func (m *Model) navOverlay(spec kittui.FrameSpec, view string, room kittui.Rect) (string, []kittui.Target) {
	if over, ot, ok := m.pnav.Overlay(m.styles, view, m.width, m.height, room); ok {
		return over, ot
	}
	spec.Footer, spec.Input, spec.Position, spec.Back = m.pnav.Hints(), kittui.InputNavigation, "", nil
	room = m.frame.Workspace(spec)
	wv, wt := m.pnav.View(m.styles, room.Width, room.Height)
	spec.Workspace = wv
	view, _ = m.frame.Render(m.styles, spec)
	return view, kittui.OffsetTargets(wt, room.X, room.Y)
}

// logFilterOverlay draws the period form of a log over the frame, or in the workspace where the screen is
// too small.
func (m *Model) logFilterOverlay(lv *kittui.LogView, spec kittui.FrameSpec, view string, room kittui.Rect) (string, []kittui.Target) {
	if over, ot, ok := lv.ModalOverlay(m.styles, view, m.width, m.height, room); ok {
		return over, ot
	}
	spec.Footer, spec.Input, spec.Position, spec.Back = lv.Hints(), lv.Input(), "", nil
	room = m.frame.Workspace(spec)
	wv, wt := lv.ModalView(m.styles, room.Width, room.Height)
	spec.Workspace = wv
	view, _ = m.frame.Render(m.styles, spec)
	return view, kittui.OffsetTargets(wt, room.X, room.Y)
}
