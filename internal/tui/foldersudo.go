package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// Adding a stack folder, adopting a foreign project and adding a remote go the same steps without root, through
// the public commands run as `sudo qhost …`. The inspection only reads and runs at once, sudo asks for the password
// itself. The host key and the comparison ask the git server, so each of them is asked first with its command. The
// last question shows the exact command that applies what was shown: it takes along the confirmed host key, the
// effect of the inspection and the state of the comparison, and refuses, changing nothing, when they differ by then.

// codeStateChanged is the code of a command that refused because the folder or its remote is no longer what was
// shown; it changed nothing.
const codeStateChanged = "state-changed"

// stateChanged is the answer of a last command that refused with codeStateChanged.
type stateChanged struct{ flow *addFlow }

// sudoInspection is what `source inspect` prints as root: the answer of the service, and what adding would do to
// the folder.
type sudoInspection struct {
	control.SourceInspectResult
	Missing     bool `json:"missing"`
	Permissions *struct {
		Changed int      `json:"changed"`
		Summary []string `json:"summary"`
	} `json:"permissions"`
	Effect string `json:"effect"`
}

// noPermissions says adding the folder changes no permission. A plan read through sudo has no handle, so its lines
// count as well.
func (f *addFlow) noPermissions() bool {
	return f.plan.Empty() && f.plan.Changed == 0 && len(f.plan.Summary) == 0
}

// inspectSudo inspects the folder with `sudo qhost source inspect` and goes on like the inspection as root. It only
// reads, so it asks nothing first. foreign is the project the folder belongs to when it is adopted.
func (m *Model) inspectSudo(path string, foreign *foreignSource) tea.Cmd {
	return m.inspectSudoFor(path, foreign, nil)
}

// inspectSudoFor is inspectSudo for a folder a proposal names: the flow then ends in approving the proposal.
func (m *Model) inspectSudoFor(path string, foreign *foreignSource, proposal *proposalCtx) tea.Cmd {
	gen := m.beginSetting()
	args := []string{"source", "inspect", "--output", "json", "--", path}
	return m.runSudo(args, func(r sudoRun) tea.Cmd {
		msg := settingMsg{gen: gen, op: opInspect, name: path}
		var got sudoInspection
		// A folder that can be taken over always has an effect when root inspected it.
		if !r.decode(&got) || got.Path == "" || got.Case == "" || (got.Case != control.SourceCaseReject && got.Effect == "") {
			msg.err = sudoProblem(r.problem)
			return m.applySetting(msg)
		}
		flow := &addFlow{res: got.SourceInspectResult, effect: got.Effect, foreign: foreign,
			missing: got.Missing && got.Case == control.SourceCaseClone, proposal: proposal}
		if proposal != nil {
			flow.proposal.effect = got.Effect
		}
		if p := got.Permissions; p != nil {
			flow.plan = PermPlan{Changed: p.Changed, Summary: p.Summary}
		}
		msg.res = inspected{flow}
		return m.applySetting(msg)
	})
}

// askProbeSudo asks to read the host key of the server of the address with `sudo qhost repo probe`.
func (m *Model) askProbeSudo(flow *addFlow) {
	t, url := m.text, flow.url
	m.askSetting(settingChange{title: flow.title(), question: t.Lookup(textAskProbe, url), remote: t.Lookup(textSudoRemoteGit),
		consequence: t.Lookup(textWhatProbe), op: opProbe,
		direct: func(b Backend) (any, error) { return b.RepoProbe(m.ctx, url) },
		args:   []string{"repo", "probe", "--output", "json", "--", url},
		decode: decodeSudo[control.RepoProbeResult](func(r control.RepoProbeResult) bool { return r.Fingerprint != "" })})
}

// askCompareSudo asks to compare the folder with the repository at the address with `sudo qhost source compare`,
// with the host key confirmed before.
func (m *Model) askCompareSudo(flow *addFlow) {
	t, path, url, fp := m.text, flow.res.Path, flow.url, flow.probe.Fingerprint
	m.askSetting(settingChange{title: flow.title(), question: t.Lookup(textAskCompare, path, url), remote: t.Lookup(textSudoRemoteGit),
		consequence: t.Lookup(textWhatCompare), op: opCompare, name: path,
		direct: func(b Backend) (any, error) {
			return b.SourceCompare(m.ctx, control.SourceCompareArgs{Path: path, URL: url, Fingerprint: fp})
		},
		args:   []string{"source", "compare", "--fingerprint", fp, "--output", "json", "--", path, url},
		decode: decodeSudo[control.SourceCompareResult](func(r control.SourceCompareResult) bool { return r.State != "" })})
}

// sudoArgs are the arguments after qhost of the command that applies what the flow showed.
func (f *addFlow) sudoArgs() []string {
	if f.proposal != nil {
		return f.proposalArgs()
	}
	path, end := f.res.Path, []string{"--yes", "--output", "json", "--"}
	if f.remote {
		args := append([]string{"source", "remote", "--fingerprint", f.probe.Fingerprint}, f.expectArgs()...)
		return append(append(args, end...), path, f.url)
	}
	args := []string{"source", "add"}
	switch f.res.Case {
	case control.SourceCaseClone, control.SourceCaseAdopt:
		args = append(args, "--url", f.url, "--fingerprint", f.probe.Fingerprint, "--expect-effect", f.effect)
	case control.SourceCaseFiles:
		if f.url != "" {
			args = append(args, "--url", f.url, "--fingerprint", f.probe.Fingerprint, "--expect-effect", f.effect)
			args = append(args, f.expectArgs()...)
			break
		}
		args = append(args, "--expect-effect", f.effect)
	default:
		args = append(args, "--expect-effect", f.effect)
	}
	return append(append(args, end...), path)
}

// expectArgs are the state and the commit of the comparison that was shown; the commit is left out when the
// remote has none.
func (f *addFlow) expectArgs() []string {
	if f.cmp == nil {
		return nil
	}
	args := []string{"--expect-state", f.cmp.State}
	if f.cmp.Commit != "" {
		args = append(args, "--expect-commit", f.cmp.Commit)
	}
	return args
}

// sudoLines are the command of the last question, dimmed as a question with commands shows them; none as root. The
// command breaks only between its arguments, so every option stays whole, and an argument longer than the line
// breaks where it must.
func (m *Model) sudoLines(flow *addFlow, width int) []string {
	if m.euid() == 0 {
		return nil
	}
	return m.commandLines(flow.sudoArgs(), width)
}

// commandLines is the command sudo runs for args, dimmed, below an empty line.
func (m *Model) commandLines(args []string, width int) []string {
	const prompt = "$ "
	room := max(width-detailIndent-len(prompt), 1)
	var rows []string
	row := ""
	for _, arg := range strings.Split(kittui.Escape(sudoCommand(args)), " ") {
		switch {
		case row == "":
			row = arg
		case ansi.StringWidth(row)+1+ansi.StringWidth(arg) <= room:
			row += " " + arg
		default:
			rows = append(rows, row)
			row = arg
		}
		for ansi.StringWidth(row) > room {
			rows = append(rows, ansi.Truncate(row, room, ""))
			row = ansi.TruncateLeft(row, room, "")
		}
	}
	rows = append(rows, row)
	lines := []string{""}
	for i, r := range rows {
		lead := strings.Repeat(" ", len(prompt))
		if i == 0 {
			lead = prompt
		}
		lines = append(lines, strings.Repeat(" ", detailIndent)+m.styles.Render(kittui.StyleHint, lead+r))
	}
	return lines
}

// lastStep is what yes to the last question does: as root the step itself, otherwise the command through sudo.
func (m *Model) lastStep(flow *addFlow, root func() tea.Cmd) func() tea.Cmd {
	if m.euid() == 0 {
		return root
	}
	return func() tea.Cmd { return m.applySudo(flow) }
}

// applySudo runs the command of the last question and reads its answer as the step as root would give it.
func (m *Model) applySudo(flow *addFlow) tea.Cmd {
	op, path := opFolderAdd, flow.res.Path
	if flow.remote {
		op = opRemote
	}
	gen := m.beginSetting()
	return m.runSudo(flow.sudoArgs(), func(r sudoRun) tea.Cmd {
		msg := settingMsg{gen: gen, op: op, name: path}
		res, ok := decodeApplied(flow, r)
		switch {
		case ok && flow.proposal != nil:
			msg.res = proposalDone{res: res}
		case ok:
			msg.res = res
		case r.code == codeStateChanged:
			msg.res = stateChanged{flow}
		case flow.proposal != nil:
			// The proposal may be locked or failed by now, so the view reads it again.
			msg.res = proposalDone{err: sudoProblem(r.problem)}
		default:
			msg.err = sudoProblem(r.problem)
		}
		return m.applySetting(msg)
	})
}

// decodeApplied reads what the last command printed: source remote its result, source add for a clone the fields
// of repo set, and for a local folder or a folder of files the added source.
func decodeApplied(flow *addFlow, r sudoRun) (any, bool) {
	switch {
	case flow.remote:
		return decodeSudo[control.SourceRemoteResult](func(v control.SourceRemoteResult) bool { return v.Path != "" })(r)
	case flow.res.Case == control.SourceCaseClone || flow.res.Case == control.SourceCaseAdopt:
		var v struct {
			URL         string `json:"url"`
			Dir         string `json:"dir"`
			Fingerprint string `json:"host_key_fingerprint"`
			Head        string `json:"head"`
			Cloned      bool   `json:"cloned"`
		}
		if !r.decode(&v) || v.Dir == "" {
			return nil, false
		}
		return control.RepoSetResult{URL: v.URL, Dir: v.Dir, Fingerprint: v.Fingerprint, Head: v.Head, Cloned: v.Cloned}, true
	}
	return decodeSudo[control.SourceAddResult](func(v control.SourceAddResult) bool { return v.Path != "" })(r)
}

// stateChangedDone says that the command refused because the folder or its remote changed since it was shown,
// that nothing changed, and how to see the current state. The flow ends here.
func (m *Model) stateChangedDone(res stateChanged) {
	t, flow := m.text, res.flow
	text := t.Lookup(textStateChangedFolder, flow.res.Path)
	if flow.cmp != nil {
		text = t.Lookup(textStateChangedRemote, flow.res.Path)
	}
	next := textAgainAdd
	if flow.proposal != nil {
		text, next = t.Lookup(textStateChangedProposal, flow.res.Path), textAgainProposal
	}
	switch {
	case flow.remote:
		next = textAgainRemote
	case flow.proposal != nil:
	case flow.foreign != nil:
		next = textAgainAdopt
	}
	m.tell(flow.title(), text, []string{t.Lookup(next)})
}

// proposalArgs are the arguments after qhost of the command that approves the proposal of the flow bound to what
// was shown: the proposal as it was read, the address and the host key, the effect of the inspection and the state
// of the comparison.
func (f *addFlow) proposalArgs() []string {
	p := f.proposal
	args := []string{"proposal", "approve", p.id, "--fingerprint", p.fingerprint, "--confirm"}
	if f.url != "" && f.res.Case != control.SourceCaseLocal {
		args = append(args, "--url", f.url, "--host-key-fingerprint", f.probe.Fingerprint)
	}
	args = append(args, "--expect-effect", f.effect)
	if f.url != "" && f.res.Case == control.SourceCaseFiles {
		args = append(args, f.expectArgs()...)
	}
	return append(args, "--output", "json")
}
