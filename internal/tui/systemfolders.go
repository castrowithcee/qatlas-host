package tui

import (
	"errors"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/castrowithcee/qatlas-kit/projectpath"
	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/host/hostadmin"
	"github.com/castrowithcee/qatlas-host/internal/host/hostchange"
)

// System > Folders shows owner, group and the three levels of one folder. enter asks for its path in a dialog
// that completes directories; e changes it in a form that offers owner and group from the accounts of the host
// and three levels per position, never free text or numbers. A change of everything below it counts the entries
// first and names the number in the confirmation; a system path asks in red, which is the only way to pass the
// acknowledgement the root action demands.

// The form of the folders page.
const formHostFolder formKind = formGroupRename + 1

// The fields of the folder form.
const (
	fieldFolderOwner  = "owner"
	fieldFolderGroup  = "group"
	fieldFolderUser   = "useracc"
	fieldFolderGroups = "groupacc"
	fieldFolderOther  = "otheracc"
	fieldFolderBelow  = "below"
)

// folderPage is what the folders page holds: the path asked for last and what was read for it. read, count and
// suggest reach the file system; tests replace them.
type folderPage struct {
	asked   string
	gen     int
	reading bool
	folder  *hostadmin.Folder
	err     string
	read    func(path string) (hostadmin.Folder, error)
	count   func(path string, recursive bool) (int, error)
	suggest func(start string) func(typed string) []string
}

// folderMsg carries a read of a folder.
type folderMsg struct {
	gen    int
	folder hostadmin.Folder
	err    error
}

// readFolder reads the folder path off the event loop; an empty path reads nothing.
func (m *Model) readFolder(path string) tea.Cmd {
	p := &m.sys.fold
	if path == "" {
		return nil
	}
	p.gen++
	p.reading = true
	gen, read := p.gen, p.read
	if read == nil {
		h := m.accountsReader()
		read = h.Folder
	}
	return func() tea.Msg {
		f, err := read(path)
		return folderMsg{gen: gen, folder: f, err: err}
	}
}

// rereadFolder reads the folder shown again, as after a change or on r.
func (m *Model) rereadFolder() tea.Cmd {
	if path := m.sys.fold.asked; filepath.IsAbs(path) {
		return m.readFolder(path)
	}
	return nil
}

// applyFolder takes a read; one that was overtaken by a newer path is dropped.
func (m *Model) applyFolder(msg folderMsg) {
	p := &m.sys.fold
	if msg.gen != p.gen {
		return
	}
	p.reading, p.folder, p.err = false, nil, ""
	t := m.text
	switch {
	case errors.Is(msg.err, hostadmin.ErrNotFound):
		p.err = t.Lookup(textFolderNotFound, p.asked)
	case errors.Is(msg.err, fs.ErrPermission):
		p.err = t.Lookup(textFolderDenied, p.asked)
	case msg.err != nil:
		p.err = t.Lookup(textFolderFailed, p.asked, msg.err.Error())
	default:
		p.folder = &msg.folder
	}
}

// openFolderPath asks for the path of a folder; the directories that complete it stand under the input.
func (m *Model) openFolderPath() tea.Cmd {
	p := &m.sys.fold
	start := "/"
	if p.folder != nil {
		start = filepath.Dir(realPath(*p.folder))
	}
	newSource := p.suggest
	if newSource == nil {
		newSource = func(start string) func(string) []string {
			return projectpath.NewSuggester(projectpath.Directories, start, "").Suggest
		}
	}
	source := newSource(start)
	suggest := func(typed string) []kittui.Suggestion {
		var out []kittui.Suggestion
		for _, s := range source(typed) {
			out = append(out, kittui.Suggestion{Text: s, Continues: strings.HasSuffix(s, string(filepath.Separator))})
		}
		return out
	}
	dialog, err := kittui.NewDialog(kittui.DialogSpec{Title: textTitleFolderPath, Question: m.text.Lookup(textAskFolderPath),
		Input:   &kittui.DialogInput{Value: p.asked, Suggest: suggest},
		Answers: []kittui.Answer{{Action: actEnter, Label: textAnswerShow}, {Action: actBack, Label: textAnswerCancel}},
		Default: actEnter,
		Keys:    kittui.DialogKeys{Enter: actEnter, Up: actUp, Down: actDown, Complete: actNext}}, m.keys, m.text)
	if err != nil {
		m.failure = m.text.Lookup(kittui.TextError, err.Error())
		return nil
	}
	m.modal = &modal{kind: modalAsk, dialog: dialog, input: m.takeFolderPath}
	return nil
}

// takeFolderPath reads the folder typed into the path dialog. A path that is not absolute is not read, as the
// working directory of the interface means nothing to the person.
func (m *Model) takeFolderPath(typed string) tea.Cmd {
	p := &m.sys.fold
	typed = strings.TrimSpace(typed)
	if typed == "" {
		return nil
	}
	path, err := projectpath.Expand(typed)
	if err != nil {
		path = typed
	}
	p.gen++
	p.asked, p.folder, p.err, p.reading = path, nil, "", false
	if !filepath.IsAbs(path) {
		p.err = m.text.Lookup(textFolderNotAbs, typed)
		return nil
	}
	p.asked = filepath.Clean(path)
	return m.readFolder(p.asked)
}

// realPath is the path a change is made on: the one without links, as the root action demands.
func realPath(f hostadmin.Folder) string {
	if f.Resolved != "" {
		return f.Resolved
	}
	return f.Path
}

// folderSystemPath reports whether a change of the folder touches a system path, written or behind a link.
func folderSystemPath(f hostadmin.Folder) bool {
	return f.Protected || hostchange.IsSystemPath(realPath(f))
}

// levelText is the word of a level.
func levelText(a hostadmin.Access) kittui.TextID {
	switch a {
	case hostadmin.AccessReadWrite:
		return textLevelReadWrite
	case hostadmin.AccessRead:
		return textLevelRead
	}
	return textLevelNothing
}

// folderLines are the facts of the folder in plain words.
func (m *Model) folderLines(f hostadmin.Folder, width int) []string {
	t := m.text
	line := func(style kittui.TextStyle, id kittui.TextID, args ...any) string {
		return m.line(detailIndent, style, t.Lookup(id, args...), width)
	}
	kind := textKindOther
	switch f.Kind {
	case "directory":
		kind = textKindFolder
	case "file":
		kind = textKindFile
	}
	var lines []string
	if f.Resolved != "" && f.Resolved != f.Path {
		lines = append(lines, line(kittui.StyleText, textFolderResolved, f.Resolved))
	}
	lines = append(lines,
		line(kittui.StyleText, textFolderKind, t.Lookup(kind)),
		line(kittui.StyleText, textFolderOwnerMay, f.Owner, t.Lookup(levelText(f.OwnerAcc))),
		line(kittui.StyleText, textFolderGroupMay, f.Group, t.Lookup(levelText(f.GroupAcc))),
		line(kittui.StyleText, textFolderOthersMay, t.Lookup(levelText(f.OthersAcc))),
		line(kittui.StyleHint, textFolderMode, f.Mode))
	switch {
	case f.ACL == nil:
		lines = append(lines, line(kittui.StyleHint, textFolderACLUnknown))
	case *f.ACL:
		lines = append(lines, line(kittui.StyleWarn, textFolderACL))
	}
	var special []string
	for _, b := range []struct {
		on   bool
		name string
	}{{f.Setuid, "setuid"}, {f.Setgid, "setgid"}, {f.Sticky, "sticky"}} {
		if b.on {
			special = append(special, b.name)
		}
	}
	if len(special) > 0 {
		lines = append(lines, line(kittui.StyleHint, textFolderSpecial, strings.Join(special, ", ")))
	}
	if folderSystemPath(f) {
		lines = append(lines, line(kittui.StyleWarn, textFolderSystem))
	}
	return lines
}

// foldersView draws the folders page in width by height cells.
func (m *Model) foldersView(width, height int) string {
	p, t := &m.sys.fold, m.text
	var lines []string
	switch {
	case p.asked == "":
		lines = m.wrapped(detailMargin, kittui.StyleHint, t.Lookup(textFoldersHint), width)
	case p.err != "":
		lines = append([]string{m.line(detailMargin, kittui.StyleActive, p.asked, width), ""},
			m.wrapped(detailMargin, kittui.StyleFail, p.err, width)...)
	case p.folder == nil:
		lines = []string{m.line(detailMargin, kittui.StyleHint, t.Lookup(textFolderReading, p.asked), width)}
	default:
		f := *p.folder
		lines = m.framed(f.Path, "", width, height, func(w int) []string { return m.folderLines(f, w) })
	}
	return strings.Join(lines[:min(len(lines), height)], "\n")
}

// foldersFooter are the own keys of the folders page.
func (m *Model) foldersFooter() []kittui.Hint {
	own := []kittui.Hint{{Action: actEnter, Label: textKeyChooseFolder, Priority: 3}}
	if _, ok := m.changeableFolder(); ok {
		own = append(own, kittui.Hint{Action: actEdit, Label: textKeyChange, Priority: 3})
	}
	return own
}

// changeableFolder is the folder e changes: one that was read and is a folder, not a file.
func (m *Model) changeableFolder() (hostadmin.Folder, bool) {
	f := m.sys.fold.folder
	if f == nil || f.Kind != "directory" {
		return hostadmin.Folder{}, false
	}
	return *f, true
}

// accountChoices are the names to choose from with current among them, so that saving never changes it by
// accident.
func accountChoices(names []string, current string) []kittui.Choice {
	names = slices.Clone(names)
	slices.Sort(names)
	if !slices.Contains(names, current) {
		names = append([]string{current}, names...)
	}
	return namedChoices(names)
}

func levelField(id string, label kittui.TextID, current hostadmin.Access) kittui.FormField {
	var choices []kittui.Choice
	for _, l := range []hostadmin.Access{hostadmin.AccessNothing, hostadmin.AccessRead, hostadmin.AccessReadWrite} {
		choices = append(choices, kittui.Choice{ID: kittui.ChoiceID(l), Label: levelText(l)})
	}
	return choiceField(id, label, textHintFolderLevel, choices, string(current))
}

// openFolderChange opens the form that changes the folder shown.
func (m *Model) openFolderChange() tea.Cmd {
	f, ok := m.changeableFolder()
	if !ok {
		if m.sys.fold.folder != nil {
			m.warn(m.text.Lookup(textFolderOnlyDir))
		} else {
			m.warn(m.text.Lookup(textFolderNone))
		}
		return nil
	}
	var users, groups []string
	for _, u := range m.sys.users {
		users = append(users, u.Name)
	}
	for _, g := range m.sys.groups {
		groups = append(groups, g.Name)
	}
	form := m.newSettingsForm(formHostFolder, textTitleChangeFolder, []kittui.FormField{
		choiceField(fieldFolderOwner, textFieldFolderOwner, textHintFolderOwner, accountChoices(users, f.Owner), f.Owner),
		choiceField(fieldFolderGroup, textFieldFolderGroup, textHintFolderGroup, accountChoices(groups, f.Group), f.Group),
		levelField(fieldFolderUser, textFieldOwnerMay, f.OwnerAcc),
		levelField(fieldFolderGroups, textFieldGroupMay, f.GroupAcc),
		levelField(fieldFolderOther, textFieldOthersMay, f.OthersAcc),
		choiceField(fieldFolderBelow, textFieldBelow, textHintFolderBelow, yesNoChoices(), choiceNo),
	})
	if form == nil {
		return nil
	}
	form.folder, form.titleArg = &f, realPath(f)
	lead := textLeadFolder
	if folderSystemPath(f) {
		lead = textLeadFolderSystem
	}
	form.form.SetLead(lead)
	return m.openForm(form)
}

// countFolder counts what a change of path touches.
func (m *Model) countFolder(path string, recursive bool) (int, error) {
	if c := m.sys.fold.count; c != nil {
		return c(path, recursive)
	}
	return hostchange.CountFolderEntries(path, recursive)
}

func (m *Model) submitFolderChange(f *settingsForm, v map[kittui.FieldID]kittui.FormValue) tea.Cmd {
	t, folder := m.text, *f.folder
	path := realPath(folder)
	level := func(id string) hostadmin.Access { return hostadmin.Access(v[kittui.FieldID(id)].Choice) }
	req := hostchange.FolderRequest{Path: path, Owner: string(v[fieldFolderOwner].Choice), Group: string(v[fieldFolderGroup].Choice),
		UserAccess: level(fieldFolderUser), GroupAccess: level(fieldFolderGroups), OtherAccess: level(fieldFolderOther),
		Recursive: v[fieldFolderBelow].Choice == choiceYes, SystemPath: folderSystemPath(folder)}
	question := textAskFolder
	if req.Recursive {
		question = textAskFolderBelow
	}
	a := hostAsk{id: hostchange.ActionFolderSetPermissions, args: hostchange.FolderArgs(req),
		question: t.Lookup(question, path, req.Owner, req.Group, t.Lookup(levelText(req.UserAccess)),
			t.Lookup(levelText(req.GroupAccess)), t.Lookup(levelText(req.OtherAccess)))}
	if req.Recursive {
		// The plan is asked first, so a path the root action refuses as a whole is not walked at all.
		if _, refusal := m.planHost(a); refusal != "" {
			f.problem(refusal, t.Lookup(textFormFix))
			return nil
		}
		n, err := m.countFolder(path, true)
		switch {
		case errors.Is(err, hostchange.ErrTooManyEntries):
			f.problem(t.Lookup(textFolderTooMany, path, hostchange.MaxFolderEntries), t.Lookup(textFolderSmaller))
			return nil
		case err != nil:
			f.problem(t.Lookup(textFolderCountFailed, err.Error()), t.Lookup(textFolderSmaller))
			return nil
		case n == 1:
			a.note = t.Lookup(textFolderCountOne)
		default:
			a.note = t.Lookup(textFolderCount, n)
		}
	}
	// The acknowledgement of a system path is in the arguments that y runs; this red question is the only way
	// to reach that run.
	if req.SystemPath {
		a.risk = t.Lookup(textRiskFolderSystem)
	}
	if m.askHost(a, f) {
		m.dropForm()
	}
	return nil
}
