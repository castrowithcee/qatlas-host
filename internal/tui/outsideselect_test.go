package tui

import (
	"fmt"
	"github.com/castrowithcee/qatlas-kit/i18n"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	kittui "github.com/castrowithcee/qatlas-kit/tui"
)

func TestAClickOutsideTheHelpClosesItLikeEsc(t *testing.T) {
	m, _ := newMouseModel(t)
	send(m, "?")
	m.render()
	clickAt(m, 0, 0)
	if m.helpOpen {
		t.Fatal("a click outside left the help open")
	}
}

func TestAClickOutsideAQuestionAnswersLikeEscAndNeverADanger(t *testing.T) {
	m, b := newMouseModel(t)
	send(m, "a")
	m.render()
	clickAt(m, 0, 0)
	if m.modal != nil || len(b.actions()) != 0 {
		t.Fatalf("outside click: modal=%v calls=%v", m.modal != nil, b.actions())
	}
	m.ask(textAnswerYes, "Remove the thing?", "It is gone.", nil, true, func() tea.Cmd { t.Fatal("answered"); return nil })
	m.render()
	clickAt(m, 0, 0)
	if m.modal == nil {
		t.Fatal("a click outside answered a destructive question")
	}
}

func TestADialogWithBackAsItsAnswerGetsCloseOutside(t *testing.T) {
	m, _ := newMouseModel(t)
	d, err := newDialog(kittui.DialogSpec{Title: textAnswerYes, Question: "Name?", Input: &kittui.DialogInput{},
		Answers: []kittui.Answer{{Action: actEnter, Label: textAnswerYes}, {Action: actBack, Label: textAnswerCancel}}}, m.keys, m.text)
	if err != nil {
		t.Fatal(err)
	}
	m.modal = &modal{kind: modalAsk, dialog: d}
	m.render()
	res := kittui.MouseResult{Handled: true, Zone: kittui.ZoneOutside}
	if got, _ := d.Click(res); got != actBack {
		t.Fatalf("outside answer = %q", got)
	}
}

func TestDraggingOverTextCopiesItAndAClickWithoutMotionActsAsBefore(t *testing.T) {
	m, _ := newMouseModel(t)
	x, y := cellOf(t, m, "demo-stack")
	m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	m.Update(tea.MouseMotionMsg{X: x + 5, Y: y, Button: tea.MouseLeft})
	_, cmd := m.Update(tea.MouseReleaseMsg{X: x + 5, Y: y, Button: tea.MouseLeft})
	if cmd == nil {
		t.Fatal("no clipboard command after a drag")
	}
	if got := fmt.Sprint(cmd()); !strings.HasPrefix(got, "demo-s") {
		t.Fatalf("clipboard = %q", got)
	}
	m.render()
	if m.area != areaStacks {
		t.Fatal("a drag acted as a click")
	}
	press(m, "j")
	if m.sel.Holds() {
		t.Fatal("a key left the selection held")
	}
}

func TestWithTheMouseOffAClickOutsideAndADragDoNothing(t *testing.T) {
	m, _ := newMouseModel(t)
	send(m, "?")
	m.render()
	m.mouseOn = false
	m.render()
	clickAt(m, 0, 0)
	m.Update(tea.MouseClickMsg{X: 20, Y: 10, Button: tea.MouseLeft})
	m.Update(tea.MouseMotionMsg{X: 30, Y: 10, Button: tea.MouseLeft})
	if _, cmd := m.Update(tea.MouseReleaseMsg{X: 30, Y: 10, Button: tea.MouseLeft}); cmd != nil || !m.helpOpen {
		t.Fatal("the mouse acted although it is off")
	}
}

// dragFrom presses at the first cell of needle, moves n cells right and releases; it returns the text handed to
// the clipboard, empty where nothing was.
func dragFrom(t *testing.T, m *Model, needle string, n int) string {
	t.Helper()
	x, y := cellOf(t, m, needle)
	m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	m.Update(tea.MouseMotionMsg{X: x + n, Y: y, Button: tea.MouseLeft})
	_, cmd := m.Update(tea.MouseReleaseMsg{X: x + n, Y: y, Button: tea.MouseLeft})
	if cmd == nil {
		return ""
	}
	return fmt.Sprint(cmd())
}

func TestDraggingInALogViewCopiesTheShownText(t *testing.T) {
	m := followed(t, i18n.English, demoBackend())
	m.mouseOn = true
	fillLog(m.log.view.LogView, 60)
	m.render()
	if got := dragFrom(t, m, "filler line", 10); !strings.HasPrefix(got, "filler line") {
		t.Fatalf("run log clipboard = %q", got)
	}
	c := openAppLog(t, clogBackend())
	c.mouseOn = true
	fillLog(c.clog.view.LogView, 100)
	c.render()
	if got := dragFrom(t, c, "filler line", 10); !strings.HasPrefix(got, "filler line") {
		t.Fatalf("container log clipboard = %q", got)
	}
}

func TestDraggingInAFormCopiesTheShownValueAndKeepsTheFormOpen(t *testing.T) {
	m := newFormMouseModel(t, secretFormKeysFromStart()...)
	form := m.set.form
	if got := dragFrom(t, m, "SERVICE_TOKEN", 6); got != "SERVICE" {
		t.Fatalf("form clipboard = %q", got)
	}
	if m.set.form != form || m.modal != nil {
		t.Fatal("a drag closed or answered the form")
	}
	if strings.Contains(fmt.Sprint(m.render()), hiddenA) && dragFrom(t, m, hiddenA, 3) != "" {
		t.Fatal("a masked value was copied")
	}
}

func TestDraggingInADialogAndInTheHelpCopiesAndKeepsThemOpen(t *testing.T) {
	m, _ := newMouseModel(t)
	send(m, "a")
	m.render()
	if got := dragFrom(t, m, "Starts a run", 11); got != "Starts a run" || m.modal == nil {
		t.Fatalf("dialog: clipboard %q, open %v", got, m.modal != nil)
	}
	send(m, "n")
	send(m, "?")
	m.render()
	if got := dragFrom(t, m, "qhost runs as", 12); got != "qhost runs as" || !m.helpOpen {
		t.Fatalf("help: clipboard %q, open %v", got, m.helpOpen)
	}
}

func TestAClickInsideTheFrameOnNoTargetClosesNeitherHelpNorDialog(t *testing.T) {
	m, _ := newMouseModel(t)
	send(m, "a")
	m.render()
	x, y := cellOf(t, m, "Starts a run")
	clickAt(m, x-1, y) // the padding between the border and the text
	if m.modal == nil {
		t.Fatal("a click inside the dialog frame closed it")
	}
	send(m, "n")
	send(m, "?")
	m.render()
	x, y = cellOf(t, m, "qhost runs as")
	clickAt(m, x-1, y)
	if !m.helpOpen {
		t.Fatal("a click inside the help frame closed it")
	}
}

func TestAClickWithoutMotionActsAsBefore(t *testing.T) {
	m, _ := newMouseModel(t)
	x, y := cellOf(t, m, "mail-relay")
	m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	if marked(t, m.list) == "mail-relay" {
		t.Fatal("the press acted before its release")
	}
	m.Update(tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft})
	if marked(t, m.list) != "mail-relay" {
		t.Fatal("a click without motion did not mark the row")
	}
	x, y = cellOf(t, m, "Runs")
	clickAt(m, x, y)
	if m.area != areaRuns {
		t.Fatalf("a click on a tab left area %s", m.area)
	}
}
