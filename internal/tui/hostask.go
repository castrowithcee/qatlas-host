package tui

import (
	"bytes"
	"encoding/json"
	"strings"

	tea "charm.land/bubbletea/v2"

	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/host/hostadmin"
	"github.com/castrowithcee/qatlas-host/internal/host/hostchange"
)

// A change of the host is asked in three parts: a sentence in plain words in the language of the interface, the
// exact commands dimmed below it, and y or n. Only y hands the terminal to sudo; the root command checks every
// rule again and stays the authority.

// hostAsk is one change the person asked for. secret is the standard input of an action that reads one; it is
// overwritten when the run is over or the question is dropped. mask are texts inside it that no message may show.
type hostAsk struct {
	id     string
	args   []string
	secret []byte
	mask   [][]byte
	// question is the sentence of the confirmation.
	question string
	// risk names why the change touches root, a system account or a system group; it makes the question red.
	risk string
	// note is a sentence in front of the consequence that does not make the question red.
	note string
	// final replaces the consequence of a change that cannot be undone with one that names what goes.
	final string
	// danger makes the confirming answer red without a risk, as a delete does.
	danger bool
	// self is set where the change gives the person behind the interface the group qhost.
	self bool
	// apply, when set, replaces the arguments of host apply, as for a proposal: the root command takes the change
	// from the proposal, so id and args only describe it here.
	apply []string
	// undo marks the undo of a journal entry, whose report names the new entry, which is not undone in turn.
	undo bool
}

// wipe overwrites the secret of the change.
func (a hostAsk) wipe() { zeroAll(append([][]byte{a.secret}, a.mask...)...) }

// hostPlanned is a change the registry accepted, ready to be confirmed.
type hostPlanned struct {
	ask           hostAsk
	commands      []string
	notReversible string
}

// planHost describes the change without running or reading anything; the text is the reason it is refused.
func (m *Model) planHost(a hostAsk) (hostPlanned, string) {
	plan, err := m.hostRegistry().Describe(a.id, a.args, a.secret)
	if err != nil {
		return hostPlanned{}, err.Error()
	}
	commands := make([]string, len(plan.Commands))
	for i, c := range plan.Commands {
		commands[i] = c.Display()
	}
	return hostPlanned{ask: a, commands: commands, notReversible: plan.NotReversible}, ""
}

// askHost plans the change and shows its confirmation; a refused plan goes to the form that asked, or to the
// message line. It reports whether the confirmation is open.
func (m *Model) askHost(a hostAsk, f *settingsForm) bool {
	p, refusal := m.planHost(a)
	if refusal != "" {
		a.wipe()
		if f != nil {
			f.problem(refusal, m.text.Lookup(textFormFix))
		} else {
			m.failure = m.text.Lookup(kittui.TextError, refusal)
		}
		return false
	}
	m.confirmHost(p)
	return m.modal != nil
}

// confirmHost shows the confirmation of a planned change; y runs it, every other answer drops its secret.
func (m *Model) confirmHost(p hostPlanned) {
	t, a := m.text, p.ask
	consequence := t.Lookup(textHostReversible)
	irreversible := false
	if p.notReversible != "" {
		consequence, irreversible = a.final, true
		if consequence == "" {
			consequence = t.Lookup(textHostIrreversible)
		}
	}
	if a.note != "" {
		consequence, irreversible = a.note+" "+consequence, true
	}
	danger := a.danger || p.notReversible != ""
	if a.risk != "" {
		consequence, irreversible, danger = a.risk+" "+consequence, true, true
	}
	m.askChange(a.question, consequence, irreversible, danger, nil, p.commands, func() tea.Cmd { return m.runHostChange(a) }, a.wipe)
}

// askChange shows the question of a change of the host: the sentence, the consequence, the lead lines and the exact
// commands dimmed below them, and y or n. y calls yes; every other answer, and a dialog that cannot be shown, calls
// drop where it is set. danger makes the yes red.
func (m *Model) askChange(question, consequence string, irreversible, danger bool, lead []kittui.DialogLine, commands []string,
	yes func() tea.Cmd, drop func()) {
	m.askCommands(textTitleHostChange, question, consequence, irreversible, danger, lead, commands, yes, drop)
}

// askCommands is askChange under the title of the action.
func (m *Model) askCommands(title kittui.TextID, question, consequence string, irreversible, danger bool, lead []kittui.DialogLine,
	commands []string, yes func() tea.Cmd, drop func()) {
	details := append([]kittui.DialogLine{}, lead...)
	for _, c := range commands {
		details = append(details, kittui.DialogLine{Prefix: "$ ", Text: c, Style: kittui.StyleHint})
	}
	spec := kittui.DialogSpec{Title: title, Question: question, Consequence: consequence, Details: details,
		Irreversible: irreversible, Answers: []kittui.Answer{{Action: actYes, Label: textAnswerRunChange}, {Action: actNo, Label: textAnswerCancel}},
		Default: actNo, Keys: kittui.DialogKeys{Enter: actEnter}}
	if danger {
		spec.Danger = actYes
	}
	dialog, err := kittui.NewDialog(spec, m.keys, m.text)
	if err != nil {
		if drop != nil {
			drop()
		}
		m.failure = m.text.Lookup(kittui.TextError, err.Error())
		return
	}
	m.modal = &modal{kind: modalAsk, dialog: dialog, yes: yes, drop: drop}
}

// runHostChange runs host apply and shows what it reported. The lists are read again afterwards: the host may
// have changed even when the run failed and was restored.
func (m *Model) runHostChange(a hostAsk) tea.Cmd {
	argv := append([]string{"host", "apply", "--confirm", "--output", "json", "--", a.id}, a.args...)
	if a.apply != nil {
		argv = append([]string{"host"}, a.apply...)
	}
	req := HostRequest{Args: argv, Secret: a.secret}
	after := func(out []byte, err error) tea.Cmd {
		report, ok := parseReport(out)
		if !ok {
			m.hostFailed(out, err, true)
			return tea.Batch(m.loadSystem(true), m.afterHostChange())
		}
		m.hostResult(report, a)
		if report.Result == hostchange.ResultRefused {
			return nil
		}
		return tea.Batch(m.loadSystem(true), m.afterHostChange())
	}
	if len(a.secret) > 0 {
		return m.runHostSecret(req, a.mask, after)
	}
	return m.runHost(req, after)
}

// hostResult shows the report of a change. A failure is no red button: red stays with the confirming button of a
// destructive question.
func (m *Model) hostResult(r hostReport, a hostAsk) {
	t := m.text
	var title string
	var note kittui.DialogLine
	switch r.Result {
	case hostchange.ResultApplied:
		title = t.Lookup(textHostDone, a.question)
		note = kittui.DialogLine{Text: t.Lookup(textHostDoneUndo, r.JournalID), Style: kittui.StyleHint}
		switch {
		case a.undo:
			note = kittui.DialogLine{Text: t.Lookup(textUndoDone, r.JournalID), Style: kittui.StyleHint}
		case !r.Reversible:
			note = kittui.DialogLine{Text: t.Lookup(textHostDoneFinal), Style: kittui.StyleHint}
		}
	case hostchange.ResultRefused:
		title, note = t.Lookup(textHostRefused, r.Error), kittui.DialogLine{Text: t.Lookup(textHostRefusedNote), Style: kittui.StyleWarn}
		r.Error = ""
	case hostchange.ResultRestored:
		title, note = t.Lookup(textHostRestored, a.question), kittui.DialogLine{Text: t.Lookup(textHostRestoredNote), Style: kittui.StyleWarn}
	default:
		title, note = t.Lookup(textHostFailed, a.question), kittui.DialogLine{Text: t.Lookup(textHostFailedNote), Style: kittui.StyleWarn}
	}
	var more []kittui.DialogLine
	if r.Error != "" {
		more = append(more, kittui.DialogLine{Text: r.Error, Style: kittui.StyleWarn})
	}
	switch r.Check {
	case hostchange.CheckPassed:
		more = append(more, kittui.DialogLine{Text: t.Lookup(textHostCheckPassed), Style: kittui.StyleHint})
	case hostchange.CheckFailed:
		more = append(more, kittui.DialogLine{Text: t.Lookup(textHostCheckFailed, r.CheckError), Style: kittui.StyleWarn})
	}
	if r.Warning != "" {
		more = append(more, kittui.DialogLine{Text: t.Lookup(textHostAuditWarning, r.Warning), Style: kittui.StyleWarn})
	}
	if a.self && r.Result == hostchange.ResultApplied && m.euid() != 0 {
		more = append(more, kittui.DialogLine{Text: t.Lookup(textHostRegroup), Style: kittui.StyleWarn})
	}
	details := []kittui.DialogLine{note}
	if len(more) > 0 {
		details = append(append(details, kittui.DialogLine{}), more...)
	}
	m.hostNotice(title, details)
}

// hostNotice shows a calm message about a run through sudo.
func (m *Model) hostNotice(title string, details []kittui.DialogLine) {
	dialog, err := kittui.NewDialog(kittui.DialogSpec{Title: textTitleHostResult, Question: title, Notice: true, Details: details,
		Answers: []kittui.Answer{{Action: actNo, Label: textAnswerClose}}, Default: actNo,
		Keys: kittui.DialogKeys{Enter: actEnter}}, m.keys, m.text)
	if err != nil {
		m.failure = m.text.Lookup(kittui.TextError, err.Error())
		return
	}
	m.modal = &modal{kind: modalAsk, dialog: dialog}
}

// hostFailed explains a run that gave no usable answer. change says a host change was asked for, where an
// interrupted or failed run leaves open what was done.
func (m *Model) hostFailed(out []byte, err error, change bool) {
	t := m.text
	kind, exit := classifyRun(out, err)
	reason := stderrLines(err)
	title := t.Lookup(textHostReadFailed)
	if change {
		title = t.Lookup(textHostChangeFailed)
	}
	var text string
	switch {
	case noCredential(reason):
		text = t.Lookup(textHostErrNoCredential)
	case change && reason != "" && !strings.HasPrefix(reason, "sudo:") && (kind == failDenied || kind == failExit):
		// A line of sudo itself ("sudo: 3 incorrect password attempts") keeps the message about sudo; anything else
		// is the reason qhost gave.
		text = t.Lookup(textHostErrReported, reason)
	default:
		switch kind {
		case failNoSudo:
			text = t.Lookup(textHostErrNoSudo)
			if err != nil && !strings.Contains(err.Error(), ErrNoSudo.Error()) && err != errNoRunner {
				text = t.Lookup(textHostErrStart, err.Error())
			}
		case failDenied:
			text = t.Lookup(textHostErrDenied)
		case failAborted:
			text = t.Lookup(textHostErrAbortedRead)
			if change {
				text = t.Lookup(textHostErrAbortedChange)
			}
		case failExit:
			text = t.Lookup(textHostErrExitRead, exit)
			if change {
				text = t.Lookup(textHostErrExitChange, exit)
			}
		default:
			text = t.Lookup(textHostErrOutput)
		}
	}
	m.hostNotice(title, []kittui.DialogLine{{Text: text, Style: kittui.StyleWarn}})
}

// readUserKeys reads the SSH keys of a user as root with `host user`, for an interface that cannot read them
// itself; then is given the user with the keys read.
func (m *Model) readUserKeys(u hostadmin.User, then func(hostadmin.User) tea.Cmd) tea.Cmd {
	return m.runHost(HostRequest{Args: []string{"host", "user", "--output", "json", "--", u.Name}}, func(out []byte, err error) tea.Cmd {
		var read struct {
			SSHKeys []hostadmin.SSHKey `json:"ssh_keys"`
		}
		dec := json.NewDecoder(bytes.NewReader(bytes.TrimSpace(out)))
		if err != nil || dec.Decode(&read) != nil || read.SSHKeys == nil {
			m.hostFailed(out, err, false)
			return nil
		}
		u.SSHKeys = read.SSHKeys
		return then(u)
	})
}
