package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-kit/i18n"
	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// pathTree is a folder with two folders and a file, the place the tests type paths into.
func pathTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, d := range []string{"alpha-one/deep-needle", "beta-two"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "key-file"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestFolderFieldSuggestsTheEntriesOfEachLevel(t *testing.T) {
	root := pathTree(t)
	m := newSettingsModel(t, foldersBackend())
	send(m, seq(toFolders, keys("n"), typed(root+"/al"))...)
	contains(t, m, "alpha-one/")
	if screen := m.render(); strings.Contains(screen, "beta-two") || strings.Contains(screen, "key-file") {
		t.Fatalf("an entry that does not match, or a file, is offered:\n%s", screen)
	}
	send(m, "down", "tab")
	send(m, typed("d")...)
	contains(t, m, "deep-needle/")
}

func TestFolderFieldSearchesFromTheStartPath(t *testing.T) {
	root := pathTree(t)
	m := newSettingsModel(t, foldersBackend())
	send(m, seq(toFolders, keys("n"), typed(root+"/*needle"))...)
	contains(t, m, filepath.Join(root, "alpha-one", "deep-needle"))
}

func TestFolderFieldOffersTheFoldersOfForeignProjectsFirst(t *testing.T) {
	b := foldersBackend()
	b.containers = control.ContainerListResult{Groups: []control.ContainerGroup{
		{Kind: control.ContainerKindForeign, Name: "legacy-blog", WorkingDir: "/srv/example/blog"},
		{Kind: control.ContainerKindForeign, Name: "bound", WorkingDir: stackDir},
		{Kind: control.ContainerKindForeign, Name: "readonly", ReadOnly: true, WorkingDir: "/srv/example/readonly"},
		{Kind: control.ContainerKindOwn, Name: "own", WorkingDir: "/srv/example/own"},
	}}
	m := newSettingsModel(t, b)
	send(m, seq(toFolders, keys("n"))...)
	if b.called("containers") != 1 {
		t.Fatalf("the containers were read %d times, want once", b.called("containers"))
	}
	contains(t, m, "/srv/example/blog/")
	screen := m.render()
	for _, no := range []string{stackDir + "/", "/srv/example/readonly", "/srv/example/own"} {
		if strings.Contains(screen, no) {
			t.Fatalf("%s is offered:\n%s", no, screen)
		}
	}
	// Opening it again takes the list read before.
	send(m, "esc", "n")
	contains(t, m, "/srv/example/blog/")
	if b.called("containers") != 1 {
		t.Fatalf("the containers were read again")
	}
}

func TestFolderFieldKeepsWhatIsTypedWhenTheContainersArrive(t *testing.T) {
	b := foldersBackend()
	b.containers = control.ContainerListResult{Groups: []control.ContainerGroup{
		{Kind: control.ContainerKindForeign, Name: "legacy-blog", WorkingDir: "/srv/example/blog"}}}
	m := newSettingsModel(t, b)
	_, cmd := m.Update(keyMsg("1"))
	drain(m, cmd)
	_, cmd = m.Update(keyMsg("n"))
	for _, k := range typed("/srv/ex") {
		_, c := m.Update(keyMsg(k))
		drain(m, c)
	}
	drain(m, cmd)
	if got := m.set.form.form.Values()["path"].Text; got != "/srv/ex" {
		t.Fatalf("the typed text is %q", got)
	}
	contains(t, m, "/srv/example/blog/")
}

func TestKeyFieldSuggestsFilesToo(t *testing.T) {
	root := pathTree(t)
	m := newSettingsModel(t, keyBackend())
	send(m, toKey...)
	send(m, "e", "f2", "down")
	send(m, typed(root+"/ke")...)
	contains(t, m, "key-file")
}

func TestFieldSuggestionsNeverAskForSudo(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads every folder")
	}
	root := pathTree(t)
	closed := filepath.Join(root, "closed")
	if err := os.Mkdir(closed, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(closed, 0o755) })
	b := foldersBackend()
	m, f := sudoFolderModel(t, i18n.English, b, wideWidth, 40, nil)
	send(m, seq(toFolders, keys("n"), typed(closed+"/x"), typed("*y"))...)
	if m.modal != nil {
		t.Fatalf("a question is open:\n%s", m.render())
	}
	wantCommands(t, m, f)
	untouched(t, b)
	if res := answerPath(context.Background(), kittui.PathLoad{Typed: closed + "/", Kind: kittui.PathDirs}); len(res.Entries) != 0 {
		t.Fatalf("entries of a folder that cannot be read: %v", res.Entries)
	}
}

func TestSearchEndsWhenCanceled(t *testing.T) {
	root := pathTree(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res := answerPath(ctx, kittui.PathLoad{Typed: root + "/*needle", Search: true, Kind: kittui.PathDirs})
	if res.End != kittui.PathEndCanceled {
		t.Fatalf("end = %v, want canceled", res.End)
	}
}

// foreignFoldersBackend is the folders host with two foreign projects that can be taken over.
func foreignFoldersBackend() *fakeBackend {
	b := foldersBackend()
	b.containers = control.ContainerListResult{Groups: []control.ContainerGroup{
		{Kind: control.ContainerKindForeign, Name: "legacy-blog", WorkingDir: "/srv/example/blog"},
		{Kind: control.ContainerKindForeign, Name: "wiki", WorkingDir: "/srv/example/wiki"},
	}}
	return b
}

func TestGoldenStacksAddFolderList(t *testing.T) {
	goldenScreens(t, "stacks.add.pathlist", settingsRows, foreignFoldersBackend, seq(toFolders, keys("n"), typed("/srv/ex"), keys("down"))...)
}
