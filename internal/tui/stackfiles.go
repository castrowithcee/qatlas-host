package tui

import (
	"path"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// The file browser of System over one stack folder: the files key on the row of the folder in Stacks, and in the
// stack detail, opens it in the Stacks area with the folder as its root, which esc at the root leaves again. The
// detail opens the folder the stack comes from at the stack's own folder, so the settings in .qatlas/qhost and the
// uncommitted changes of the whole folder are those the row shows. Reads, edits and changes take the ways of
// System: directly where the rights of the administrator reach, else through sudo, and every write is a root
// action with its journal entry. Saving commits nothing.

// stackFiles is the file browser over the stack folder folder while open is set.
type stackFiles struct {
	view   fileView
	open   bool
	folder control.SourceStatus
	// mounts are the bind mounts below the folder of the running containers of its stacks, as read when it opened;
	// gen separates that read from an older one.
	mounts []dataMount
	gen    int
	// changes is the view of the uncommitted changes of a stack folder, which covers the list or the browser.
	changes stackChanges
	// history is the view of the history of a stack folder, which covers the list or the browser as well.
	history stackHistory
}

// dataMount is a folder a running container has mounted.
type dataMount struct{ path, container string }

// shownStackFiles is the browser over a stack folder while the Stacks area shows it, nil otherwise.
func (m *Model) shownStackFiles() *fileView {
	if m.area != areaStacks || !m.sf.open || m.set.keyOpen || m.sf.changes.open || m.sf.history.open {
		return nil
	}
	return &m.sf.view
}

// stackRowsShown reports whether the Stacks area shows its list, where the keys of the stacks act.
func (m *Model) stackRowsShown() bool {
	return m.area == areaStacks && m.shownStackFiles() == nil && m.shownChanges() == nil && m.shownHistory() == nil
}

// openFolderFiles opens the browser over the stack folder of the marked row.
func (m *Model) openFolderFiles() tea.Cmd {
	f, ok := m.selectedFolder()
	if !ok {
		return nil
	}
	return m.openStackFiles(f, nil)
}

// openDetailFiles opens the browser over the stack folder of the stack of the detail, in the folder of the stack.
func (m *Model) openDetailFiles() tea.Cmd {
	stack, ok := m.detailStack()
	if !ok {
		return nil
	}
	info, ok := m.stackInfo(stack)
	if !ok || info.SourceDir == "" {
		m.warn(m.text.Lookup(textStackFilesNoFolder, stack))
		return nil
	}
	f := control.SourceStatus{Path: info.SourceDir, Engine: info.Engine}
	for _, src := range m.snap.status.Engine.Sources {
		if src.Path == info.SourceDir && inEngine(src.Engine, info.Engine) {
			f = src
		}
	}
	var dir []string
	if rel := path.Clean(info.Folder); rel != "." && !path.IsAbs(rel) && !strings.HasPrefix(rel, "..") {
		dir = strings.Split(rel, "/")
	}
	return m.openStackFiles(f, dir)
}

// openStackFiles shows the browser over the folder f, at the folder dir below it. The accounts are read for the
// owners and the editor, the running containers for the warning about their data folders.
func (m *Model) openStackFiles(f control.SourceStatus, dir []string) tea.Cmd {
	s := &m.sf
	if m.activeEdit() != nil {
		m.warn(m.text.Lookup(textActionBusy))
		return nil
	}
	if s.view.root != f.Path {
		old := s.view
		s.view = fileView{root: f.Path, readPath: old.readPath, hooks: old.hooks, rules: m.stackRules(f.Path)}
		s.view.chg.measure = old.chg.measure
	}
	s.folder, s.open, s.mounts = f, true, nil
	m.enter(areaStacks)
	v := &s.view
	b := m.fileBrowser(v)
	if b == nil {
		return nil
	}
	v.started, v.preview = true, nil
	v.fileGen++
	return tea.Batch(m.loadSystem(false), m.fileResult(v, b.Load(dir)), m.readMounts())
}

// stackRules are the rules of the browser over the stack folder root.
func (m *Model) stackRules(root string) fileRules {
	return fileRules{rootLabel: root, readOnly: stackReadOnly, spans: stackFileSpans, beforeWrite: m.warnDataWrite,
		afterWrite: m.afterStackWrite}
}

// stackReadOnly is the rule of a stack folder: .git belongs to Git, and the settings in .qatlas/qhost have their
// forms in the stack detail.
func stackReadOnly(p string) kittui.TextID {
	parts := strings.Split(p, "/")
	for i, part := range parts {
		if part == ".git" || part == ".qatlas" && i+1 < len(parts) && parts[i+1] == "qhost" {
			return textStackFileLocked
		}
	}
	return ""
}

// closeStackFiles leaves the browser for the list; the marked row is still the folder.
func (m *Model) closeStackFiles() {
	m.sf.open = false
	m.sf.view.preview = nil
	m.sf.view.fileGen++
}

// stacksInput is the input focus of the Stacks list or of the browser.
func (m *Model) stacksInput() kittui.InputFocus {
	if h := m.shownHistory(); h != nil {
		return m.historyInput(h)
	}
	if c := m.shownChanges(); c != nil {
		return m.changesInput(c)
	}
	if v := m.shownStackFiles(); v != nil {
		return m.filesInput(v)
	}
	return m.list.Input()
}

// stacksTyped gives a key to the browser, or to the Stacks list, where enter opens the detail of a stack.
func (m *Model) stacksTyped(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	if h := m.shownHistory(); h != nil {
		return m.historyTyped(h, msg)
	}
	if c := m.shownChanges(); c != nil {
		return m.changesTyped(c, msg)
	}
	if v := m.shownStackFiles(); v != nil {
		return m.filesTyped(v, msg)
	}
	open, handled := m.stackUpdate(msg)
	return m.open(open), handled
}

// stacksAction acts on a letter in the Stacks area: in the changes, in the browser on its entries, else on the marked
// row.
func (m *Model) stacksAction(id kittui.ActionID) (tea.Cmd, bool) {
	if h := m.shownHistory(); h != nil {
		return m.historyAction(h, id)
	}
	if c := m.shownChanges(); c != nil {
		return m.changesAction(c, id)
	}
	v := m.shownStackFiles()
	switch {
	case id == actChanges:
		return m.openFolderChanges(v)
	case id == actHistory:
		return m.openFolderHistory(v)
	case v == nil && id == actFiles:
		if _, ok := m.selectedFolder(); !ok {
			return nil, false
		}
		return m.openFolderFiles(), true
	case v == nil:
		return m.stackFolderAction(id)
	}
	act := m.fileKey(v, id)
	switch {
	case act == nil:
		return nil, false
	case m.sys.busy:
		m.warn(m.text.Lookup(textActionBusy))
		return nil, true
	}
	return act(), true
}

// stacksBack is esc in the Stacks area: it leaves the changes; in the browser it closes the preview, clears a filter
// and goes up, and at the root it leaves the browser; in the list it clears a search, then leads to the tab line.
func (m *Model) stacksBack(msg tea.KeyPressMsg) tea.Cmd {
	if h := m.shownHistory(); h != nil {
		m.historyBack(h, msg)
		return nil
	}
	if c := m.shownChanges(); c != nil {
		m.changesBack(c, msg)
		return nil
	}
	v := m.shownStackFiles()
	if v == nil {
		m.back(m.list, msg)
		return nil
	}
	if cmd, handled := m.filesBack(v, msg); handled {
		return cmd
	}
	m.closeStackFiles()
	return nil
}

// stacksPosition is the position and the back key of the header in the Stacks area.
func (m *Model) stacksPosition() (string, *kittui.Hint) {
	if m.shownStackFiles() != nil || m.shownChanges() != nil || m.shownHistory() != nil {
		return "", &kittui.Hint{Action: actBack, Label: kittui.TextKeyBack}
	}
	return m.list.Position(), nil
}

// reloadStackFiles reads what the browser shows again, as on R, with the mounts of the running containers; over it the
// changes are read again.
func (m *Model) reloadStackFiles() tea.Cmd {
	if m.shownHistory() != nil {
		return m.readHistory()
	}
	if m.shownChanges() != nil {
		return m.readChanges()
	}
	v := m.shownStackFiles()
	if v == nil {
		return nil
	}
	return tea.Batch(m.reloadFiles(v), m.readMounts())
}

// stackFilesView draws the browser, below a line about uncommitted changes of the folder where it has any, which a
// click opens.
func (m *Model) stackFilesView(v *fileView, width, height int) string {
	if !m.folderChanged(m.sf.folder) || height < 4 {
		return m.filesView(v, width, height)
	}
	note := m.line(detailMargin, kittui.StyleWarn, m.text.Lookup(textDetailUncommitted), width)
	// The parts of the browser lie one row lower.
	from := len(m.parts)
	view := m.filesView(v, width, height-1)
	for i := from; i < len(m.parts); i++ {
		m.parts[i].rect.Y++
	}
	m.changesTarget(0, width, 1)
	return note + "\n" + view
}

// stacksOfFolder are the stacks the service reads from the stack folder f.
func (m *Model) stacksOfFolder(f control.SourceStatus) []control.StackInfo {
	var out []control.StackInfo
	for _, s := range m.snap.stacks.Stacks {
		if s.SourceDir == f.Path && inEngine(s.Engine, f.Engine) {
			out = append(out, s)
		}
	}
	return out
}

// atOrBelow reports whether p is dir or lies below it.
func atOrBelow(dir, p string) bool {
	return p == dir || strings.HasPrefix(p, strings.TrimSuffix(dir, "/")+"/")
}

// mountsMsg carries the bind mounts of the running containers of the folder the browser opened for.
type mountsMsg struct {
	gen    int
	mounts []dataMount
}

func (msg mountsMsg) applyTo(m *Model) {
	if msg.gen == m.sf.gen {
		m.sf.mounts = msg.mounts
	}
}

// readMounts reads the bind mounts below the folder of the running containers of its stacks. A part that cannot
// be read gives no mount, so the warning is missing, never the write.
func (m *Model) readMounts() tea.Cmd {
	s := &m.sf
	s.gen++
	gen, root, ctx, b := s.gen, s.folder.Path, m.engineCtx(s.folder.Engine), m.backend
	var stacks []string
	for _, st := range m.stacksOfFolder(s.folder) {
		stacks = append(stacks, st.Name)
	}
	return func() tea.Msg {
		list, err := b.Containers(ctx)
		if err != nil {
			return mountsMsg{gen: gen}
		}
		var out []dataMount
		for _, g := range list.Groups {
			if g.Kind != control.ContainerKindOwn || !slices.Contains(stacks, g.Name) {
				continue
			}
			for _, c := range g.Containers {
				if c.EngineState != "running" {
					continue
				}
				d, err := b.Container(ctx, c.Name)
				if err != nil {
					continue
				}
				for _, mt := range d.Mounts {
					if mt.Type == "bind" && atOrBelow(root, mt.Source) {
						out = append(out, dataMount{path: mt.Source, container: c.Name})
					}
				}
			}
		}
		return mountsMsg{gen: gen, mounts: out}
	}
}

// warnDataWrite adds to the question of a change of paths the warning that a running container uses one of them as
// its data folder; the change is not refused.
func (m *Model) warnDataWrite(a *hostAsk, paths ...string) {
	for _, p := range paths {
		for _, d := range m.sf.mounts {
			if atOrBelow(d.path, p) || atOrBelow(p, d.path) {
				a.note = strings.TrimSpace(m.text.Lookup(textStackFileDataRunning, d.path, d.container) + " " + a.note)
				return
			}
		}
	}
}

// afterStackWrite reads the service again for the uncommitted changes the row shows, and after a write of
// secrets.env offers to deploy its stack, where the new values take effect.
func (m *Model) afterStackWrite(paths ...string) tea.Cmd {
	for _, p := range paths {
		if path.Base(p) == secretsFile {
			m.offerSecretsDeploy(p)
			break
		}
	}
	return m.reloadNow()
}
