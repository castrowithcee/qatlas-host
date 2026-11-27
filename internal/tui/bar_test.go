package tui

import (
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-kit/i18n"
)

func TestTheFileBrowserShowsItsActionsInTheBarAndAClickRunsThem(t *testing.T) {
	m, _ := systemModel(t, i18n.English, wideWidth, 40)
	fakeHostFolders(m, 42)
	toFolderPage(t, m)
	m.mouseOn = true
	screen := m.render()
	if !strings.Contains(screen, "[ n new ]") || !strings.Contains(screen, "[ e edit ]") {
		t.Fatalf("the bar does not show the actions of the browser\n%s", screen)
	}
	if strings.Contains(screen, "esc up") {
		t.Fatalf("the browser still draws a button below its list\n%s", screen)
	}
	lines := strings.Split(screen, "\n")
	if strings.Contains(strings.Join(lines[len(lines)-3:], "\n"), "n new") {
		t.Fatalf("the footer repeats an action of the bar\n%s", screen)
	}
	if formOpen(m) {
		t.Fatal("a form is open before the click")
	}
	clickOn(t, m, "[ n new ]")
	if !formOpen(m) {
		t.Fatalf("a click on New did not open the form the key opens\n%s", m.render())
	}
}
