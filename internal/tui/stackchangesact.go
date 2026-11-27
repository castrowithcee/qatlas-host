package tui

import (
	"errors"
	"strings"

	tea "charm.land/bubbletea/v2"

	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// The actions of the view Changes. Each needs root: started as root the interface calls the service, otherwise it
// runs the public command through sudo after a question with the exact command. A diff is shown only in the text
// view, which escapes what a terminal would act on; it never reaches a message, a question or the command line.

// codeConfigInvalid is how the command reports a stack that does not load.
const codeConfigInvalid = "config-invalid"

// refusals are the words of the refusals of a commit, by the code of the service or of the command.
var refusals = map[string]kittui.TextID{
	control.CodeSecrets: textRefusedSecrets, control.CodePushRejected: textRefusedPush, control.CodeHeadMoved: textRefusedHead,
	control.CodeNotChanged: textRefusedList, control.CodeUnreadable: textRefusedUnreadable, codeConfigInvalid: textRefusedUnreadable,
	control.CodeRunActive: textRefusedRunActive,
}

// diffMsg carries the diff of the file the view asked for.
type diffMsg struct {
	gen int
	res control.SourceDiffResult
	err error
}

func (msg diffMsg) applyTo(m *Model) {
	d := m.sf.changes.diff
	if d == nil || msg.gen != m.sf.changes.diffGen {
		return
	}
	d.reading = false
	if msg.err != nil {
		d.note = m.serviceText(msg.err)
		return
	}
	v, err := kittui.NewTextView(kittui.TextViewSpec{Keys: textKeys()}, m.keys, m.text)
	if err != nil {
		d.note = m.text.Lookup(kittui.TextError, err.Error())
		return
	}
	v.SetText(msg.res.Diff, diffSpans(msg.res.Diff))
	d.text, d.truncated = v, msg.res.Truncated
}

// askDiff shows the diff of the marked file: as root at once, otherwise after the question of sudo qhost source diff.
func (m *Model) askDiff() tea.Cmd {
	ch, ok := m.selectedChange()
	cb, can := m.backend.(ChangesBackend)
	if !ok || !can {
		return nil
	}
	c, t := &m.sf.changes, m.text
	f := c.folder
	begin := func() int {
		c.diffGen++
		c.diff = &changeDiff{file: ch.Path, reading: true}
		return c.diffGen
	}
	if m.euid() == 0 {
		gen, ctx := begin(), m.engineCtx(f.Engine)
		return func() tea.Msg {
			res, err := cb.SourceDiff(ctx, f.Path, ch.Path)
			return diffMsg{gen: gen, res: res, err: err}
		}
	}
	var gen int
	m.askSudo(sudoAsk{title: textTitleDiff, question: t.Lookup(textAskDiff, ch.Path), consequence: t.Lookup(textWhatDiff),
		args:  m.engineArgs(f.Engine, []string{"source", "diff", "--output", "json", "--", f.Path, ch.Path}),
		begin: func() { gen = begin() },
		done: func(r sudoRun) tea.Cmd {
			msg := diffMsg{gen: gen}
			if !r.decode(&msg.res) || msg.res.File == "" {
				msg.err = sudoProblem(r.problem)
			}
			msg.applyTo(m)
			return nil
		}})
	return nil
}

// diffSpans colour a unified diff for the text view, which has no format for one: the heads of a file as keys, the
// head of a hunk as a comment, added lines as strings and removed lines as keywords.
func diffSpans(text string) [][]kittui.TextSpan {
	text = strings.TrimSuffix(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	out := make([][]kittui.TextSpan, len(lines))
	hunk := false
	for i, l := range lines {
		var role kittui.TextRole
		switch {
		case strings.HasPrefix(l, "diff "):
			hunk, role = false, kittui.TextRoleKey
		case strings.HasPrefix(l, "@@"):
			hunk, role = true, kittui.TextRoleComment
		case !hunk:
			role = kittui.TextRoleKey
		case strings.HasPrefix(l, "+"):
			role = kittui.TextRoleString
		case strings.HasPrefix(l, "-"):
			role = kittui.TextRoleKeyword
		case strings.HasPrefix(l, `\`):
			role = kittui.TextRoleComment
		}
		if role != kittui.TextRolePlain && l != "" {
			out[i] = []kittui.TextSpan{{End: len(l), Role: role}}
		}
	}
	return out
}

// askCommit asks to commit every file of the list at the head it was read at.
func (m *Model) askCommit() tea.Cmd {
	c, t := &m.sf.changes, m.text
	cb, can := m.backend.(ChangesBackend)
	if !can {
		return nil
	}
	if c.res.Truncated || len(c.res.Changes) > control.MaxDiscardPaths {
		m.warn(t.Lookup(textChangesCut))
		return nil
	}
	f, head := c.folder, c.head
	files := make([]string, len(c.res.Changes))
	for i, ch := range c.res.Changes {
		files[i] = ch.Path
	}
	details := files[:min(len(files), shownEntries)]
	if len(files) > shownEntries {
		details = append(details[:shownEntries:shownEntries], t.Lookup(textMoreEntries, len(files)-shownEntries))
	}
	what := textWhatCommitLocal
	if f.Kind == control.SourceKindRemote {
		what = textWhatCommitRemote
	}
	ctx, uid := m.engineCtx(f.Engine), uint32(m.humanUID())
	m.askSetting(settingChange{title: textTitleCommit, question: t.Lookup(textAskCommit, len(files), f.Path),
		consequence: t.Lookup(what), details: details, op: opChanges, name: f.Path,
		direct: func(Backend) (any, error) {
			res, err := cb.SourceCommit(ctx, control.SourceCommitArgs{Path: f.Path, Files: files, Head: head, SudoUID: uid})
			return commitAnswer(res, err)
		},
		args:   m.engineArgs(f.Engine, append([]string{"source", "commit", "--head", head, "--output", "json", "--", f.Path}, files...)),
		decode: decodeCommit})
	return nil
}

// committed is a commit that was made; commitRefused one the service refused, with the words for its code and the
// message of the service, which names places, never a value.
type committed struct{ control.SourceCommitResult }

type commitRefused struct {
	text   kittui.TextID
	detail string
}

// commitAnswer turns a refusal of the service into its words; any other failure stays one.
func commitAnswer(res control.SourceCommitResult, err error) (any, error) {
	var remote *control.Error
	if errors.As(err, &remote) {
		if text, ok := refusals[remote.Code]; ok {
			return commitRefused{text: text, detail: kittui.Escape(remote.Message)}, nil
		}
	}
	if err != nil {
		return nil, err
	}
	return committed{res}, nil
}

// decodeCommit reads what source commit printed, or the refusal it reported.
func decodeCommit(r sudoRun) (any, bool) {
	var res control.SourceCommitResult
	if r.decode(&res) && res.Commit != "" {
		return committed{res}, true
	}
	if text, ok := refusals[r.code]; ok {
		return commitRefused{text: text, detail: r.problem}, true
	}
	return nil, false
}

// finish says what was committed and offers the runs it starts.
func (c committed) finish(m *Model, _ func(string, ...string)) tea.Cmd {
	t := m.text
	dialog, err := newDialog(kittui.DialogSpec{Title: textTitleCommit,
		Question:    t.Lookup(textDoneCommitted, len(c.Committed), short(c.Commit)),
		Consequence: t.Lookup(textCommitRuns), Default: actNo, Keys: kittui.DialogKeys{Enter: actEnter},
		Answers: []kittui.Answer{{Action: actNo, Label: textAnswerClose}, {Action: actShowLog, Label: textAnswerRuns}}}, m.keys, m.text)
	if err != nil {
		m.failure = t.Lookup(kittui.TextError, err.Error())
	} else {
		m.modal = &modal{kind: modalAsk, screen: titleScreen(textTitleCommit), dialog: dialog, on: map[kittui.ActionID]func() tea.Cmd{actShowLog: func() tea.Cmd { return m.showRuns(nil) }}}
	}
	return tea.Batch(m.reloadNow(), m.readChanges())
}

// finish says in words why nothing was committed and reads the list again.
func (r commitRefused) finish(m *Model, _ func(string, ...string)) tea.Cmd {
	m.tell(textTitleCommitRefused, m.text.Lookup(r.text), []string{r.detail})
	return tea.Batch(m.reloadNow(), m.readChanges())
}

// discarded is the discard of one file of the kind it had.
type discarded struct {
	control.SourceDiscardResult
	file, kind string
}

func (d discarded) finish(m *Model, done func(string, ...string)) tea.Cmd {
	text := textDoneDiscarded
	switch d.kind {
	case control.ChangeNew:
		text = textDoneDiscardedNew
	case control.ChangeDeleted:
		text = textDoneDiscardedDeleted
	}
	done(m.text.Lookup(text, d.file))
	return tea.Batch(m.reloadNow(), m.readChanges())
}

// askDiscard asks to take back the change of the marked file; a new file is named as deleted.
func (m *Model) askDiscard() tea.Cmd {
	ch, ok := m.selectedChange()
	cb, can := m.backend.(ChangesBackend)
	if !ok || !can {
		return nil
	}
	f, t := m.sf.changes.folder, m.text
	ask, what, danger := textAskDiscardChange, textWhatDiscardChange, true
	switch ch.Kind {
	case control.ChangeNew:
		ask, what = textAskDiscardNew, textWhatDiscardNew
	case control.ChangeDeleted:
		ask, what, danger = textAskDiscardDeleted, textWhatDiscardDeleted, false
	}
	ctx := m.engineCtx(f.Engine)
	m.askSetting(settingChange{title: textTitleDiscardChange, question: t.Lookup(ask, ch.Path), consequence: t.Lookup(what),
		danger: danger, op: opChanges, name: f.Path,
		direct: func(Backend) (any, error) {
			res, err := cb.SourceDiscard(ctx, f.Path, []string{ch.Path})
			return discarded{res, ch.Path, ch.Kind}, err
		},
		args: m.engineArgs(f.Engine, []string{"source", "discard", "--yes", "--output", "json", "--", f.Path, ch.Path}),
		decode: func(r sudoRun) (any, bool) {
			var res control.SourceDiscardResult
			if !r.decode(&res) || res.Path == "" {
				return nil, false
			}
			return discarded{res, ch.Path, ch.Kind}, true
		}})
	return nil
}
