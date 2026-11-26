package tui

import (
	"errors"
	"path"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"

	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/host/hostchange"
)

// n, r, c and d on the browser of System > Folders create, rename, copy and remove an entry of the shown folder as
// the root actions file.write (an empty file), file.create-folder, file.rename, file.copy and file.remove. The name
// comes from a form and is checked here, the root command checks it again. The entry comes unchanged from the
// browser, a link is never resolved. A folder is counted before a copy or a removal: beyond what an undo keeps the
// question is red and asks for the option permanent.

// The forms of the browser.
const (
	formFileNew    formKind = formGrant + 1
	formFileRename formKind = formFileNew + 1
	formFileCopy   formKind = formFileRename + 1
)

const (
	fieldFileName    = "filename"
	fieldFileKind    = "filekind"
	choiceFileFile   = "file"
	choiceFileFolder = "folder"
)

// fileChanges is what the browser knows of its folder for a change.
type fileChanges struct {
	// names are the entries of the folder shown; nil while none is read, or after a change the list does not show.
	names map[string]bool
	// viaSudo says the folder was read through sudo, which a second read would ask for again.
	viaSudo bool
	// target is what the open form is about, and form that form.
	target fileTarget
	form   *settingsForm
	// measure counts the entries and the size of the files of a tree; nil takes hostchange.MeasureTree.
	measure func(path string) (int, int64, error)
}

func (c *fileChanges) listed(load *kittui.FileLoad, viaSudo bool) {
	if load != nil {
		c.viaSudo = viaSudo
	}
}

func (c *fileChanges) setNames(entries []hostEntry) {
	c.names = make(map[string]bool, len(entries))
	for _, e := range entries {
		c.names[e.Name] = true
	}
}

// fileTarget is what a form of the browser is about: the shown folder and the marked entry, if any.
type fileTarget struct {
	dir   string
	entry kittui.FileEntry
}

func (t fileTarget) path() string { return path.Join(t.dir, t.entry.Name) }

// pageFolderAction runs the key id on the folders page; handled is false for a key that is none here.
func (m *Model) pageFolderAction(id kittui.ActionID) (tea.Cmd, bool) {
	act := m.folderKey(id)
	switch {
	case act == nil:
		return nil, false
	case m.sys.busy:
		m.warn(m.text.Lookup(textActionBusy))
		return nil, true
	}
	return act(), true
}

// folderKey is what the key id does on the folders page, nil if nothing. Permissions take only e, the other keys act
// on the file browser.
func (m *Model) folderKey(id kittui.ActionID) func() tea.Cmd {
	switch {
	case id == actEdit:
		return m.folderEditKey
	case m.sys.fold.asked != "":
		return nil
	}
	return m.fileKey(m.hostFiles(), id)
}

// fileKey is what the key id does in the place, nil if nothing. The preview takes only e, the other keys act on the
// browser.
func (m *Model) fileKey(v *fileView, id kittui.ActionID) func() tea.Cmd {
	switch {
	case id == actEdit:
		return func() tea.Cmd { return m.fileEditKey(v) }
	case v.preview != nil:
		return nil
	case id == actAdd, id == actRename, id == actCopy, id == actRemove:
		return func() tea.Cmd {
			if b := m.fileBrowser(v); b != nil {
				r, _ := b.Act(id)
				return m.fileResult(v, r)
			}
			return nil
		}
	}
	return nil
}

// fileChange starts the change a button or key of the browser of the place asks for; ok is false for any other
// action.
func (m *Model) fileChange(v *fileView, a kittui.FileAction) (cmd tea.Cmd, ok bool) {
	switch a.Action {
	case actAdd, actRename, actCopy, actRemove:
	default:
		return nil, false
	}
	t := fileTarget{dir: v.path(a.Dir...), entry: a.Entry}
	if a.Action != actAdd && !a.HasEntry {
		return nil, true
	}
	if refusal := m.fileChangeRefusal(v, t, a.Action); refusal != "" {
		m.warn(refusal)
		return nil, true
	}
	switch a.Action {
	case actAdd:
		m.openFileForm(v, formFileNew, textTitleFileNew, textLeadFileNew, t, "", t.dir)
	case actRename:
		m.openFileForm(v, formFileRename, textTitleFileRename, textLeadFileRename, t, t.entry.Name, t.path())
	case actCopy:
		m.openFileForm(v, formFileCopy, textTitleFileCopy, textLeadFileCopy, t, copyName(t.entry.Name), t.path())
	default:
		m.askFileRemove(v, t)
	}
	return nil, true
}

// fileChangeRefusal says why nothing is changed in the place: an edit under way, a path the place only reads, or a
// folder not read.
func (m *Model) fileChangeRefusal(v *fileView, t fileTarget, action kittui.ActionID) string {
	readOnly := v.readOnlyText(t.dir)
	if readOnly == "" && action != actAdd {
		readOnly = v.readOnlyText(t.path())
	}
	switch {
	case m.activeEdit() != nil:
		return m.text.Lookup(textActionBusy)
	case readOnly != "":
		return m.text.Lookup(readOnly, t.path())
	case action == actAdd && (v.chg.names == nil || m.fileBrowser(v).Loading()):
		// A new file is written in place, so only the names of the folder keep it from replacing one.
		return m.text.Lookup(textFileReadFirst, t.dir)
	}
	return ""
}

// copyName suggests the name of a copy, keeping the extension.
func copyName(name string) string {
	ext := path.Ext(name)
	if base := strings.TrimSuffix(name, ext); base != "" {
		return base + "-copy" + ext
	}
	return name + "-copy"
}

func (m *Model) openFileForm(v *fileView, kind formKind, title, lead kittui.TextID, t fileTarget, name, titleArg string) {
	var fields []kittui.FormField
	switch kind {
	case formFileNew:
		fields = []kittui.FormField{
			choiceField(fieldFileKind, textFieldFileKind, textHintFileKind,
				[]kittui.Choice{choice(choiceFileFile, textKindFile), choice(choiceFileFolder, textKindFolder)}, choiceFileFile),
			textField(fieldFileName, textFieldFileName, textHintFileName, name)}
	case formFileCopy:
		fields = []kittui.FormField{textField(fieldFileName, textFieldCopyName, textHintFileName, name)}
	default:
		fields = []kittui.FormField{textField(fieldFileName, textFieldNewName, textHintFileName, name)}
	}
	f := m.newSettingsForm(kind, title, fields)
	if f == nil {
		return
	}
	v.chg.target, v.chg.form, f.titleArg = t, f, titleArg
	f.form.SetLead(lead)
	m.set.form = f
}

// fileNameProblem checks the name of an entry of dir in the place: one name, not empty, not . or .., free, and no
// path the place only reads.
func (m *Model) fileNameProblem(v *fileView, name, dir string) []kittui.FormProblem {
	readOnly := v.readOnlyText(path.Join(dir, name))
	switch {
	case name == "":
		return []kittui.FormProblem{m.bad(fieldFileName, textProblemFileEmpty)}
	case name == "." || name == "..":
		return []kittui.FormProblem{m.bad(fieldFileName, textProblemFileDots)}
	case strings.ContainsRune(name, '/') || strings.IndexFunc(name, unicode.IsControl) >= 0:
		return []kittui.FormProblem{m.bad(fieldFileName, textProblemFileChars)}
	case v.chg.names[name]:
		return []kittui.FormProblem{m.bad(fieldFileName, textProblemFileTaken, path.Join(dir, name))}
	case readOnly != "":
		return []kittui.FormProblem{m.bad(fieldFileName, readOnly, path.Join(dir, name))}
	}
	return nil
}

// submitFileForm checks the name and asks for the change.
func (m *Model) submitFileForm(f *settingsForm, v map[kittui.FieldID]kittui.FormValue) tea.Cmd {
	place := m.formFiles(f)
	if place == nil {
		return nil
	}
	t, name := place.chg.target, strings.TrimSpace(v[fieldFileName].Text)
	if ps := m.fileNameProblem(place, name, t.dir); len(ps) > 0 {
		f.problems(ps...)
		return nil
	}
	target := path.Join(t.dir, name)
	var a hostAsk
	switch f.kind {
	case formFileNew:
		a = m.fileNewAsk(target, v[fieldFileKind].Choice == choiceFileFolder)
	case formFileRename:
		a = hostAsk{id: hostchange.ActionFileRename, args: hostchange.TreeArgs(false, t.path(), target),
			question: m.text.Lookup(textAskFileRename, t.path(), target)}
	default:
		var problem string
		if a, problem = m.sizedAsk(place, t, hostchange.ActionFileCopy, target); problem != "" {
			f.problem(problem, m.text.Lookup(textFileNothingChange))
			return nil
		}
	}
	m.finishAsk(place, &a, t.path(), target)
	if m.askHost(a, f) {
		m.dropForm()
	}
	return nil
}

// finishAsk sets what runs after the change and makes the question red where a path is a system path; only this
// question reaches the acknowledgement the arguments carry. The place may add to the question.
func (m *Model) finishAsk(v *fileView, a *hostAsk, paths ...string) {
	a.then = func() tea.Cmd { return tea.Batch(m.afterFileChange(v), v.wrote(paths...)) }
	for _, p := range paths {
		if hostchange.IsSystemPath(p) {
			a.risk = m.text.Lookup(textRiskFileWriteSystem)
		}
	}
	v.beforeWrite(a, paths...)
}

func (m *Model) fileNewAsk(target string, folder bool) hostAsk {
	if folder {
		return hostAsk{id: hostchange.ActionFileCreateFolder, args: hostchange.FileArgs(target),
			question: m.text.Lookup(textAskFolderNew, target)}
	}
	return hostAsk{id: hostchange.ActionFileWrite, args: hostchange.FileArgs(target), emptyInput: true,
		question: m.text.Lookup(textAskFileNew, target)}
}

func (m *Model) askFileRemove(v *fileView, t fileTarget) {
	a, problem := m.sizedAsk(v, t, hostchange.ActionFileRemove, "")
	if problem != "" {
		m.warn(problem)
		return
	}
	a.danger = true
	m.finishAsk(v, &a, t.path())
	m.askHost(a, nil)
}

// treeSize is what a copy or removal of an entry touches, as far as it could be read.
type treeSize struct {
	entries int
	bytes   int64
	// unknown is why the folder was not counted.
	unknown string
}

// beyondUndo reports whether the journal keeps nothing of a change of this size.
func (s treeSize) beyondUndo() bool {
	return s.unknown == "" && (s.entries > hostchange.MaxFolderEntries || s.bytes > hostchange.MaxUndoBytes)
}

// measureEntry counts a folder directly, with the rights of the administrator; a link or a file needs no read. A folder
// only root may read is not counted through sudo, as that would ask for the password a second time before the
// confirmation; the question says so, and the root command still refuses a tree beyond the limit.
func (m *Model) measureEntry(v *fileView, t fileTarget) (treeSize, error) {
	if t.entry.Kind != kittui.FileDir {
		return treeSize{entries: 1, bytes: max(t.entry.Size, 0)}, nil
	}
	measure := v.chg.measure
	if measure == nil {
		measure = hostchange.MeasureTree
	}
	n, size, err := measure(t.path())
	if errors.Is(err, hostchange.ErrTooManyEntries) {
		return treeSize{}, err
	}
	if err != nil {
		return treeSize{unknown: err.Error()}, nil
	}
	return treeSize{entries: n, bytes: size}, nil
}

// sizedAsk is the question of a copy to target or, with an empty target, of the removal of the entry, with what it
// touches. problem is the reason when nothing can be asked.
func (m *Model) sizedAsk(v *fileView, t fileTarget, id, target string) (a hostAsk, problem string) {
	tx, src := m.text, t.path()
	size, err := m.measureEntry(v, t)
	if err != nil {
		return hostAsk{}, tx.Lookup(textFileTooMany, src, hostchange.MaxTreeEntries)
	}
	permanent := size.beyondUndo()
	a.id, a.final = id, tx.Lookup(textFileFinal, hostchange.MaxFolderEntries, hostchange.MaxUndoBytes>>20)
	if target == "" {
		a.args, a.question = hostchange.TreeArgs(permanent, src), tx.Lookup(removeQuestion(t.entry.Kind), src)
	} else {
		a.args, a.question = hostchange.TreeArgs(permanent, src, target), tx.Lookup(copyQuestion(t.entry.Kind), src, target)
	}
	switch {
	case size.unknown != "":
		a.note = tx.Lookup(textFileSizeUnknown, size.unknown, hostchange.MaxFolderEntries, hostchange.MaxUndoBytes>>20)
	case t.entry.Kind == kittui.FileDir || permanent:
		a.note = tx.Lookup(textFileCount, size.entries, humanSize(size.bytes))
	}
	return a, ""
}

func removeQuestion(kind kittui.FileKind) kittui.TextID {
	if kind == kittui.FileDir {
		return textAskFolderRemove
	}
	return textAskFileRemove
}

func copyQuestion(kind kittui.FileKind) kittui.TextID {
	if kind == kittui.FileDir {
		return textAskFolderCopy
	}
	return textAskFileCopy
}

// afterFileChange reads the folder shown in the place again. A folder read through sudo is not read again, as that
// would ask for the password a second time: the list says so and R reads it.
func (m *Model) afterFileChange(v *fileView) tea.Cmd {
	b := m.fileBrowser(v)
	switch {
	case b == nil || (v.rules.covered != nil && v.rules.covered()) || v.preview != nil:
		return nil
	case v.chg.viaSudo:
		v.chg.names = nil
		if r := b.Load(b.Dir()); r.Load != nil {
			b.SetError(*r.Load, errors.New(m.text.Lookup(textFileReadAgain)))
		}
		return nil
	}
	return m.reloadFiles(v)
}
