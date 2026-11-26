package tui

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/host/hostchange"
)

// System > Folders is a file browser over the whole host, from /. What the administrator may read is read
// directly; anything else is read as root through sudo qhost host file, after a question with the sentence and the
// exact command. Started as root, the interface reads everything directly. A link is shown with its target and never
// followed. e edits the marked file in the editor of the person (systemedit.go). Rights shows the permissions of the
// marked entry, where e changes them; nothing inside a .git folder is changed here.

// hostEntry is one entry of the file system as it was read, a link not followed. The tags are those of
// qhost host file.
type hostEntry struct {
	Name     string    `json:"name"`
	Kind     string    `json:"kind"`
	Size     int64     `json:"size"`
	Owner    string    `json:"owner"`
	Mode     string    `json:"mode"`
	Modified time.Time `json:"modified"`
	Target   string    `json:"target"`
}

// hostRead is what one read of a path found: the entries of a folder, or the content of a file.
type hostRead struct {
	hostEntry
	Folder   bool        `json:"-"`
	Entries  []hostEntry `json:"-"`
	Content  string      `json:"content"`
	TooLarge bool        `json:"too_large"`
	Binary   bool        `json:"binary"`
}

// fileReadMsg carries a direct read of path for the place view. load is the request of the browser a folder
// answers; a file read for the preview has none and carries the gen of its preview.
type fileReadMsg struct {
	view *fileView
	gen  int
	path string
	load *kittui.FileLoad
	read hostchange.PathContent
	err  error
}

func (msg fileReadMsg) applyTo(m *Model) { m.applyFileRead(msg) }

// fileBrowser is the browser of the place, made on first use.
func (m *Model) fileBrowser(v *fileView) *kittui.FileBrowser {
	if v.browser == nil {
		actions := []kittui.FormButton{{Action: actEdit, Label: textKeyEditFile}, {Action: actAdd, Label: kittui.TextKeyNew},
			{Action: actRename, Label: kittui.TextKeyRename}, {Action: actCopy, Label: kittui.TextKeyCopy},
			{Action: actRemove, Label: kittui.TextKeyDelete}}
		if v.rules.rights != nil {
			actions = append(actions, kittui.FormButton{Action: actRights, Label: textKeyRights})
		}
		b, err := kittui.NewFileBrowser(kittui.FileBrowserSpec{Keys: fileBrowserKeys(), Actions: actions, RootLabel: v.rules.rootLabel},
			m.keys, m.text)
		if err != nil {
			m.failure = m.text.Lookup(kittui.TextError, err.Error())
			return nil
		}
		v.browser = b
	}
	return v.browser
}

// showFolders reads the root when the page is shown for the first time.
func (m *Model) showFolders() tea.Cmd { return m.startFiles(m.hostFiles()) }

// startFiles reads the root of the place when it is shown for the first time.
func (m *Model) startFiles(v *fileView) tea.Cmd {
	b := m.fileBrowser(v)
	if b == nil || v.started {
		return nil
	}
	v.started = true
	return m.fileResult(v, b.Load(nil))
}

// reloadFolders reads what the page shows again, as on R; the permissions are read again by loadSystem.
func (m *Model) reloadFolders() tea.Cmd {
	if m.sys.fold.asked != "" {
		return nil
	}
	return m.reloadFiles(m.hostFiles())
}

// reloadFiles reads what the place shows again: the preview, or the folder of the browser.
func (m *Model) reloadFiles(v *fileView) tea.Cmd {
	if v.preview != nil {
		return m.openFilePreview(v, v.preview.path)
	}
	if b := m.fileBrowser(v); b != nil {
		r, _ := b.Act(actReload)
		return m.fileResult(v, r)
	}
	return nil
}

// gitPath reports whether path lies in a .git folder or is one, which the page only reads.
func gitPath(path string) bool { return slices.Contains(strings.Split(path, "/"), ".git") }

// fileResult does what a key or a click on the browser of the place asked for.
func (m *Model) fileResult(v *fileView, r kittui.FileResult) tea.Cmd {
	switch {
	case r.Load != nil:
		load := *r.Load
		return m.readHostPath(v, v.path(load.Dir...), &load, 0)
	case r.Action == nil:
		return nil
	}
	a := *r.Action
	if cmd, ok := m.fileChange(v, a); ok {
		return cmd
	}
	path := v.path(a.Dir...)
	if a.HasEntry {
		path = v.path(append(a.Dir, a.Entry.Name)...)
	}
	switch {
	case a.Action == actRights && v.rules.rights != nil:
		return v.rules.rights(path)
	case a.Action == actEnter && a.HasEntry:
		return m.openFilePreview(v, path)
	case a.Action == actEdit && a.HasEntry:
		return m.editFile(v, path, a.Entry.Kind)
	}
	return nil
}

// readHostPath reads path directly for the place, off the event loop.
func (m *Model) readHostPath(v *fileView, path string, load *kittui.FileLoad, gen int) tea.Cmd {
	read := v.readPath
	if read == nil {
		read = hostchange.ReadPath
	}
	return func() tea.Msg {
		c, err := read(path)
		return fileReadMsg{view: v, gen: gen, path: path, load: load, read: c, err: err}
	}
}

// applyFileRead shows a direct read. What the rights of the administrator do not reach is asked for through sudo.
func (m *Model) applyFileRead(msg fileReadMsg) {
	v := msg.view
	if msg.load == nil && !v.previewCurrent(msg.gen) {
		return
	}
	switch {
	case msg.err == nil:
		v.chg.listed(msg.load, false)
		m.showHostRead(v, msg.load, msg.gen, m.directRead(msg.read))
	case errors.Is(msg.err, fs.ErrPermission) && m.euid() != 0:
		m.askReadRoot(v, msg.path, msg.load, msg.gen)
	default:
		m.failHostRead(v, msg.load, msg.gen, msg.err.Error())
	}
}

// askReadRoot asks to read path as root; y hands the terminal to sudo qhost host file.
func (m *Model) askReadRoot(v *fileView, path string, load *kittui.FileLoad, gen int) {
	t := m.text
	m.askSudo(sudoAsk{title: textTitleReadRoot, question: t.Lookup(textAskReadRoot, path), consequence: t.Lookup(textWhatReadRoot),
		args: []string{"host", "file", "--output", "json", "--", path},
		done: func(r sudoRun) tea.Cmd {
			if read, ok := decodeHostFile(r.out); ok {
				v.chg.listed(load, true)
				m.showHostRead(v, load, gen, read)
			} else {
				m.failHostRead(v, load, gen, r.problem)
			}
			return nil
		},
		drop: func() { m.failHostRead(v, load, gen, fs.ErrPermission.Error()) }})
}

// decodeHostFile reads the answer of qhost host file: a list for a folder, an object for anything else.
func decodeHostFile(out []byte) (hostRead, bool) {
	out = bytes.TrimSpace(out)
	if bytes.HasPrefix(out, []byte("[")) {
		r := hostRead{Folder: true}
		return r, json.Unmarshal(out, &r.Entries) == nil
	}
	var r hostRead
	return r, json.Unmarshal(out, &r) == nil && r.Kind != ""
}

// directRead turns a read of the file system into what the page shows, the owners by name.
func (m *Model) directRead(c hostchange.PathContent) hostRead {
	r := hostRead{hostEntry: m.entryOf(c.Entry), Content: string(c.Content), TooLarge: c.TooLarge, Binary: c.Binary,
		Folder: c.Entry.Kind == hostchange.KindFolder}
	for _, e := range c.Entries {
		r.Entries = append(r.Entries, m.entryOf(e))
	}
	return r
}

func (m *Model) entryOf(e hostchange.PathEntry) hostEntry {
	owner := strconv.Itoa(e.UID)
	for _, u := range m.sys.users {
		if u.UID == e.UID {
			owner = u.Name
			break
		}
	}
	return hostEntry{Name: e.Name, Kind: e.Kind, Size: e.Size, Owner: owner, Mode: fmt.Sprintf("%04o", e.Mode),
		Modified: e.ModTime, Target: e.Target}
}

// fileEntry is the row of an entry in the browser.
func fileEntry(e hostEntry) kittui.FileEntry {
	f := kittui.FileEntry{Name: e.Name, Size: e.Size, Owner: e.Owner, Modified: e.Modified}
	if mode, err := strconv.ParseUint(e.Mode, 8, 32); err == nil {
		f.Rights = fs.FileMode(mode & 0o777).String()[1:]
	}
	switch e.Kind {
	case hostchange.KindFolder:
		f.Kind = kittui.FileDir
	case hostchange.KindLink:
		f.Kind, f.LinkTarget = kittui.FileLink, e.Target
	case hostchange.KindOther:
		f.Size = -1
	}
	return f
}

// showHostRead shows a read: the entries of a folder in the browser, or a file in the preview.
func (m *Model) showHostRead(v *fileView, load *kittui.FileLoad, gen int, r hostRead) {
	if load == nil {
		m.showPreview(v, gen, r)
		return
	}
	b := m.fileBrowser(v)
	if b == nil {
		return
	}
	if !r.Folder {
		b.SetError(*load, errors.New(m.text.Lookup(textFileNoFolder)))
		return
	}
	entries := make([]kittui.FileEntry, len(r.Entries))
	for i, e := range r.Entries {
		entries[i] = fileEntry(e)
	}
	// A refused folder is shown as failed with its reason.
	if current, err := b.SetEntries(*load, entries); current && err == nil {
		v.chg.setNames(r.Entries)
	}
}

// failHostRead shows why a read gave nothing.
func (m *Model) failHostRead(v *fileView, load *kittui.FileLoad, gen int, reason string) {
	if load == nil {
		if v.previewCurrent(gen) {
			p := v.preview
			p.reading, p.note = false, m.text.Lookup(textFolderFailed, p.path, reason)
		}
		return
	}
	if b := m.fileBrowser(v); b != nil {
		b.SetError(*load, errors.New(reason))
		v.chg.names = nil
	}
}

// foldersInput is the input focus of the folders page.
func (m *Model) foldersInput() kittui.InputFocus {
	if m.sys.page != pageFolders || m.sys.fold.asked != "" {
		return kittui.InputNavigation
	}
	return m.filesInput(m.hostFiles())
}

// filesInput is the input focus of the place.
func (m *Model) filesInput(v *fileView) kittui.InputFocus {
	switch {
	case v.preview != nil:
		if t := v.previewText(); t != nil {
			return t.Input()
		}
		return kittui.InputNavigation
	case v.browser != nil:
		return v.browser.Input()
	}
	return kittui.InputNavigation
}

// foldersTyped gives a key to the browser or the open preview.
func (m *Model) foldersTyped(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	if m.sys.page != pageFolders || m.sys.fold.asked != "" {
		return nil, false
	}
	return m.filesTyped(m.hostFiles(), msg)
}

// filesTyped gives a key to the browser of the place or its open preview.
func (m *Model) filesTyped(v *fileView, msg tea.KeyPressMsg) (tea.Cmd, bool) {
	if v.preview != nil {
		t := v.previewText()
		handled := t != nil && t.Update(msg)
		revealAsked(t)
		return nil, handled
	}
	b := m.fileBrowser(v)
	if b == nil {
		return nil, false
	}
	r, handled := b.Update(msg)
	return m.fileResult(v, r), handled
}

// foldersBack is esc on the folders page: it closes the permissions or the preview, clears a filter or a search,
// goes up one folder, and at the root leads to the sub-tab line.
func (m *Model) foldersBack(msg tea.KeyPressMsg) tea.Cmd {
	p := &m.sys.fold
	if p.asked != "" {
		p.gen++
		p.asked, p.folder, p.err, p.reading = "", nil, "", false
		return m.reloadFolders()
	}
	if cmd, handled := m.filesBack(m.hostFiles(), msg); handled {
		return cmd
	}
	m.setZone(m.above())
	return nil
}

// filesBack is esc in the place: it closes the preview, clears a filter or a search and goes up one folder. At the
// root it is not handled, so the place is left rather than its root.
func (m *Model) filesBack(v *fileView, msg tea.KeyPressMsg) (tea.Cmd, bool) {
	if v.preview != nil {
		if t := v.previewText(); t != nil && t.Update(msg) {
			return nil, true
		}
		v.preview = nil
		v.fileGen++
		return nil, true
	}
	if b := m.fileBrowser(v); b != nil {
		if r, handled := b.Update(msg); handled {
			return m.fileResult(v, r), true
		}
	}
	return nil, false
}

// foldersFooter are the own keys of the folders page.
func (m *Model) foldersFooter() []kittui.Hint {
	if m.sys.fold.asked != "" {
		back := kittui.Hint{Action: actBack, Label: kittui.TextKeyBack, Priority: 1}
		if _, ok := m.changeableFolder(); ok {
			return []kittui.Hint{{Action: actEdit, Label: textKeyChange, Priority: 3}, back}
		}
		return []kittui.Hint{back}
	}
	return m.filesFooter(m.hostFiles())
}

// filesFooter are the own keys of the place.
func (m *Model) filesFooter(v *fileView) []kittui.Hint {
	back := kittui.Hint{Action: actBack, Label: kittui.TextKeyBack, Priority: 1}
	switch {
	case v.preview != nil:
		if t := v.previewText(); t != nil {
			return append(t.Hints(), kittui.Hint{Action: actEdit, Label: textKeyEditFile, Priority: 3}, back)
		}
		return []kittui.Hint{back}
	case v.browser == nil:
		return nil
	}
	// The reload key stands among the common keys already.
	return slices.DeleteFunc(v.browser.Hints(), func(h kittui.Hint) bool { return h.Action == actReload })
}

// filesView draws the place: the preview, or the browser.
func (m *Model) filesView(v *fileView, width, height int) string {
	if v.preview != nil {
		return m.previewView(v, width, height)
	}
	return m.browserView(v, width, height)
}

// browserView draws the browser of the place and registers it for the mouse.
func (m *Model) browserView(v *fileView, width, height int) string {
	b := m.fileBrowser(v)
	if b == nil {
		return ""
	}
	b.SetFocused(m.focus() == kittui.ZoneRows)
	view, targets := b.View(m.styles, width, height)
	m.parts = append(m.parts, mousePart{rect: kittui.Rect{Width: width, Height: height}, targets: targets, scroll: b.Wheel,
		click: func(res kittui.MouseResult) tea.Cmd {
			r, _ := b.Click(res)
			m.setZone(kittui.ZoneRows)
			return m.fileResult(v, r)
		}})
	return view
}
