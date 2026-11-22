package tui

import (
	"context"
	"encoding/json"

	tea "charm.land/bubbletea/v2"

	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// Without root a proposal is decided through the public commands run as `sudo qhost proposal …`. Approving one is
// bound to the fingerprint of the proposal as it was shown, and --confirm takes the place of the question the
// command would ask: the question of this interface is the one answered. A folder to add takes its own steps
// (foldersudo.go) and ends in the same command.

// proposalApproveArgs are the arguments after qhost that approve the proposal as it was shown, for a kind that
// takes nothing else.
func proposalApproveArgs(p *proposalCtx) []string {
	return []string{"proposal", "approve", p.id, "--fingerprint", p.fingerprint, "--confirm", "--output", "json"}
}

// askProposalRejectSudo asks to reject the proposal with the reason, which may be empty; y runs the command
// through sudo.
func (m *Model) askProposalRejectSudo(id, reason, engine string) {
	args := []string{"proposal", "reject", id}
	if reason != "" {
		args = append(args, "--reason", reason)
	}
	args = m.engineArgs(engine, append(args, "--output", "json"))
	var gen int
	m.askSudo(sudoAsk{title: textTitleProposalReject,
		question:    m.text.Lookup(textAskProposalReject, kittui.Escape(proposalShort(id))),
		consequence: m.text.Lookup(textWhatProposalReject), danger: true, args: args,
		begin: func() { gen = m.beginSetting() },
		done: func(r sudoRun) tea.Cmd {
			msg := settingMsg{gen: gen, op: opProposalReject, name: id}
			if res, ok := decodeSudo[control.ProposalInfo](func(p control.ProposalInfo) bool { return p.ID != "" })(r); ok {
				msg.res = res
			} else {
				msg.err = sudoProblem(r.problem)
			}
			return m.applySetting(msg)
		}})
}

// askKindSudo asks to approve a proposal of the kind stack-settings, window or notify-channel with the effect
// shown; y runs the command through sudo. input, where set, is the standard input of the command, and secrets are
// the texts that no message may show.
func (m *Model) askKindSudo(eff *kindEffect, shown *proposalCtx, question string, consequence kittui.TextID,
	input func() []byte, secrets ...string) {
	var gen int
	args := m.engineArgs(shown.engine, proposalApproveArgs(shown))
	if eff.kind == control.ProposalKindWindow && hostWindowEngine(shown.engine) {
		// Without --engine the command sets the host window; with "system" it would set the own window of that service.
		args = proposalApproveArgs(shown)
	}
	a := sudoAsk{title: textTitleProposalApprove, question: question, consequence: m.text.Lookup(consequence),
		body:  func(width int) []string { return m.kindEffectLines(eff, width) },
		args:  args,
		begin: func() { gen = m.beginSetting() },
		done: func(r sudoRun) tea.Cmd {
			msg := settingMsg{gen: gen, op: opProposalApply, name: shown.id}
			if res, ok := decodeKindApplied(eff, r); ok {
				msg.res = proposalDone{res: res}
			} else {
				// The proposal may be locked or failed by now, so the view reads it again.
				msg.res = proposalDone{err: sudoProblem(scrubSecrets(r.problem, secrets...))}
			}
			return m.applySetting(msg)
		}}
	if input != nil {
		a.input = func(context.Context, Backend) ([]byte, error) { return input(), nil }
	}
	m.askSudo(a)
}

// scrubSecrets takes every secret out of text, as typed and as JSON writes it.
func scrubSecrets(text string, secrets ...string) string {
	for _, s := range secrets {
		if s == "" {
			continue
		}
		quoted, _ := json.Marshal(s)
		text = scrub(text, []byte(s), quoted[1:len(quoted)-1])
	}
	return text
}

// decodeKindApplied reads what `proposal approve` prints for the kind of eff.
func decodeKindApplied(eff *kindEffect, r sudoRun) (kindApplied, bool) {
	applied := kindApplied{eff: eff}
	switch eff.kind {
	case control.ProposalKindStackSettings:
		var v struct {
			Stack   string `json:"stack"`
			Changed bool   `json:"changed"`
			Commit  string `json:"commit"`
		}
		if !r.decode(&v) || v.Stack == "" {
			return applied, false
		}
		applied.settings = control.StackSettingsSetResult{Stack: v.Stack, Changed: v.Changed, Commit: v.Commit}
	case control.ProposalKindNotifyChannel:
		res, ok := decodeNotifyAdd(r)
		if !ok {
			return applied, false
		}
		applied.channel = res.(control.NotifyChannelInfo)
	default:
		res, ok := decodeWindow(r)
		if !ok {
			return applied, false
		}
		applied.window = res.(control.WindowResult)
	}
	return applied, true
}

// channelSecretsInput is what `proposal approve` reads on standard input for a channel: the secret parts as JSON,
// in the order the type asks for them. It is empty for a type without a secret part.
func channelSecretsInput(add control.NotifyAddArgs) []byte {
	parts := map[string]string{}
	switch add.Type {
	case control.NotifyWebhook:
		parts["secret"], parts["key"] = add.Secret, add.Key
	case control.NotifySlack, control.NotifyTelegram:
		parts["secret"] = add.Secret
	case control.NotifySMTP:
		if add.User != "" {
			parts["secret"] = add.Secret
		}
	}
	if len(parts) == 0 {
		return nil
	}
	raw, _ := json.Marshal(parts)
	return raw
}
