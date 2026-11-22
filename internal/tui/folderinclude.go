package tui

import (
	"slices"

	tea "charm.land/bubbletea/v2"

	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// formFolderInclude chooses which unclear files of a folder of files the first commit takes.
const formFolderInclude formKind = formProposalPassword + 1

// actChoose is the answer that opens the choice of unclear files.
const actChoose kittui.ActionID = "choose"

// unclear are the files of the plan a stack mounts writable or names as a secrets file, chosen or not, by path.
// The service decides which file is unclear; this only picks them out of its answer by reason.
func unclear(res control.SourceInspectResult) []control.SourceExcluded {
	var out []control.SourceExcluded
	for _, e := range res.Excluded {
		if control.SourceUnclear(e.Reason) {
			out = append(out, e)
		}
	}
	out = append(out, res.Included...)
	slices.SortFunc(out, func(a, b control.SourceExcluded) int {
		switch {
		case a.Path < b.Path:
			return -1
		case a.Path > b.Path:
			return 1
		}
		return 0
	})
	return out
}

// unclearLines lists the unclear files apart from the files that are left out: each with its reason and whether
// it is chosen. Nothing is chosen unless the person did.
func (m *Model) unclearLines(res control.SourceInspectResult, width int) []string {
	list := unclear(res)
	if len(list) == 0 {
		return nil
	}
	t := m.text
	chosen := map[string]bool{}
	for _, e := range res.Included {
		chosen[e.Path] = true
	}
	lines := []string{"", m.line(detailMargin, kittui.StyleActive, t.Lookup(textHeadUnclear, len(list)), width)}
	lines = append(lines, m.wrapped(detailIndent, kittui.StyleHint, t.Lookup(textUnclearHint), width)...)
	var entries []string
	for _, e := range list {
		id := textUnclearLeft
		if chosen[e.Path] {
			id = textUnclearChosen
		}
		entries = append(entries, t.Lookup(id, e.Path, e.Reason))
	}
	return append(lines, m.entryLines(entries, width)...)
}

// offerChoice adds the answer that chooses unclear files to the question just opened, when the plan has some.
func (m *Model) offerChoice(flow *addFlow) {
	if m.modal == nil || len(unclear(flow.res)) == 0 {
		return
	}
	m.modal.choose = func() tea.Cmd { return m.openForm(m.includeForm(flow)) }
}

// chooseAnswers puts the answer that chooses unclear files before the last one when the plan has some.
func chooseAnswers(flow *addFlow, answers []kittui.Answer) []kittui.Answer {
	if len(unclear(flow.res)) == 0 {
		return answers
	}
	last := len(answers) - 1
	return append(slices.Clone(answers[:last]), kittui.Answer{Action: actChoose, Label: textAnswerChoose}, answers[last])
}

// includeForm asks which unclear files to take, with the chosen ones ticked.
func (m *Model) includeForm(flow *addFlow) *settingsForm {
	field := kittui.FormField{ID: "include", Kind: kittui.FieldMultiple, Label: textFieldInclude, Hint: textHintInclude}
	for _, e := range unclear(flow.res) {
		field.Choices = append(field.Choices, kittui.Choice{ID: kittui.ChoiceID(e.Path),
			Label: kittui.TextID(kittui.Escape(m.text.Lookup(textUnclearForm, e.Path, e.Reason)))})
	}
	for _, e := range flow.res.Included {
		field.Value.Selected = append(field.Value.Selected, kittui.ChoiceID(e.Path))
	}
	return m.newSettingsForm(formFolderInclude, textTitleInclude, []kittui.FormField{field})
}

// submitFolderInclude plans the folder again with the choice. The host and the service change nothing here; the
// flow goes on with the new plan, so the list shown, the effect and the command that applies it match the choice.
func (m *Model) submitFolderInclude(v map[kittui.FieldID]kittui.FormValue) tea.Cmd {
	flow := m.set.add
	if flow == nil || flow.res.Case != control.SourceCaseFiles {
		m.dropForm()
		return nil
	}
	var include []string
	for _, id := range v["include"].Selected {
		include = append(include, string(id))
	}
	slices.Sort(include)
	return m.replan(flow, include)
}

// replan inspects the folder of flow again with the choice of unclear files and keeps what the flow belongs to:
// the foreign project or the proposal.
func (m *Model) replan(flow *addFlow, include []string) tea.Cmd {
	path := flow.res.Path
	if m.euid() != 0 {
		return m.inspectSudoFor(path, flow.foreign, flow.proposal, include, flow.engine)
	}
	ctx, host, foreign, shown, engineSet := m.engineCtx(flow.engine), m.folders, flow.foreign, flow.proposal, flow.engineSet
	if host == nil {
		return nil
	}
	return m.writeCall(opInspect, path, func(b Backend) (any, error) {
		if shown != nil {
			// The proposal is read again with the choice; only its effect may differ from what was shown.
			now := loadProposal(ctx, b, host, shown.gen, shown.id, true, include)
			if now.err != nil {
				return nil, now.err
			}
			if now.res.Fingerprint != shown.fingerprint || now.res.State != control.ProposalOpen || now.bad ||
				now.effectErr != nil || now.flow == nil {
				return proposalRefused{now}, nil
			}
			now.flow.engineSet = true
			replanned := *shown
			replanned.effect = effectSignature(now.flow)
			now.flow.proposal = &replanned
			return inspected{now.flow}, nil
		}
		got, err := inspectFolder(ctx, b, host, path, include)
		if err != nil {
			return nil, err
		}
		if in, ok := got.(inspected); ok {
			in.flow.foreign, in.flow.engineSet = foreign, engineSet || in.flow.engine != ""
		}
		return got, nil
	})
}

// includeArgs are the options that send the choice of unclear files to a command.
func includeArgs(include []string) []string {
	var args []string
	for _, p := range include {
		args = append(args, "--include", p)
	}
	return args
}
