package tui

import (
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// The promotion of the versions of another stack, from the stack detail: choose the source among the other
// stacks of the host, read the preview from the services, confirm, run. The service checks every condition.

// actPromote opens the choice of the stack whose image versions the stack of the detail takes over.
const actPromote kittui.ActionID = "promote"

func promoteKeys() []kittui.Action {
	return []kittui.Action{act(actPromote, kittui.LevelLetter, textKeyPromote, "M")}
}

func (msg promoteCheckMsg) applyTo(m *Model) { m.applyPromoteCheck(msg) }

// opPromote labels the write; no branch reads the op of a promotion, so it shares the one of the return.
const opPromote = opReturn

// settingDone is the result of a write that tells what became of it itself.
type settingDone interface {
	finish(m *Model, done func(string, ...string)) tea.Cmd
}

// promoted is the answer of a promotion, from the service or read from the command.
type promoted struct{ control.PromoteRunResult }

func (p promoted) finish(m *Model, done func(string, ...string)) tea.Cmd {
	return m.promoteDone(p.PromoteRunResult, done)
}

// promoteStacks are the stacks of the host other than stack, by name; names are unique across engines.
func (m *Model) promoteStacks(stack string) []stackRow {
	var rows []stackRow
	for _, r := range m.rows {
		if r.name != stack {
			rows = append(rows, r)
		}
	}
	slices.SortFunc(rows, func(a, b stackRow) int { return strings.Compare(a.name, b.name) })
	return rows
}

// versionAction takes the keys of the detail that move the versions of the stack: the return and the promotion.
func (m *Model) versionAction(id kittui.ActionID) tea.Cmd {
	if id == actRollback {
		return m.rollbackFromDetail()
	}
	return m.openPromotePick()
}

func (m *Model) openPromotePick() tea.Cmd {
	stack, ok := m.detailStack()
	if !ok || m.detailBusy() {
		return nil
	}
	rows := m.promoteStacks(stack)
	if _, ok := m.backend.(PromoteBackend); !ok {
		return nil
	}
	if len(rows) == 0 {
		m.warn(m.text.Lookup(textPromoteNoSource))
		return nil
	}
	items := make([]kittui.ListItem, len(rows))
	for i, r := range rows {
		cell := r.name
		if m.multiEngine() && r.engine != "" {
			cell += " (" + r.engine + ")"
		}
		items[i] = kittui.ListItem{ID: kittui.ItemID(r.name), Cells: []string{cell}}
	}
	m.pick = m.newPicker(pickPromote, stack, textTitlePromotePick, []kittui.Column{{Title: textColStack}}, textPromoteNoSource, items)
	if m.pick != nil {
		m.pick.list.Select(kittui.ItemID(m.rowOf(m.stackEngineOf(stack), stack).promoteSource)) // no-op when unset or the stack is gone
	}
	return nil
}

// promoteCheckMsg carries the preview of a promotion: the source as read and what the target's service says.
type promoteCheckMsg struct {
	gen            int
	target, source string
	src            control.PromoteSource
	res            control.PromoteCheckResult
	err            error
}

// previewPromote reads the source at its service and asks the service of the target for the preview.
func (m *Model) previewPromote(target, source string) tea.Cmd {
	m.promoteGen++
	gen, b := m.promoteGen, m.backend.(PromoteBackend)
	sctx, tctx := m.engineCtx(m.stackEngineOf(source)), m.engineCtx(m.stackEngineOf(target))
	return func() tea.Msg {
		msg := promoteCheckMsg{gen: gen, target: target, source: source}
		if msg.src, msg.err = b.PromoteSource(sctx, source); msg.err != nil {
			return msg
		}
		msg.res, msg.err = b.PromoteCheck(tctx, control.PromoteArgs{Stack: target, Source: msg.src})
		return msg
	}
}

func (m *Model) applyPromoteCheck(msg promoteCheckMsg) {
	if msg.gen != m.promoteGen || m.modal != nil {
		return
	}
	t := m.text
	switch {
	case msg.err != nil:
		m.failure = t.Lookup(kittui.TextError, m.serviceText(msg.err))
	case len(msg.res.Steps) == 0:
		m.warn(t.Lookup(textPromoteNothing, msg.target, msg.source))
	default:
		m.askPromote(msg)
	}
}

// askPromote shows the versions, the source and its observation time, and asks for the run. The run is bound to
// the source commit of the preview: the service refuses when the source deployed another one since.
func (m *Model) askPromote(msg promoteCheckMsg) {
	t, res := m.text, msg.res
	details := []string{t.Lookup(textPromoteSource, msg.source, short(res.SourceCommit))}
	if o := res.Observe; o != nil {
		details = append(details, t.Lookup(textPromoteObserve, o.Until.In(m.loc).Format(lastRunLayout)))
	}
	for _, s := range res.Steps {
		details = append(details, t.Lookup(textRollbackStep, s.Variable, s.From, s.To))
	}
	target, source, commit := msg.target, msg.source, res.SourceCommit
	sengine, tengine := m.stackEngineOf(source), m.stackEngineOf(target)
	sctx, tctx := m.engineCtx(sengine), m.engineCtx(tengine)
	args := []string{"stack", "promote", "--confirm", "--source-commit", commit, "--output", "json"}
	if m.multiEngine() && sengine != "" {
		args = append(args, "--source-engine", sengine)
	}
	m.askSetting(settingChange{title: textTitlePromote, question: t.Lookup(textAskPromote, target, source),
		consequence: t.Lookup(textWhatPromote), details: details, op: opPromote, name: target,
		direct: func(bk Backend) (any, error) {
			b := bk.(PromoteBackend)
			// Read the source again: the service refuses when it is not at the commit the preview showed.
			src, err := b.PromoteSource(sctx, source)
			if err != nil {
				return control.PromoteRunResult{}, err
			}
			res, err := b.Promote(tctx, control.PromoteArgs{Stack: target, Source: src, Commit: commit})
			return promoted{res}, err
		},
		args:   m.engineArgs(tengine, append(args, "--", source, target)),
		decode: decodePromote})
}

// decodePromote reads the rows `stack promote --confirm` prints, also of a run that did not succeed.
func decodePromote(r sudoRun) (any, bool) {
	var rows []updateRunRow
	if !r.decode(&rows) || len(rows) == 0 {
		return nil, false
	}
	res := control.PromoteRunResult{Run: rows[0].Run, Commit: rows[0].Commit, Result: rows[0].Result, Error: rows[0].Error}
	res.Stack = rows[0].Stack
	for _, row := range rows {
		if row.Variable != "" {
			res.Steps = append(res.Steps, control.PromoteStepInfo{Variable: row.Variable, From: row.From, To: row.To})
		}
	}
	return promoted{res}, true
}

// promoteDone shows a finished promotion; a run that did not succeed names the run whose log tells why.
func (m *Model) promoteDone(res control.PromoteRunResult, done func(string, ...string)) tea.Cmd {
	t := m.text
	switch {
	case len(res.Steps) == 0:
		m.warn(t.Lookup(textPromoteNothing, res.Stack, res.Source))
	case res.Result != "succeeded":
		m.failure = t.Lookup(kittui.TextError, t.Lookup(textPromoteFailed, res.Stack, res.Result, res.Run))
		m.keep = m.failure
	default:
		parts := make([]string, len(res.Steps))
		for i, s := range res.Steps {
			parts[i] = fmt.Sprintf("%s %s -> %s", s.Variable, s.From, s.To)
		}
		done(t.Lookup(textDonePromote, res.Stack, res.Run), t.Lookup(textRollbackVersions, strings.Join(parts, ", ")))
	}
	return m.reloadNow()
}
