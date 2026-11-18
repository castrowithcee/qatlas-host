package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/castrowithcee/qatlas-kit/i18n"

	"github.com/castrowithcee/qatlas-host/internal/userfile"
)

// saveInto makes the model's saves go to a new home under a temporary folder, as root acting for the user of
// the test, and returns that home.
func saveInto(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	old := saveLogFile
	saveLogFile = func(name string, data []byte) (string, error) {
		p, err := userfile.Operator(userfile.Env{
			Getenv: func(k string) string {
				return map[string]string{"SUDO_UID": strconv.Itoa(os.Getuid()), "SUDO_GID": strconv.Itoa(os.Getgid())}[k]
			},
			Euid: func() int { return 0 },
			Lookup: func(uid int) (userfile.Account, error) {
				return userfile.Account{UID: uid, GID: os.Getgid(), Home: home}, nil
			},
		})
		if err != nil {
			return "", err
		}
		return p.Save(name, data)
	}
	t.Cleanup(func() { saveLogFile = old })
	return home
}

// savedFile is the only file in the folder qhost of home.
func savedFile(t *testing.T, home string) (name, content string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(home, "qhost"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("entries %v, %v", entries, err)
	}
	b, err := os.ReadFile(filepath.Join(home, "qhost", entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	return entries[0].Name(), string(b)
}

func TestSaveRunLogWritesOnlyTheLinesOfThePeriod(t *testing.T) {
	home := saveInto(t)
	m := filterModel(t, demoBackend(), wideWidth)
	lastFilter(m, 1, "2", 1) // 2 hours: the two lines after 16:00
	send(m, "W")
	name, content := savedFile(t, home)
	want := "2026-10-30 16:30:00 info middle line\n2026-10-30 17:50:00 info late line\n"
	if content != want {
		t.Fatalf("content %q, want %q", content, want)
	}
	if !strings.HasPrefix(name, "qhost-run-") || !strings.HasSuffix(name, "-20261030-180000.log") {
		t.Fatalf("name %s", name)
	}
	st, _ := os.Stat(filepath.Join(home, "qhost", name))
	if st.Mode().Perm() != 0o600 || int(st.Sys().(*syscall.Stat_t).Uid) != os.Getuid() {
		t.Fatalf("mode %v", st.Mode())
	}
	// The message wraps in the message line; the path is whole once the line breaks and their indent go.
	if screen := strings.Join(strings.Fields(m.render()), ""); !strings.Contains(screen, filepath.Join(home, "qhost", name)) {
		t.Fatalf("the screen lacks the path:\n%s", m.render())
	}
}

func TestSaveRunLogWithoutAPeriodWritesEveryLine(t *testing.T) {
	home := saveInto(t)
	m := filterModel(t, demoBackend(), wideWidth)
	send(m, "W", "W")
	entries, _ := os.ReadDir(filepath.Join(home, "qhost"))
	if len(entries) != 2 {
		t.Fatalf("%d files, want a new one each time", len(entries))
	}
	b, _ := os.ReadFile(filepath.Join(home, "qhost", entries[0].Name()))
	if strings.Count(string(b), "\n") != 4 || !strings.Contains(string(b), "2026-10-30 11:00:00 step check\n") {
		t.Fatalf("content %q", b)
	}
}

func TestSaveContainerLogWritesTheLinesWithTheirSource(t *testing.T) {
	home := saveInto(t)
	m := openAppLog(t, clogBackend())
	m.now = func() time.Time { return clogStart.Add(time.Hour) }
	send(m, "W")
	name, content := savedFile(t, home)
	if !strings.HasPrefix(name, "qhost-container-"+clogApp+"-") || !strings.Contains(content, "2026-10-30 17:00:00 app started on port 8080\n") ||
		strings.Count(content, "\n") != 3 {
		t.Fatalf("%s: %q", name, content)
	}
}

func TestSaveServiceLogWritesTheEntries(t *testing.T) {
	home := saveInto(t)
	m := openServiceLog(t, svcBackend())
	send(m, "W")
	name, content := savedFile(t, home)
	if !strings.HasPrefix(name, "qhost-service-qhost-") || !strings.Contains(content, "2026-10-30 17:00:00 INFO service started version=1.2.3\n") ||
		strings.Count(content, "\n") != 4 {
		t.Fatalf("%s: %q", name, content)
	}
}

func TestSaveLogEscapesControlCharacters(t *testing.T) {
	home := saveInto(t)
	b := demoBackend()
	b.setLog(finishedRun, "2026-10-30T17:50:00Z info a\x1b[31mred\x1b[0m\n")
	m := newTestModel(t, i18n.English, b, wideWidth, 20)
	send(m, "3", "down", "enter", "W")
	_, content := savedFile(t, home)
	if strings.ContainsRune(content, 0x1b) || !strings.Contains(content, `\x1b[31mred`) {
		t.Fatalf("content %q", content)
	}
}

func TestSaveLogShowsTheError(t *testing.T) {
	old := saveLogFile
	saveLogFile = func(string, []byte) (string, error) { return "", errors.New("disk is full") }
	t.Cleanup(func() { saveLogFile = old })
	m := filterModel(t, demoBackend(), wideWidth)
	send(m, "W")
	contains(t, m, "The log was not saved: disk is full")
}

// The confirmation names the full path; the home and the time are fixed so the screen is the same everywhere.
func TestGoldenLogSaved(t *testing.T) {
	old := saveLogFile
	saveLogFile = func(name string, _ []byte) (string, error) { return "/home/demo/qhost/" + name, nil }
	t.Cleanup(func() { saveLogFile = old })
	logs := map[string]struct {
		setup func() *fakeBackend
		keys  []string
	}{
		"run":       {demoBackend, []string{"3", "down", "enter", "W"}},
		"container": {clogBackend, []string{"5", "down", "down", "down", "down", "down", "o", "W"}},
		"service":   {svcBackend, []string{"3", "v", "W"}},
	}
	for kind, tt := range logs {
		for name, lang := range goldenLangs {
			for size, width := range map[string]int{"narrow": narrowWidth, "wide": wideWidth} {
				t.Run(kind+"."+name+"."+size, func(t *testing.T) {
					m := newTestModel(t, lang, tt.setup(), width, goldenRows)
					m.now = func() time.Time { return clogStart.Add(time.Hour) }
					send(m, tt.keys...)
					screen := m.render()
					checkScreen(t, screen, width, goldenRows)
					checkGolden(t, "logsaved."+kind+"."+name+"."+size+".golden", screen+"\n")
				})
			}
		}
	}
}
