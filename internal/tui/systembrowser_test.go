package tui

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/castrowithcee/qatlas-kit/i18n"
	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/host/hostchange"
)

// demoNode is one entry of the invented tree the folders page browses; nothing of the machine is read for it.
type demoNode struct {
	kind    string
	uid     int
	mode    uint32
	content string
	target  string
	// rootOnly entries refuse a read without root, as a file of mode 0600 owned by root does.
	rootOnly, binary, large bool
}

const demoCompose = "# invented stack\nservices:\n  web:\n    image: example/web:1.4\n    ports:\n      - \"8080:80\"\n    restart: unless-stopped\n"

var demoTree = map[string]demoNode{
	"/":                      {kind: hostchange.KindFolder, mode: 0o755},
	"/etc":                   {kind: hostchange.KindFolder, mode: 0o755},
	"/etc/demo-app":          {kind: hostchange.KindFolder, mode: 0o755},
	"/etc/demo-app/app.yaml": {kind: hostchange.KindFile, mode: 0o644, content: demoCompose},
	"/etc/demo-app/logo.bin": {kind: hostchange.KindFile, mode: 0o644, binary: true},
	"/etc/demo-app/dump.sql": {kind: hostchange.KindFile, mode: 0o644, large: true},
	"/etc/hosts":             {kind: hostchange.KindFile, mode: 0o644, content: "127.0.0.1 localhost\n"},
	"/etc/motd":              {kind: hostchange.KindLink, mode: 0o777, target: "/srv/media/motd.txt"},
	"/etc/secret-demo":       {kind: hostchange.KindFile, mode: 0o600, rootOnly: true, content: "token=demo-only-0000\n"},
	"/etc/vault-demo":        {kind: hostchange.KindFolder, mode: 0o700, rootOnly: true},
	"/etc/vault-demo/key":    {kind: hostchange.KindFile, mode: 0o600, rootOnly: true, content: "demo\n"},
	"/srv":                   {kind: hostchange.KindFolder, mode: 0o755},
	"/srv/media":             {kind: hostchange.KindFolder, uid: 1000, mode: 0o750},
	"/srv/media/.git":        {kind: hostchange.KindFolder, uid: 1000, mode: 0o755},
	"/srv/media/.git/config": {kind: hostchange.KindFile, uid: 1000, mode: 0o644, content: "[core]\n\tbare = false\n"},
	"/srv/media/motd.txt":    {kind: hostchange.KindFile, uid: 1000, mode: 0o644, content: "Welcome.\n"},
}

var demoTime = time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)

func demoEntry(p string, n demoNode) hostchange.PathEntry {
	size := int64(len(n.content))
	switch {
	case n.large:
		size = 3 << 20
	case n.binary:
		size = 2048
	case n.kind == hostchange.KindFolder:
		size = 4096
	}
	return hostchange.PathEntry{Name: path.Base(p), Kind: n.kind, Size: size, UID: n.uid, GID: n.uid, Mode: n.mode,
		ModTime: demoTime, Target: n.target}
}

// readDemo reads the invented tree as ReadPath reads the host: rootOnly refuses unless root reads.
func readDemo(tree map[string]demoNode, p string, root bool) (hostchange.PathContent, error) {
	n, ok := tree[p]
	switch {
	case !ok:
		return hostchange.PathContent{}, hostchange.Refusef("%s does not exist", p)
	case n.rootOnly && !root:
		return hostchange.PathContent{}, fmt.Errorf("read %s: %w", p, fs.ErrPermission)
	}
	out := hostchange.PathContent{Entry: demoEntry(p, n)}
	switch {
	case n.kind == hostchange.KindFolder:
		out.Entries = []hostchange.PathEntry{}
		for k, c := range tree {
			if k != "/" && path.Dir(k) == p {
				out.Entries = append(out.Entries, demoEntry(k, c))
			}
		}
		slices.SortFunc(out.Entries, func(a, b hostchange.PathEntry) int { return strings.Compare(a.Name, b.Name) })
	case n.kind != hostchange.KindFile:
	case n.binary:
		out.Binary = true
	case n.large:
		out.TooLarge = true
	default:
		out.Content = []byte(n.content)
	}
	return out, nil
}

// demoTreeReader reads the invented tree with the rights of the interface m.
func demoTreeReader(m *Model) func(string) (hostchange.PathContent, error) {
	return func(p string) (hostchange.PathContent, error) { return readDemo(demoTree, p, m.euid() == 0) }
}

// demoHostFile is what sudo qhost host file --output json prints for p in the invented tree.
func demoHostFile(t *testing.T, p string) string {
	t.Helper()
	c, err := readDemo(demoTree, p, true)
	if err != nil {
		t.Fatal(err)
	}
	row := func(e hostchange.PathEntry) map[string]any {
		return map[string]any{"name": e.Name, "kind": e.Kind, "size": e.Size, "owner": "root", "group": "root",
			"mode": fmt.Sprintf("%04o", e.Mode), "modified": e.ModTime.Format(time.RFC3339), "target": e.Target}
	}
	var v any
	if c.Entries != nil {
		rows := []map[string]any{}
		for _, e := range c.Entries {
			rows = append(rows, row(e))
		}
		v = rows
	} else {
		obj := row(c.Entry)
		delete(obj, "name")
		obj["path"], obj["content"], obj["too_large"], obj["binary"] = p, string(c.Content), c.TooLarge, c.Binary
		v = obj
	}
	out, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// browserModel is the folders page of the interface without root over the invented tree; sudo answers with the
// invented host file.
func browserModel(t *testing.T, lang i18n.Lang, width, rows int) (*Model, *fakeRoot) {
	t.Helper()
	m, f := systemModel(t, lang, width, rows)
	fakeHostFolders(m, 1)
	f.reports = func(args []string) string { return demoHostFile(t, args[len(args)-1]) }
	toFolderPage(t, m)
	return m, f
}

// mark marks the entry name of the shown folder with the filter of the browser.
func mark(t *testing.T, m *Model, name string) {
	t.Helper()
	send(m, "/")
	send(m, typed(name)...)
	send(m, "enter")
	if e, ok := m.sys.fold.files.browser.Selected(); !ok || e.Name != name {
		t.Fatalf("marked %+v, want %s:\n%s", e, name, m.render())
	}
}

// browseTo opens the entries of p one after another from the root, as a person types them.
func browseTo(t *testing.T, m *Model, p string) {
	t.Helper()
	for _, name := range strings.Split(strings.TrimPrefix(p, "/"), "/") {
		mark(t, m, name)
		send(m, "enter")
	}
}

func TestBrowserStartsAtTheRootAndGoesUpWithEsc(t *testing.T) {
	m, _ := browserModel(t, i18n.English, wideWidth, 40)
	if got := m.sys.fold.files.browser.Dir(); len(got) != 0 {
		t.Fatalf("the browser starts in %v, want the root", got)
	}
	contains(t, m, "etc/", "srv/")
	browseTo(t, m, "/etc")
	contains(t, m, "hosts", "motd -> /srv/media/motd.txt", "rw-r--r--", "root")
	send(m, "esc")
	if got := m.sys.fold.files.browser.Dir(); len(got) != 0 {
		t.Fatalf("esc went to %v, want the root", got)
	}
	send(m, "esc")
	if m.focus() != kittui.ZoneSubtabs {
		t.Fatalf("esc at the root left the focus on %v", m.focus())
	}
}

// What the rights of the administrator reach is read directly; anything else only after the question, through
// sudo qhost host file.
func TestReadIsDirectWhereTheRightsReachAndThroughSudoElse(t *testing.T) {
	m, f := browserModel(t, i18n.English, wideWidth, 40)
	browseTo(t, m, "/etc/hosts")
	if len(f.reqs) != 0 || m.modal != nil {
		t.Fatalf("a readable file asked for root: %v", f.reqs)
	}
	contains(t, m, "/etc/hosts", "127.0.0.1 localhost")
	send(m, "esc", "esc") // the preview, then the filter that marked the file

	mark(t, m, "secret-demo")
	send(m, "enter")
	if m.modal == nil || len(f.reqs) != 0 {
		t.Fatalf("no question before sudo, or something ran: %v\n%s", f.reqs, m.render())
	}
	containsFlat(t, m, "Only root may read /etc/secret-demo.", "sudo qhost host file --output json -- /etc/secret-demo")
	noLeak(t, m.render(), "demo-only-0000")
	send(m, "n")
	if len(f.reqs) != 0 {
		t.Fatalf("n ran something: %v", f.reqs)
	}
	contains(t, m, "permission denied")

	send(m, "esc", "esc")
	mark(t, m, "secret-demo")
	send(m, "enter", "y")
	if got, want := f.lastArgs(t), []string{"file", "--output", "json", "--", "/etc/secret-demo"}; !slices.Equal(got, want) {
		t.Fatalf("arguments %q, want %q", got, want)
	}
	contains(t, m, "token=demo-only-0000")

	send(m, "esc", "esc")
	mark(t, m, "vault-demo")
	send(m, "enter", "y")
	contains(t, m, "key")
}

// Started as root, the interface reads everything directly and never asks for sudo.
func TestRootReadsDirectly(t *testing.T) {
	m, f := browserModel(t, i18n.English, wideWidth, 40)
	m.euid = func() int { return 0 }
	browseTo(t, m, "/etc/secret-demo")
	if len(f.reqs) != 0 || m.modal != nil {
		t.Fatalf("root asked for sudo: %v", f.reqs)
	}
	contains(t, m, "token=demo-only-0000")
}

func TestBinaryLargeAndLinkedFilesAreNamedNotShown(t *testing.T) {
	m, _ := browserModel(t, i18n.English, wideWidth, 40)
	for p, want := range map[string]string{
		"/etc/demo-app/logo.bin": "/etc/demo-app/logo.bin is a binary file. It is not shown.",
		"/etc/demo-app/dump.sql": "/etc/demo-app/dump.sql is larger than 1 MiB. It is not shown.",
		"/etc/motd":              "/etc/motd is a link to /srv/media/motd.txt.",
	} {
		send(m, "esc", "esc", "esc", "esc", "esc")
		m.setZone(kittui.ZoneRows)
		browseTo(t, m, p)
		if !strings.Contains(flat(m.render()), want) {
			t.Errorf("%s: want %q:\n%s", p, want, m.render())
		}
	}
}

// A .git folder is shown and read, but its permissions are not changed here.
func TestGitFoldersAreOnlyRead(t *testing.T) {
	for p, want := range map[string]bool{"/srv/media/.git": true, "/srv/media/.git/config": true, "/srv/media": false,
		"/srv/media.git": false, "/.github": false} {
		if gitPath(p) != want {
			t.Errorf("gitPath(%s) = %v", p, !want)
		}
	}
	m, _ := browserModel(t, i18n.English, wideWidth, 40)
	browseTo(t, m, "/srv/media/.git/config")
	contains(t, m, "bare = false")
	send(m, "esc", "esc", "esc")
	mark(t, m, ".git")
	send(m, "i")
	contains(t, m, "Owner cleo may: read and write")
	if slices.ContainsFunc(m.footer(), func(h kittui.Hint) bool { return h.Action == actEdit }) {
		t.Errorf("the permissions of a .git folder offer a change:\n%s", m.render())
	}
	send(m, "e")
	if m.set.form != nil || !strings.Contains(flat(m.render()), "lies in a .git folder, which is only read here.") {
		t.Fatalf("e changed a .git folder:\n%s", m.render())
	}
	send(m, "esc", "esc")
	mark(t, m, "media")
	send(m, "i", "e")
	if m.set.form == nil {
		t.Fatalf("the folder beside it cannot be changed:\n%s", m.render())
	}
}

// Names and contents of the host are foreign text: no control sequence of them reaches the terminal.
func TestNamesAndContentsAreEscaped(t *testing.T) {
	m, _ := browserModel(t, i18n.English, wideWidth, 40)
	tree := map[string]demoNode{
		"/":            {kind: hostchange.KindFolder, mode: 0o755},
		"/bad\x1b[2Jx": {kind: hostchange.KindFile, mode: 0o644, content: "a\x1b]0;owned\x07b\n"},
	}
	m.sys.fold.files.readPath = func(p string) (hostchange.PathContent, error) { return readDemo(tree, p, false) }
	send(m, "R")
	send(m, "enter")
	screen := m.render()
	for _, bad := range []string{"\x1b[2J", "\x1b]0;", "\x07"} {
		if strings.Contains(screen, bad) {
			t.Fatalf("%q reached the screen:\n%q", bad, screen)
		}
	}
	contains(t, m, `\x1b`)
}

func TestKeyTableHoldsTheTextViewActions(t *testing.T) {
	keys, err := kittui.NewKeymap(keyTable(), kittui.CharsetUnicode)
	if err != nil {
		t.Fatal(err)
	}
	want := map[kittui.ActionID]string{kittui.ActionTextNumbers: "#", kittui.ActionTextReveal: "V", actRights: "i"}
	for _, a := range keys.Actions() {
		if k, ok := want[a.ID]; ok {
			if !slices.Contains(a.DefaultKeys, k) {
				t.Errorf("%s has %v, want %s", a.ID, a.DefaultKeys, k)
			}
			delete(want, a.ID)
		}
		if a.ID == actLogLevel && slices.Contains(a.DefaultKeys, "V") {
			t.Errorf("the minimum level of the service log still holds V")
		}
	}
	if len(want) != 0 {
		t.Errorf("missing actions %v", want)
	}
}
