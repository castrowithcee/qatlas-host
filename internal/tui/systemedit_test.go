package tui

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/castrowithcee/qatlas-kit/exteditor"
	"github.com/castrowithcee/qatlas-kit/i18n"
	kittui "github.com/castrowithcee/qatlas-kit/tui"
)

// editFake stands in for the editor and the folders of the copy: it records what an edit would hand the editor and
// never starts one. runtime is a private folder of the test user that counts as memory unless ram says otherwise.
type editFake struct {
	runtime string
	ram     map[string]bool
	env     map[string]string
	reqs    []exteditor.Request
	// texts are the texts handed over, kept before the interface overwrites them.
	texts [][]byte
}

// fakeEditor is an invented program name in a folder of the test; it is looked up, never run.
func fakeEditor(t *testing.T, name string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// fakeEdit makes m edit with the invented editor nano through an editFake.
func fakeEdit(t *testing.T, m *Model) *editFake {
	t.Helper()
	f := &editFake{runtime: filepath.Join(t.TempDir(), "run"), ram: map[string]bool{}, env: map[string]string{}}
	if err := os.Mkdir(f.runtime, 0o700); err != nil {
		t.Fatal(err)
	}
	f.ram[f.runtime] = true
	f.env["XDG_RUNTIME_DIR"] = f.runtime
	m.sys.fold.hooks = editHooks{getenv: func(k string) string { return f.env[k] }, runUser: t.TempDir(), shm: t.TempDir(),
		tmpfs: func(dir string) bool { return f.ram[dir] },
		start: func(r exteditor.Request) (tea.Cmd, error) {
			f.reqs = append(f.reqs, r)
			f.texts = append(f.texts, bytes.Clone(r.Text))
			return func() tea.Msg { return nil }, nil
		}}
	m.prefs.Editor = fakeEditor(t, "nano")
	return f
}

// editModel is the folders page of the interface without root with the invented editor.
func editModel(t *testing.T) (*Model, *fakeRoot, *editFake) {
	t.Helper()
	m, f := browserModel(t, i18n.English, wideWidth, 40)
	return m, f, fakeEdit(t, m)
}

// editEntry marks the entry p in the browser and presses e on it.
func editEntry(t *testing.T, m *Model, p string) {
	t.Helper()
	browseTo(t, m, filepath.Dir(p))
	mark(t, m, filepath.Base(p))
	send(m, "e")
}

func edited(m *Model, msg tea.Msg) {
	_, cmd := m.Update(msg)
	drain(m, cmd)
}

func TestEditResolvesThePreferenceBeforeVisualAndRefusesAMissingOne(t *testing.T) {
	m, _, e := editModel(t)
	nano := m.prefs.Editor
	e.env["VISUAL"] = fakeEditor(t, "demo-visual")
	drain(m, m.editFile("/etc/hosts", kittui.FileRegular))
	if len(e.reqs) != 1 || e.reqs[0].Editor.Path != nano || !e.reqs[0].Editor.NoTraces {
		t.Fatalf("requests %+v, want nano from the preference", e.reqs)
	}
	edited(m, exteditor.UnchangedMsg{})

	m.prefs.Editor = ""
	drain(m, m.editFile("/etc/hosts", kittui.FileRegular))
	send(m, "y") // VISUAL names an editor the kit does not know
	if len(e.reqs) != 2 || e.reqs[1].Editor.Path != e.env["VISUAL"] {
		t.Fatalf("requests %+v, want the editor of VISUAL", e.reqs)
	}
	edited(m, exteditor.UnchangedMsg{})

	m.prefs.Editor = "no-such-editor-demo"
	drain(m, m.editFile("/etc/hosts", kittui.FileRegular))
	if len(e.reqs) != 2 || !strings.Contains(m.notice, "No editor found") {
		t.Fatalf("a missing editor of the preference must not fall back: %d requests, notice %q", len(e.reqs), m.notice)
	}
}

func TestEditHandsTheFileToTheEditorWithoutTarget(t *testing.T) {
	m, _, e := editModel(t)
	editEntry(t, m, "/srv/media/motd.txt")
	if len(e.reqs) != 1 {
		t.Fatalf("%d requests, notice %q, failure %q:\n%s", len(e.reqs), m.notice, m.failure, m.render())
	}
	r := e.reqs[0]
	if r.Target != nil || r.Dir != e.runtime || r.Name != "motd.txt" || string(e.texts[0]) != "Welcome.\n" || r.MaxSize != 1<<20 {
		t.Fatalf("request %+v with text %q", r, e.texts[0])
	}
}

func TestEditUnderSudoRunsTheEditorAsTheUserBehindSudo(t *testing.T) {
	m, f, e := editModel(t)
	m.euid, m.human, f.root = func() int { return 0 }, func() int { return 1000 }, true
	var asked []uint32
	m.sys.fold.hooks.pick = func(uid int, target *exteditor.Target) (privateDir, error) {
		asked = append(asked, uint32(uid), target.UID)
		return privateDir{path: e.runtime, ram: true}, nil
	}
	drain(m, m.editFile("/etc/secret-demo", kittui.FileRegular))
	if len(e.reqs) != 1 {
		t.Fatalf("%d requests, notice %q", len(e.reqs), m.notice)
	}
	want := exteditor.Target{UID: 1000, GID: 1000, Home: "/home/ada"}
	if tg := e.reqs[0].Target; tg == nil || *tg != want || !slices.Equal(asked, []uint32{1000, 1000}) {
		t.Fatalf("target %+v, folder asked for %v; want %+v", tg, asked, want)
	}
	if string(e.texts[0]) != "token=demo-only-0000\n" || len(f.reqs) != 0 {
		t.Fatalf("root reads directly: text %q, %d sudo requests", e.texts[0], len(f.reqs))
	}
}

func TestEditAsRootWithoutSudoIsRefused(t *testing.T) {
	m, _, e := editModel(t)
	m.euid, m.human = func() int { return 0 }, func() int { return 0 }
	drain(m, m.editFile("/etc/hosts", kittui.FileRegular))
	if len(e.reqs) != 0 || m.sys.fold.edit != nil || !strings.Contains(m.notice, "never runs as root") {
		t.Fatalf("%d requests, notice %q", len(e.reqs), m.notice)
	}
}

func TestUnknownEditorAsksBeforeItStarts(t *testing.T) {
	m, _, e := editModel(t)
	m.prefs.Editor = fakeEditor(t, "demo-edit")
	editEntry(t, m, "/srv/media/motd.txt")
	if len(e.reqs) != 0 || m.modal == nil || !strings.Contains(flat(m.render()), "backup or swap files") {
		t.Fatalf("no question before an unknown editor:\n%s", m.render())
	}
	send(m, "n")
	if len(e.reqs) != 0 || m.sys.fold.edit != nil {
		t.Fatalf("n started the editor or kept the edit")
	}
	send(m, "e", "y")
	if len(e.reqs) != 1 {
		t.Fatalf("y did not start the editor")
	}
}

func TestPrivateFolderIsTheRuntimeFolderElseOneUnderDevShm(t *testing.T) {
	uid := os.Getuid()
	for _, tc := range []struct {
		name                     string
		runtime, runRAM, shmRAM  bool
		wantRuntime, ram, target bool
	}{
		{name: "runtime in memory", runtime: true, runRAM: true, shmRAM: true, wantRuntime: true, ram: true},
		{name: "runtime on disk", runtime: true, shmRAM: true, ram: true},
		{name: "nothing in memory", runtime: true, wantRuntime: true},
		{name: "no runtime", shmRAM: true, ram: true},
		{name: "no runtime and no memory"},
		{name: "under sudo", runtime: true, runRAM: true, wantRuntime: true, ram: true, target: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runUser, shm, xdg := t.TempDir(), t.TempDir(), filepath.Join(t.TempDir(), "xdg")
			runtime := xdg
			var target *exteditor.Target
			if tc.target {
				// As root the runtime folder of the person counts, never the XDG_RUNTIME_DIR of root.
				runtime, target = filepath.Join(runUser, strconv.Itoa(uid)), &exteditor.Target{UID: uint32(uid), GID: uint32(os.Getgid()), Home: "/home/demo"}
			}
			if tc.runtime {
				if err := os.Mkdir(runtime, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			h := editHooks{getenv: func(string) string { return xdg }, runUser: runUser, shm: shm,
				tmpfs: func(dir string) bool { return (dir == runtime && tc.runRAM) || (dir == shm && tc.shmRAM) }}
			d, err := h.choosePrivateDir(uid, target)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(d.remove)
			if (d.path == runtime) != tc.wantRuntime || d.ram != tc.ram || d.made == tc.wantRuntime {
				t.Fatalf("chose %+v, runtime %s", d, runtime)
			}
			if !tc.wantRuntime && (filepath.Dir(d.path) != shm || !ownedPrivate(d.path, uid)) {
				t.Fatalf("%s is no private folder under %s", d.path, shm)
			}
		})
	}
}

func TestFileOnlyRootReadsIsEditedOnlyWithACopyInMemory(t *testing.T) {
	m, f, e := editModel(t)
	e.ram[e.runtime] = false
	drain(m, m.editFile("/etc/secret-demo", kittui.FileRegular))
	if len(f.reqs) != 0 || len(e.reqs) != 0 || !strings.Contains(m.notice, "tmpfs") {
		t.Fatalf("without memory: %d sudo requests, %d edits, notice %q", len(f.reqs), len(e.reqs), m.notice)
	}
	drain(m, m.editFile("/etc/hosts", kittui.FileRegular))
	if len(e.reqs) != 1 {
		t.Fatalf("a file the administrator reads needs no memory: notice %q", m.notice)
	}
	edited(m, exteditor.UnchangedMsg{})

	e.ram[e.runtime] = true
	m.notice = ""
	drain(m, m.editFile("/etc/secret-demo", kittui.FileRegular))
	if m.modal == nil || len(e.reqs) != 1 {
		t.Fatalf("no question before the read through sudo:\n%s", m.render())
	}
	send(m, "y")
	if len(e.reqs) != 2 || string(e.texts[1]) != "token=demo-only-0000\n" {
		t.Fatalf("the text read through sudo did not reach the editor: %d requests", len(e.reqs))
	}
}

func TestEntriesThatAreNoTextFilesAreNotEdited(t *testing.T) {
	for _, tc := range []struct{ path, want string }{
		{"/etc/demo-app/logo.bin", "binary file"},
		{"/etc/demo-app/dump.sql", "larger than 1 MiB"},
		{"/etc/motd", "is a link"},
		{"/etc/demo-app", "is a folder"},
		{"/srv/media/.git/config", ".git folder"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			m, _, e := editModel(t)
			editEntry(t, m, tc.path)
			if len(e.reqs) != 0 || m.sys.fold.edit != nil || !strings.Contains(m.notice, tc.want) {
				t.Fatalf("%d requests, notice %q, want %q", len(e.reqs), m.notice, tc.want)
			}
		})
	}
}

func TestCheckSyntaxByExtensionNamesTheLineOnly(t *testing.T) {
	for _, tc := range []struct {
		name, text, format string
		line               int
	}{
		{"app.yaml", "a: 1\n---\nb: c: d\n", "YAML", 3},
		{"app.yml", "a: 1\n", "", 0},
		{"app.json", "{\n  \"a\": 1,\n  \"b\" 2\n}\n", "JSON", 3},
		{"app.json", "{\"a\": [1, 2]}", "", 0},
		{".env", "A=1\nB=\"open\n", ".env", 2},
		{"secrets.env", "export A='x' # note\n", "", 0},
		{"x.env", "A=\"a \\\" b\"\n# c\nC=1\n", "", 0},
		{"x.env", "A=1\nnot a statement\n", ".env", 2},
		{"notes.txt", "{ not: [checked", "", 0},
	} {
		got := checkSyntax(tc.name, []byte(tc.text))
		switch {
		case tc.format == "" && got != nil:
			t.Errorf("%s %q: %+v, want no problem", tc.name, tc.text, *got)
		case tc.format != "" && (got == nil || got.format != tc.format || got.line != tc.line):
			t.Errorf("%s %q: %+v, want %s at line %d", tc.name, tc.text, got, tc.format, tc.line)
		}
	}
}

func TestFailedCheckOffersToEditAgainOrToDiscard(t *testing.T) {
	m, f, e := editModel(t)
	drain(m, m.editFile("/etc/demo-app/app.yaml", kittui.FileRegular))
	bad := "services: [web\n# demo-only-marker\n"
	edited(m, exteditor.ChangedMsg{Text: []byte(bad)})
	screen := flat(m.render())
	if m.modal == nil || !strings.Contains(screen, "as YAML failed at line") || strings.Contains(screen, "demo-only-marker") {
		t.Fatalf("the check failed without the question, or quoted the text:\n%s", screen)
	}
	send(m, "y")
	if len(e.reqs) != 2 || string(e.texts[1]) != bad {
		t.Fatalf("edit again did not reopen the changed text: %d requests", len(e.reqs))
	}
	edited(m, exteditor.ChangedMsg{Text: []byte("services: [web\n")})
	send(m, "n")
	if m.modal != nil || m.sys.fold.edit != nil || len(f.reqs) != 0 {
		t.Fatalf("discard left the edit or asked root: %d requests", len(f.reqs))
	}
}

func TestUnchangedEditDoesNothing(t *testing.T) {
	m, f, e := editModel(t)
	e.ram[e.runtime] = false // the copy goes to a folder of its own under the stand-in for /dev/shm
	e.ram[m.sys.fold.hooks.shm] = true
	drain(m, m.editFile("/etc/hosts", kittui.FileRegular))
	if len(e.reqs) != 1 || filepath.Dir(e.reqs[0].Dir) != m.sys.fold.hooks.shm {
		t.Fatalf("requests %+v", e.reqs)
	}
	edited(m, exteditor.UnchangedMsg{})
	if m.modal != nil || m.sys.fold.edit != nil || len(f.reqs) != 0 || m.failure != "" {
		t.Fatalf("unchanged: modal %v, %d requests, failure %q", m.modal != nil, len(f.reqs), m.failure)
	}
	if _, err := os.Stat(e.reqs[0].Dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the folder made for the copy stays: %v", err)
	}
}

func TestFailedEditorWritesNothing(t *testing.T) {
	m, f, _ := editModel(t)
	drain(m, m.editFile("/etc/hosts", kittui.FileRegular))
	edited(m, exteditor.FailedMsg{Err: exteditor.ErrEditorFailed})
	if m.sys.fold.edit != nil || len(f.reqs) != 0 || !strings.Contains(m.failure, "nothing was written") {
		t.Fatalf("failure %q, %d requests", m.failure, len(f.reqs))
	}
}

func TestEditedFileIsWrittenThroughFileWriteWithTheTextOnlyOnStandardInput(t *testing.T) {
	for _, tc := range []struct {
		path string
		args []string
	}{
		{"/srv/media/motd.txt", []string{"/srv/media/motd.txt"}},
		{"/etc/hosts", []string{"/etc/hosts", "system-path"}},
	} {
		t.Run(tc.path, func(t *testing.T) {
			m, f, _ := editModel(t)
			f.report, f.reports = appliedReport("file.write"), nil
			drain(m, m.editFile(tc.path, kittui.FileRegular))
			text := []byte("demo-only-content 4711\n")
			sent := bytes.Clone(text)
			edited(m, exteditor.ChangedMsg{Text: sent})
			if m.modal == nil || !strings.Contains(flat(m.render()), "Write the edited file "+tc.path+" back in place") {
				t.Fatalf("no confirmation:\n%s", m.render())
			}
			send(m, "y")
			want := append([]string{"apply", "--confirm", "--output", "json", "--", "file.write"}, tc.args...)
			if got := f.lastArgs(t); !slices.Equal(got, want) {
				t.Fatalf("arguments %q, want %q", got, want)
			}
			if len(f.stdins) == 0 || !bytes.Equal(f.stdins[len(f.stdins)-1], text) {
				t.Fatalf("standard input %q, want the edited text", f.stdins)
			}
			for _, c := range f.built {
				if strings.Contains(strings.Join(append(c.Args, c.Env...), " "), "demo-only-content") {
					t.Fatalf("the text reached the command line or the environment: %q %q", c.Args, c.Env)
				}
			}
			if !bytes.Equal(sent, make([]byte, len(sent))) {
				t.Fatalf("the text was not overwritten after the run")
			}
		})
	}
}
