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

// fileReadMsg carries a direct read of path. load is the request of the browser a folder answers; a file read
// for the preview has none and carries the gen of its preview.
type fileReadMsg struct {
	gen  int
	path string
	load *kittui.FileLoad
	read hostchange.PathContent
	err  error
}

func (msg fileReadMsg) applyTo(m *Model) { m.applyFileRead(msg) }

// folderBrowser is the browser of the page, made on first use.
func (m *Model) folderBrowser() *kittui.FileBrowser {
	p := &m.sys.fold
	if p.browser == nil {
		b, err := kittui.NewFileBrowser(kittui.FileBrowserSpec{Keys: fileBrowserKeys(),
			Actions: []kittui.FormButton{{Action: actEdit, Label: textKeyEditFile}, {Action: actAdd, Label: kittui.TextKeyNew},
				{Action: actRename, Label: kittui.TextKeyRename}, {Action: actCopy, Label: kittui.TextKeyCopy},
				{Action: actRemove, Label: kittui.TextKeyDelete}, {Action: actRights, Label: textKeyRights}}},
			m.keys, m.text)
		if err != nil {
			m.failure = m.text.Lookup(kittui.TextError, err.Error())
			return nil
		}
		p.browser = b
	}
	return p.browser
}

// showFolders reads the root when the page is shown for the first time.
func (m *Model) showFolders() tea.Cmd {
	p := &m.sys.fold
	b := m.folderBrowser()
	if b == nil || p.started {
		return nil
	}
	p.started = true
	return m.fileResult(b.Load(nil))
}

// reloadFolders reads what the page shows again, as on R; the permissions are read again by loadSystem.
func (m *Model) reloadFolders() tea.Cmd {
	p := &m.sys.fold
	switch {
	case p.asked != "":
		return nil
	case p.preview != nil:
		return m.openFilePreview(p.preview.path)
	}
	if b := m.folderBrowser(); b != nil {
		r, _ := b.Act(actReload)
		return m.fileResult(r)
	}
	return nil
}

// hostPath is the absolute path of the parts below the root.
func hostPath(parts ...string) string { return "/" + strings.Join(parts, "/") }

// gitPath reports whether path lies in a .git folder or is one, which the page only reads.
func gitPath(path string) bool { return slices.Contains(strings.Split(path, "/"), ".git") }

// fileResult does what a key or a click on the browser asked for.
func (m *Model) fileResult(r kittui.FileResult) tea.Cmd {
	switch {
	case r.Load != nil:
		load := *r.Load
		return m.readHostPath(hostPath(load.Dir...), &load, 0)
	case r.Action == nil:
		return nil
	}
	a := *r.Action
	if cmd, ok := m.fileChange(a); ok {
		return cmd
	}
	path := hostPath(a.Dir...)
	if a.HasEntry {
		path = hostPath(append(a.Dir, a.Entry.Name)...)
	}
	switch {
	case a.Action == actRights:
		p := &m.sys.fold
		p.asked, p.folder, p.err = path, nil, ""
		return m.readFolder(path)
	case a.Action == actEnter && a.HasEntry:
		return m.openFilePreview(path)
	case a.Action == actEdit && a.HasEntry:
		return m.editFile(path, a.Entry.Kind)
	}
	return nil
}

// readHostPath reads path directly, off the event loop.
func (m *Model) readHostPath(path string, load *kittui.FileLoad, gen int) tea.Cmd {
	read := m.sys.fold.readPath
	if read == nil {
		read = hostchange.ReadPath
	}
	return func() tea.Msg {
		c, err := read(path)
		return fileReadMsg{gen: gen, path: path, load: load, read: c, err: err}
	}
}

// applyFileRead shows a direct read. What the rights of the administrator do not reach is asked for through sudo.
func (m *Model) applyFileRead(msg fileReadMsg) {
	if msg.load == nil && !m.previewCurrent(msg.gen) {
		return
	}
	switch {
	case msg.err == nil:
		m.sys.fold.chg.listed(msg.load, false)
		m.showHostRead(msg.load, msg.gen, m.directRead(msg.read))
	case errors.Is(msg.err, fs.ErrPermission) && m.euid() != 0:
		m.askReadRoot(msg.path, msg.load, msg.gen)
	default:
		m.failHostRead(msg.load, msg.gen, msg.err.Error())
	}
}

// askReadRoot asks to read path as root; y hands the terminal to sudo qhost host file.
func (m *Model) askReadRoot(path string, load *kittui.FileLoad, gen int) {
	t := m.text
	m.askSudo(sudoAsk{title: textTitleReadRoot, question: t.Lookup(textAskReadRoot, path), consequence: t.Lookup(textWhatReadRoot),
		args: []string{"host", "file", "--output", "json", "--", path},
		done: func(r sudoRun) tea.Cmd {
			if read, ok := decodeHostFile(r.out); ok {
				m.sys.fold.chg.listed(load, true)
				m.showHostRead(load, gen, read)
			} else {
				m.failHostRead(load, gen, r.problem)
			}
			return nil
		},
		drop: func() { m.failHostRead(load, gen, fs.ErrPermission.Error()) }})
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
func (m *Model) showHostRead(load *kittui.FileLoad, gen int, r hostRead) {
	if load == nil {
		m.showPreview(gen, r)
		return
	}
	b := m.folderBrowser()
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
		m.sys.fold.chg.setNames(r.Entries)
	}
}

// failHostRead shows why a read gave nothing.
func (m *Model) failHostRead(load *kittui.FileLoad, gen int, reason string) {
	if load == nil {
		if m.previewCurrent(gen) {
			p := m.sys.fold.preview
			p.reading, p.note = false, m.text.Lookup(textFolderFailed, p.path, reason)
		}
		return
	}
	if b := m.folderBrowser(); b != nil {
		b.SetError(*load, errors.New(reason))
		m.sys.fold.chg.names = nil
	}
}

// foldersInput is the input focus of the folders page.
func (m *Model) foldersInput() kittui.InputFocus {
	p := &m.sys.fold
	switch {
	case m.sys.page != pageFolders || p.asked != "":
		return kittui.InputNavigation
	case p.preview != nil:
		if v := p.previewText(); v != nil {
			return v.Input()
		}
		return kittui.InputNavigation
	case p.browser != nil:
		return p.browser.Input()
	}
	return kittui.InputNavigation
}

// foldersTyped gives a key to the browser or the open preview.
func (m *Model) foldersTyped(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	p := &m.sys.fold
	switch {
	case m.sys.page != pageFolders || p.asked != "":
		return nil, false
	case p.preview != nil:
		v := p.previewText()
		return nil, v != nil && v.Update(msg)
	}
	b := m.folderBrowser()
	if b == nil {
		return nil, false
	}
	r, handled := b.Update(msg)
	return m.fileResult(r), handled
}

// foldersBack is esc on the folders page: it closes the permissions or the preview, clears a filter or a search,
// goes up one folder, and at the root leads to the sub-tab line.
func (m *Model) foldersBack(msg tea.KeyPressMsg) tea.Cmd {
	p := &m.sys.fold
	switch {
	case p.asked != "":
		p.gen++
		p.asked, p.folder, p.err, p.reading = "", nil, "", false
		return m.reloadFolders()
	case p.preview != nil:
		if v := p.previewText(); v != nil && v.Update(msg) {
			return nil
		}
		p.preview = nil
		p.fileGen++
		return nil
	}
	if b := m.folderBrowser(); b != nil {
		if r, handled := b.Update(msg); handled {
			return m.fileResult(r)
		}
	}
	m.setZone(m.above())
	return nil
}

// foldersFooter are the own keys of the folders page.
func (m *Model) foldersFooter() []kittui.Hint {
	p := &m.sys.fold
	back := kittui.Hint{Action: actBack, Label: kittui.TextKeyBack, Priority: 1}
	switch {
	case p.asked != "":
		if _, ok := m.changeableFolder(); ok {
			return []kittui.Hint{{Action: actEdit, Label: textKeyChange, Priority: 3}, back}
		}
		return []kittui.Hint{back}
	case p.preview != nil:
		if v := p.previewText(); v != nil {
			return append(v.Hints(), kittui.Hint{Action: actEdit, Label: textKeyEditFile, Priority: 3}, back)
		}
		return []kittui.Hint{back}
	case p.browser == nil:
		return nil
	}
	// The reload key stands among the common keys already.
	return slices.DeleteFunc(p.browser.Hints(), func(h kittui.Hint) bool { return h.Action == actReload })
}

// browserView draws the browser and registers it for the mouse.
func (m *Model) browserView(width, height int) string {
	b := m.folderBrowser()
	if b == nil {
		return ""
	}
	b.SetFocused(m.focus() == kittui.ZoneRows)
	view, targets := b.View(m.styles, width, height)
	m.parts = append(m.parts, mousePart{rect: kittui.Rect{Width: width, Height: height}, targets: targets, scroll: b.Wheel,
		click: func(res kittui.MouseResult) tea.Cmd {
			r, _ := b.Click(res)
			m.setZone(kittui.ZoneRows)
			return m.fileResult(r)
		}})
	return view
}
