package tui

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-kit/exteditor"
	"github.com/castrowithcee/qatlas-kit/i18n"
	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/host/hostchange"
)

// The invented values of secrets.env; no screen shows them unless the preview uncovers them.
const (
	stackSecretA = "demo-only-secret-4711"
	stackSecretB = "demo-only-token-0815"
)

const stackSecrets = "# invented values\nDB_PASSWORD=" + stackSecretA + "\nexport API_TOKEN=\"" + stackSecretB + "\"\n"

// stackTree is the invented stack folder the browser of the Stacks area reads; nothing of the machine is read.
var stackTree = map[string]demoNode{
	stackDir:                                    {kind: hostchange.KindFolder, uid: 1000, mode: 0o2775},
	stackDir + "/.git":                          {kind: hostchange.KindFolder, uid: 1000, mode: 0o2775},
	stackDir + "/.git/HEAD":                     {kind: hostchange.KindFile, uid: 1000, mode: 0o664, content: "ref: refs/heads/main\n"},
	stackDir + "/.qatlas":                       {kind: hostchange.KindFolder, uid: 1000, mode: 0o2775},
	stackDir + "/.qatlas/qhost":                 {kind: hostchange.KindFolder, uid: 1000, mode: 0o2775},
	stackDir + "/.qatlas/qhost/demo-stack.yaml": {kind: hostchange.KindFile, uid: 1000, mode: 0o664, content: "update:\n  mode: automatic\n"},
	stackDir + "/demo-stack":                    {kind: hostchange.KindFolder, uid: 1000, mode: 0o2775},
	stackDir + "/demo-stack/compose.yaml":       {kind: hostchange.KindFile, uid: 1000, mode: 0o664, content: demoCompose},
	stackDir + "/demo-stack/.env":               {kind: hostchange.KindFile, uid: 1000, mode: 0o664, content: "APP_VERSION=1.4.2\n"},
	stackDir + "/demo-stack/secrets.env":        {kind: hostchange.KindFile, uid: 1000, mode: 0o600, content: stackSecrets},
	stackDir + "/demo-stack/data":               {kind: hostchange.KindFolder, uid: 1001, mode: 0o750},
	stackDir + "/demo-stack/data/notes.txt":     {kind: hostchange.KindFile, uid: 1001, mode: 0o640, content: "demo\n"},
	stackDir + "/demo-stack/shared":             {kind: hostchange.KindLink, uid: 1000, mode: 0o777, target: "/etc"},
}

// stackFilesBackend is the folders of foldersBackend; demo-stack lies in its own folder of the first one, and its
// app container runs with the data folder mounted.
func stackFilesBackend() *fakeBackend {
	b := foldersBackend()
	for i := range b.stacks.Stacks {
		b.stacks.Stacks[i].Folder = b.stacks.Stacks[i].Name
	}
	app := summary("demo-stack-app-1", "example.invalid/app:1.4.2", control.ContainerOK, "running", 3600, 0)
	b.containers = control.ContainerListResult{Groups: []control.ContainerGroup{
		{Kind: control.ContainerKindOwn, Name: "demo-stack", State: control.ContainerOK, Containers: []control.ContainerSummary{app}}}}
	b.containerDetails = map[string]control.ContainerDetail{"demo-stack-app-1": {ContainerSummary: app, Kind: control.ContainerKindOwn,
		Project: "demo-stack", Mounts: []control.ContainerMount{{Type: "bind", Source: stackDir + "/demo-stack/data", Destination: "/data"}}}}
	return b
}

// fakeStackFiles makes the browser of the Stacks area read the invented stack folder, and edit nowhere.
func fakeStackFiles(m *Model) *[]string {
	var reads []string
	m.sf.view.readPath = func(p string) (hostchange.PathContent, error) {
		reads = append(reads, p)
		return readDemo(stackTree, p, m.euid() == 0)
	}
	m.sf.view.hooks.pick = func(int, *exteditor.Target) (privateDir, error) {
		return privateDir{}, errors.New("no private folder in tests")
	}
	return &reads
}

// stackFilesModel is the Stacks area without root, with the browser open on the first stack folder.
func stackFilesModel(t *testing.T, lang i18n.Lang, b *fakeBackend, width, rows int) (*Model, *fakeRoot, *[]string) {
	t.Helper()
	m := newTestModel(t, lang, b, width, rows)
	m.euid, m.human = func() int { return 1000 }, func() int { return 1000 }
	f := &fakeRoot{reports: func(argv []string) string {
		if slices.Contains(argv, "apply") {
			return appliedReport("file.write")
		}
		return stackReports(argv)
	}}
	m.root = f.hook()
	reads := fakeStackFiles(m)
	send(m, "up", "Q")
	if m.shownStackFiles() == nil || m.sf.view.root != stackDir {
		t.Fatalf("Q on the folder row did not open its files:\n%s", m.render())
	}
	return m, f, reads
}

// markStackEntry marks the entry name of the folder the stack browser shows.
func markStackEntry(t *testing.T, m *Model, name string) {
	t.Helper()
	send(m, "/")
	send(m, typed(name)...)
	send(m, "enter")
	if e, ok := m.sf.view.browser.Selected(); !ok || e.Name != name {
		t.Fatalf("marked %+v, want %s:\n%s", e, name, m.render())
	}
}

// openStackEntries opens the entries of rel below the stack folder one after another.
func openStackEntries(t *testing.T, m *Model, rel string) {
	t.Helper()
	for _, name := range strings.Split(rel, "/") {
		markStackEntry(t, m, name)
		send(m, "enter")
	}
}

// The acceptance: the stack folder is the root; esc there leaves the browser, every read lies below the folder and
// a link is shown as a link, never opened as a folder.
func TestStackFilesRootIsNotLeft(t *testing.T) {
	m, _, reads := stackFilesModel(t, i18n.English, stackFilesBackend(), wideWidth, 40)
	contains(t, m, stackDir, ".qatlas/", "demo-stack/")
	openStackEntries(t, m, "demo-stack")
	markStackEntry(t, m, "shared")
	send(m, "enter")
	containsFlat(t, m, "is a link to /etc.")
	send(m, "esc", "esc", "esc")
	if got := m.sf.view.browser.Dir(); len(got) != 0 {
		t.Fatalf("esc went to %v, want the root of the stack folder", got)
	}
	send(m, "esc")
	if m.shownStackFiles() != nil {
		t.Fatalf("esc at the root kept the browser:\n%s", m.render())
	}
	if f, ok := m.selectedFolder(); !ok || f.Path != stackDir {
		t.Fatalf("the list marks %+v, want the folder", f)
	}
	for _, p := range *reads {
		if !atOrBelow(stackDir, p) {
			t.Errorf("read %s outside the stack folder", p)
		}
	}
}

// A stack folder that has become a link is not opened through it.
func TestStackFolderAsLinkIsNotFollowed(t *testing.T) {
	m := newTestModel(t, i18n.English, stackFilesBackend(), wideWidth, 40)
	m.euid = func() int { return 1000 }
	fakeStackFiles(m)
	m.sf.view.readPath = func(p string) (hostchange.PathContent, error) {
		return readDemo(map[string]demoNode{stackDir: {kind: hostchange.KindLink, mode: 0o777, target: "/etc"}}, p, false)
	}
	send(m, "up", "Q")
	containsFlat(t, m, "this is no longer a folder")
}

func TestStackReadOnlyPaths(t *testing.T) {
	for p, want := range map[string]bool{
		stackDir + "/.git": true, stackDir + "/.git/HEAD": true, stackDir + "/.qatlas/qhost": true,
		stackDir + "/.qatlas/qhost/demo-stack.yaml": true, stackDir + "/.qatlas": false, stackDir + "/.qatlas/other": false,
		stackDir + "/demo-stack/.env": false, stackDir + "/.github": false,
	} {
		if got := stackReadOnly(p) != ""; got != want {
			t.Errorf("stackReadOnly(%s) = %v, want %v", p, got, want)
		}
	}
}

// The acceptance: .qatlas/qhost and .git are read, never changed, and the refusal points to the forms.
func TestStackSettingsAndGitAreOnlyRead(t *testing.T) {
	m, f, _ := stackFilesModel(t, i18n.English, stackFilesBackend(), wideWidth, 40)
	fakeEditIn(t, m, &m.sf.view)
	openStackEntries(t, m, ".qatlas/qhost/demo-stack.yaml")
	contains(t, m, "mode: automatic")
	send(m, "e")
	containsFlat(t, m, "is only read here", "the forms of the stack detail change")
	send(m, "esc", "esc")
	for _, key := range []string{"n", "r", "d", "e"} {
		send(m, key)
		if m.set.form != nil || m.modal != nil {
			t.Fatalf("%s acts in .qatlas/qhost:\n%s", key, m.render())
		}
	}
	send(m, "esc", "esc")
	markStackEntry(t, m, ".git")
	send(m, "d")
	if m.modal != nil || !strings.Contains(flat(m.render()), "is only read here") {
		t.Fatalf("d on .git asks:\n%s", m.render())
	}
	if len(f.reqs) != 0 {
		t.Fatalf("something ran: %v", f.reqs)
	}
}

// The acceptance: the values of secrets.env are masked, V uncovers them and hides them again.
func TestSecretsEnvValuesAreMasked(t *testing.T) {
	m, _, _ := stackFilesModel(t, i18n.English, stackFilesBackend(), wideWidth, 40)
	openStackEntries(t, m, "demo-stack/secrets.env")
	contains(t, m, "DB_PASSWORD", "API_TOKEN", "# invented values", "reveal")
	noLeak(t, m.render(), stackSecretA, stackSecretB)
	send(m, "V")
	contains(t, m, stackSecretA, stackSecretB)
	send(m, "V")
	noLeak(t, m.render(), stackSecretA, stackSecretB)
	send(m, "V", "esc", "esc")
	openStackEntries(t, m, "secrets.env")
	noLeak(t, m.render(), stackSecretA, stackSecretB)
}

func TestSecretsMaskCoversEveryValue(t *testing.T) {
	text := "# note\n\nA=one\n  export B = 'two'\nC=\"three\nfour=five\"\nbroken line\n"
	m := newTestModel(t, i18n.English, demoBackend(), wideWidth, 30)
	tv, err := kittui.NewTextView(kittui.TextViewSpec{Keys: textKeys()}, m.keys, m.text)
	if err != nil {
		t.Fatal(err)
	}
	tv.SetText(text, stackFileSpans("/srv/x/secrets.env", text))
	view, _ := tv.View(m.styles, 60, 12)
	for _, hidden := range []string{"one", "two", "three", "four", "five", "broken"} {
		if strings.Contains(view, hidden) {
			t.Errorf("%q shows:\n%s", hidden, view)
		}
	}
	for _, shown := range []string{"# note", "A=", "export B ="} {
		if !strings.Contains(view, shown) {
			t.Errorf("%q is missing:\n%s", shown, view)
		}
	}
	if spans := stackFileSpans("/srv/x/.env", "A=one\n"); slices.ContainsFunc(spans[0], func(s kittui.TextSpan) bool { return s.Masked }) {
		t.Errorf(".env is masked: %v", spans)
	}
}

// editStackFile edits rel below the stack folder with the invented editor and saves text.
func editStackFile(t *testing.T, m *Model, rel, text string) {
	t.Helper()
	openStackEntries(t, m, rel)
	send(m, "e")
	if m.sf.view.edit == nil {
		t.Fatalf("no edit of %s: %q\n%s", rel, m.notice, m.render())
	}
	edited(m, exteditor.ChangedMsg{Text: []byte(text)})
	containsFlat(t, m, "Write the edited file", "back in place")
	send(m, "y")
}

// The acceptance: after a save in the stack folder the row of the folder names the uncommitted changes.
func TestSavingInTheStackFolderShowsTheUncommittedChanges(t *testing.T) {
	b := stackFilesBackend()
	m, f, _ := stackFilesModel(t, i18n.English, b, wideWidth, 40)
	fakeEditIn(t, m, &m.sf.view)
	if strings.Contains(m.render(), "uncommitted") {
		t.Fatalf("a hint before any change:\n%s", m.render())
	}
	b.changes = map[string][]control.SourceChange{stackDir: {{Path: "demo-stack/.env", Kind: control.ChangeModified, Stack: "demo-stack"}}}
	editStackFile(t, m, "demo-stack/.env", "APP_VERSION=1.5.0\n")
	if got := f.lastArgs(t); !slices.Equal(got, []string{"apply", "--confirm", "--output", "json", "--", "file.write", stackDir + "/demo-stack/.env"}) {
		t.Fatalf("arguments %q", got)
	}
	send(m, "enter")
	containsFlat(t, m, "This folder has uncommitted changes")
	send(m, "esc", "esc", "esc", "esc")
	// The row of the folder carries the short note; the panel below it says what it means.
	row := func(l string) bool { return strings.Contains(l, stackDir+" (") && strings.Contains(l, "uncommitted") }
	if !slices.ContainsFunc(strings.Split(m.render(), "\n"), row) {
		t.Fatalf("the row of the folder names no uncommitted change:\n%s", m.render())
	}
	containsFlat(t, m, "updates leave it out until they are committed or discarded")
	if m.modal != nil || m.shownStackFiles() != nil {
		t.Fatalf("the list is not shown:\n%s", m.render())
	}
}

// After saving secrets.env the interface says it takes effect at the next deploy and deploys through sudo on y.
func TestSavingSecretsOffersToDeploy(t *testing.T) {
	m, f, _ := stackFilesModel(t, i18n.English, stackFilesBackend(), wideWidth, 40)
	fakeEditIn(t, m, &m.sf.view)
	editStackFile(t, m, "demo-stack/secrets.env", "DB_PASSWORD="+stackSecretB+"\n")
	send(m, "enter")
	containsFlat(t, m, "takes effect when the stack demo-stack is deployed next. Deploy it now?")
	send(m, "y")
	want := []string{"stack", "deploy", "--output", "json", "--", "demo-stack"}
	if !shows(m.render(), "sudo qhost "+strings.Join(want, " ")) {
		t.Fatalf("no sudo question for the deploy:\n%s", m.render())
	}
	send(m, "y")
	if got := commandsOf(f); !slices.Equal(got[len(got)-1], want) {
		t.Fatalf("commands %v, want %v last", got, want)
	}
	noLeak(t, m.render()+m.failure+m.notice+m.result, stackSecretA, stackSecretB)
}

// The acceptance: a change in the data folder of a running container warns in the question but is not refused.
func TestDataFolderOfARunningContainerWarns(t *testing.T) {
	for _, running := range []bool{true, false} {
		b := stackFilesBackend()
		if !running {
			b.containers.Groups[0].Containers[0].EngineState = "exited"
		}
		m, f, _ := stackFilesModel(t, i18n.English, b, wideWidth, 40)
		openStackEntries(t, m, "demo-stack/data")
		send(m, "n", "down")
		send(m, typed("demo.txt")...)
		send(m, "f2")
		screen := flat(m.render())
		warned := strings.Contains(screen, "is the data folder of the running container demo-stack-app-1")
		if m.modal == nil || warned != running {
			t.Fatalf("running %v: warned %v:\n%s", running, warned, m.render())
		}
		send(m, "y")
		if got := f.lastArgs(t); !slices.Contains(got, stackDir+"/demo-stack/data/demo.txt") {
			t.Fatalf("running %v: arguments %q", running, got)
		}
	}
}

// The detail opens the files of its stack in the folder of the stack.
func TestDetailOpensTheFilesOfItsStack(t *testing.T) {
	m := newTestModel(t, i18n.English, stackFilesBackend(), wideWidth, 40)
	fakeStackFiles(m)
	send(m, "enter")
	if m.area != areaDetail {
		t.Fatalf("area %s", m.area)
	}
	send(m, "Q")
	if m.shownStackFiles() == nil || !slices.Equal(m.sf.view.browser.Dir(), []string{"demo-stack"}) {
		t.Fatalf("the detail opened %v:\n%s", m.sf.view.browser, m.render())
	}
	contains(t, m, "compose.yaml", "secrets.env")
}
