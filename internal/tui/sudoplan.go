package tui

import (
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// Without root the actions of the Stacks area run the public command of the action through sudo, one command per
// stack and one after another, and read what it printed as JSON.

// sudoPlanArgs are the arguments after qhost of an action on one stack; the check has no stack.
func sudoPlanArgs(kind actionKind, stack string) []string {
	switch kind {
	case kindStart, kindStop, kindRestart:
		return []string{"stack", ctVerbs[kind], "--yes", "--output", "json", "--", stack}
	case kindCheck:
		return []string{"stack", "updates", "--refresh", "--output", "json"}
	}
	verb := map[actionKind]string{kindDeploy: "deploy", kindUpdate: "update", kindPause: "pause", kindUnpause: "unpause",
		kindPin: "pin", kindUnpin: "unpin", kindResume: "resume"}[kind]
	return []string{"stack", verb, "--output", "json", "--", stack}
}

// askSudoPlan asks for the action with the exact commands. The check asks the registries, which it says first.
func (m *Model) askSudoPlan(p plan) {
	info := kindTexts[p.kind]
	t := m.text
	a := sudoAsk{title: info.word, question: t.Lookup(info.ask, m.subject(p)), consequence: t.Lookup(info.consequence),
		lead: dialogLines(m.targetDetails(p.stacks))}
	first := ""
	if p.kind == kindCheck {
		a.remote = t.Lookup(textSudoRemoteRegistry)
	} else {
		first = p.stacks[0]
		for i, name := range p.stacks[1:] {
			a.also = append(a.also, m.engineArgs(p.engineAt(i+1), sudoPlanArgs(p.kind, name)))
		}
	}
	a.args = m.engineArgs(p.engineAt(0), sudoPlanArgs(p.kind, first))
	var gen int
	a.begin = func() {
		m.runGen++
		gen = m.runGen
		m.run = &running{gen: gen, plan: p, sudo: true}
	}
	a.done = func(r sudoRun) tea.Cmd { return m.sudoStepDone(gen, 0, p.kind, first)(r) }
	m.askSudo(a)
}

// sudoStepDone takes what the command of one step printed as the answer of that step.
func (m *Model) sudoStepDone(gen, index int, kind actionKind, stack string) func(sudoRun) tea.Cmd {
	return func(r sudoRun) tea.Cmd {
		msg := stepMsg{gen: gen, index: index}
		if res, ok := decodeSudoStep(kind, stack, r); ok {
			msg.result = res
		} else {
			msg.err = sudoProblem(r.problem)
		}
		return m.applyStep(msg)
	}
}

// decodeSudoStep reads the JSON of the command of an action into the result the summary reads; false when it printed
// no answer. A command that failed after printing its rows, as an update of a stack that was refused, still answers.
func decodeSudoStep(kind actionKind, stack string, r sudoRun) (any, bool) {
	switch kind {
	case kindDeploy:
		var w struct {
			Run    string `json:"run"`
			Stack  string `json:"stack"`
			Commit string `json:"commit"`
		}
		if !r.decode(&w) || w.Run == "" {
			return nil, false
		}
		return control.DeployResult{ID: w.Run, Stack: w.Stack, Commit: w.Commit}, true
	case kindUpdate:
		var rows []updateRunRow
		if !r.decode(&rows) || len(rows) == 0 {
			return nil, false
		}
		return updateRunOf(rows), true
	case kindPause, kindUnpause, kindPin, kindUnpin:
		return decodeSudo[control.HoldResult](func(v control.HoldResult) bool { return v.Stack != "" })(r)
	case kindResume:
		var w struct {
			Stack       string `json:"stack"`
			PausedSince string `json:"paused_since"`
			Run         string `json:"run"`
			Reason      string `json:"reason"`
		}
		if !r.decode(&w) || w.Stack == "" {
			return nil, false
		}
		res := control.ResumeResult{Stack: w.Stack}
		if w.PausedSince != "" {
			res.Pause = &control.PauseInfo{Run: w.Run, Reason: w.Reason}
			res.Pause.Since, _ = time.Parse(time.RFC3339, w.PausedSince)
		}
		return res, true
	case kindCheck:
		var rows []updateRow
		if !r.decode(&rows) {
			return nil, false
		}
		return updatesOf(rows), true
	}
	return decodeSudo[control.StackActionResult](func(v control.StackActionResult) bool { return v.Stack != "" })(r)
}

// updateRunRow is a row of `stack update --output json`.
type updateRunRow struct {
	Stack    string `json:"stack"`
	Variable string `json:"variable"`
	From     string `json:"from"`
	To       string `json:"to"`
	Run      string `json:"run"`
	Commit   string `json:"commit"`
	Result   string `json:"result"`
	Error    string `json:"error"`
}

// updateRunOf turns the rows of `stack update` back into what the service answered: one entry per stack, refused
// with its error, current without steps, or with one step per row.
func updateRunOf(rows []updateRunRow) control.UpdateRunResult {
	var res control.UpdateRunResult
	at := map[string]int{}
	for _, row := range rows {
		i, ok := at[row.Stack]
		if !ok {
			i = len(res.Stacks)
			at[row.Stack] = i
			res.Stacks = append(res.Stacks, control.StackUpdateRun{Name: row.Stack})
		}
		run := &res.Stacks[i]
		switch {
		case row.Variable == "" && row.Result == "refused":
			run.Error = row.Error
			if run.Error == "" {
				run.Error = row.Result
			}
		case row.Variable == "" && row.Result == "current":
		default:
			result := row.Result
			if result == "skipped" {
				result = ""
			}
			run.Steps = append(run.Steps, control.UpdateStepInfo{Variable: row.Variable, From: row.From, To: row.To,
				Run: row.Run, Commit: row.Commit, Result: result, Error: row.Error})
		}
	}
	return res
}

// updateRow is a row of `stack updates --output json`.
type updateRow struct {
	Stack          string `json:"stack"`
	Update         string `json:"update"`
	Variable       string `json:"variable"`
	Current        string `json:"current"`
	Available      string `json:"available"`
	MajorAvailable string `json:"major_available"`
	Status         string `json:"status"`
	Checked        string `json:"checked"`
	Error          string `json:"error"`
}

// updatesOf turns the rows of `stack updates` back into what the service answered.
func updatesOf(rows []updateRow) control.UpdatesResult {
	var res control.UpdatesResult
	at := map[string]int{}
	for _, row := range rows {
		i, ok := at[row.Stack]
		if !ok {
			i = len(res.Stacks)
			at[row.Stack] = i
			res.Stacks = append(res.Stacks, control.StackUpdates{Name: row.Stack, Update: row.Update})
		}
		su := &res.Stacks[i]
		if t, err := time.Parse(time.RFC3339, row.Checked); err == nil {
			su.Checked = t
		}
		switch row.Status {
		case "unreadable":
			su.Error = row.Error
		case "never-checked", "no-variables":
		default:
			su.Items = append(su.Items, control.UpdateInfo{Variable: row.Variable, Current: row.Current, Selected: row.Available,
				MajorAvailable: row.MajorAvailable, Status: row.Status, Error: row.Error})
		}
	}
	return res
}

// askSudoChangelog asks for the release notes the update preview shows; they come from GitHub, so the question says
// so before it shows the command. The preview opens with the notes once they are read.
func (m *Model) askSudoChangelog(md *modal, wanted []string) {
	t := m.text
	args := []string{"stack", "updates", "--changelog", "--output", "json"}
	if len(wanted) == 1 {
		args = m.engineArgs(md.plan.engineOf(wanted[0]), append(args, "--", wanted[0]))
	}
	m.askSudo(sudoAsk{title: textKeyUpdate, question: t.Lookup(textAskChangelog, m.subject(md.plan)),
		consequence: t.Lookup(textWhatChangelog), remote: t.Lookup(textSudoRemoteGitHub),
		lead: dialogLines(m.targetDetails(wanted)), args: args,
		done: func(r sudoRun) tea.Cmd {
			var rows []changelogRow
			if !r.decode(&rows) {
				m.failure = m.errorText(sudoProblem(r.problem))
				return nil
			}
			notes := changelogsOf(rows, m.checkedAt)
			for _, name := range wanted {
				e := &changelogEntry{done: true}
				if n, ok := notes[name]; ok {
					e.res, e.err = n.res, n.err
				}
				md.changelogs[name] = e
			}
			if m.setPreviewDialog(md) {
				m.modal = md
			}
			return nil
		}})
}

// changelogRow is a row of `stack updates --changelog --output json`.
type changelogRow struct {
	Stack     string `json:"stack"`
	Variable  string `json:"variable"`
	Current   string `json:"current"`
	Target    string `json:"target"`
	Status    string `json:"status"`
	Version   string `json:"version"`
	Published string `json:"published"`
	Name      string `json:"name"`
	URL       string `json:"url"`
	Source    string `json:"source"`
	Link      string `json:"link"`
	Reason    string `json:"reason"`
	Omitted   int    `json:"omitted"`
	Body      string `json:"body"`
}

// stackNotes is what the rows say of the changelog of one stack.
type stackNotes struct {
	res control.ChangelogResult
	err error
}

// changelogsOf turns the rows back into one answer per stack: a row that is a release joins the item of its image
// version variable, the others stand for an item without notes or for a stack that could not be read.
//
// The rows do not carry the time of the check. Every row but "never-checked" says the stack was checked, so the time
// comes from checked, the stored detection the interface holds.
func changelogsOf(rows []changelogRow, checked func(stack string) time.Time) map[string]stackNotes {
	out := map[string]stackNotes{}
	for _, row := range rows {
		n := out[row.Stack]
		n.res.Stack = row.Stack
		switch row.Status {
		case "unreadable":
			n.err = sudoProblem(row.Reason)
		case "never-checked":
		case "no-updates":
			n.res.Checked = checked(row.Stack)
		default:
			n.res.Checked = checked(row.Stack)
			i := slices.IndexFunc(n.res.Items, func(it control.ChangelogInfo) bool {
				return it.Variable == row.Variable && it.Target == row.Target
			})
			if i < 0 {
				i = len(n.res.Items)
				n.res.Items = append(n.res.Items, control.ChangelogInfo{Variable: row.Variable, Current: row.Current, Target: row.Target,
					Source: row.Source})
			}
			it := &n.res.Items[i]
			if row.Link != "" {
				it.Link = row.Link
			}
			if row.Reason != "" {
				it.Reason = row.Reason
			}
			if row.Omitted > 0 {
				it.Omitted = row.Omitted
			}
			if row.Status == "release" {
				rel := control.ChangelogRelease{Version: row.Version, Name: row.Name, URL: row.URL, Body: row.Body}
				rel.Published, _ = time.Parse(time.RFC3339, row.Published)
				it.Releases = append(it.Releases, rel)
			}
		}
		out[row.Stack] = n
	}
	return out
}

// checkedAt is when the stored detection last checked the stack for updates; when the interface holds none, the
// time of now, since the rows of a checked stack only exist after a check.
func (m *Model) checkedAt(stack string) time.Time {
	engine := m.stackEngineOf(stack)
	for _, su := range m.snap.updates.Stacks {
		if su.Name == stack && inEngine(su.Engine, engine) && !su.Checked.IsZero() {
			return su.Checked
		}
	}
	return time.Now()
}
