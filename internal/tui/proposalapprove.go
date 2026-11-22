package tui

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// formProposalReject asks for the optional reason of rejecting a proposal.
const formProposalReject formKind = formCheck + 1

// formProposalSecrets asks for the secret parts of the channel a proposal names.
const formProposalSecrets = formProposalReject + 1

// maxRejectReason is how long the service keeps the reason of a rejection.
const maxRejectReason = 300

// proposalCtx is what travels with the flow of adding a folder that a proposal started: the proposal as the
// person was shown it and the effect that was shown with it.
type proposalCtx struct {
	id string
	// fingerprint is the one of the proposal as shown; the service takes the approval only for it.
	fingerprint string
	// effect is the signature of the effect as shown.
	effect string
	// url is the address the proposal carries; the address step starts with it and the person may change it.
	url string
	// gen is the generation of the view the proposal was opened in.
	gen int
}

// proposalRefused is the answer when the proposal or its effect differs from what was shown: nothing was
// applied, and msg is the proposal as it reads now.
type proposalRefused struct{ msg proposalMsg }

// proposalDone is the answer of approving: what applying the change came to, and whether keeping the result on
// the proposal failed. A refused approval is an err with no res.
type proposalDone struct {
	res       any
	err       error
	resultErr error
}

// effectSignature is what the person is shown of the effect: the case, the planned files, what is left out, the
// containers that would collide and the plan of permissions. Two flows with the same signature show the same.
func effectSignature(flow *addFlow) string {
	sorted := func(list []string) []string {
		out := slices.Clone(list)
		slices.Sort(out)
		return out
	}
	var left, chosen, conflicts, notChecked []string
	for _, e := range flow.res.Excluded {
		left = append(left, e.Path+"\x00"+e.Reason+"\x00"+e.Stack)
	}
	for _, e := range flow.res.Included {
		chosen = append(chosen, e.Path+"\x00"+e.Reason)
	}
	for _, st := range flow.res.Stacks {
		for _, c := range st.Conflicts {
			conflicts = append(conflicts, st.Name+"\x00"+c.Container+"\x00"+c.Project+"\x00"+engineName(c.Engine))
		}
	}
	for _, nc := range flow.res.EnginesNotChecked {
		notChecked = append(notChecked, nc.Engine+"\x00"+nc.Reason)
	}
	raw, _ := json.Marshal(struct {
		Case       string
		Reason     string
		ReadOnly   bool
		Missing    bool
		Files      []string
		Excluded   []string
		Included   []string
		Conflicts  []string
		NotChecked []string
		Changed    int
		Plan       []string
	}{flow.res.Case, flow.res.Reason, flow.readOnly, flow.missing, sorted(flow.res.PlannedFiles), sorted(left), sorted(chosen),
		sorted(conflicts), sorted(notChecked), flow.plan.Changed, flow.plan.Summary})
	return string(raw)
}

// proposalChanged says the proposal read now is not the one shown: another fingerprint, no longer open, or an
// effect that differs.
func proposalChanged(now proposalMsg, shown *proposalCtx) bool {
	sig := ""
	switch {
	case now.flow != nil:
		sig = effectSignature(now.flow)
	case now.kind != nil:
		sig = now.kind.sig
	}
	return now.res.Fingerprint != shown.fingerprint || now.res.State != control.ProposalOpen || now.bad ||
		now.effectErr != nil || sig == "" || sig != shown.effect
}

// canApprove says the open proposal can be approved here: an open proposal of the kind this version applies,
// content it can read and an effect it worked out. Without root the effect of a folder to add is worked out
// when approving starts, through sudo, so the folder needs none yet.
func (m *Model) canApprove() bool {
	v := &m.pv
	if !(v.open && v.loaded && v.info.State == control.ProposalOpen && !v.contentBad && v.effectErr == nil) {
		return false
	}
	if isKindProposal(v.info.Kind) {
		return v.kind != nil && v.kind.worked
	}
	if v.info.Kind != control.ProposalKindSourceAdd {
		return false
	}
	if m.euid() != 0 {
		return v.needsRoot
	}
	return m.folders != nil && v.flow != nil
}

// canReject says the open proposal can be rejected: as long as it is open. As root it needs the host side as well.
func (m *Model) canReject() bool {
	v := &m.pv
	return (m.euid() != 0 || m.folders != nil) && v.open && v.loaded && v.info.State == control.ProposalOpen
}

// proposalHints are the keys of the open proposal that decide it.
func (m *Model) proposalHints() []kittui.Hint {
	var hints []kittui.Hint
	if m.canApprove() {
		hints = append(hints, kittui.Hint{Action: actApprove, Label: textKeyApprove, Priority: 3})
	}
	if m.canReject() {
		hints = append(hints, kittui.Hint{Action: actReject, Label: textKeyReject, Priority: 3})
	}
	return hints
}

// proposalAction takes the keys that decide the proposal shown; handled is false for any other.
func (m *Model) proposalAction(id kittui.ActionID) (cmd tea.Cmd, handled bool) {
	if id != actApprove && id != actReject {
		return nil, false
	}
	switch {
	case m.settingsBusy():
		m.warn(m.text.Lookup(textActionBusy))
	case !m.pv.loaded || m.pv.info.State != control.ProposalOpen:
		m.warn(m.text.Lookup(textPropNotOpen))
	case m.euid() == 0 && m.folders == nil && !isKindProposal(m.pv.info.Kind):
		m.warn(m.text.Lookup(textPropCannotApprove))
	case id == actReject:
		return m.openForm(m.proposalRejectForm()), true
	case !m.canApprove():
		m.warn(m.text.Lookup(textPropCannotApprove))
	default:
		return m.approveProposal(), true
	}
	return nil, true
}

// approveProposal starts adding the folder of the proposal the way adding it by hand starts, with the path of
// the proposal. The proposal is read and its effect worked out again first; nothing is changed.
func (m *Model) approveProposal() tea.Cmd {
	v := &m.pv
	if isKindProposal(v.info.Kind) {
		return m.approveKind()
	}
	if m.euid() != 0 {
		shown := &proposalCtx{id: v.id, fingerprint: v.info.Fingerprint, url: v.content.URL, gen: v.gen}
		return m.inspectSudoFor(v.content.Path, nil, shown, nil)
	}
	shown := &proposalCtx{id: v.id, fingerprint: v.info.Fingerprint, effect: effectSignature(v.flow), url: v.content.URL, gen: v.gen}
	ctx, host, path := m.ctx, m.folders, v.content.Path
	return m.writeCall(opInspect, path, func(b Backend) (any, error) {
		now := loadProposal(ctx, b, host, shown.gen, shown.id, true, nil)
		if now.err != nil {
			return nil, now.err
		}
		if proposalChanged(now, shown) {
			return proposalRefused{now}, nil
		}
		now.flow.proposal = shown
		return inspected{now.flow}, nil
	})
}

// approveAndApply is the last step of approving, in one background operation: the proposal is read again and its
// effect worked out again, both compared with what was shown; the service takes the approval for the shown
// fingerprint, which locks the proposal; then the change is made as for a folder added by hand and its result
// is kept on the proposal. A difference or a refusal before the change leaves everything as it was.
func approveAndApply(ctx context.Context, b Backend, host Folders, flow *addFlow) (any, error) {
	p := flow.proposal
	now := loadProposal(ctx, b, host, p.gen, p.id, true, flow.include)
	if now.err != nil {
		return nil, now.err
	}
	if proposalChanged(now, p) {
		return proposalRefused{now}, nil
	}
	if _, err := b.ProposalApprove(ctx, control.ProposalApproveArgs{ID: p.id, Fingerprint: p.fingerprint}); err != nil {
		return proposalDone{err: err}, nil
	}
	res, err := applyFolder(ctx, b, host, flow)
	if _, changed := res.(listChanged); changed {
		res, err = nil, errors.New("the files of the folder changed while it was approved")
	}
	result := control.ProposalResultArgs{ID: p.id, Fingerprint: p.fingerprint, Applied: err == nil}
	if err != nil {
		result.Message = failureMessage(err)
	}
	_, resultErr := b.ProposalResult(ctx, result)
	return proposalDone{res: res, err: err, resultErr: resultErr}, nil
}

// failureMessage is the reason of a failed change as the submitter may read it, in the words of the service or
// the host and never in the language of this screen.
func failureMessage(err error) string {
	var remote *control.Error
	if errors.As(err, &remote) {
		return remote.Message
	}
	return err.Error()
}

// proposalDecided shows the proposal as it is after approving or rejecting it, and says when its result could
// not be kept on it.
func (m *Model) proposalDecided(d *proposalDone) tea.Cmd {
	if d == nil {
		return nil
	}
	m.set.add = nil
	if d.resultErr != nil {
		m.failure = m.text.Lookup(kittui.TextError, m.text.Lookup(textPropResultLost, m.serviceText(d.resultErr)))
	}
	m.keep = m.failure
	return tea.Batch(m.reloadNow(), m.readProposal())
}

func (m *Model) proposalRejectForm() *settingsForm {
	return m.newSettingsForm(formProposalReject, textTitleProposalReject, []kittui.FormField{
		textField("reason", textFieldPropReason, textHintPropReason, ""),
	})
}

// submitProposalReject checks the reason and asks before the proposal is closed.
func (m *Model) submitProposalReject(f *settingsForm, reason string) tea.Cmd {
	if utf8.RuneCountInString(reason) > maxRejectReason || strings.IndexFunc(reason, unicode.IsControl) >= 0 || !utf8.ValidString(reason) {
		f.problems(m.bad("reason", textProblemPropReason))
		return nil
	}
	id := m.pv.id
	label := kittui.Escape(proposalShort(id))
	if m.euid() != 0 {
		m.askProposalRejectSudo(id, reason)
		return nil
	}
	m.ask(textTitleProposalReject, m.text.Lookup(textAskProposalReject, label), m.text.Lookup(textWhatProposalReject), nil, true, func() tea.Cmd {
		return m.writeCall(opProposalReject, id, func(b Backend) (any, error) {
			return b.ProposalReject(m.ctx, control.ProposalRejectArgs{ID: id, Reason: reason})
		})
	})
	return nil
}

// finalQuestion is the question of the last step of a flow: that of the folder, or for a proposal the approval.
func (m *Model) finalQuestion(flow *addFlow, question string) string {
	if p := flow.proposal; p != nil {
		return m.text.Lookup(textAskProposalApprove, kittui.Escape(proposalShort(p.id)), flow.res.Path)
	}
	return question
}

// finalConsequence is the sentence on what follows the last question; for a proposal it also names the check.
func (m *Model) finalConsequence(flow *addFlow, folder kittui.TextID) string {
	what := m.text.Lookup(folder)
	if flow.proposal != nil {
		what = m.text.Lookup(textWhatProposalApprove)
	}
	if hasConflicts(flow.res) {
		what += " " + m.text.Lookup(textConflictConsequence)
	}
	return what
}
