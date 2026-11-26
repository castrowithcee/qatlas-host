package tui

import (
	"io/fs"
	"slices"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-kit/i18n"
	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/host/hostchange"
)

const bigTree = 12 << 20

// changeModel is the browser of the invented tree, open in dir, with a measure that answers for every folder.
func changeModel(t *testing.T, dir string, entries int, size int64) (*Model, *fakeRoot) {
	t.Helper()
	m, f := browserModel(t, i18n.English, wideWidth, 40)
	m.sys.fold.chg.measure = func(string) (int, int64, error) { return entries, size, nil }
	f.report, f.reports = appliedReport("file"), nil
	browseTo(t, m, dir)
	return m, f
}

// applied is the apply command the last run asked for, after "apply --confirm --output json --".
func applied(t *testing.T, f *fakeRoot) []string {
	t.Helper()
	got := f.lastArgs(t)
	if len(got) < 5 || !slices.Equal(got[:5], []string{"apply", "--confirm", "--output", "json", "--"}) {
		t.Fatalf("arguments %q", got)
	}
	return got[5:]
}

// fill replaces the name field of the open form with name and saves it.
func fill(m *Model, name string) {
	send(m, slices.Repeat([]string{"backspace"}, 40)...)
	send(m, seq(typed(name), keys("f2"))...)
}

func TestNewCreatesAnEmptyFileOrAFolderAsRootActions(t *testing.T) {
	for _, tc := range []struct {
		dir, name string
		folder    bool
		want      []string
		sentence  string
	}{
		{"/srv/media", "notes.txt", false, []string{"file.write", "/srv/media/notes.txt"}, "Create the empty file /srv/media/notes.txt."},
		{"/srv/media", "clips", true, []string{"file.create-folder", "/srv/media/clips"}, "Create the folder /srv/media/clips."},
		{"/etc", "demo.conf", false, []string{"file.write", "/etc/demo.conf", "system-path"}, "This is a system path"},
		{"/etc", "demo.d", true, []string{"file.create-folder", "/etc/demo.d", "system-path"}, "This is a system path"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, f := changeModel(t, tc.dir, 1, 0)
			send(m, "n")
			if m.set.form == nil || m.set.form.kind != formFileNew {
				t.Fatalf("no form:\n%s", m.render())
			}
			if tc.folder {
				send(m, "right")
			}
			send(m, "down")
			fill(m, tc.name)
			containsFlat(t, m, tc.sentence)
			send(m, "y")
			if got := applied(t, f); !slices.Equal(got, tc.want) {
				t.Fatalf("arguments %q, want %q", got, tc.want)
			}
			last := f.reqs[len(f.reqs)-1]
			if last.EmptyInput != !tc.folder || len(last.Secret) != 0 {
				t.Fatalf("empty input %v, secret %q", last.EmptyInput, last.Secret)
			}
		})
	}
}

// A file is written in place, so a name that exists is refused before anything is asked.
func TestNewRefusesAnExistingNameBeforeAnyCommand(t *testing.T) {
	m, f := changeModel(t, "/srv/media", 1, 0)
	send(m, "n", "down")
	fill(m, "motd.txt")
	containsFlat(t, m, "/srv/media/motd.txt exists already. Nothing is replaced.")
	if m.modal != nil || len(f.reqs) != 0 {
		t.Fatalf("something was asked or run: %v", f.reqs)
	}
}

func TestNamesAreCheckedBeforeAnyCommand(t *testing.T) {
	for name, want := range map[string]string{
		"":          "The name is empty.",
		"  ":        "The name is empty.",
		".":         "The name is . or ..",
		"..":        "The name is . or ..",
		"a/b":       "holds a / or a control character",
		"/srv/evil": "holds a / or a control character",
		"hosts":     "/etc/hosts exists already",
		".git":      "lies in a .git folder",
	} {
		m, f := changeModel(t, "/etc", 1, 0)
		mark(t, m, "motd")
		send(m, "r")
		fill(m, name)
		if m.modal != nil || len(f.reqs) != 0 || !strings.Contains(flat(m.render()), want) {
			t.Errorf("%q: want %q without a question:\n%s", name, want, m.render())
		}
	}
	// A control character cannot be typed into the form, but a pasted name may hold one.
	m, _ := changeModel(t, "/etc", 1, 0)
	if len(m.fileNameProblem("a\x07b", "/etc")) == 0 {
		t.Error("a name with a control character was accepted")
	}
}

func TestRenameAndCopyAskWithTheExactArguments(t *testing.T) {
	for _, tc := range []struct {
		dir, entry, key, name string
		want                  []string
		sentence              string
	}{
		{"/srv/media", "motd.txt", "r", "note.txt", []string{"file.rename", "/srv/media/motd.txt", "/srv/media/note.txt"}, "Rename /srv/media/motd.txt to /srv/media/note.txt."},
		{"/srv/media", "motd.txt", "c", "motd-2.txt", []string{"file.copy", "/srv/media/motd.txt", "/srv/media/motd-2.txt"}, "Copy /srv/media/motd.txt to /srv/media/motd-2.txt."},
		{"/etc", "hosts", "r", "hosts.old", []string{"file.rename", "/etc/hosts", "/etc/hosts.old", "system-path"}, "This is a system path"},
		{"/etc", "hosts", "c", "hosts.bak", []string{"file.copy", "/etc/hosts", "/etc/hosts.bak", "system-path"}, "This is a system path"},
	} {
		t.Run(tc.dir+tc.key, func(t *testing.T) {
			m, f := changeModel(t, tc.dir, 1, 100)
			mark(t, m, tc.entry)
			send(m, tc.key)
			fill(m, tc.name)
			containsFlat(t, m, tc.sentence)
			send(m, "y")
			if got := applied(t, f); !slices.Equal(got, tc.want) {
				t.Fatalf("arguments %q, want %q", got, tc.want)
			}
		})
	}
}

// The form starts with the name of the entry, and with a suggested name for a copy.
func TestRenameAndCopyStartWithTheNameOfTheEntry(t *testing.T) {
	if got := copyName("motd.txt"); got != "motd-copy.txt" {
		t.Errorf("copy of motd.txt is %q", got)
	}
	if got := copyName(".profile"); got != ".profile-copy" {
		t.Errorf("copy of .profile is %q", got)
	}
	m, _ := changeModel(t, "/srv/media", 1, 0)
	mark(t, m, "motd.txt")
	send(m, "r")
	containsFlat(t, m, "New name motd.txt")
	send(m, "esc", "esc")
	mark(t, m, "motd.txt")
	send(m, "c")
	containsFlat(t, m, "Name of the copy motd-copy.txt")
}

func TestRemoveAsksInRedAndNamesTheEntry(t *testing.T) {
	m, f := changeModel(t, "/srv/media", 1, 0)
	mark(t, m, "motd.txt")
	send(m, "d")
	containsFlat(t, m, "Remove /srv/media/motd.txt.", "file-remove /srv/media/motd.txt")
	send(m, "y")
	if got := applied(t, f); !slices.Equal(got, []string{"file.remove", "/srv/media/motd.txt"}) {
		t.Fatalf("arguments %q", got)
	}
}

func TestFolderQuestionNamesEntriesAndSizeUpToTheUndoLimit(t *testing.T) {
	for key, tc := range map[string]struct{ ask, id string }{
		"d": {"Remove the folder /srv/media/clips with everything in it.", "file.remove"},
		"c": {"Copy the folder /srv/media/clips with everything in it to /srv/media/clips-copy.", "file.copy"},
	} {
		m, f := changeModel(t, "/srv/media", 40, hostchange.MaxUndoBytes)
		m.sys.fold.readPath = func(p string) (hostchange.PathContent, error) {
			return readDemo(map[string]demoNode{"/": demoTree["/"], "/srv": demoTree["/srv"], "/srv/media": demoTree["/srv/media"],
				"/srv/media/clips": {kind: hostchange.KindFolder, mode: 0o755}}, p, false)
		}
		send(m, "R")
		mark(t, m, "clips")
		send(m, key)
		if key == "c" {
			send(m, "f2")
		}
		screen := flat(m.render())
		for _, want := range []string{tc.ask, "Entries: 40, size of the files: 10.0 MiB."} {
			if !strings.Contains(screen, want) {
				t.Fatalf("%s: want %q:\n%s", key, want, m.render())
			}
		}
		if strings.Contains(screen, "final") {
			t.Fatalf("%s: a folder within the undo limit is final:\n%s", key, m.render())
		}
		send(m, "y")
		want := []string{tc.id, "/srv/media/clips"}
		if key == "c" {
			want = append(want, "/srv/media/clips-copy")
		}
		if got := applied(t, f); !slices.Equal(got, want) {
			t.Fatalf("%s: arguments %q, want %q", key, got, want)
		}
	}
}

// Beyond what an undo keeps, the question says it is final and the command carries the option permanent.
func TestFolderBeyondTheUndoLimitIsFinalAndPermanent(t *testing.T) {
	for _, tc := range []struct {
		entries int
		size    int64
	}{{40, bigTree}, {hostchange.MaxFolderEntries + 1, 10}} {
		for _, key := range []string{"d", "c"} {
			m, f := changeModel(t, "/srv", tc.entries, tc.size)
			mark(t, m, "media")
			send(m, key)
			if key == "c" {
				send(m, "f2")
			}
			containsFlat(t, m, "This is final", "the journal keeps nothing of it", "10 MiB")
			send(m, "y")
			got := applied(t, f)
			if got[0] == "file.copy" == (key == "d") || !slices.Contains(got, "permanent") {
				t.Fatalf("%s %d entries: arguments %q", key, tc.entries, got)
			}
		}
	}
}

// A system path is red and carries both options.
func TestSystemFolderBeyondTheUndoLimitCarriesBothOptions(t *testing.T) {
	m, f := changeModel(t, "/etc", 3, bigTree)
	mark(t, m, "demo-app")
	send(m, "d")
	containsFlat(t, m, "This is a system path", "This is final")
	send(m, "y")
	if got, want := applied(t, f), []string{"file.remove", "/etc/demo-app", "permanent", "system-path"}; !slices.Equal(got, want) {
		t.Fatalf("arguments %q, want %q", got, want)
	}
}

func TestAFolderTheAdministratorCannotCountSaysSoAndLeavesTheLimitToTheCommand(t *testing.T) {
	m, f := changeModel(t, "/etc", 0, 0)
	m.sys.fold.chg.measure = func(string) (int, int64, error) { return 0, 0, fs.ErrPermission }
	mark(t, m, "demo-app")
	send(m, "d")
	containsFlat(t, m, "The entries and their size cannot be read here", "permission denied", "refuses and nothing changes")
	send(m, "y")
	if got := applied(t, f); slices.Contains(got, "permanent") {
		t.Fatalf("permanent without a count: %q", got)
	}
}

func TestATreeBeyondWhatOneChangeWalksIsRefused(t *testing.T) {
	m, f := changeModel(t, "/srv", 0, 0)
	m.sys.fold.chg.measure = func(string) (int, int64, error) { return 0, 0, hostchange.ErrTooManyEntries }
	mark(t, m, "media")
	send(m, "d")
	if m.modal != nil || len(f.reqs) != 0 {
		t.Fatalf("a tree beyond the limit was asked:\n%s", m.render())
	}
	containsFlat(t, m, "/srv/media holds more than 100000 entries")
}

// A file larger than an undo keeps needs permanent as well; its size comes from the list, with no read.
func TestLargeFileIsRemovedOnlyPermanently(t *testing.T) {
	m, f := changeModel(t, "/etc/demo-app", 0, 0)
	m.sys.fold.chg.measure = func(string) (int, int64, error) { t.Fatal("a file was measured"); return 0, 0, nil }
	mark(t, m, "dump.sql") // 3 MiB: within the limit
	send(m, "d")
	send(m, "y")
	if got := applied(t, f); slices.Contains(got, "permanent") {
		t.Fatalf("arguments %q", got)
	}
	m.sys.fold.readPath = func(p string) (hostchange.PathContent, error) {
		c, err := readDemo(demoTree, p, false)
		for i := range c.Entries {
			if c.Entries[i].Name == "dump.sql" {
				c.Entries[i].Size = bigTree
			}
		}
		return c, err
	}
	send(m, "esc")
	send(m, "R")
	mark(t, m, "dump.sql")
	send(m, "d")
	containsFlat(t, m, "This is final")
	send(m, "y")
	if got := applied(t, f); !slices.Contains(got, "permanent") {
		t.Fatalf("arguments %q", got)
	}
}

func TestGitPathsOnlyRead(t *testing.T) {
	m, f := changeModel(t, "/srv/media", 1, 0)
	for _, tc := range []struct{ entry, key string }{{".git", "r"}, {".git", "c"}, {".git", "d"}} {
		mark(t, m, tc.entry)
		send(m, tc.key)
		if m.set.form != nil || m.modal != nil || !strings.Contains(flat(m.render()), "lies in a .git folder, which is only read here.") {
			t.Fatalf("%s on %s:\n%s", tc.key, tc.entry, m.render())
		}
		send(m, "esc")
	}
	mark(t, m, ".git")
	send(m, "enter")
	send(m, "n")
	if m.set.form != nil || !strings.Contains(flat(m.render()), "lies in a .git folder, which is only read here.") {
		t.Fatalf("n in .git:\n%s", m.render())
	}
	if len(f.reqs) != 0 {
		t.Fatalf("commands were asked for: %v", f.reqs)
	}
}

// The refusal of the root action reaches the person in words.
func TestRefusalOfTheRootActionIsShown(t *testing.T) {
	m, f := changeModel(t, "/srv/media", 1, 0)
	f.report = `{"journal_id":"","action":"file.rename","sentence":"x","commands":[],"result":"refused","check":"","check_error":"",` +
		`"restored":false,"reversible":false,"error":"/srv/media/note.txt exists already; nothing is replaced","warning":""}`
	mark(t, m, "motd.txt")
	send(m, "r")
	fill(m, "note.txt")
	send(m, "y")
	containsFlat(t, m, "exists already; nothing is replaced")
}

// After a change the folder is read again, directly where the administrator reads it.
func TestChangeReadsTheFolderAgain(t *testing.T) {
	m, _ := changeModel(t, "/srv/media", 1, 0)
	reads := 0
	read := m.sys.fold.readPath
	m.sys.fold.readPath = func(p string) (hostchange.PathContent, error) { reads++; return read(p) }
	mark(t, m, "motd.txt")
	send(m, "d", "y")
	if reads == 0 {
		t.Fatalf("the folder was not read after the removal:\n%s", m.render())
	}
}

// A folder read through sudo is not read again by itself: that would ask for the password twice.
func TestChangeInAFolderReadThroughSudoDoesNotAskAgain(t *testing.T) {
	m, f := browserModel(t, i18n.English, wideWidth, 40)
	f.reports = func(args []string) string {
		if slices.Contains(args, "file") {
			return demoHostFile(t, args[len(args)-1])
		}
		return appliedReport("file.remove")
	}
	m.sys.fold.chg.measure = func(string) (int, int64, error) { return 1, 1, nil }
	browseTo(t, m, "/etc/vault-demo")
	send(m, "y")
	if !m.sys.fold.chg.viaSudo {
		t.Fatalf("the folder was not read through sudo:\n%s", m.render())
	}
	before := len(f.reqs)
	mark(t, m, "key")
	send(m, "d", "y")
	for _, r := range f.reqs[before:] {
		if slices.Contains(r.Args, "file") {
			t.Fatalf("the folder was read through sudo again: %v", r.Args)
		}
	}
	send(m, "enter") // the result
	containsFlat(t, m, "Press R to read it again")
	send(m, "n")
	if m.set.form != nil {
		t.Fatalf("a new entry was offered without the names:\n%s", m.render())
	}
	containsFlat(t, m, "is not read yet or must be read again")
}

// The command line offers the commands where the keys act, and the footer names the keys of the browser.
func TestBrowserFooterOffersTheChangeKeys(t *testing.T) {
	m, _ := changeModel(t, "/srv/media", 1, 0)
	mark(t, m, "motd.txt")
	var got []kittui.ActionID
	for _, h := range m.footer() {
		got = append(got, h.Action)
	}
	for _, want := range []kittui.ActionID{actEdit, actAdd, actRename, actCopy, actRemove} {
		if !slices.Contains(got, want) {
			t.Errorf("the footer lacks %s: %v", want, got)
		}
	}
}

func TestGoldenSystemFolderRemoveFinal(t *testing.T) {
	goldenSystem(t, "system.folder.remove.final", func(t *testing.T, m *Model) {
		m.profile = kittui.ProfileANSI16
		m.restyle()
		fakeHostFolders(m, 42)
		m.sys.fold.chg.measure = func(string) (int, int64, error) { return 312, bigTree, nil }
		toFolderPage(t, m)
		browseTo(t, m, "/srv")
		mark(t, m, "media")
		send(m, "d")
	})
}
