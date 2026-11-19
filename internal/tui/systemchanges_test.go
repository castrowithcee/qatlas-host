package tui

import (
	"slices"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-kit/i18n"
	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/host/hostadmin"
	"github.com/castrowithcee/qatlas-host/internal/host/hostchange"
)

// The invented journal of `host journal --output json`: a folder change that can be undone, a deleted home that
// cannot, and a change that was undone already. Nothing of the machine is read.
const (
	demoChangeID = "20261102T090000.000000Z-a1b2c3"
	demoJournal  = `[{"id":"` + demoChangeID + `","time":"2026-11-02T09:00:00Z","action":"folder.set-permissions",` +
		`"sentence":"Change /srv/media: owner cleo, group media; the owner gets read and write, the group read, everyone else nothing.",` +
		`"result":"applied","reversible":true,"sudo_uid":1000,` +
		`"undo_sentence":"Restore the previous owner, group and permissions of the folder /srv/media.",` +
		`"undo_commands":["chown -h -- 1000:1000 /srv/media","chmod -- 0755 /srv/media"]},` +
		`{"id":"20261101T170000.000000Z-d4e5f6","time":"2026-11-01T17:00:00Z","action":"user.delete",` +
		`"sentence":"Delete the user ben together with its home and mail spool.","result":"applied","reversible":false,` +
		`"sudo_uid":1000,"undo_sentence":"","undo_commands":[]},` +
		`{"id":"20261101T080000.000000Z-0a1b2c","time":"2026-11-01T08:00:00Z","action":"user.lock",` +
		`"sentence":"Lock cleo: no login is possible any more, with a password or an SSH key.","result":"undone","reversible":false,` +
		`"sudo_uid":0,"undo_sentence":"","undo_commands":[]}]`
)

// toChangesPage opens the changes page of the System area.
func toChangesPage(t *testing.T, m *Model) {
	t.Helper()
	m.setZone(kittui.ZoneSubtabs)
	send(m, "right", "right", "right", "right")
	if m.sys.page != pageChanges {
		t.Fatalf("page %s, want the changes page", m.sys.page)
	}
	send(m, "down")
}

// readChanges reads the invented journal through the fake sudo, as r does.
func readChanges(t *testing.T, m *Model, f *fakeRoot) {
	t.Helper()
	f.report = demoJournal
	send(m, "r")
	if !m.sys.jrnl.read {
		t.Fatalf("the journal was not read:\n%s", m.render())
	}
}

func TestChangesWithoutRootReadTheJournalOnlyOnReload(t *testing.T) {
	m, f := systemModel(t, i18n.English, wideWidth, 40)
	toChangesPage(t, m)
	if len(f.reqs) != 0 {
		t.Fatalf("the journal was read without being asked for: %v", f.reqs)
	}
	if screen := flat(m.render()); !strings.Contains(screen, "only root can read. Press r to read it through sudo.") {
		t.Fatalf("no hint how to read the journal:\n%s", screen)
	}
	readChanges(t, m, f)
	if got, want := f.lastArgs(t), []string{"journal", "--output", "json"}; !slices.Equal(got, want) {
		t.Fatalf("arguments %q, want %q", got, want)
	}
	if len(f.execs) != 1 || f.execs[0].Args[0] != "sudo-fake" {
		t.Fatalf("the journal was not read through sudo on the terminal: %v", f.execs)
	}
	screen := m.render()
	for _, want := range []string{"Change /srv/media", "Delete the user ben", "2026-11-02 09:00", "ada", "possible", "undone",
		"Undo: Restore the previous owner", "$ chmod -- 0755 /srv/media"} {
		if !strings.Contains(screen, want) {
			t.Errorf("the changes page lacks %q:\n%s", want, screen)
		}
	}
}

func TestChangesAsRootReadTheJournalOnTheirOwn(t *testing.T) {
	m, f := systemModel(t, i18n.English, wideWidth, 40)
	m.euid = func() int { return 0 }
	f.root, f.report = true, demoJournal
	toChangesPage(t, m)
	if !m.sys.jrnl.read || len(f.bgs) != 1 {
		t.Fatalf("an interface that runs as root did not read the journal on its own: %v\n%s", f.reqs, m.render())
	}
}

func TestUndoAsksWithTheSentenceAndTheCommandsAndRunsHostUndo(t *testing.T) {
	m, f := systemModel(t, i18n.English, wideWidth, 40)
	toChangesPage(t, m)
	readChanges(t, m, f)
	asked := len(f.reqs)
	send(m, "enter")
	if m.modal == nil {
		t.Fatalf("no question:\n%s", m.render())
	}
	screen := flat(m.render())
	for _, want := range []string{"Undo the change: Change /srv/media", "The undo itself cannot be undone.",
		"Restore the previous owner, group and permissions of the folder /srv/media.", "$ chown -h -- 1000:1000 /srv/media"} {
		if !strings.Contains(screen, want) {
			t.Errorf("the question lacks %q:\n%s", want, screen)
		}
	}
	if len(f.reqs) != asked {
		t.Fatalf("something ran before the answer: %v", f.reqs[asked:])
	}
	f.report = appliedReport(hostchange.ActionFolderSetPermissions)
	send(m, "y")
	if got, want := f.lastArgs(t), []string{"undo", "--confirm", "--output", "json", "--", demoChangeID}; !slices.Equal(got, want) {
		t.Fatalf("arguments %q, want %q", got, want)
	}
	screen = flat(m.render())
	if !strings.Contains(screen, "Done: Undo the change") || !strings.Contains(screen, "An undo is not undone.") {
		t.Fatalf("the result does not name the undo:\n%s", screen)
	}
	send(m, "enter")
	if e, _ := m.selectedJournal(); e.undoable() || e.Result != hostchange.ResultUndone {
		t.Errorf("the undone entry is still offered: %+v", e)
	}
	if !strings.Contains(flat(m.render()), "The host was changed since the journal was read.") {
		t.Errorf("the page does not say that the journal is older than the host:\n%s", m.render())
	}
}

func TestChangeWithoutUndoSaysWhyInsteadOfAsking(t *testing.T) {
	for id, want := range map[string]string{
		"20261101T170000.000000Z-d4e5f6": "This change cannot be undone, as a deleted home or an undo itself.",
		"20261101T080000.000000Z-0a1b2c": "This change was undone already.",
	} {
		m, f := systemModel(t, i18n.English, wideWidth, 40)
		toChangesPage(t, m)
		readChanges(t, m, f)
		if !m.sys.jrnl.list.Select(kittui.ItemID(id)) {
			t.Fatalf("no row %s", id)
		}
		asked := len(f.reqs)
		send(m, "enter")
		if screen := flat(m.render()); !strings.Contains(screen, want) || strings.Contains(screen, "yes, with sudo") {
			t.Errorf("%s: the notice lacks %q or asks:\n%s", id, want, screen)
		}
		if len(f.reqs) != asked {
			t.Errorf("%s: something ran: %v", id, f.reqs[asked:])
		}
	}
}

func TestAChangeFromTheInterfaceMarksTheJournalOlderThanTheHost(t *testing.T) {
	m, f := systemModel(t, i18n.English, wideWidth, 40)
	toChangesPage(t, m)
	readChanges(t, m, f)
	f.report = appliedReport(hostchange.ActionFolderSetPermissions)
	fakeHostFolders(m, 1)
	// The folder of the scenario: 0755, everyone else may read.
	m.sys.fold.read = func(path string) (hostadmin.Folder, error) {
		f := demoFolder(path)
		f.Mode, f.OthersAcc = "0755", hostadmin.AccessRead
		return f, nil
	}
	m.setZone(kittui.ZoneSubtabs)
	send(m, "left", "left", "down")
	showFolder(t, m, "/srv/media")
	// The keys of the scenario: everyone else may nothing.
	send(m, "e", "down", "down", "down", "down", "left", "f2", "y")
	want := []string{"apply", "--confirm", "--output", "json", "--", hostchange.ActionFolderSetPermissions, "/srv/media", "cleo", "media",
		"read-write", "read", "nothing"}
	if got := f.lastArgs(t); !slices.Equal(got, want) {
		t.Fatalf("arguments %q, want %q", got, want)
	}
	send(m, "enter")
	if !m.sys.jrnl.stale {
		t.Fatal("the journal is not marked older than the host after a change")
	}
	reads := len(f.reqs)
	m.setZone(kittui.ZoneSubtabs)
	send(m, "right", "right")
	if len(f.reqs) != reads {
		t.Errorf("an interface without root read the journal unasked: %v", f.reqs[reads:])
	}
}
