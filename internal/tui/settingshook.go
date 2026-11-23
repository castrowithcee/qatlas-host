package tui

import (
	tea "charm.land/bubbletea/v2"

	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/deploysetup"
)

// The Webhooks page shows whether the webhook receiver is set up, its state, its address and the number of hooks,
// and sets the receiver up or moves it to another address. Both are the command `hook-service setup`, which needs
// root, so they run like the other settings: directly as root, else through sudo. The page never holds the
// address of a hook.

const formHookListen formKind = formFolderEngine + 1

// hookListenLocal reports whether an address reaches only this host; an address that is not valid is not local.
func hookListenLocal(listen string) bool {
	local, err := deploysetup.ValidateHookListen(listen)
	return err == nil && local
}

// hookListenOf is the address the receiver listens on, or would by default.
func (m *Model) hookListenOf() string {
	if l := m.set.hookInfo.Listen; l != "" {
		return l
	}
	return deploysetup.DefaultHookListen
}

func (m *Model) hookLines(width int) []string {
	s, t := m.set, m.text
	if !s.hooks.loaded || s.hooks.err != nil {
		return m.partLines(s.hooks, width)
	}
	info := s.hookInfo
	state := t.Lookup(textHookNotSetUp)
	switch {
	case info.SetUp && info.Running:
		state = t.Lookup(textHookRunning)
	case info.SetUp:
		state = t.Lookup(textHookStopped)
	}
	return m.framed(t.Lookup(textPageHooks), state, width, 0, func(width int) []string { return m.hookBody(info, width) })
}

func (m *Model) hookBody(info HookServiceInfo, width int) []string {
	t := m.text
	if !info.SetUp {
		return m.wrapped(detailIndent, kittui.StyleText, t.Lookup(textHookBody, deploysetup.DefaultHookListen), width)
	}
	if info.Problem != "" {
		return m.wrapped(detailIndent, kittui.StyleFail, t.Lookup(textHookProblem, kittui.Escape(info.Problem)), width)
	}
	lines := []string{m.line(detailIndent, kittui.StyleText, t.Lookup(textHookAddress, kittui.Escape(info.Listen)), width),
		m.line(detailIndent, kittui.StyleText, t.Lookup(textHookCount, info.Hooks), width)}
	if !hookListenLocal(info.Listen) {
		lines = append(lines, m.wrapped(detailIndent, kittui.StyleWarn, t.Lookup(textHookProxy), width)...)
	}
	return lines
}

// hookHints are the keys of the page: the setup only while there is none, the address once the state is read.
func (m *Model) hookHints() []kittui.Hint {
	s := m.set
	if !s.hooks.loaded || s.hooks.err != nil {
		return nil
	}
	var hints []kittui.Hint
	if !s.hookInfo.SetUp {
		hints = append(hints, kittui.Hint{Action: actAdd, Label: textKeyHookSetup, Priority: 3})
	}
	return append(hints, kittui.Hint{Action: actEdit, Label: textKeyHookListen, Priority: 3})
}

// hookAction acts on the keys of the page: a sets the receiver up on its default address, e asks for the address.
func (m *Model) hookAction(id kittui.ActionID) (cmd tea.Cmd, handled bool) {
	s := m.set
	if !s.hooks.loaded || s.hooks.err != nil {
		return nil, false
	}
	switch {
	case id == actAdd && !s.hookInfo.SetUp:
		m.askHookSetup("")
		return nil, true
	case id == actEdit:
		return m.openHookListen(), true
	}
	return nil, false
}

func (m *Model) openHookListen() tea.Cmd {
	return m.openForm(m.newSettingsForm(formHookListen, textTitleHookListen, []kittui.FormField{
		textField("listen", textFieldHookListen, textHintHookListen, m.hookListenOf())}))
}

func (m *Model) submitHookListen(f *settingsForm, listen string) tea.Cmd {
	if _, err := deploysetup.ValidateHookListen(listen); err != nil {
		f.problems(m.bad("listen", textProblemHookListen))
		return nil
	}
	if m.set.hookInfo.SetUp && listen == m.set.hookInfo.Listen {
		f.problem(m.text.Lookup(textProblemHookSame), m.text.Lookup(textFormFix))
		return nil
	}
	m.askHookSetup(listen)
	return nil
}

// askHookSetup asks to set the receiver up, or to move it when it is set up already. An empty listen leaves the
// address to the command: the default at a first setup, else the stored one.
func (m *Model) askHookSetup(listen string) {
	t, shown := m.text, listen
	if shown == "" {
		shown = m.hookListenOf()
	}
	title, ask, what := textTitleHook, textAskHookSetup, textWhatHookSetup
	if m.set.hookInfo.SetUp {
		ask, what = textAskHookListen, textWhatHookListen
	}
	consequence := t.Lookup(what)
	if !hookListenLocal(shown) {
		consequence += " " + t.Lookup(textHookProxy)
	}
	args := []string{"hook-service", "setup"}
	if listen != "" {
		args = append(args, "--listen", listen)
	}
	args = append(args, "--confirm", "--yes", "--output", "json")
	m.askSetting(settingChange{title: title, question: t.Lookup(ask, kittui.Escape(shown)), consequence: consequence,
		op: opHookSetup, name: listen,
		direct: func(b Backend) (any, error) { return b.HookServiceSetup(m.ctx, listen) },
		args:   args, decode: decodeHookSetup})
}

// decodeHookSetup reads what `hook-service setup` prints.
func decodeHookSetup(r sudoRun) (any, bool) {
	var v struct {
		SetUp  bool   `json:"set_up"`
		Listen string `json:"listen"`
	}
	if !r.decode(&v) || !v.SetUp {
		return nil, false
	}
	return HookSetupResult{Listen: v.Listen}, true
}
