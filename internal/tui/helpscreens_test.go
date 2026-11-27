package tui

import (
	"slices"
	"strings"
	"testing"
)

func TestHelpScreenIDsPerKind(t *testing.T) {
	m := newActionModel(t, demoBackend())
	if got := m.helpScreens(); !slices.Equal(got, []string{"stacks"}) {
		t.Errorf("area: %v", got)
	}
	m.set.keyOpen = true
	if got := m.helpScreens(); !slices.Equal(got, []string{"stacks/key", "stacks"}) {
		t.Errorf("state: %v", got)
	}
	m.set.keyOpen = false
	send(m, "a")
	if got := m.helpScreens(); !slices.Equal(got, []string{"action:deploy", "stacks"}) {
		t.Errorf("stack action: %v", got)
	}
	send(m, "esc")

	m = newActionModel(t, demoBackend())
	send(m, "4")
	if got := m.helpScreens(); !slices.Equal(got, []string{"settings/general", "settings"}) {
		t.Errorf("tab: %v", got)
	}

	m = newSettingsModel(t, settingsBackend())
	send(m, toFolders...)
	send(m, "n")
	want := []string{"title:" + string(m.set.form.title), "stacks"}
	if got := m.helpScreens(); !slices.Equal(got, want) || !strings.HasPrefix(want[0], "title:title.") {
		t.Errorf("form: %v, want %v", got, want)
	}
}

func TestEveryActionKindHasAScreenID(t *testing.T) {
	for id, kind := range actionKinds {
		if got := kind.screen(); got != "action:"+string(id) {
			t.Errorf("%v: %q", id, got)
		}
	}
}

func TestHelpOpensOnTheStartPageWhenNoScreenHasOne(t *testing.T) {
	m := newActionModel(t, demoBackend())
	send(m, "?")
	if topic, name := m.help.Page(); topic != "start" || name != "index" {
		t.Errorf("page %s/%s, want start/index", topic, name)
	}
}

func TestF9OpensTheHelpOverADialogAndClosingReturnsToIt(t *testing.T) {
	m := newActionModel(t, demoBackend())
	send(m, "a")
	if m.modal == nil || m.modal.screen == "" {
		t.Fatalf("no dialog with a screen ID: %+v", m.modal)
	}
	send(m, "f9")
	if !m.helpOpen || m.modal == nil {
		t.Fatalf("help %v, dialog %v", m.helpOpen, m.modal)
	}
	send(m, "f9")
	if m.helpOpen || m.modal == nil {
		t.Fatalf("f9 did not close the help and keep the dialog: help %v, dialog %v", m.helpOpen, m.modal)
	}
}

func TestF9OpensTheHelpOverAFormAndKeepsTheInput(t *testing.T) {
	m := newSettingsModel(t, settingsBackend())
	send(m, toFolders...)
	send(m, "n", "a", "b", "?")
	if !m.formFocused() || m.helpOpen {
		t.Fatal("the question mark in a text field did not type")
	}
	contains(t, m, "ab?")
	send(m, "f9")
	if !m.helpOpen {
		t.Fatal("f9 did not open the help over the form")
	}
	send(m, "esc")
	if m.helpOpen || m.set.form == nil {
		t.Fatalf("closing the help left the form: help %v, form %v", m.helpOpen, m.set.form)
	}
	contains(t, m, "ab?")
}
