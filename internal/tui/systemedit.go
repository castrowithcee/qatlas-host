package tui

import (
	"errors"
	"io/fs"
	"os"
	"path"

	tea "charm.land/bubbletea/v2"

	"github.com/castrowithcee/qatlas-kit/exteditor"
	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/host/hostchange"
)

// e in a file browser edits a file in the editor of the person, never as root: started with sudo, the editor runs
// as the user behind sudo. The text is read directly or through sudo qhost host file, edited in a private copy the
// kit makes and removes, checked by its extension and written back as the root action file.write, the text on
// standard input only. A file only root may read is edited only where the copy lies in memory.

// fileEdit is the edit under way in the place view; there is at most one, as the editor takes the terminal.
type fileEdit struct {
	view   *fileView
	path   string
	editor exteditor.Editor
	target *exteditor.Target
	dir    privateDir
	// viaSudo says the text was read through sudo, which a preview of the file would need again.
	viaSudo bool
	text    []byte
}

// The messages of the editor, which Update hands to applyEdit.
type (
	editorUnchangedMsg = exteditor.UnchangedMsg
	editorChangedMsg   = exteditor.ChangedMsg
	editorFailedMsg    = exteditor.FailedMsg
)

// editReadMsg carries the direct read of the file an edit is for.
type editReadMsg struct {
	edit *fileEdit
	read hostchange.PathContent
	err  error
}

// folderEditKey is e on the folders page: it changes the permissions shown, or acts as e of the file browser.
func (m *Model) folderEditKey() tea.Cmd {
	if m.sys.fold.asked != "" {
		return m.openFolderChange()
	}
	return m.fileEditKey(m.hostFiles())
}

// fileEditKey is e in the place: it edits the file shown in the preview, or acts as the edit button of the browser
// on the marked entry.
func (m *Model) fileEditKey(v *fileView) tea.Cmd {
	if v.preview != nil {
		if v.previewText() != nil {
			return m.editFile(v, v.preview.path, kittui.FileRegular)
		}
		return nil
	}
	if b := m.fileBrowser(v); b != nil {
		r, _ := b.Act(actEdit)
		return m.fileResult(v, r)
	}
	return nil
}

// editFile starts the edit of the file at path in the place, kind as the browser shows it.
func (m *Model) editFile(v *fileView, path string, kind kittui.FileKind) tea.Cmd {
	t, readOnly := m.text, v.readOnlyText(path)
	var refusal string
	switch {
	case m.sys.busy || m.activeEdit() != nil:
		refusal = t.Lookup(textActionBusy)
	case readOnly != "":
		refusal = t.Lookup(readOnly, path)
	case kind == kittui.FileDir:
		refusal = t.Lookup(textEditFolder, path)
	case kind == kittui.FileLink:
		refusal = t.Lookup(textEditLink, path)
	}
	if refusal != "" {
		m.warn(refusal)
		return nil
	}
	e, refusal := m.prepareEdit(v, path)
	if refusal != "" {
		m.warn(refusal)
		return nil
	}
	v.edit = e
	read := v.readPath
	if read == nil {
		read = hostchange.ReadPath
	}
	return func() tea.Msg {
		c, err := read(path)
		return editReadMsg{edit: e, read: c, err: err}
	}
}

// prepareEdit finds the editor, the user it runs as and the private folder of the copy.
func (m *Model) prepareEdit(v *fileView, path string) (*fileEdit, string) {
	t, h := m.text, v.hooks.withDefaults()
	editor, err := exteditor.Resolve(m.prefs.Editor, h.getenv)
	if err != nil {
		return nil, t.Lookup(textEditNoEditor, err.Error())
	}
	target, refusal := m.editTarget()
	if refusal != "" {
		return nil, refusal
	}
	uid := os.Getuid()
	if target != nil {
		uid = int(target.UID)
	}
	dir, err := h.pick(uid, target)
	if err != nil {
		return nil, t.Lookup(textEditNoDir, err.Error())
	}
	return &fileEdit{view: v, path: path, editor: editor, target: target, dir: dir}, ""
}

// applyEdit takes the read of the file and the end of the editor.
func (m *Model) applyEdit(msg tea.Msg) tea.Cmd {
	e := m.activeEdit()
	if e == nil {
		return nil
	}
	t := m.text
	switch msg := msg.(type) {
	case editReadMsg:
		if msg.edit == e {
			return m.applyEditRead(e, msg)
		}
	case exteditor.UnchangedMsg:
		m.endEdit()
	case exteditor.FailedMsg:
		m.endEdit()
		m.failure = t.Lookup(kittui.TextError, t.Lookup(textEditFailed, e.path, msg.Err.Error()))
	case exteditor.ChangedMsg:
		zeroAll(e.text)
		e.text = msg.Text
		if problem := checkSyntax(path.Base(e.path), e.text); problem != nil {
			m.askEditAgain(e, *problem)
			return nil
		}
		m.askFileWrite(e)
	}
	return nil
}

// applyEditRead opens the editor on a direct read; what the rights of the administrator do not reach is read
// through sudo, but only into a copy in memory.
func (m *Model) applyEditRead(e *fileEdit, msg editReadMsg) tea.Cmd {
	t := m.text
	switch {
	case msg.err == nil && m.euid() == 0 && !e.dir.ram && !readableWithoutSudo(msg.read.Entry.Mode, msg.read.Entry.UID, int(e.target.UID)):
		m.endEdit()
		m.warn(t.Lookup(textEditNoRAM, e.path))
	case msg.err == nil:
		return m.openFileEditor(e, m.directRead(msg.read))
	case errors.Is(msg.err, fs.ErrPermission) && m.euid() != 0 && !e.dir.ram:
		m.endEdit()
		m.warn(t.Lookup(textEditNoRAM, e.path))
	case errors.Is(msg.err, fs.ErrPermission) && m.euid() != 0:
		m.askSudo(sudoAsk{title: textTitleReadRoot, question: t.Lookup(textAskReadRoot, e.path), consequence: t.Lookup(textWhatReadRoot),
			args: []string{"host", "file", "--output", "json", "--", e.path},
			done: func(r sudoRun) tea.Cmd {
				read, ok := decodeHostFile(r.out)
				if !ok || e.view.edit != e {
					m.endEdit()
					m.failure = t.Lookup(kittui.TextError, t.Lookup(textFolderFailed, e.path, r.problem))
					return nil
				}
				e.viaSudo = true
				return m.openFileEditor(e, read)
			},
			drop: m.endEdit})
	default:
		m.endEdit()
		m.failure = t.Lookup(kittui.TextError, t.Lookup(textFolderFailed, e.path, msg.err.Error()))
	}
	return nil
}

// openFileEditor hands the text read to the editor, after a question where the editor may leave traces.
func (m *Model) openFileEditor(e *fileEdit, r hostRead) tea.Cmd {
	t := m.text
	var refusal string
	switch {
	case r.Folder:
		refusal = t.Lookup(textEditFolder, e.path)
	case r.Kind == hostchange.KindLink:
		refusal = t.Lookup(textEditLink, e.path)
	case r.Kind != hostchange.KindFile:
		refusal = t.Lookup(textEditOther, e.path)
	case r.Binary:
		refusal = t.Lookup(textEditBinary, e.path)
	case r.TooLarge:
		refusal = t.Lookup(textEditTooLarge, e.path, hostchange.MaxFileContent>>20)
	}
	if refusal != "" {
		m.endEdit()
		m.warn(refusal)
		return nil
	}
	e.text = []byte(r.Content)
	if e.editor.NoTraces {
		return m.launchEditor(e)
	}
	m.askQuestion(kittui.DialogSpec{Title: textTitleEditTraces, Question: t.Lookup(textAskEditTraces, e.editor.Path, e.path),
		Consequence: t.Lookup(textWhatEditTraces), Answers: []kittui.Answer{{Action: actYes, Label: textAnswerOpenEditor},
			{Action: actNo, Label: textAnswerCancel}}}, func() tea.Cmd { return m.launchEditor(e) }, m.endEdit)
	return nil
}

// launchEditor hands the terminal to the editor on the text of e.
func (m *Model) launchEditor(e *fileEdit) tea.Cmd {
	cmd, err := e.view.hooks.withDefaults().start(exteditor.Request{Editor: e.editor, Dir: e.dir.path, Name: path.Base(e.path),
		Text: e.text, MaxSize: hostchange.MaxFileContent, Target: e.target})
	if err != nil {
		m.endEdit()
		m.failure = m.text.Lookup(kittui.TextError, m.text.Lookup(textEditFailed, e.path, err.Error()))
		return nil
	}
	return cmd
}

// askEditAgain names the failed check and offers to edit the text again or to discard it.
func (m *Model) askEditAgain(e *fileEdit, problem syntaxProblem) {
	t := m.text
	question := t.Lookup(textAskEditInvalid, e.path, problem.format)
	if problem.line > 0 {
		question = t.Lookup(textAskEditInvalidLine, e.path, problem.format, problem.line)
	}
	m.askQuestion(kittui.DialogSpec{Title: textTitleEditInvalid, Question: question, Consequence: t.Lookup(textWhatEditInvalid),
		Answers: []kittui.Answer{{Action: actYes, Label: textAnswerEditAgain}, {Action: actNo, Label: textAnswerDiscardEdit}}},
		func() tea.Cmd { return m.launchEditor(e) }, m.endEdit)
}

// askQuestion shows a question whose yes runs yes; every other answer, and a dialog that cannot be shown, runs drop.
func (m *Model) askQuestion(spec kittui.DialogSpec, yes func() tea.Cmd, drop func()) {
	spec.Default, spec.Keys = actNo, kittui.DialogKeys{Enter: actEnter}
	dialog, err := newDialog(spec, m.keys, m.text)
	if err != nil {
		drop()
		m.failure = m.text.Lookup(kittui.TextError, err.Error())
		return
	}
	m.modal = &modal{kind: modalAsk, dialog: dialog, yes: yes, drop: drop}
}

// askFileWrite asks to write the edited text back as the root action file.write; the confirmation owns the text
// from here and overwrites it when it is answered or the run is over. A system path asks in red, the only way to
// the acknowledgement in the arguments.
func (m *Model) askFileWrite(e *fileEdit) {
	t, p := m.text, e.view
	p.edit = nil
	e.dir.remove()
	a := hostAsk{id: hostchange.ActionFileWrite, args: hostchange.FileArgs(e.path), secret: e.text,
		question: t.Lookup(textAskFileWrite, e.path), then: func() tea.Cmd { return m.afterFileWrite(p, e.path, e.viaSudo) }}
	if hostchange.IsSystemPath(e.path) {
		a.risk = t.Lookup(textRiskFileWriteSystem)
	}
	p.beforeWrite(&a, e.path)
	m.askHost(a, nil)
}

// afterFileWrite shows the written file again in the preview of the place, then runs what the place runs after a
// write. A preview read through sudo is closed instead, so that no second password question follows the result.
func (m *Model) afterFileWrite(p *fileView, path string, viaSudo bool) tea.Cmd {
	var cmd tea.Cmd
	switch {
	case p.preview == nil || p.preview.path != path:
	case viaSudo:
		p.preview = nil
		p.fileGen++
	default:
		cmd = m.openFilePreview(p, path)
	}
	return tea.Batch(cmd, p.wrote(path))
}

// endEdit ends the edit under way: the text is overwritten and a folder made for the copy removed.
func (m *Model) endEdit() {
	if e := m.activeEdit(); e != nil {
		e.view.edit = nil
		zeroAll(e.text)
		e.dir.remove()
	}
}
