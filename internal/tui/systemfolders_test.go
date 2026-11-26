package tui

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-kit/i18n"
	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/host/hostadmin"
	"github.com/castrowithcee/qatlas-host/internal/host/hostchange"
)

// demoFolder is an invented folder owned by cleo and the group media; nothing of the machine is read for it.
func demoFolder(path string) hostadmin.Folder {
	no := false
	return hostadmin.Folder{Path: path, Resolved: path, Kind: "directory", Owner: "cleo", Group: "media", Mode: "0750",
		OwnerAcc: hostadmin.AccessReadWrite, GroupAcc: hostadmin.AccessRead, OthersAcc: hostadmin.AccessNothing, ACL: &no,
		Protected: hostchange.IsSystemPath(path)}
}

// fakeHostFolders makes the folders page read invented folders and the invented tree, and count n entries. It
// returns the number of reads of a folder's permissions so far.
func fakeHostFolders(m *Model, n int) *int {
	reads := 0
	p := &m.sys.fold
	p.read = func(path string) (hostadmin.Folder, error) { reads++; return demoFolder(path), nil }
	p.count = func(string, bool) (int, error) { return n, nil }
	p.readPath = demoTreeReader(m)
	return &reads
}

// toFolderPage opens the folders page of the System area.
func toFolderPage(t *testing.T, m *Model) {
	t.Helper()
	m.setZone(kittui.ZoneSubtabs)
	send(m, "right", "right", "down")
	if m.sys.page != pageFolders || m.focus() != kittui.ZoneRows {
		t.Fatalf("page %s in zone %v, want the content of the folders page", m.sys.page, m.focus())
	}
}

// showFolder opens Rights for path as the action of the browser on its marked entry does.
func showFolder(t *testing.T, m *Model, path string) {
	t.Helper()
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	drain(m, m.fileResult(kittui.FileResult{Action: &kittui.FileAction{Action: actRights, Dir: parts[:len(parts)-1],
		Entry: kittui.FileEntry{Name: parts[len(parts)-1]}, HasEntry: true}}))
	if m.sys.fold.asked != path {
		t.Fatalf("Rights shows %q, want %q:\n%s", m.sys.fold.asked, path, m.render())
	}
}

// changeBelow opens the change of the folder shown, sets Everything below it to yes and saves.
func changeBelow(m *Model) {
	send(m, "e", "down", "down", "down", "down", "down", "right", "f2")
}

func TestFolderShowsOwnerGroupAndLevelsInPlainWords(t *testing.T) {
	m, _ := systemModel(t, i18n.English, wideWidth, 40)
	fakeHostFolders(m, 1)
	toFolderPage(t, m)
	showFolder(t, m, "/srv/media")
	screen := m.render()
	for _, want := range []string{"Owner cleo may: read and write", "Group media may: read", "Everyone else may: nothing", "Mode: 0750"} {
		if !strings.Contains(screen, want) {
			t.Errorf("the folder lacks %q:\n%s", want, screen)
		}
	}
	if strings.Contains(screen, "system path") {
		t.Errorf("an ordinary folder is called a system path:\n%s", screen)
	}
}

// The acceptance: a change of everything below the folder names the number of entries before anything runs. The
// entries are counted on an invented tree; only reading the folder is faked, so that its owner is a known user.
func TestRecursiveFolderChangeNamesTheEntriesBeforeTheConfirmation(t *testing.T) {
	dir := t.TempDir()
	for _, d := range []string{"a", "b"} {
		if err := os.Mkdir(filepath.Join(dir, d), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "a", "note.txt"), []byte("demo"), 0o640); err != nil {
		t.Fatal(err)
	}
	m, f := systemModel(t, i18n.English, wideWidth, 40)
	f.report = appliedReport(hostchange.ActionFolderSetPermissions)
	reads := fakeHostFolders(m, 0)
	m.sys.fold.count = nil
	toFolderPage(t, m)
	showFolder(t, m, dir)
	changeBelow(m)
	if m.modal == nil {
		t.Fatalf("no confirmation:\n%s", m.render())
	}
	screen := flat(m.render())
	for _, want := range []string{"and everything below it", "4 entries change: the folder and everything below it.", "chown -R -h -- cleo:media"} {
		if !strings.Contains(screen, want) {
			t.Errorf("the confirmation lacks %q:\n%s", want, screen)
		}
	}
	if len(f.reqs) != 0 {
		t.Fatalf("something ran before the answer: %v", f.reqs)
	}
	before := *reads
	send(m, "y")
	want := []string{"apply", "--confirm", "--output", "json", "--", hostchange.ActionFolderSetPermissions, dir, "cleo", "media",
		"read-write", "read", "nothing", "recursive"}
	if got := f.lastArgs(t); !slices.Equal(got, want) {
		t.Fatalf("arguments %q, want %q", got, want)
	}
	if *reads <= before {
		t.Error("the folder was not read again after the change")
	}
}

func TestSystemPathAsksRedAndPassesTheAcknowledgement(t *testing.T) {
	m, f := systemModel(t, i18n.English, wideWidth, 40)
	f.report = appliedReport(hostchange.ActionFolderSetPermissions)
	fakeHostFolders(m, 1)
	toFolderPage(t, m)
	showFolder(t, m, "/etc/demo")
	if !strings.Contains(m.render(), "This is a system path: a wrong owner or permission here can break the host.") {
		t.Fatalf("the system path is not named:\n%s", m.render())
	}
	send(m, "e", "f2")
	if m.modal == nil {
		t.Fatalf("no confirmation:\n%s", m.render())
	}
	if !strings.Contains(flat(m.render()), "wrong owners or permissions here can stop services") {
		t.Fatalf("the confirmation does not name the risk:\n%s", m.render())
	}
	send(m, "y")
	if args := f.lastArgs(t); args[len(args)-1] != "system-path" {
		t.Fatalf("the acknowledgement of the system path is missing: %q", args)
	}
}

// A system root is never changed together with everything below it: the plan refuses it before a single entry is
// counted.
func TestRecursiveChangeOfASystemRootIsRefusedBeforeCounting(t *testing.T) {
	m, f := systemModel(t, i18n.English, wideWidth, 40)
	fakeHostFolders(m, 1)
	m.sys.fold.count = func(string, bool) (int, error) {
		t.Error("a refused path was counted")
		return 0, nil
	}
	toFolderPage(t, m)
	showFolder(t, m, "/etc")
	changeBelow(m)
	if m.modal != nil || m.set.form == nil {
		t.Fatalf("want the form with its problem, no confirmation:\n%s", m.render())
	}
	if !strings.Contains(flat(m.render()), "not changed together with everything below it") {
		t.Fatalf("the refusal is not shown:\n%s", m.render())
	}
	if len(f.reqs) != 0 {
		t.Fatalf("something ran: %v", f.reqs)
	}
}

func TestTooManyEntriesKeepTheForm(t *testing.T) {
	m, _ := systemModel(t, i18n.English, wideWidth, 40)
	fakeHostFolders(m, 1)
	m.sys.fold.count = func(string, bool) (int, error) { return 0, hostchange.ErrTooManyEntries }
	toFolderPage(t, m)
	showFolder(t, m, "/srv/media")
	changeBelow(m)
	if m.modal != nil || m.set.form == nil || !strings.Contains(flat(m.render()), "/srv/media holds more than 5000 entries.") {
		t.Fatalf("want the form with the limit:\n%s", m.render())
	}
}

func TestAFileIsNotChanged(t *testing.T) {
	m, _ := systemModel(t, i18n.English, wideWidth, 40)
	fakeHostFolders(m, 1)
	m.sys.fold.read = func(path string) (hostadmin.Folder, error) {
		f := demoFolder(path)
		f.Kind = "file"
		return f, nil
	}
	toFolderPage(t, m)
	showFolder(t, m, "/srv/media/note.txt")
	send(m, "e")
	if m.set.form != nil || !strings.Contains(m.render(), "Only a folder can be changed here, not a file.") {
		t.Fatalf("a file got a form:\n%s", m.render())
	}
}
