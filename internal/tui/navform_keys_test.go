package tui

import (
	"testing"

	"github.com/castrowithcee/qatlas-kit/i18n"
	kittui "github.com/castrowithcee/qatlas-kit/tui"
	"github.com/castrowithcee/qatlas-kit/uiprefs"
)

// formModel is a started model on the form, with the mouse on.
func formModel(t *testing.T, form string) *Model {
	t.Helper()
	m := newTestModel(t, i18n.English, demoBackend(), wideWidth, goldenRows)
	m.prefs.Navigation, m.mouseOn = form, true
	m.setZone(kittui.ZoneTabs)
	m.render()
	return m
}

func TestAreasPassThePathNavigationCheck(t *testing.T) {
	m := formModel(t, uiprefs.NavigationPathLine)
	nav, err := kittui.NewPathNav(pathNavKeys(), m.keys, m.text)
	if err != nil {
		t.Fatal(err)
	}
	if err := nav.CheckAreas(areas()); err != nil {
		t.Fatal(err)
	}
}

func TestPathFormsMoveTheFocusBetweenTheNavigationAndTheWork(t *testing.T) {
	for _, form := range []string{uiprefs.NavigationPathLine, uiprefs.NavigationPathHeader} {
		m := formModel(t, form)
		if f := m.frameSpec(); f.Focus != kittui.ZonePath || !f.PathFocusArea || f.Nav != navForm(form) {
			t.Fatalf("%s: frame focus %s, area %v", form, f.Focus, f.PathFocusArea)
		}
		send(m, "down")
		wantZone(t, m, kittui.ZoneRows)
		if m.frameSpec().Focus != kittui.ZoneRows {
			t.Fatalf("%s: the frame keeps the path focus", form)
		}
		send(m, "up")
		wantZone(t, m, kittui.ZoneTabs)
		send(m, "4", "up") // settings: its tab section
		send(m, "esc")
		if m.frameSpec().Focus != kittui.ZonePath {
			t.Fatalf("%s: no path focus in settings", form)
		}
	}
}

func TestPathFocusStepsBetweenTheSectionsAndEnterOpensThem(t *testing.T) {
	m := formModel(t, uiprefs.NavigationPathLine)
	send(m, "4")
	m.setZone(kittui.ZoneTabs)
	send(m, "right")
	wantZone(t, m, kittui.ZoneSubtabs)
	if m.frameSpec().PathFocusArea {
		t.Fatal("the focus is on the area section")
	}
	send(m, "enter")
	if !m.navOpen() {
		t.Fatal("enter on the tab section opens nothing")
	}
	send(m, "esc")
	send(m, "left")
	wantZone(t, m, kittui.ZoneTabs)
	send(m, "m")
	if !m.navOpen() {
		t.Fatal("m opens no overview")
	}
	send(m, "esc")
	if m.navOpen() || m.area != areaSettings {
		t.Fatalf("esc: open %v, area %s", m.navOpen(), m.area)
	}
}

func TestChoiceFormEnterOpensTheAreasAndLeftRightStepThem(t *testing.T) {
	m := formModel(t, uiprefs.NavigationHeaderChoice)
	if f := m.frameSpec(); f.Focus != kittui.ZoneTabs || f.Nav != kittui.NavHeaderChoice {
		t.Fatalf("frame focus %s", f.Focus)
	}
	send(m, "enter")
	if !m.navOpen() {
		t.Fatal("enter on the field opens no areas")
	}
	send(m, "esc")
	send(m, "right")
	if m.area != areaDetail {
		t.Fatalf("right: area %s", m.area)
	}
	send(m, "m")
	if !m.navOpen() {
		t.Fatal("m opens no overview")
	}
}

func TestTabsIgnoreTheMenuKeyAndEnterGoesDown(t *testing.T) {
	m := formModel(t, uiprefs.NavigationTabs)
	send(m, "m")
	if m.navOpen() {
		t.Fatal("m opens an overview on tabs")
	}
	send(m, "enter")
	wantZone(t, m, kittui.ZoneRows)
}

func TestAClickOnThePathOpensTheOverviewAndAChoiceSwitchesTheArea(t *testing.T) {
	m := formModel(t, uiprefs.NavigationPathLine)
	x, y := cellOf(t, m, "Stacks")
	clickAt(m, x, y)
	if !m.navOpen() {
		t.Fatal("the click on the area section opened nothing")
	}
	x, y = cellOf(t, m, "Runs")
	clickAt(m, x, y)
	if m.navOpen() || m.area != areaRuns {
		t.Fatalf("open %v, area %s", m.navOpen(), m.area)
	}
}

func TestAClickOnTheChoiceFieldOpensTheAreas(t *testing.T) {
	m := formModel(t, uiprefs.NavigationHeaderChoice)
	x, y := cellOf(t, m, "Stacks")
	clickAt(m, x, y)
	if !m.navOpen() {
		t.Fatal("the click on the field opened nothing")
	}
}

// The goldens of the navigation forms: the System area, whose sub-tab line makes the path two sections, with
// the focus on the navigation; narrow and wide, in English and German.
func TestGoldenNavigationForms(t *testing.T) {
	sizes := map[string]int{"narrow": narrowWidth, "wide": wideWidth}
	for _, form := range []string{uiprefs.NavigationPathLine, uiprefs.NavigationPathHeader, uiprefs.NavigationHeaderChoice} {
		for name, lang := range goldenLangs {
			for size, width := range sizes {
				t.Run(form+"."+name+"."+size, func(t *testing.T) {
					m := newTestModel(t, lang, demoBackend(), width, goldenRows)
					m.prefs.Navigation = form
					send(m, "7")
					m.setZone(kittui.ZoneSubtabs)
					screen := m.render()
					checkScreen(t, screen, width, goldenRows)
					checkGolden(t, "navigation."+form+"."+name+"."+size+".golden", screen+"\n")
				})
			}
		}
	}
}

func TestTheMenuCommandOpensTheOverviewWhereMDoes(t *testing.T) {
	m := formModel(t, uiprefs.NavigationPathLine)
	runCommand(m, "menu")
	if !m.navOpen() {
		t.Fatal("the menu command opens no overview on the path line")
	}
	m = formModel(t, uiprefs.NavigationTabs)
	runCommand(m, "menu")
	if m.navOpen() {
		t.Fatal("the menu command opened an overview on the tab line, where m does nothing")
	}
}
