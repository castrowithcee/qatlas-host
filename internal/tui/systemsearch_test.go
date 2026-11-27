package tui

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/castrowithcee/qatlas-kit/i18n"
	"github.com/castrowithcee/qatlas-kit/projectpath"
	kittui "github.com/castrowithcee/qatlas-kit/tui"
)

// searchModel is the folders page over a real invented tree in a temporary folder, read with the rights of the
// test; sudo is faked and must stay unasked.
func searchModel(t *testing.T) (*Model, *fakeRoot, string) {
	t.Helper()
	root := t.TempDir()
	for _, d := range []string{"a/deep/deeper", "b", "locked"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{"a/deep/deeper/needle.txt", "b/needle.md", "locked/needle-hidden.txt", "b/other.txt"} {
		if err := os.WriteFile(filepath.Join(root, f), []byte("demo\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m, f := systemModel(t, i18n.English, wideWidth, 40)
	fakeHostFolders(m, 1)
	m.sys.fold.files.root, m.sys.fold.files.readPath = root, nil
	f.reports = func(args []string) string { t.Errorf("sudo asked: %v", args); return "" }
	toFolderPage(t, m)
	return m, f, root
}

func TestSearchWithStarFindsBelowTheShownFolderAndJumps(t *testing.T) {
	m, f, _ := searchModel(t)
	send(m, "/")
	send(m, typed("*needle")...)
	contains(t, m, "a/deep/deeper/needle.txt", "b/needle.md")
	send(m, "enter")
	b := m.sys.fold.files.browser
	if e, ok := b.Selected(); !ok || e.Name != "needle.md" || b.Searching() || !slices.Equal(b.Dir(), []string{"b"}) {
		t.Fatalf("no jump to the match: %+v in %v\n%s", e, b.Dir(), m.render())
	}
	if len(f.reqs) != 0 || m.modal != nil {
		t.Fatalf("the search asked for root: %v", f.reqs)
	}
}

func TestSearchSkipsFoldersWithoutReadRightAndNeverAsksSudo(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads every folder")
	}
	m, f, root := searchModel(t)
	if err := os.Chmod(filepath.Join(root, "locked"), 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(root, "locked"), 0o755) })
	send(m, "/")
	send(m, typed("*needle")...)
	contains(t, m, "needle.md")
	noLeak(t, m.render(), "needle-hidden")
	if len(f.reqs) != 0 || m.modal != nil {
		t.Fatalf("the search asked for sudo: %v", f.reqs)
	}
}

func TestNewSearchAndLeavingStopTheRunningOne(t *testing.T) {
	m, _, _ := searchModel(t)
	v := m.hostFiles()
	b := m.fileBrowser(v)
	send(m, "/")
	send(m, typed("*needle")...)
	_, first := m.Update(keyMsg("s"))
	if v.cancel == nil || first == nil {
		t.Fatal("no search started")
	}
	m.searchFiles(v, kittui.FileSearch{Query: "other", Token: 99})
	if got := first().(fileSearchMsg).res.End; got != projectpath.Canceled {
		t.Fatalf("the first search ended %v, want canceled by the new one", got)
	}
	send(m, "esc")
	if b.Searching() || v.cancel != nil {
		t.Fatal("leaving the search left it running")
	}
}

func TestSearchWithoutStarStillFilters(t *testing.T) {
	m, _, _ := searchModel(t)
	send(m, "/")
	// The filter also matches the owner column, which names the real UID of the test run, so the query fits only a name.
	send(m, typed("lock")...)
	send(m, "enter")
	if m.sys.fold.files.cancel != nil {
		t.Fatal("a plain filter started a search")
	}
	if e, ok := m.sys.fold.files.browser.Selected(); !ok || e.Name != "locked" {
		t.Fatalf("marked %+v:\n%s", e, m.render())
	}
}
