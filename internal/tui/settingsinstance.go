package tui

import (
	"context"
	"slices"

	tea "charm.land/bubbletea/v2"

	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// Credentials and notification channels belong to one deploy service each. With several services the pages for
// them name the instance they show and write to; they send nothing to another one. Which one the page starts with
// follows the rule of the commands: the system engine when it has a service, else the person chooses.

// instances are the deploy services the pages can show, by engine id; fewer than two need no choice.
func (m *Model) instances() []string {
	if e := m.engines(); len(e) > 1 {
		return e
	}
	return nil
}

// perInstance reports whether the page holds the settings of one deploy service.
func perInstance(p settingsPage) bool { return p == pageCreds || p == pageNotify }

// chooses reports whether the person chooses an instance on the page, as the keys of the page act on it. The window
// page shows every instance and uses the choice only for the own window.
func chooses(p settingsPage) bool { return perInstance(p) || p == pageWindow }

// settingsEngine is the instance the page shows: empty with one deploy service, and also empty while several
// exist and none is chosen (see instanceChosen).
func (m *Model) settingsEngine() string {
	inst := m.instances()
	if inst == nil {
		return ""
	}
	if e := m.set.engine; slices.Contains(inst, e) {
		return e
	}
	if slices.Contains(inst, control.EngineSystem) {
		return control.EngineSystem
	}
	return ""
}

// instanceChosen reports whether the page has an instance to act on: always with one deploy service.
func (m *Model) instanceChosen() bool {
	return m.instances() == nil || m.settingsEngine() != ""
}

// settingsCtx is the context of a call of the page: it names the instance when there are several.
func (m *Model) settingsCtx() context.Context { return m.engineCtx(m.settingsEngine()) }

// settingsArgs adds the instance to the command the page hands to sudo.
func (m *Model) settingsArgs(args []string) []string { return m.engineArgs(m.settingsEngine(), args) }

// needInstance warns that the instance must be chosen first and reports whether it must.
func (m *Model) needInstance() bool {
	if m.instanceChosen() {
		return false
	}
	m.warn(m.text.Lookup(textInstanceChoose))
	return true
}

// nextInstance chooses the next instance and reads the page again for it.
func (m *Model) nextInstance() tea.Cmd {
	inst := m.instances()
	if inst == nil || !chooses(m.set.page) || m.settingsBusy() {
		return nil
	}
	at := slices.Index(inst, m.settingsEngine())
	s := m.set
	s.engine = inst[(at+1)%len(inst)]
	if m.set.page == pageWindow {
		return nil
	}
	s.creds, s.notify = part{}, part{}
	m.setCredentials(nil)
	m.setChannels(nil)
	return m.loadPage(true)
}

// instanceLines are the lines above a page with the settings of one instance: which one it shows.
func (m *Model) instanceLines(width int) []string {
	if m.instances() == nil || !chooses(m.set.page) {
		return nil
	}
	t := m.text
	shown := m.settingsEngine()
	if shown == "" {
		return m.wrapped(2, kittui.StyleWarn, t.Lookup(textInstanceChooseIn), width)
	}
	return []string{m.line(2, kittui.StyleActive, t.Lookup(textInstanceLine, shown), width)}
}

// instanceHints is the key that chooses the instance, only where there is a choice.
func (m *Model) instanceHints() []kittui.Hint {
	if m.instances() == nil {
		return nil
	}
	return []kittui.Hint{{Action: actInstance, Label: textKeyInstance, Priority: 3}}
}
