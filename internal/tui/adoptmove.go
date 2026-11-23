package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// A foreign project can also move into a source that exists. The preview and the move are the public command
// `qhost source move`, run through sudo, or directly as root: only the command adds what the host tells to the
// answer of the service (other file system, other engine, taken names), and it moves with the way back when a step
// fails. The interface shows its preview with the blocks and warnings and asks before the command moves.

// formAdoptTarget chooses where a foreign project goes: a source of its own or one that exists.
const formAdoptTarget formKind = formHookListen + 1

// moveOwn is the choice of the form that takes the project over as a source of its own; a path never equals it.
const moveOwn = "own"

// The results of the move command.
const moveMoved = "moved"

// moveFlow is a project being moved: the labels it was chosen by, the source it goes into and the preview.
type moveFlow struct {
	src  foreignSource
	into string
	res  control.SourceMoveResult
}

// movePreviewed is the answer of the preview.
type movePreviewed struct{ flow *moveFlow }

// moveOutcome is what the move command prints at its end, also when it took everything back.
type moveOutcome struct {
	Target     string     `json:"target"`
	Project    string     `json:"project"`
	Result     string     `json:"result"`
	Location   string     `json:"location"`
	FailedStep string     `json:"failed_step"`
	Error      string     `json:"error"`
	Commit     string     `json:"commit"`
	Rollback   []moveStep `json:"rollback"`
	Remaining  []string   `json:"remaining"`
}

type moveStep struct {
	Step   string `json:"step"`
	Result string `json:"result"`
	Detail string `json:"detail"`
}

// moveTargets are the sources the project can be moved into: those of its engine that are no root stack, as the
// command refuses a root stack. The command decides again.
func (m *Model) moveTargets(src foreignSource) []control.SourceStatus {
	root := map[stackRef]bool{}
	for _, s := range m.snap.stacks.Stacks {
		if s.Folder == rootStackFolder {
			root[stackRef{s.Engine, s.SourceDir}] = true
		}
	}
	var out []control.SourceStatus
	for _, s := range m.snap.status.Engine.Sources {
		if inEngine(s.Engine, src.engine) && !root[stackRef{s.Engine, s.Path}] {
			out = append(out, s)
		}
	}
	return out
}

// adoptTargetForm asks where the project goes, preselecting a source of its own, which the flow was before.
func (m *Model) adoptTargetForm(src foreignSource, targets []control.SourceStatus) *settingsForm {
	t := m.text
	field := kittui.FormField{ID: "target", Kind: kittui.FieldChoice, Label: textFieldMoveTarget, Hint: textHintMoveTarget,
		Value:   kittui.FormValue{Choice: moveOwn},
		Choices: []kittui.Choice{choice(moveOwn, textMoveOwn)}}
	for _, s := range targets {
		field.Choices = append(field.Choices, choice(s.Path, kittui.TextID(kittui.Escape(t.Lookup(textMoveInto, s.Path)))))
	}
	return m.newSettingsForm(formAdoptTarget, textTitleAdopt, []kittui.FormField{field})
}

// submitAdoptTarget goes on as before for a source of its own, else asks for the preview of the move.
func (m *Model) submitAdoptTarget(v map[kittui.FieldID]kittui.FormValue) tea.Cmd {
	flow := m.set.move
	target := string(v["target"].Choice)
	if flow == nil || target == "" {
		m.dropForm()
		return nil
	}
	if target == moveOwn {
		m.set.move = nil
		return m.inspectForeign(flow.src)
	}
	flow.into = target
	return m.previewMove(flow)
}

// previewArgs are the arguments after qhost of the preview, moveArgs those of the move itself. They carry no
// --engine on a host with several engines: source move takes the engine from the folder and has no such flag.
func (f *moveFlow) previewArgs() []string {
	return []string{"source", "move", "--into", f.into, "--output", "json", "--", f.src.dir}
}

func (f *moveFlow) moveArgs() []string {
	return []string{"source", "move", "--into", f.into, "--yes", "--output", "json", "--", f.src.dir}
}

// previewMove runs the preview. The command reads from the terminal it is given and would ask to move, so it
// gets a standard input that is none (the password check comes first, on the real terminal): then it only
// shows the preview and changes nothing.
func (m *Model) previewMove(f *moveFlow) tea.Cmd {
	gen := m.beginSetting()
	return m.runSudoInput(f.previewArgs(), []byte("\n"), func(r sudoRun) tea.Cmd {
		msg := settingMsg{gen: gen, op: opMovePreview, name: f.src.dir}
		var got control.SourceMoveResult
		if !r.decode(&got) || got.Path == "" {
			msg.err = sudoProblem(r.problem)
			return m.applySetting(msg)
		}
		f.res, msg.res = got, movePreviewed{f}
		return m.applySetting(msg)
	})
}

// showMovePreview shows what the move would do. A block ends the flow: the question that moves is not asked.
func (m *Model) showMovePreview(f *moveFlow) {
	t, res := m.text, f.res
	body := func(width int) []string { return m.moveLines(f, width) }
	if len(res.Blocks) > 0 {
		m.askSheet(textTitleMove, t.Lookup(textAskMoveBlocked, f.src.project, f.into), t.Lookup(textWhatNothing), body, nil, nil)
		return
	}
	// The last question shows the command that yes runs without root.
	final := func(width int) []string {
		if m.euid() == 0 {
			return body(width)
		}
		return append(body(width), m.commandLines(f.moveArgs(), width)...)
	}
	yes := []kittui.Answer{{Action: actYes, Label: textAnswerYes}, {Action: actNo, Label: textAnswerCancel}}
	m.askSheet(textTitleMove, t.Lookup(textAskMove, f.src.project, f.into, res.Target), t.Lookup(textWhatMove), final, yes,
		func() tea.Cmd { return m.runMove(f) })
}

// moveLines are the preview: where the project goes, the files of the commit, what is left out, the blocks with
// what to do, and the warnings.
func (m *Model) moveLines(f *moveFlow, width int) []string {
	t, res := m.text, f.res
	text := func(style kittui.TextStyle, s string) []string { return m.wrapped(detailIndent, style, s, width) }
	lines := []string{""}
	lines = append(lines, text(kittui.StyleText, t.Lookup(textDetailDir, f.src.dir))...)
	lines = append(lines, text(kittui.StyleText, t.Lookup(textDetailMoveInto, f.into))...)
	if res.Target != "" {
		lines = append(lines, text(kittui.StyleText, t.Lookup(textDetailMoveTo, res.Target))...)
	}
	if res.Project != "" {
		lines = append(lines, text(kittui.StyleText, t.Lookup(textAdoptProject, res.Project))...)
	}
	if res.NewProject != "" && res.NewProject != res.Project {
		lines = append(lines, text(kittui.StyleText, t.Lookup(textDetailMoveName, res.NewProject))...)
	}
	if res.Engine != "" {
		lines = append(lines, text(kittui.StyleText, t.Lookup(textDetailEngine, res.Engine))...)
	}
	if len(res.PlannedFiles) > 0 {
		lines = append(lines, "", m.line(detailMargin, kittui.StyleActive, t.Lookup(textHeadFiles, len(res.PlannedFiles)), width))
		lines = append(lines, m.entryLines(res.PlannedFiles, width)...)
	}
	if len(res.Excluded) > 0 {
		var left []string
		for _, e := range res.Excluded {
			left = append(left, t.Lookup(textExcludedEntry, e.Path, e.Reason))
		}
		lines = append(lines, "", m.line(detailMargin, kittui.StyleActive, t.Lookup(textHeadExcluded, len(left)), width))
		lines = append(lines, m.entryLines(left, width)...)
	}
	issues := func(list []control.SourceMoveIssue, style kittui.TextStyle) {
		for _, i := range list {
			lines = append(lines, text(style, t.Lookup(textMoveIssue, i.Code, i.Reason))...)
			if i.Fix != "" {
				lines = append(lines, text(kittui.StyleText, t.Lookup(textDetailFix, i.Fix))...)
			}
			if len(i.Items) > 0 {
				lines = append(lines, text(kittui.StyleHint, t.Lookup(textMoveItems, strings.Join(i.Items, ", ")))...)
			}
		}
	}
	if len(res.Blocks) > 0 {
		lines = append(lines, "", m.line(detailMargin, kittui.StyleActive, t.Lookup(textHeadBlocks, len(res.Blocks)), width))
		issues(res.Blocks, kittui.StyleFail)
	}
	if len(res.Warnings) > 0 {
		lines = append(lines, "", m.heading(textHeadWarning, width))
		issues(res.Warnings, kittui.StyleWarn)
	}
	return lines
}

// runMove runs the move; yes to its question is the only way here.
func (m *Model) runMove(f *moveFlow) tea.Cmd {
	gen := m.beginSetting()
	return m.runSudo(f.moveArgs(), func(r sudoRun) tea.Cmd {
		msg := settingMsg{gen: gen, op: opMove, name: f.src.dir}
		// A move that was taken back exits with a failure and still prints its result.
		var got moveOutcome
		if !r.decode(&got) || got.Result == "" {
			msg.err = sudoProblem(r.problem)
		} else {
			msg.res = got
		}
		return m.applySetting(msg)
	})
}

// moveDone says how the move ended, with the way back and what is left to do by hand; the lists are read again,
// since even a move that did not finish may have changed what runs.
func (m *Model) moveDone(res moveOutcome, done func(string, ...string)) []tea.Cmd {
	t := m.text
	cmds := []tea.Cmd{m.reloadNow(), m.rereadForeign()}
	if res.Result == moveMoved {
		details := []string{t.Lookup(textMoveLocation, res.Location)}
		if res.Commit != "" {
			details = append(details, t.Lookup(textDetailHead, short(res.Commit)))
		}
		done(t.Lookup(textDoneMoved, res.Project, res.Target), details...)
		return cmds
	}
	// Anything but a clean way back counts as unfinished, so the remaining work is never hidden.
	text, details := t.Lookup(textMoveRolledBack, res.FailedStep, res.Error), []string{t.Lookup(textMoveLocation, res.Location)}
	if len(res.Remaining) > 0 || res.Result != "rolled-back" {
		text = t.Lookup(textMoveIncomplete, res.FailedStep, res.Error)
	}
	for _, s := range res.Rollback {
		details = append(details, t.Lookup(textMoveBackStep, s.Step, strings.TrimSpace(s.Result+" "+s.Detail)))
	}
	for _, r := range res.Remaining {
		details = append(details, t.Lookup(textMoveRemaining, r))
	}
	m.tell(textTitleMove, text, details)
	return cmds
}
