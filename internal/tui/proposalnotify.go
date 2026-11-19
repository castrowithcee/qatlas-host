package tui

import (
	"context"
	"errors"
	"strings"

	tea "charm.land/bubbletea/v2"

	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// proposalSecretsForm asks for the secret parts of the channel of a proposal, with the fields and the type's
// order of the form that adds a channel by hand. An SMTP channel has a password only with a user. Without a field
// to fill, it asks for the approval right away.
func (m *Model) proposalSecretsForm(eff *kindEffect, shown *proposalCtx) *settingsForm {
	var fields []kittui.FormField
	if eff.channel.Type != control.NotifySMTP || eff.channel.User != "" {
		fields = notifySecretFields(eff.channel.Type)
	}
	if len(fields) == 0 {
		m.askProposalChannel(eff, shown, notifyAddFromProposal(eff.channel))
		return nil
	}
	f := m.newSettingsForm(formProposalSecrets, textTitleProposalSecrets, fields)
	if f != nil {
		f.eff, f.shown = eff, shown
	}
	return f
}

// notifyAddFromProposal is the call that adds the channel of a proposal, before the secret parts are set.
func notifyAddFromProposal(c control.ProposalNotifyChannel) control.NotifyAddArgs {
	return control.NotifyAddArgs{Name: c.Name, Type: c.Type, Events: c.Events, Host: c.Host, Port: c.Port,
		Security: c.Security, User: c.User, From: c.From, To: c.To, ChatID: c.ChatID, BaseURL: c.BaseURL}
}

// submitProposalSecrets checks the secret parts with the rule of the form that adds a channel and asks for the
// approval. The parts live only in the call that the answer starts.
func (m *Model) submitProposalSecrets(f *settingsForm, v map[kittui.FieldID]kittui.FormValue) tea.Cmd {
	add := notifyAddFromProposal(f.eff.channel)
	var ps []kittui.FormProblem
	for _, field := range notifySecretFields(add.Type) {
		s := v[field.ID].Text
		if !validCredentialValue(s) {
			ps = append(ps, m.bad(string(field.ID), textProblemSecret))
		}
		if field.ID == "key" {
			add.Key = s
		} else {
			add.Secret = s
		}
	}
	if len(ps) > 0 {
		f.problems(ps...)
		return nil
	}
	m.askProposalChannel(f.eff, f.shown, add)
	return nil
}

func (m *Model) askProposalChannel(eff *kindEffect, shown *proposalCtx, add control.NotifyAddArgs) {
	t := m.text
	body := func(width int) []string { return m.kindEffectLines(eff, width) }
	m.askSheet(textTitleProposalApprove,
		t.Lookup(textAskProposalChannel, kittui.Escape(proposalShort(shown.id)), eff.channel.Name, kittui.Escape(eff.channel.Type)),
		t.Lookup(textWhatProposalChannel), body,
		[]kittui.Answer{{Action: actYes, Label: textAnswerYes}, {Action: actNo, Label: textAnswerCancel}},
		func() tea.Cmd {
			ctx := m.ctx
			return m.writeCall(opProposalApply, shown.id, func(b Backend) (any, error) {
				return approveChannelAndAdd(ctx, b, shown, eff, add)
			})
		})
}

// approveChannelAndAdd is the last step of approving a channel, in one background operation: the proposal and
// the channels are read again and compared with what was shown, which also refuses a channel of the name that
// appeared; then the approval is taken, the channel is added with the secret parts the person entered, and the
// result is kept on the proposal. Neither an error nor the message kept on the proposal contains a secret part.
func approveChannelAndAdd(ctx context.Context, b Backend, p *proposalCtx, eff *kindEffect, add control.NotifyAddArgs) (any, error) {
	now := loadProposal(ctx, b, nil, p.gen, p.id, true)
	if now.err != nil {
		return nil, now.err
	}
	if proposalChanged(now, p) {
		return proposalRefused{now}, nil
	}
	if _, err := b.ProposalApprove(ctx, control.ProposalApproveArgs{ID: p.id, Fingerprint: p.fingerprint}); err != nil {
		return proposalDone{err: err}, nil
	}
	applied := kindApplied{eff: eff}
	var err error
	applied.channel, err = b.NotifyAdd(ctx, add)
	result := control.ProposalResultArgs{ID: p.id, Fingerprint: p.fingerprint, Applied: err == nil}
	var res any = applied
	if err != nil {
		err = scrubbed(err, add.Secret, add.Key)
		result.Message, res = failureMessage(err), nil
	}
	_, resultErr := b.ProposalResult(ctx, result)
	return proposalDone{res: res, err: err, resultErr: resultErr}, nil
}

// scrubbed is err with every secret part replaced where its text names one, keeping the code of a service error.
func scrubbed(err error, secrets ...string) error {
	text := failureMessage(err)
	for _, s := range secrets {
		if s != "" {
			text = strings.ReplaceAll(text, s, "***")
		}
	}
	var remote *control.Error
	if errors.As(err, &remote) {
		return control.NewError(remote.Code, text)
	}
	return errors.New(text)
}
