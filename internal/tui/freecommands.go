package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// A proposal of free commands carries shell commands and a reason that nothing checks. The interface shows both
// word for word with their control characters escaped, never words them differently and never hands them to a
// shell of its own: running one is `sudo qhost proposal run`, which takes the id and the fingerprint of the
// proposal as it was shown and the argument list as it is. The switch that allows these proposals is turned
// with `sudo qhost proposal commands enable|disable` from Settings > General.

// fieldFreeCmds is the field of the General form that holds the switch; it exists only once the switch was read.
const fieldFreeCmds = "freecmds"

// The two values of that field.
const (
	freeChoiceOn  = "on"
	freeChoiceOff = "off"
)

// freeSwitch is the switch for proposals of free commands as the host service answered last. known is false
// until a read worked; the switch counts as off then.
type freeSwitch struct {
	known, enabled bool
}

// freeSwitchMsg carries the answer of a read of the switch.
type freeSwitchMsg struct {
	res control.HostCommandsResult
	err error
}

// readFreeSwitch reads the switch off the event loop.
func (m *Model) readFreeSwitch() tea.Cmd {
	h, ctx := m.hostProps, m.ctx
	return func() tea.Msg {
		res, err := h.CommandsStatus(ctx)
		return freeSwitchMsg{res: res, err: err}
	}
}

// applyFreeSwitch takes a read of the switch. The General form shows it, so a form without unsaved changes is
// built again.
func (m *Model) applyFreeSwitch(msg freeSwitchMsg) {
	m.free = freeSwitch{known: msg.err == nil, enabled: msg.err == nil && msg.res.Enabled}
	m.refreshGeneral()
}

func (m *Model) refreshGeneral() {
	if f := m.set.form; f != nil && f.kind == formGeneral && !f.form.Changed() {
		m.set.form = m.generalForm()
	}
}

// freeSwitchField is the field of the General form for the switch.
func (m *Model) freeSwitchField() kittui.FormField {
	value := freeChoiceOff
	if m.free.enabled {
		value = freeChoiceOn
	}
	return kittui.FormField{ID: fieldFreeCmds, Kind: kittui.FieldChoice, Label: textFieldFreeCmds, Hint: textHintFreeCmds,
		Choices: []kittui.Choice{choice(freeChoiceOn, textFreeOn), choice(freeChoiceOff, textFreeOff)},
		Value:   kittui.FormValue{Choice: kittui.ChoiceID(value)}}
}

// askFreeSwitch asks to turn the switch on or off; y runs the command through sudo.
func (m *Model) askFreeSwitch(on bool) {
	t := m.text
	verb, question, consequence := "disable", textAskFreeOff, textWhatFreeOff
	if on {
		verb, question, consequence = "enable", textAskFreeOn, textWhatFreeOn
	}
	m.askSudo(sudoAsk{title: textTitleFreeSwitch, question: t.Lookup(question), consequence: t.Lookup(consequence), danger: on,
		args: []string{"proposal", "commands", verb, "--output", "json"},
		done: func(r sudoRun) tea.Cmd {
			var v map[string]any
			enabled, ok := false, r.decode(&v)
			if ok {
				enabled, ok = v["enabled"].(bool)
			}
			if !ok {
				m.failure = t.Lookup(kittui.TextError, r.problem)
				return nil
			}
			m.free = freeSwitch{known: true, enabled: enabled}
			m.refreshGeneral()
			if enabled {
				m.result = t.Lookup(textFreeSwitchedOn)
			} else {
				m.result = t.Lookup(textFreeSwitchedOff)
			}
			return nil
		}})
}

// canRunFree says a proposal of free commands may be run from here: it was read, is open and passes as a proposal
// of this kind, and the switch is on.
func (m *Model) canRunFree(d hostProposalDetail) bool {
	return d.approvable() && m.free.enabled
}

// freeRunArgs are the arguments after qhost that run the proposal as it was shown.
func freeRunArgs(id, fingerprint string) []string {
	return []string{"proposal", "run", id, "--fingerprint", fingerprint, "--confirm", "--output", "json"}
}

// askFreeRun asks to run the commands of the proposal; the question names how many there are. y hands the
// terminal to sudo.
func (m *Model) askFreeRun(d hostProposalDetail) tea.Cmd {
	t := m.text
	if !m.canRunFree(d) {
		if d.err == nil && d.info.State == control.ProposalOpen {
			m.warn(t.Lookup(textFreeSwitchOff))
		} else {
			m.warn(t.Lookup(textPropNotOpen))
		}
		return nil
	}
	id, args := d.info.ID, freeRunArgs(d.info.ID, d.info.Fingerprint)
	question := t.Lookup(textAskFreeRun, len(d.free.Commands), kittui.Escape(proposalShort(id)))
	m.askCommands(textTitleFreeRun, question, t.Lookup(textWhatFreeRun), true, true, nil, []string{sudoCommand(args)},
		func() tea.Cmd { return m.runFree(id, len(d.free.Commands), args) }, nil)
	return nil
}

// freeRunCommand and freeRunReport are what `proposal run` prints with --output json.
type freeRunCommand struct {
	Command   string `json:"command"`
	Ran       bool   `json:"ran"`
	ExitCode  int    `json:"exit_code"`
	Output    string `json:"output"`
	Truncated bool   `json:"truncated"`
	TimedOut  bool   `json:"timed_out"`
}

type freeRunReport struct {
	JournalID string           `json:"journal_id"`
	Result    string           `json:"result"`
	Commands  []freeRunCommand `json:"commands"`
}

// ok says the command ran and exited with 0.
func (c freeRunCommand) ok() bool { return c.Ran && !c.TimedOut && c.ExitCode == 0 }

// parseFreeRun reads the report of a run. The command exits with a failure after printing it when a command
// failed, so the report counts whatever the exit status was.
func parseFreeRun(out []byte) (freeRunReport, bool) {
	var r freeRunReport
	if json.Unmarshal(bytes.TrimSpace(out), &r) != nil || r.Result == "" || len(r.Commands) == 0 {
		return freeRunReport{}, false
	}
	return r, true
}

// runFree runs `sudo qhost proposal run` and shows what each command did. Nothing about the commands passes
// through the interface: it only reads the report.
func (m *Model) runFree(id string, n int, args []string) tea.Cmd {
	return m.runHost(HostRequest{Args: args}, func(out []byte, err error) tea.Cmd {
		report, ok := parseFreeRun(out)
		if !ok {
			m.hostFailed(out, err, true)
		} else {
			m.showFreeRun(id, report)
		}
		return tea.Batch(m.loadSystem(true), m.afterHostChange())
	})
}

// showFreeRun shows the report in a sheet: which commands ran with which exit code and output, and which did not
// run any more.
func (m *Model) showFreeRun(id string, r freeRunReport) {
	t := m.text
	failed := -1
	for i, c := range r.Commands {
		if !c.ok() {
			failed = i
			break
		}
	}
	question := t.Lookup(textFreeRunDone, kittui.Escape(proposalShort(id)), len(r.Commands))
	consequence := t.Lookup(textFreeRunDoneNote, kittui.Escape(r.JournalID))
	if failed >= 0 {
		skipped := 0
		for _, c := range r.Commands[failed+1:] {
			if !c.Ran {
				skipped++
			}
		}
		question = t.Lookup(textFreeRunFailed, kittui.Escape(proposalShort(id)), failed+1, len(r.Commands), skipped)
		consequence = t.Lookup(textFreeRunFailedNote, kittui.Escape(r.JournalID))
	}
	body := func(width int) []string { return m.freeRunLines(r, width) }
	m.askSheet(textTitleFreeResult, question, consequence, body, nil, nil)
}

// freeRunLines are the commands of a run, numbered, each with its exit code and its output.
func (m *Model) freeRunLines(r freeRunReport, width int) []string {
	t := m.text
	var lines []string
	for i, c := range r.Commands {
		lines = append(lines, "")
		lines = append(lines, m.numberedLines(i+1, kittui.StyleText, c.Command, width)...)
		style, status := kittui.StyleOK, t.Lookup(textFreeExit, c.ExitCode)
		switch {
		case !c.Ran:
			style, status = kittui.StyleHint, t.Lookup(textFreeNotRun)
		case c.TimedOut:
			style, status = kittui.StyleFail, t.Lookup(textFreeTimedOut)
		case c.ExitCode != 0:
			style = kittui.StyleFail
		}
		lines = append(lines, m.wrapped(detailIndent+freeNumberWidth, style, status, width)...)
		if !c.Ran {
			continue
		}
		if c.Output == "" {
			lines = append(lines, m.wrapped(detailIndent+freeNumberWidth, kittui.StyleHint, t.Lookup(textFreeNoOutput), width)...)
		} else {
			lines = append(lines, m.wrapped(detailIndent+freeNumberWidth, kittui.StyleHint, c.Output, width)...)
		}
		if c.Truncated {
			lines = append(lines, m.wrapped(detailIndent+freeNumberWidth, kittui.StyleWarn, t.Lookup(textFreeCut), width)...)
		}
	}
	return lines
}

// freeNumberWidth is the width of the number in front of a command: two digits, a dot and a space.
const freeNumberWidth = 4

// numberedLines is text as it was written, escaped, with its number in front and the following lines under the
// first.
func (m *Model) numberedLines(n int, style kittui.TextStyle, text string, width int) []string {
	lines := m.wrapped(detailIndent+freeNumberWidth, style, text, width)
	lead := strings.Repeat(" ", detailIndent) + fmt.Sprintf("%2d. ", n)
	if len(lines) == 0 {
		return []string{lead}
	}
	lines[0] = lead + strings.TrimPrefix(lines[0], strings.Repeat(" ", detailIndent+freeNumberWidth))
	return lines
}

// quotedLines is text as it was written, escaped, with a mark in front of every line.
func (m *Model) quotedLines(style kittui.TextStyle, text string, width int) []string {
	const mark = "> "
	lines := m.wrapped(detailIndent+len(mark), style, text, width)
	for i, l := range lines {
		lines[i] = strings.Repeat(" ", detailIndent) + mark + strings.TrimPrefix(l, strings.Repeat(" ", detailIndent+len(mark)))
	}
	if len(lines) == 0 {
		return []string{strings.Repeat(" ", detailIndent) + mark}
	}
	return lines
}

// freeProposalLines are the content of a proposal of free commands: the reason as a quote and the commands
// numbered, both exactly as the submitter wrote them.
func (m *Model) freeProposalLines(d hostProposalDetail, width int) []string {
	t := m.text
	reason := d.free.Reason
	if reason == "" {
		reason = t.Lookup(textNoValue)
	}
	lines := []string{"", m.heading(textFreeHeadReason, width)}
	lines = append(lines, m.quotedLines(kittui.StyleText, reason, width)...)
	lines = append(lines, "", m.heading(textFreeHeadCommands, width))
	for i, c := range d.free.Commands {
		lines = append(lines, m.numberedLines(i+1, kittui.StyleText, c, width)...)
	}
	if d.err == nil && d.info.State == control.ProposalOpen && !m.free.enabled {
		lines = append(lines, "", m.line(detailMargin, kittui.StyleHint, t.Lookup(textFreeSwitchOff), width))
	}
	return lines
}

// freeNotice is the fixed warning at the top of a proposal of free commands.
func (m *Model) freeNotice(width int) []string {
	text := m.styles.Glyph(kittui.GlyphWarning) + " " + m.text.Lookup(textFreeNotice)
	return append(m.wrapped(detailMargin, kittui.StyleWarn, text, width), "")
}
