package tui

import (
	"slices"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-kit/i18n"
	kittui "github.com/castrowithcee/qatlas-kit/tui"
)

type commandState struct {
	name    string
	backend func() *fakeBackend
	keys    []string
	// text is set where a text field has the keys, so the colon is typed there.
	text bool
}

// commandStates are the views the command line is tried in: every area, its pages, the lists and the opened
// parts, a log and a form.
func commandStates() []commandState {
	states := []commandState{
		{"stacks", foldersBackend, []string{"down"}, false}, {"stack folder", foldersBackend, []string{"end"}, false},
		{"detail", detailGrantBackend, []string{"down", "enter"}, false}, {"detail form", detailGrantBackend, []string{"down", "enter", "b"}, false},
		{"detail text field", detailGrantBackend, []string{"down", "enter", "n"}, true},
		{"runs", svcBackend, []string{"3"}, false}, {"run log", svcBackend, []string{"3", "enter"}, false},
		{"service log", svcBackend, []string{"3", "v"}, false},
		{"containers", containersBackend, []string{"5"}, false}, {"container", containersBackend, []string{"5", "enter"}, false},
		{"container log", clogBackend, []string{"5", "o"}, false},
		{"proposals", proposalBackend, []string{"6"}, false}, {"proposal", proposalBackend, []string{"6", "enter"}, false},
	}
	// The tab line and the sub-tab line have the focus.
	states = append(states, commandState{"stacks tabs", foldersBackend, []string{"up"}, false},
		commandState{"runs tabs", svcBackend, []string{"3", "up"}, false},
		commandState{"system sub-tabs", demoBackend, []string{"7", "up"}, false},
		commandState{"system tabs", demoBackend, []string{"7", "up", "up"}, false},
		commandState{"settings sub-tabs", settingsBackend, []string{"4", "up", "right"}, false})
	for area, backend := range map[string]func() *fakeBackend{"4": settingsBackend, "7": demoBackend} {
		for _, page := range []int{1, 2} {
			keys := append([]string{area, "up"}, slices.Repeat([]string{"right"}, page)...)
			states = append(states, commandState{"page " + area + "/" + string(rune('0'+page)), backend, append(keys, "down"), false})
		}
	}
	// The file browser of System > Folders and the preview of a text file, over the invented tree.
	browser := []string{"7", "esc", "right", "right", "down"}
	preview := slices.Clone(browser)
	for _, name := range []string{"etc", "demo-app", "app.yaml"} {
		preview = append(append(append(preview, "/"), typed(name)...), "enter", "enter")
	}
	states = append(states, commandState{"file browser", demoBackend, browser, false},
		commandState{"file preview", demoBackend, preview, false})
	// The browser over a stack folder and the masked preview of its secrets.env, over the invented stack folder.
	secrets := []string{"up", "Q"}
	for _, name := range []string{"demo-stack", "secrets.env"} {
		secrets = append(append(append(secrets, "/"), typed(name)...), "enter", "enter")
	}
	return append(states, commandState{"stack file browser", stackFilesBackend, []string{"up", "Q"}, false},
		commandState{"stack secrets preview", stackFilesBackend, secrets, false})
}

// release drops the timer record the test models keep in a global map, which would hold every model of the
// matrix until the end of the run.
func release(ms ...*Model) {
	for _, m := range ms {
		delete(timersOf, m)
	}
}

func commandModel(t *testing.T, lang i18n.Lang, st commandState) *Model {
	t.Helper()
	m := newTestModel(t, lang, st.backend(), wideWidth, 30)
	// System > Folders and the stack folders browse the invented trees, not the machine.
	fakeHostFolders(m, 1)
	fakeStackFiles(m)
	send(m, st.keys...)
	return m
}

// rendered is the screen without the message line, which a refused key sets.
func rendered(m *Model) string {
	m.notice, m.result, m.keep = "", "", ""
	return m.render()
}

// runCommand types the name of a command into the open command line and runs it.
func runCommand(m *Model, name string) {
	for _, k := range append(append([]string{":"}, strings.Split(name, "")...), "enter") {
		_, cmd := m.Update(keyMsg(k))
		drain(m, cmd)
	}
	m.render()
}

func TestColonOpensTheCommandLineInEveryView(t *testing.T) {
	for _, st := range commandStates() {
		m := commandModel(t, i18n.English, st)
		send(m, ":")
		if open := m.cmdline != nil; open == st.text {
			t.Errorf("%s: command line open %v, text field has the keys %v", st.name, open, st.text)
		}
		if st.text {
			continue
		}
		if !strings.Contains(m.render(), "Commands") {
			t.Errorf("%s: no command line on the screen:\n%s", st.name, m.render())
		}
		send(m, "esc")
		if m.cmdline != nil {
			t.Errorf("%s: esc left the command line open", st.name)
		}
	}
}

func TestCommandLineBuildsInBothLanguages(t *testing.T) {
	for name, lang := range goldenLangs {
		m := newTestModel(t, lang, demoBackend(), wideWidth, 30)
		send(m, ":")
		if m.cmdline == nil || m.failure != "" {
			t.Errorf("%s: command line not built: %q", name, m.failure)
		}
	}
}

// A command runs through the key of its action: the screen after the command is the screen after the key. Where
// the view does not offer the action, its key changes nothing but the message line.
func TestCommandsActWhereTheirKeyActs(t *testing.T) {
	saveInto(t)
	saveDiagnoseTo(t, nil)
	for _, st := range commandStates() {
		if st.text {
			continue
		}
		probe := commandModel(t, i18n.English, st)
		send(probe, ":")
		commands := probe.cmdline.Matches()
		release(probe)
		if len(commands) != len(areas())+len(commandRows) {
			t.Fatalf("%s: %d commands listed", st.name, len(commands))
		}
		// A key that changes nothing leaves the model as it was, so the next key tries the same screen.
		var byKey *Model
		for _, c := range commands {
			if c.Area != "" {
				continue
			}
			ok, reason := c.Available(kittui.Context{})
			if byKey == nil {
				byKey = commandModel(t, i18n.English, st)
			}
			before := rendered(byKey)
			key, found := byKey.keyOf(c.Action)
			if !found {
				if ok {
					t.Errorf("%s: %q is available but its action has no key here", st.name, c.Name)
				}
				continue
			}
			send(byKey, key.String())
			if !ok {
				if reason == "" {
					t.Errorf("%s: %q is unavailable without a reason", st.name, c.Name)
				}
				if rendered(byKey) != before {
					t.Errorf("%s: %q is unavailable, but its key acts:\n%s", st.name, c.Name, byKey.render())
					release(byKey)
					byKey = nil
				}
				continue
			}
			byLine := commandModel(t, i18n.English, st)
			runCommand(byLine, c.Name)
			if byLine.cmdline != nil || rendered(byLine) != rendered(byKey) {
				t.Errorf("%s: %q differs from its key:\n%s\nkey:\n%s", st.name, c.Name, rendered(byLine), rendered(byKey))
			}
			release(byKey, byLine)
			byKey = nil
		}
		release(byKey)
	}
}

// An area opens as its tab does, and the line is closed for the screen it opens.
func TestAreaCommandOpensTheArea(t *testing.T) {
	m := newTestModel(t, i18n.English, demoBackend(), wideWidth, 30)
	runCommand(m, "runs")
	if m.cmdline != nil || m.area != areaRuns {
		t.Fatalf("area %s, command line open %v", m.area, m.cmdline != nil)
	}
	runCommand(m, "stacks")
	if m.area != areaStacks {
		t.Fatalf("area %s", m.area)
	}
}

// Where the view does not offer an action, the line stays open and says where to go.
func TestUnavailableCommandNamesTheNextStep(t *testing.T) {
	m := newTestModel(t, i18n.English, demoBackend(), wideWidth, 30)
	runCommand(m, "rollback")
	if m.cmdline == nil || !strings.Contains(m.render(), "rollback: open the area Runs first") {
		t.Fatalf("no refusal:\n%s", m.render())
	}
	send(m, "esc")
	runCommand(m, "approve")
	send(m, "esc")
	m = newTestModel(t, i18n.German, demoBackend(), wideWidth, 30)
	runCommand(m, "zurücksetzen")
	if !strings.Contains(m.render(), "zurücksetzen: öffne zuerst den Bereich Läufe") {
		t.Fatalf("no German refusal:\n%s", m.render())
	}
}

// A command that asks, asks as its key does, and n answers it.
func TestCommandAsksAsItsKeyDoes(t *testing.T) {
	m := newTestModel(t, i18n.English, demoBackend(), wideWidth, 30)
	runCommand(m, "update")
	if m.modal == nil {
		t.Fatalf("no question:\n%s", m.render())
	}
	send(m, "n")
	if m.modal != nil {
		t.Fatal("n did not answer the question")
	}
}

func TestGoldenCommandLine(t *testing.T) {
	goldenScreens(t, "commandline", goldenRows, demoBackend, ":")
	goldenScreens(t, "commandline.typed", goldenRows, demoBackend, ":", "r", "e", "s")
}
