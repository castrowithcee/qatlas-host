package tui

import (
	"testing"

	kittui "github.com/castrowithcee/qatlas-kit/tui"
	"github.com/castrowithcee/qatlas-kit/uiprefs"
)

func TestNavigationSchemaOffersAllFormsWithTabsAsDefault(t *testing.T) {
	s, err := PrefsSchema()
	if err != nil {
		t.Fatal(err)
	}
	if err := uiprefs.Validate(uiprefs.Default(s), s); err != nil {
		t.Fatal(err)
	}
	if len(s.Navigations) != 4 || s.DefaultNavigation != uiprefs.NavigationTabs {
		t.Fatalf("navigations %v default %q", s.Navigations, s.DefaultNavigation)
	}
}

func TestEveryNavigationFormBuildsAndShowsTheArea(t *testing.T) {
	for _, name := range navigations() {
		m := generalModel(t, &savedPrefs{})
		m.prefs.Navigation = name
		if got := m.frameSpec().Nav; got != navForm(name) {
			t.Fatalf("%s: frame form %v", name, got)
		}
		contains(t, m, "Stacks")
	}
	if navForm("") != kittui.NavTabs {
		t.Fatal("an empty form is not the tab line")
	}
}

func TestGeneralSavesTheNavigationAtOnce(t *testing.T) {
	s := &savedPrefs{}
	m := generalModel(t, s)
	send(m, toGeneral...)
	contains(t, m, "Navigation")
	send(m, "down", "down", "down", "down", "down") // navigation
	send(m, "right")
	send(m, save...)
	if len(s.calls) != 1 || s.calls[0].Navigation != uiprefs.NavigationPathLine {
		t.Fatalf("saved %+v", s.calls)
	}
	if m.prefs.Navigation != uiprefs.NavigationPathLine || m.frameSpec().Nav != kittui.NavPathLine {
		t.Fatalf("not in force: %q", m.prefs.Navigation)
	}
}
