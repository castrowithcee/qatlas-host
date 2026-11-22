package tui

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/castrowithcee/qatlas-kit/approval"
	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// kindEffect is the content of a proposal of the kind stack-settings, window or notify-channel and, for an open one, how it
// differs from what applies now. The content is data: it is shown and sent to the service, never run.
type kindEffect struct {
	kind     string
	stack    string
	settings control.StackSettings
	window   control.WindowSetArgs
	channel  control.ProposalNotifyChannel
	// proposed lists every value of the content in full.
	proposed []approval.Change
	// worked is set once the difference to what applies now was worked out.
	worked  bool
	had     bool
	changes []approval.Change
	// noStack is set when the stack of the proposal does not exist.
	noStack bool
	// taken is set when a channel of the name of the proposal exists, which refuses the approval.
	taken bool
	// sig is what the person is shown of the difference; two reads with the same sig show the same.
	sig string
}

// stackNameRe is the shape of a stack name the service accepts; a name of another shape is not looked up.
var stackNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)

// isKindProposal says the kind is one that is shown with its effect and applied through the write the forms and
// the CLI use.
func isKindProposal(kind string) bool {
	return kind == control.ProposalKindStackSettings || kind == control.ProposalKindWindow ||
		kind == control.ProposalKindNotifyChannel
}

// loadKind reads the content of a proposal of a settings kind into msg and, while the proposal is open, the
// settings or window that apply now. It only reads, and needs no root.
func loadKind(ctx context.Context, b Backend, msg *proposalMsg) {
	eff := &kindEffect{kind: msg.res.Kind}
	var raw []byte
	switch msg.res.Kind {
	case control.ProposalKindStackSettings:
		var c control.ProposalStackSettings
		if json.Unmarshal(msg.res.Content, &c) != nil || !stackNameRe.MatchString(c.Stack) {
			msg.bad = true
			return
		}
		eff.stack, eff.settings = c.Stack, c.Settings
		raw, _ = json.Marshal(c.Settings)
	case control.ProposalKindNotifyChannel:
		if json.Unmarshal(msg.res.Content, &eff.channel) != nil || !channelNameRe.MatchString(eff.channel.Name) {
			msg.bad = true
			return
		}
		raw, _ = json.Marshal(eff.channel)
	default:
		if json.Unmarshal(msg.res.Content, &eff.window) != nil {
			msg.bad = true
			return
		}
		raw, _ = json.Marshal(eff.window)
	}
	var err error
	if eff.proposed, err = approval.Diff(nil, raw); err != nil {
		msg.bad = true
		return
	}
	msg.kind = eff
	if msg.res.State != control.ProposalOpen {
		return
	}
	switch msg.res.Kind {
	case control.ProposalKindStackSettings:
		msg.effectErr = settingsKindEffect(ctx, b, eff)
	case control.ProposalKindNotifyChannel:
		msg.effectErr = notifyKindEffect(ctx, b, eff)
	default:
		msg.effectErr = windowKindEffect(ctx, b, eff)
	}
	if msg.effectErr == nil && !eff.taken {
		sig, _ := json.Marshal(struct {
			Kind    string
			Stack   string
			Had     bool
			Changes []approval.Change
		}{eff.kind, eff.stack, eff.had, eff.changes})
		eff.worked, eff.sig = true, string(sig)
	}
}

// notifyKindEffect looks for a channel of the name of the proposal. It only reads.
func notifyKindEffect(ctx context.Context, b Backend, eff *kindEffect) error {
	list, err := b.Notifications(ctx)
	for _, ch := range list.Channels {
		eff.taken = eff.taken || ch.Name == eff.channel.Name
	}
	return err
}

func settingsKindEffect(ctx context.Context, b Backend, eff *kindEffect) error {
	cur, err := b.StackSettings(ctx, eff.stack)
	if err != nil {
		var remote *control.Error
		eff.noStack = errors.As(err, &remote) && remote.Code == control.CodeNoSuchStack
		return err
	}
	var before []byte
	if cur.Settings != nil {
		eff.had = true
		before, _ = json.Marshal(cur.Settings)
	}
	after, _ := json.Marshal(eff.settings)
	eff.changes, err = approval.Diff(before, after)
	return err
}

// windowKindEffect compares with the window in force; a part the proposal leaves out keeps its value.
func windowKindEffect(ctx context.Context, b Backend, eff *kindEffect) error {
	cur, err := b.Window(ctx)
	if err != nil {
		return err
	}
	type shape struct {
		Days     string `json:"days"`
		Start    string `json:"start"`
		Duration string `json:"duration"`
		Zone     string `json:"zone"`
	}
	before := shape{strings.Join(cur.Days, ","), cur.Start, cur.Duration, cur.Zone}
	after, w := before, eff.window
	if w.Days != nil {
		after.Days = strings.Join(w.Days, ",")
	}
	if w.Start != "" {
		after.Start = w.Start
	}
	if w.Duration != "" {
		after.Duration = w.Duration
	}
	if w.Zone != "" {
		after.Zone = w.Zone
	}
	bj, _ := json.Marshal(before)
	aj, _ := json.Marshal(after)
	eff.changes, err = approval.Diff(bj, aj)
	return err
}

func isBackupCommand(path string) bool {
	return path == "backup.command" || strings.HasPrefix(path, "backup.command[")
}

// kindContentLines is what the proposal holds, every value in full and wrapped, never cut. A backup command is
// marked and drawn as a warning, because it runs on the host later.
func (m *Model) kindContentLines(eff *kindEffect, width int) []string {
	t := m.text
	var lines []string
	if eff.kind == control.ProposalKindStackSettings {
		lines = m.wrapped(detailIndent, kittui.StyleText, t.Lookup(textPropStack, eff.stack), width)
	}
	command := false
	for _, ch := range eff.proposed {
		style, mark := kittui.StyleText, ""
		if isBackupCommand(ch.Path) {
			style, mark, command = kittui.StyleWarn, "!! ", true
		}
		lines = append(lines, m.wrapped(detailIndent, style, mark+ch.Path+": "+ch.After, width)...)
	}
	if command {
		lines = append(lines, m.wrapped(detailIndent, kittui.StyleWarn, t.Lookup(textPropBackupCommand), width)...)
	}
	return lines
}

// kindEffectLines is what applying the proposal now would change: each changed value before and after, in full.
func (m *Model) kindEffectLines(eff *kindEffect, width int) []string {
	t := m.text
	none := t.Lookup(textNoValue)
	value := func(s string) string {
		if s == "" {
			return none
		}
		return s
	}
	var lines []string
	switch {
	case eff.kind == control.ProposalKindNotifyChannel:
		return m.wrapped(detailIndent, kittui.StyleText, t.Lookup(textPropChannelNew), width)
	case len(eff.changes) == 0 && eff.kind == control.ProposalKindWindow:
		return m.wrapped(detailIndent, kittui.StyleHint, t.Lookup(textPropWindowSame), width)
	case len(eff.changes) == 0:
		return m.wrapped(detailIndent, kittui.StyleHint, t.Lookup(textPropSettingsSame), width)
	case eff.kind == control.ProposalKindWindow:
		lines = m.wrapped(detailIndent, kittui.StyleText, t.Lookup(textPropWindowChanges), width)
	case !eff.had:
		lines = m.wrapped(detailIndent, kittui.StyleText, t.Lookup(textPropSettingsNew), width)
	default:
		lines = m.wrapped(detailIndent, kittui.StyleText, t.Lookup(textPropSettingsChanges), width)
	}
	command := false
	for _, ch := range eff.changes {
		style, mark := kittui.StyleText, "   "
		if isBackupCommand(ch.Path) {
			style, mark, command = kittui.StyleWarn, "!! ", true
		}
		lines = append(lines, m.wrapped(detailIndent, style, mark+ch.Path, width)...)
		lines = append(lines, m.wrapped(detailIndent+3, style, t.Lookup(textPropBefore, value(ch.Before)), width)...)
		lines = append(lines, m.wrapped(detailIndent+3, style, t.Lookup(textPropAfter, value(ch.After)), width)...)
	}
	if command {
		lines = append(lines, "")
		lines = append(lines, m.wrapped(detailIndent, kittui.StyleWarn, t.Lookup(textPropBackupCommand), width)...)
	}
	return lines
}

// kindApplied is the answer of a proposal of a settings kind that was applied.
type kindApplied struct {
	eff      *kindEffect
	settings control.StackSettingsSetResult
	window   control.WindowResult
	channel  control.NotifyChannelInfo
}

// approveKind asks for the approval of the open proposal with the difference shown. Nothing is changed until the
// person says yes.
func (m *Model) approveKind() tea.Cmd {
	v := &m.pv
	eff := v.kind
	shown := &proposalCtx{id: v.id, fingerprint: v.info.Fingerprint, effect: eff.sig, gen: v.gen, engine: v.engine}
	t := m.text
	if eff.kind == control.ProposalKindNotifyChannel {
		return m.openForm(m.proposalSecretsForm(eff, shown))
	}
	question, consequence := t.Lookup(textAskProposalSettings, kittui.Escape(proposalShort(v.id)), eff.stack), textWhatProposalSettings
	if eff.kind == control.ProposalKindWindow {
		question, consequence = t.Lookup(textAskProposalWindow, kittui.Escape(proposalShort(v.id))), textWhatProposalWindow
	}
	if m.euid() != 0 {
		sudoWhat := textWhatProposalSettingsSudo
		if eff.kind == control.ProposalKindWindow {
			sudoWhat = textWhatProposalWindowSudo
		}
		m.askKindSudo(eff, shown, question, sudoWhat, nil)
		return nil
	}
	body := func(width int) []string { return m.kindEffectLines(eff, width) }
	m.askSheet(textTitleProposalApprove, question, t.Lookup(consequence), body,
		[]kittui.Answer{{Action: actYes, Label: textAnswerYes}, {Action: actNo, Label: textAnswerCancel}},
		func() tea.Cmd {
			ctx := m.engineCtx(shown.engine)
			return m.writeCall(opProposalApply, shown.id, func(b Backend) (any, error) {
				return approveKindAndApply(ctx, b, shown, eff)
			})
		})
	return nil
}

// approveKindAndApply is the last step of approving, in one background operation: the proposal and the settings
// or window in force are read again and compared with what was shown; the service takes the approval for the
// shown fingerprint, which locks the proposal; then the content shown is written through the call the forms and
// the CLI use, and its result is kept on the proposal. A difference or a refusal before the write changes nothing.
func approveKindAndApply(ctx context.Context, b Backend, p *proposalCtx, eff *kindEffect) (any, error) {
	now := loadProposal(ctx, b, nil, p.gen, p.id, true, nil)
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
	if eff.kind == control.ProposalKindStackSettings {
		applied.settings, err = b.StackSettingsSet(ctx, eff.stack, eff.settings)
	} else {
		applied.window, err = b.WindowSet(ctx, eff.window)
	}
	result := control.ProposalResultArgs{ID: p.id, Fingerprint: p.fingerprint, Applied: err == nil}
	var res any = applied
	if err != nil {
		result.Message, res = failureMessage(err), nil
	}
	_, resultErr := b.ProposalResult(ctx, result)
	return proposalDone{res: res, err: err, resultErr: resultErr}, nil
}

// kindDone says what an applied proposal of a settings kind came to.
func (m *Model) kindDone(res kindApplied, done func(text string, details ...string)) []tea.Cmd {
	t := m.text
	if res.eff.kind == control.ProposalKindNotifyChannel {
		done(t.Lookup(textDonePropChannel, res.channel.Name))
		return []tea.Cmd{m.reloadNow()}
	}
	if res.eff.kind == control.ProposalKindWindow {
		m.set.window.loaded, m.set.windowRes = true, res.window
		done(t.Lookup(textDoneWindow))
		return []tea.Cmd{m.reloadNow()}
	}
	if !res.settings.Changed {
		done(t.Lookup(textDonePropSettingsSame, res.settings.Stack))
		return nil
	}
	done(t.Lookup(textDonePropSettings, res.settings.Stack), t.Lookup(textDetailHead, short(res.settings.Commit)))
	return []tea.Cmd{m.reloadNow()}
}

// kindProposalLines are the lines of the content and, for an open proposal, of the effect below the heading.
func (m *Model) kindProposalLines(lines []string, width int) []string {
	t, v := m.text, &m.pv
	if v.kind == nil {
		return lines
	}
	lines = append(lines, m.kindContentLines(v.kind, width)...)
	if v.info.State != control.ProposalOpen {
		return lines
	}
	lines = append(lines, "", m.line(detailMargin, kittui.StyleActive, t.Lookup(textPropHeadEffect), width))
	switch {
	case v.kind.noStack:
		return append(lines, m.wrapped(detailIndent, kittui.StyleFail, t.Lookup(textPropNoStack, v.kind.stack), width)...)
	case v.kind.taken:
		return append(lines, m.wrapped(detailIndent, kittui.StyleFail, t.Lookup(textPropChannelTaken, v.kind.channel.Name), width)...)
	case v.effectErr != nil:
		return append(lines, m.wrapped(detailIndent, kittui.StyleFail, t.Lookup(textPropEffectFailed, m.serviceText(v.effectErr)), width)...)
	case !v.kind.worked:
		return lines
	}
	return append(lines, m.kindEffectLines(v.kind, width)...)
}
