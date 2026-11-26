package tui

import (
	"slices"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-kit/i18n"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// twoWindowsBackend has two instances: the system one follows the host window, the one of alice has an own window.
func twoWindowsBackend() *fakeBackend {
	b := twoEnginesBackend()
	b.window = control.WindowResult{Days: []string{"sun"}, Start: "03:00", Duration: "2h", Zone: "UTC", Origin: control.WindowOriginHost}
	host := b.window
	host.NextOpening = at(31, 3, 0)
	own := control.WindowResult{Days: []string{"mon", "thu"}, Start: "04:30", Duration: "1h", Zone: "UTC",
		Origin: control.WindowOriginOwn, NextOpening: at(30, 4, 30), LastRun: at(27, 4, 30)}
	b.windowBy = map[string]control.WindowResult{control.EngineSystem: host, "rootless:alice": own}
	return b
}

var toWindowPage = keys("4", "up", "right", "right", "down")

func TestGoldenSettingsWindowOfTwoInstances(t *testing.T) {
	goldenScreens(t, "settings.window.instances", 30, func() *fakeBackend {
		b := twoWindowsBackend()
		w := b.windowBy[control.EngineSystem]
		w.Warning = "the host window file is damaged; the default window applies"
		b.windowBy[control.EngineSystem] = w
		return b
	}, toWindowPage...)
}

func TestWindowPageShowsTheOriginOfEachInstance(t *testing.T) {
	for _, tt := range []struct {
		lang        i18n.Lang
		host, own   string
		instanceKey string
	}{{i18n.English, "host window", "own window", "Instance: system"}, {i18n.German, "Host-Fenster", "eigenes Fenster", "Instanz: system"}} {
		m := newTestModel(t, tt.lang, twoWindowsBackend(), wideWidth, 30)
		m.euid = func() int { return 0 }
		send(m, toWindowPage...)
		screen := m.render()
		for _, want := range []string{tt.host, tt.own, "rootless:alice", tt.instanceKey} {
			if !strings.Contains(screen, want) {
				t.Errorf("%v: the page lacks %q:\n%s", tt.lang, want, screen)
			}
		}
	}
}

func TestWindowPageWithOneInstanceStaysPlainWithItsOrigin(t *testing.T) {
	m := newSettingsModel(t, settingsBackend())
	send(m, toWindow...)
	screen := m.render()
	if !strings.Contains(screen, "host window") || strings.Contains(screen, "Instance:") || strings.Count(screen, "days:") != 1 {
		t.Fatalf("not a plain page:\n%s", screen)
	}
}

func TestWindowPageShowsTheWarningAndAnInstanceThatCannotBeRead(t *testing.T) {
	b := twoWindowsBackend()
	w := b.windowBy[control.EngineSystem]
	w.Warning = "host window file is damaged"
	b.windowBy[control.EngineSystem] = w
	b.errs = map[string]error{"window:rootless:alice": &control.Error{Code: control.CodeInternal, Message: "service down"}}
	m := newTestModel(t, i18n.English, b, wideWidth, 30)
	send(m, toWindowPage...)
	contains(t, m, "Warning: host window file is damaged", "service down")
}

// The host window change reaches no deploy service; the own window change reaches the chosen one only.
func TestWindowChangesAsRootGoToTheHostFileOrTheChosenInstance(t *testing.T) {
	b := twoWindowsBackend()
	m := newTestModel(t, i18n.English, b, wideWidth, 30)
	m.euid = func() int { return 0 }
	send(m, toWindowPage...)
	send(m, "e", "tab", "backspace", "backspace", "backspace", "backspace", "backspace")
	send(m, typed("05:00")...)
	send(m, "f2", "y")
	if len(b.hostWindows) != 1 || b.hostWindows[0].Start != "05:00" || len(b.windows) != 0 {
		t.Fatalf("host %+v, own %+v", b.hostWindows, b.windows)
	}
	contains(t, m, "The maintenance window is changed.")
	// The system instance follows the host, the own one of alice stays.
	if b.windowBy[control.EngineSystem].Start != "05:00" || b.windowBy["rootless:alice"].Start != "04:30" {
		t.Fatalf("windows %+v", b.windowBy)
	}
	send(m, "enter", "H", "n")
	contains(t, m, "Own window of rootless:alice")
	send(m, "tab", "backspace", "backspace", "backspace", "backspace", "backspace")
	send(m, typed("06:15")...)
	send(m, "f2")
	contains(t, m, "Set the own window of rootless:alice?")
	send(m, "y")
	if len(b.windows) != 1 || b.windows[0].Start != "06:15" || len(b.hostWindows) != 1 {
		t.Fatalf("host %+v, own %+v", b.hostWindows, b.windows)
	}
	if !slices.Contains(b.engineLog, "windowset:@rootless:alice") {
		t.Fatalf("log %v", b.engineLog)
	}
	for _, e := range b.engineLog {
		if strings.HasPrefix(e, "windowset:") && !strings.HasSuffix(e, "@rootless:alice") {
			t.Fatalf("the own window went elsewhere: %v", b.engineLog)
		}
	}
	send(m, "enter")
	if b.windowBy["rootless:alice"].Start != "06:15" || b.windowBy[control.EngineSystem].Start != "05:00" {
		t.Fatalf("windows %+v", b.windowBy)
	}
}

func TestWindowResetOnlyForAnInstanceWithAnOwnWindow(t *testing.T) {
	b := twoWindowsBackend()
	m := newTestModel(t, i18n.English, b, wideWidth, 30)
	m.euid = func() int { return 0 }
	send(m, toWindowPage...)
	send(m, "d")
	contains(t, m, "system has no own window and follows the host window.")
	if m.modal != nil {
		t.Fatal("a reset was asked for an instance without an own window")
	}
	send(m, "H", "d")
	contains(t, m, "Let rootless:alice follow the host window again?")
	send(m, "y")
	if !slices.Contains(b.engineLog, "windowreset:@rootless:alice") {
		t.Fatalf("log %v", b.engineLog)
	}
	send(m, "enter")
	if b.windowBy["rootless:alice"].Origin != control.WindowOriginHost {
		t.Fatalf("windows %+v", b.windowBy)
	}
	if strings.Count(m.render(), "host window ─") != 2 {
		t.Fatalf("both instances should follow the host:\n%s", m.render())
	}
}

func TestWindowOwnActionsNeedAChosenInstanceWhenNothingIsTheDefault(t *testing.T) {
	b := rootlessOnlyBackend()
	m := newTestModel(t, i18n.English, b, wideWidth, 30)
	m.euid = func() int { return 0 }
	send(m, toWindowPage...)
	send(m, "n")
	if m.set.form != nil {
		t.Fatal("a form opened without an instance")
	}
	send(m, "e")
	if m.set.form == nil {
		t.Fatal("the host window form needs no instance")
	}
}

// Without root every change is the command through sudo: no --engine for the host window, the instance named for an
// own window and for the reset, also with a single instance.
func TestWindowChangesWithoutRootRunTheCommands(t *testing.T) {
	for _, tt := range []struct {
		name    string
		backend func() *fakeBackend
		keys    []string
		args    []string
	}{
		{"host", twoWindowsBackend, seq(toWindowPage, keys("e", "tab", "backspace", "backspace", "backspace", "backspace", "backspace"),
			typed("05:00"), keys("f2")), []string{"window", "set", "--start", "05:00", "--output", "json"}},
		{"own", twoWindowsBackend, seq(toWindowPage, keys("H", "n", "tab", "backspace", "backspace", "backspace", "backspace", "backspace"),
			typed("05:00"), keys("f2")), []string{"window", "set", "--start", "05:00", "--output", "json", "--engine", "rootless:alice"}},
		{"reset", twoWindowsBackend, seq(toWindowPage, keys("H", "d")),
			[]string{"window", "reset", "--output", "json", "--engine", "rootless:alice"}},
		{"own of the only instance", settingsBackend, seq(toWindow, keys("n", "tab", "backspace", "backspace", "backspace", "backspace", "backspace"),
			typed("05:00"), keys("f2")), []string{"window", "set", "--start", "05:00", "--output", "json", "--engine", "system"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := tt.backend()
			m, f := sudoDetailModel(t, i18n.English, b, wideWidth, 40)
			f.report = `{"origin":"own","days":"sun","start":"05:00","duration":"2h","zone":"UTC","open":false,"next_opening":"2026-11-01T05:00:00Z"}`
			send(m, tt.keys...)
			if command := "sudo qhost " + strings.Join(tt.args, " "); !shows(m.render(), command) {
				t.Fatalf("the question lacks %q:\n%s", command, m.render())
			}
			send(m, "y")
			if got := commandsOf(f); len(got) != 1 || !slices.Equal(got[0], tt.args) {
				t.Fatalf("commands %v, want %v", got, tt.args)
			}
			if len(b.windows) != 0 || len(b.hostWindows) != 0 {
				t.Fatalf("the service was called directly: %+v %+v", b.windows, b.hostWindows)
			}
		})
	}
}

func TestGoldenSudoWindowOfTwoInstancesQuestion(t *testing.T) {
	goldenSudoSettings(t, "settings.window.instances.sudo", twoWindowsBackend,
		seq(toWindowPage, keys("H", "n", "tab", "backspace", "backspace", "backspace", "backspace", "backspace"), typed("04:45"), keys("f2"))...)
}
