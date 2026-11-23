package tui

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// formGrantExpiry asks for the expiry date of a grant the person is about to approve.
const formGrantExpiry formKind = formAdoptTarget + 1

// setUp is what is known of a part of a stack: a stack that cannot be read has neither answer.
type setUp int

const (
	setUpUnknown setUp = iota
	setUpNo
	setUpYes
)

// grantResult is what `proposal approve` prints for a grant.
type grantResult struct {
	ID       string `json:"id"`
	User     string `json:"user"`
	Stack    string `json:"stack"`
	Expires  string `json:"expires"`
	Replaced bool   `json:"replaced"`
}

// parseGrant reads the content of a proposal of the kind grant; ok is false for content this version does not
// show: another shape, a stack name or an action the service would not accept.
func parseGrant(raw json.RawMessage) (g control.ProposalGrant, ok bool) {
	if json.Unmarshal(raw, &g) != nil {
		return g, false
	}
	return g, stackNameRe.MatchString(g.Stack) && slices.Contains(control.GrantActions, g.Action)
}

// grantKindEffect reads backup and check of the stack the way `proposal show` does. It only reads. A stack that
// cannot be read for another reason than that it does not exist shows as unknown and does not stop the approval:
// the command checks the stack again.
func grantKindEffect(ctx context.Context, b Backend, eff *kindEffect) error {
	res, err := b.StackSettings(ctx, eff.stack)
	if err != nil {
		var remote *control.Error
		if errors.As(err, &remote) && remote.Code == control.CodeNoSuchStack {
			eff.noStack = true
			return err
		}
		return nil
	}
	eff.backup, eff.check = setUpNo, setUpNo
	if s := res.Settings; s != nil {
		eff.backup = boolSetUp(s.Backup != nil)
		eff.check = boolSetUp(s.Check != nil && (s.Check.Containers != nil && *s.Check.Containers || s.Check.HTTP != nil))
	}
	return nil
}

func boolSetUp(on bool) setUp {
	if on {
		return setUpYes
	}
	return setUpNo
}

// grantExpiryOK says v is a date of the form YYYY-MM-DD that keeps or shortens the requested expiry; without a
// requested expiry every date holds.
func grantExpiryOK(requested, v string) bool {
	d, err := time.Parse(control.GrantExpiresLayout, v)
	return err == nil && d.Format(control.GrantExpiresLayout) == v && (requested == "" || v <= requested)
}

// grantApproveArgs are the arguments after qhost that approve the grant as it was shown. expires is a date that
// grantExpiryOK let through, or empty to keep the requested one; no text of the proposal is part of them.
func grantApproveArgs(p *proposalCtx, expires string) []string {
	args := []string{"proposal", "approve", p.id, "--fingerprint", p.fingerprint}
	if expires != "" {
		args = append(args, "--expires", expires)
	}
	return append(args, "--confirm", "--output", "json")
}

func (m *Model) grantExpiryWord(expires string) string {
	if expires == "" {
		return m.text.Lookup(textPropGrantNever)
	}
	return m.text.Lookup(textPropGrantUntil, expires)
}

func (m *Model) setUpWord(s setUp) string {
	switch s {
	case setUpYes:
		return m.text.Lookup(textPropGrantSetUp)
	case setUpNo:
		return m.text.Lookup(textPropGrantNotSetUp)
	}
	return m.text.Lookup(textPropGrantUnknown)
}

// grantContentLines is the grant in one sentence, the expiry that was asked for and the reason as a quote.
func (m *Model) grantContentLines(eff *kindEffect, width int) []string {
	t, g := m.text, eff.grant
	what := t.Lookup(grantActionTexts[g.Action], g.Stack)
	lines := m.wrapped(detailIndent, kittui.StyleText, t.Lookup(textPropGrantSentence, proposalShort(g.User), g.UID, what), width)
	lines = append(lines, m.wrapped(detailIndent, kittui.StyleText, t.Lookup(textPropGrantAsked, m.grantExpiryWord(g.Expires)), width)...)
	lines = append(lines, m.wrapped(detailIndent, kittui.StyleHint, t.Lookup(textFreeHeadReason), width)...)
	reason := g.Reason
	if reason == "" {
		reason = t.Lookup(textNoValue)
	}
	return append(lines, m.quotedLines(kittui.StyleText, reason, width)...)
}

// grantEffectLines is what approving would grant and what the person should know first.
func (m *Model) grantEffectLines(eff *kindEffect, width int) []string {
	t := m.text
	expires := eff.grant.Expires
	if eff.expires != "" {
		expires = eff.expires
	}
	lines := m.wrapped(detailIndent, kittui.StyleText, t.Lookup(textPropGrantExpires, m.grantExpiryWord(expires)), width)
	if eff.expires != "" {
		lines = append(lines, m.wrapped(detailIndent, kittui.StyleHint, t.Lookup(textPropGrantSet), width)...)
	}
	lines = append(lines, m.wrapped(detailIndent, kittui.StyleText, t.Lookup(textPropGrantBackup, m.setUpWord(eff.backup)), width)...)
	lines = append(lines, m.wrapped(detailIndent, kittui.StyleText, t.Lookup(textPropGrantCheck, m.setUpWord(eff.check)), width)...)
	if eff.grant.Action == control.GrantActionUpdateRun {
		lines = append(lines, m.wrapped(detailIndent, kittui.StyleWarn, t.Lookup(textPropGrantPush), width)...)
	}
	return lines
}

// grantHints is the key that sets the expiry of the grant shown, while it can be approved.
func (m *Model) grantHints() []kittui.Hint {
	if v := m.pv.kind; v != nil && v.kind == control.ProposalKindGrant && m.canApprove() {
		return []kittui.Hint{{Action: actEdit, Label: textKeyGrantExpiry, Priority: 3}}
	}
	return nil
}

// grantExpiryAction opens the expiry form for the grant shown; handled is false when no such grant is open.
func (m *Model) grantExpiryAction() (cmd tea.Cmd, handled bool) {
	eff := m.pv.kind
	if eff == nil || eff.kind != control.ProposalKindGrant || !m.canApprove() {
		return nil, false
	}
	if m.settingsBusy() {
		m.warn(m.text.Lookup(textActionBusy))
		return nil, true
	}
	value := eff.expires
	if value == "" {
		value = eff.grant.Expires
	}
	f := m.newSettingsForm(formGrantExpiry, textTitleGrantExpiry, []kittui.FormField{
		textField("expires", textFieldGrantExpires, textHintGrantExpires, value)})
	return m.openForm(f), true
}

// submitGrantExpiry keeps the date for the approval. An empty date keeps what the proposal asked for.
func (m *Model) submitGrantExpiry(f *settingsForm, expires string) tea.Cmd {
	eff := m.pv.kind
	if eff == nil {
		return nil
	}
	if expires != "" && !grantExpiryOK(eff.grant.Expires, expires) {
		cause := textProblemGrantExpires
		if eff.grant.Expires != "" {
			cause = textProblemGrantExpiresMax
		}
		f.problems(m.bad("expires", cause, eff.grant.Expires))
		return nil
	}
	eff.expires = expires
	m.set.form = nil
	return nil
}

// askGrantSudo asks to approve the grant with its effect shown; y runs the command through sudo, which approves
// the proposal for the fingerprint shown and adds the grant.
func (m *Model) askGrantSudo(eff *kindEffect, shown *proposalCtx) {
	var gen int
	m.askSudo(sudoAsk{title: textTitleProposalApprove,
		question:    m.text.Lookup(textAskProposalGrant, kittui.Escape(proposalShort(shown.id))),
		consequence: m.text.Lookup(textWhatProposalGrantSudo),
		body: func(width int) []string {
			return append(m.grantContentLines(eff, width), m.grantEffectLines(eff, width)...)
		},
		args:  m.engineArgs(shown.engine, grantApproveArgs(shown, eff.expires)),
		begin: func() { gen = m.beginSetting() },
		done: func(r sudoRun) tea.Cmd {
			msg := settingMsg{gen: gen, op: opProposalApply, name: shown.id}
			var res grantResult
			if r.decode(&res) && res.ID != "" {
				msg.res = proposalDone{res: kindApplied{eff: eff, grant: res}}
			} else {
				// The proposal may be locked or failed by now, so the view reads it again.
				msg.res = proposalDone{err: sudoProblem(r.problem)}
			}
			return m.applySetting(msg)
		}})
}

// grantDone says an approved grant was added.
func (m *Model) grantDone(res kindApplied, done func(text string, details ...string)) []tea.Cmd {
	g := res.grant
	text := m.text.Lookup(textDonePropGrant, kittui.Escape(proposalShort(g.ID)), kittui.Escape(proposalShort(g.User)))
	if g.Replaced {
		text = strings.TrimSpace(text + " " + m.text.Lookup(textDonePropGrantReplaced))
	}
	done(text)
	return []tea.Cmd{m.reloadNow()}
}
