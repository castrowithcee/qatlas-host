package tui

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	kittui "github.com/castrowithcee/qatlas-kit/tui"
)

// The command line names the actions of the key table that trigger something on a screen, and the areas. A
// command runs through the key of its action or the click on the tab of its area, so it asks what they ask and
// does no more than they do.

func commandText(id kittui.ActionID, part string) kittui.TextID {
	return kittui.TextID("cmd." + string(id) + "." + part)
}

// commands are the commands in the language of the interface: the areas first, then the actions.
func (m *Model) commands() []kittui.Command {
	cmds := make([]kittui.Command, 0, len(areas())+len(commandRows))
	for i, a := range areas() {
		title := m.text.Lookup(a.Title)
		cmds = append(cmds, kittui.Command{Name: strings.ToLower(title), Description: textCmdArea, DescriptionArgs: []any{title},
			Action: actArea, Key: i + 1, Area: a.ID, Available: m.commandAvailable(actArea, "")})
	}
	for _, r := range commandRows {
		names := strings.Split(m.text.Lookup(commandText(r.id, "name")), "|")
		cmds = append(cmds, kittui.Command{Name: names[0], Aliases: names[1:], Description: commandText(r.id, "desc"),
			Action: r.id, Available: m.commandAvailable(r.id, r.home)})
	}
	return cmds
}

// commandAvailable is where the key of action id acts: the key line of the view offers it, as the help lists
// it. Elsewhere the reason names the area it belongs to, when there is one.
func (m *Model) commandAvailable(id kittui.ActionID, home kittui.AreaID) func(kittui.Context) (bool, string) {
	return func(kittui.Context) (bool, string) {
		// The tab of an area takes a click while the General form has the keys, and the click is its way there.
		if slices.ContainsFunc(m.commandHints(), func(h kittui.Hint) bool { return h.Action == id }) ||
			id == actArea && m.formShown() && m.set.form.kind == formGeneral {
			return true, ""
		}
		if i := slices.IndexFunc(areas(), func(a kittui.Area) bool { return a.ID == home }); i >= 0 && home != m.area {
			return false, m.text.Lookup(textCmdOpenArea, m.text.Lookup(areas()[i].Title))
		}
		return false, m.text.Lookup(textCmdNotHere)
	}
}

// commandHints are the hints the help lists and two keys the screens leave out: the detail of a stack takes the
// versions of another stack, and the lists of the Settings area search, each on its key without a hint.
func (m *Model) commandHints() []kittui.Hint {
	hints := m.helpScreen()
	if _, ok := m.detailStack(); ok && m.area == areaDetail && m.pick == nil && m.set.form == nil && m.modal == nil {
		hints = append(hints, kittui.Hint{Action: actPromote, Label: textKeyPromote})
	}
	if l := m.set.activeList(); l != nil && m.area == areaSettings && m.focus() == kittui.ZoneRows {
		if _, ok := l.Selected(); ok {
			hints = append(hints, kittui.Hint{Action: actSearch, Label: kittui.TextKeySearch})
		}
	}
	return hints
}

// commandKey gives a key to the open command line, or opens it on its key; taken is false for any other key.
func (m *Model) commandKey(msg tea.KeyPressMsg) (cmd tea.Cmd, taken bool) {
	if m.cmdline != nil {
		if id, ok := m.keys.Match(msg, kittui.InputNavigation); ok && id == actForceQuit {
			return tea.Quit, true
		}
		line, done := m.cmdline.Update(msg)
		return m.commandDone(line, done), true
	}
	if id, ok := m.keys.Match(msg, m.input()); !ok || id != actCommand || m.modal != nil || m.helpOpen {
		return nil, false
	}
	line, err := kittui.NewCommandLine(kittui.CommandLineSpec{Commands: m.commands(), Keys: kittui.CommandLineKeys{
		Up: actUp, Down: actDown, Next: actNext, Prev: actPrev, Enter: actEnter, Back: actBack}}, m.keys, m.text)
	if err != nil {
		m.failure = m.text.Lookup(kittui.TextError, err.Error())
		return nil, true
	}
	line.SetContext(kittui.Context{Area: m.area, Tab: m.activeTab(), Zone: m.focus(), Focus: m.input()})
	m.cmdline = line
	return nil, true
}

// commandDone closes the line and runs the command it chose as the key of the action or the tab of the area would.
func (m *Model) commandDone(c kittui.Command, done bool) tea.Cmd {
	if !done {
		return nil
	}
	m.cmdline = nil
	if c.Area != "" {
		return m.areaClick(c.Area)
	}
	if key, ok := m.keyOf(c.Action); ok {
		return m.press(key)
	}
	return nil
}

// pointer gives a mouse event to the open command line, or to the selection and the screen when it is closed.
func (m *Model) pointer(msg tea.MouseMsg) tea.Cmd {
	if m.cmdline == nil {
		return m.selectMouse(msg)
	}
	if _, ok := msg.(tea.MouseClickMsg); !ok {
		return nil
	}
	return m.commandDone(m.cmdline.Click(m.mouseMap.Handle(msg)))
}

// paint draws the open command line over the screen and sets what takes clicks.
func (m *Model) paint(view string, targets []kittui.Target) string {
	if m.cmdline == nil {
		m.setMouse(targets)
	} else {
		view, targets = m.commandScreen(view)
		m.mouseMap = kittui.NewMouseMap(targets, m.mouseOn)
	}
	return m.styles.Paint(view, m.width, m.height)
}

// commandScreen draws the line over screen where it fits, else in the workspace with its keys in the footer.
func (m *Model) commandScreen(screen string) (string, []kittui.Target) {
	if over, ts, ok := m.cmdline.Overlay(m.styles, screen, m.width, m.height, kittui.Rect{Width: m.width, Height: m.height}); ok {
		return over, ts
	}
	spec := m.frameSpec()
	spec.Footer, spec.Input, spec.Position, spec.Back, spec.HeaderKeys = m.cmdline.Hints(), kittui.InputText, "", nil, nil
	room := m.frame.Workspace(spec)
	body, targets := m.cmdline.View(m.styles, room.Width, room.Height)
	spec.Workspace = body
	view, frame := m.frame.Render(m.styles, spec)
	targets = kittui.OffsetTargets(targets, room.X, room.Y)
	for _, t := range frame {
		if t.Rect.Y >= room.Y+room.Height {
			targets = append(targets, t)
		}
	}
	return view, targets
}
