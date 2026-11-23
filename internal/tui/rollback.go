package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// The return to before an update run: from the Runs list on the run under the cursor, from the stack detail on the
// newest successful update run of the stack. The dialog chooses what happens to the data, then the confirmation shows
// the command. The service checks every other condition.

// returnable reports whether a run is a successful update run, the only kind a return applies to.
func returnable(r runRow) bool { return r.trigger == "update" && r.result == "succeeded" && !r.active }

// lastUpdateRun is the newest returnable run of the stack, as runData is newest first.
func (m *Model) lastUpdateRun(stack, engine string) (runRow, bool) {
	for _, r := range m.runData {
		if r.stack == stack && inEngine(r.engine, engine) && returnable(r) {
			return r, true
		}
	}
	return runRow{}, false
}

// selectedRun is the run under the cursor of the Runs list.
func (m *Model) selectedRun() (runRow, bool) {
	id, ok := m.runsList.Selected()
	if !ok {
		return runRow{}, false
	}
	r, ok := m.runByID[id]
	return r, ok
}

// rollbackRunHints offers the key on a returnable run.
func (m *Model) rollbackRunHints() []kittui.Hint {
	if r, ok := m.selectedRun(); ok && returnable(r) {
		return []kittui.Hint{{Action: actRollback, Label: textKeyRollback, Priority: 2}}
	}
	return nil
}

func (m *Model) rollbackFromRuns() tea.Cmd {
	r, ok := m.selectedRun()
	if !ok {
		return nil
	}
	if !returnable(r) {
		m.warn(m.text.Lookup(textRollbackNotRun))
		return nil
	}
	return m.askRollback(r)
}

func (m *Model) rollbackFromDetail() tea.Cmd {
	stack, ok := m.detailStack()
	if !ok || m.detailBusy() {
		return nil
	}
	r, found := m.lastUpdateRun(stack, m.stackEngineOf(stack))
	if !found {
		m.warn(m.text.Lookup(textRollbackNoRun))
		return nil
	}
	return m.askRollback(r)
}

// rollbackCheckMsg carries what the service says about a return, read before the dialog opens.
type rollbackCheckMsg struct {
	gen int
	row runRow
	res control.ReturnCheckResult
	err error
}

// askRollback opens the choice of what happens to the data. The service is asked first which versions the
// return sets and whether it is refused.
func (m *Model) askRollback(r runRow) tea.Cmd {
	if m.detailBusy() {
		return nil
	}
	m.rollbackGen++
	gen, b, ctx := m.rollbackGen, m.backend, m.engineCtx(r.engine)
	// keep, so a missing backup does not hide the versions; the data choice follows the run's backup.
	args := control.ReturnArgs{Stack: r.stack, Run: r.id, Data: control.ReturnKeepData}
	return func() tea.Msg {
		res, err := b.ReturnCheck(ctx, args)
		return rollbackCheckMsg{gen: gen, row: r, res: res, err: err}
	}
}

func (m *Model) applyRollbackCheck(msg rollbackCheckMsg) {
	if msg.gen != m.rollbackGen || m.modal != nil {
		return
	}
	if msg.err != nil {
		m.failure = m.text.Lookup(kittui.TextError, m.serviceText(msg.err))
		return
	}
	m.openRollbackDialog(msg.row, msg.res.Steps)
}

// openRollbackDialog shows the choice; steps are the versions the return sets.
func (m *Model) openRollbackDialog(r runRow, steps []control.ReturnStepInfo) {
	t := m.text
	var details []kittui.DialogLine
	for _, s := range steps {
		details = append(details, kittui.DialogLine{Text: t.Lookup(textRollbackStep, s.Variable, s.From, s.To)})
	}
	details = append(details, kittui.DialogLine{Text: t.Lookup(textRollbackData)}, kittui.DialogLine{Text: t.Lookup(textRollbackKeep)})
	answers := []kittui.Answer{{Action: actRollbackKeep, Label: textAnswerRollbackKeep}, {Action: actNo, Label: textAnswerCancel}}
	spec := kittui.DialogSpec{Title: textTitleRollback, Question: t.Lookup(textAskRollback, r.stack, r.id),
		Consequence: t.Lookup(textWhatRollback), Default: actNo, Keys: kittui.DialogKeys{Enter: actEnter}}
	if r.backup != "" {
		answers = append([]kittui.Answer{{Action: actRollbackData, Label: textAnswerRollbackData}}, answers...)
		spec.Danger = actRollbackData
	} else {
		details = append(details, kittui.DialogLine{Text: t.Lookup(textRollbackNoBackup), Style: kittui.StyleWarn})
	}
	spec.Details, spec.Answers = details, answers
	dialog, err := kittui.NewDialog(spec, m.keys, t)
	if err != nil {
		m.failure = t.Lookup(kittui.TextError, err.Error())
		return
	}
	m.modal = &modal{kind: modalAsk, dialog: dialog, on: map[kittui.ActionID]func() tea.Cmd{
		actRollbackData: func() tea.Cmd { m.confirmRollback(r, control.ReturnRestoreData); return nil },
		actRollbackKeep: func() tea.Cmd { m.confirmRollback(r, control.ReturnKeepData); return nil },
	}}
}

// confirmRollback asks for the return with the chosen data handling; without root the question shows the command.
func (m *Model) confirmRollback(r runRow, data string) {
	t := m.text
	flag, what := "--keep-data", textWhatRollbackKeep
	if data == control.ReturnRestoreData {
		flag, what = "--restore-data", textWhatRollbackRestore
	}
	args := control.ReturnArgs{Stack: r.stack, Run: r.id, Data: data}
	ctx := m.engineCtx(r.engine)
	m.askSetting(settingChange{title: textTitleRollback, question: t.Lookup(textAskRollbackRun, r.stack, r.id),
		consequence: t.Lookup(what), danger: data == control.ReturnRestoreData, op: opReturn, name: r.stack,
		direct: func(b Backend) (any, error) { return b.Return(ctx, args) },
		args:   m.engineArgs(r.engine, []string{"stack", "rollback", flag, "--yes", "--output", "json", "--", r.stack, r.id}),
		decode: decodeReturn})
}

// decodeReturn reads what `stack rollback` prints, also for a return that did not succeed.
func decodeReturn(r sudoRun) (any, bool) {
	var v struct {
		Stack    string `json:"stack"`
		Run      string `json:"update_run"`
		Data     string `json:"data"`
		Versions string `json:"versions"`
		ReturnID string `json:"run"`
		Result   string `json:"result"`
		Error    string `json:"error"`
	}
	if !r.decode(&v) || v.Stack == "" || v.Result == "" {
		return nil, false
	}
	// The command prints the versions as one text, which is kept whole as the only step.
	res := control.ReturnRunResult{ReturnRun: v.ReturnID, Result: v.Result, Error: v.Error}
	res.Stack, res.Run, res.Data = v.Stack, v.Run, v.Data
	if v.Versions != "" {
		res.Steps = []control.ReturnStepInfo{{Variable: v.Versions}}
	}
	return res, true
}

// returnVersions lists the versions a return set, one line per variable.
func returnVersions(steps []control.ReturnStepInfo) string {
	parts := make([]string, len(steps))
	for i, s := range steps {
		parts[i] = s.Variable
		if s.To != "" {
			parts[i] = fmt.Sprintf("%s %s -> %s", s.Variable, s.From, s.To)
		}
	}
	return strings.Join(parts, ", ")
}

// returnDone shows a finished return: a success says that the stack is pinned and how to release it, anything else
// names the run whose log tells why.
func (m *Model) returnDone(res control.ReturnRunResult, done func(string, ...string)) tea.Cmd {
	t := m.text
	if res.Result != "succeeded" {
		// The refresh that follows would wipe the failure, so it is kept.
		m.failure = t.Lookup(kittui.TextError, t.Lookup(textRollbackFailed, res.Stack, res.Result, res.ReturnRun))
		m.keep = m.failure
		return m.reloadNow()
	}
	details := []string{t.Lookup(textRollbackPinned)}
	if v := returnVersions(res.Steps); v != "" {
		details = append([]string{t.Lookup(textRollbackVersions, v)}, details...)
	}
	done(t.Lookup(textDoneRollback, res.Stack, res.Run), details...)
	return m.reloadNow()
}
