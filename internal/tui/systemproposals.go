package tui

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/client"
	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/host/hostadmin"
	"github.com/castrowithcee/qatlas-host/internal/host/hostchange"
	"github.com/castrowithcee/qatlas-host/internal/host/hostproposal"
)

// System > Proposals lists the proposals of changes to the host that administrators submitted. It reads them from
// the host service like the log page does, so it needs neither sudo nor the deploy service. A proposal is viewed
// with the sentence, the exact commands and the keys it names; approving it runs `host apply --proposal` through
// sudo with the same confirmation as a change typed by hand, and rejecting it runs `proposal reject` the same way.

// HostProposals reads the proposals of the host service.
type HostProposals interface {
	ProposalList(ctx context.Context) (control.ProposalListResult, error)
	ProposalShow(ctx context.Context, id string) (control.ProposalShowResult, error)
}

func (h *HostChannel) ProposalList(ctx context.Context) (res control.ProposalListResult, err error) {
	err = client.Do(ctx, h.endpoint, client.CallTimeout, control.ProposalListCommand, nil, &res)
	return res, err
}

func (h *HostChannel) ProposalShow(ctx context.Context, id string) (res control.ProposalShowResult, err error) {
	err = client.Do(ctx, h.endpoint, client.CallTimeout, control.ProposalShowCommand, control.ProposalIDArgs{ID: id}, &res)
	return res, err
}

// errNoHostProposals is the failure of an interface that was given no way to reach the host service.
var errNoHostProposals = errors.New("no host channel")

// optionDocker is the switch of user.create that gives the Docker right.
const optionDocker = "docker"

// formProposalPassword asks for the initial password of the user a proposal creates.
const formProposalPassword formKind = formHostFolder + 1

// hostProposalPage is what the proposals page holds. gen separates the answers of a read that was overtaken.
type hostProposalPage struct {
	list    *kittui.List
	tones   map[string]kittui.TextStyle
	byID    map[kittui.ItemID]control.ProposalInfo
	items   []control.ProposalInfo
	gen     int
	read    bool
	reading bool
	stale   bool
	err     error
	// viewGen separates the answer of the proposal asked for last.
	viewGen int
}

// hostProposalsMsg carries a read of the list, hostProposalMsg the read of one proposal for a view or an approval.
type hostProposalsMsg struct {
	gen int
	res control.ProposalListResult
	err error
}

type hostProposalMsg struct {
	gen     int
	id      string
	approve bool
	res     control.ProposalShowResult
	err     error
}

func newHostProposalList(keys *kittui.Keymap, text kittui.Translator, tones map[string]kittui.TextStyle) (*kittui.List, error) {
	tone := func(cell string) kittui.TextStyle {
		if s, ok := tones[cell]; ok {
			return s
		}
		return kittui.StyleText
	}
	l, err := kittui.NewList(kittui.ListSpec{Columns: []kittui.Column{
		{Title: textColProposal}, {Title: textColSubmitter, Flex: true}, {Title: textColSubmitted},
		{Title: textColExpires, Optional: true}, {Title: textColState, Tone: tone}},
		Keys: listKeys(), Search: kittui.SearchByKey, Empty: textHostPropEmpty}, keys, text)
	if err != nil {
		return nil, err
	}
	l.SetFocused(true)
	return l, nil
}

// readHostProposals reads the proposals of the host service off the event loop.
func (m *Model) readHostProposals() tea.Cmd {
	p := &m.sys.prop
	if m.hostProps == nil {
		p.read, p.err = true, errNoHostProposals
		return nil
	}
	p.gen++
	p.reading, p.stale = true, false
	gen, h, ctx := p.gen, m.hostProps, m.ctx
	return func() tea.Msg {
		res, err := h.ProposalList(ctx)
		return hostProposalsMsg{gen: gen, res: res, err: err}
	}
}

// applyHostProposals takes a read of the list; a failed read keeps the list read before. The first answer also
// counts for the notice at the start.
func (m *Model) applyHostProposals(msg hostProposalsMsg) {
	p := &m.sys.prop
	if msg.gen != p.gen {
		return
	}
	p.reading, p.read, p.err = false, true, msg.err
	if msg.err == nil {
		p.items = msg.res.Proposals
		m.fillHostProposals()
	}
	m.prop.hostDone = true
	m.noteProposals()
}

// fillHostProposals sets the rows of the list from what was read last.
func (m *Model) fillHostProposals() {
	p := &m.sys.prop
	clear(p.tones)
	clear(p.byID)
	items := make([]kittui.ListItem, len(p.items))
	ids := make([]string, len(p.items))
	for i, it := range p.items {
		ids[i] = it.ID
	}
	for i, id := range uniqueIDs(ids) {
		it := p.items[i]
		word, tone := m.proposalStateWord(it.State)
		p.tones[word] = tone
		items[i] = kittui.ListItem{ID: id, Cells: []string{proposalShort(it.ID), proposalShort(it.SubmitterName),
			m.proposalTime(it.Submitted), m.proposalTime(it.Expires), word}}
		p.byID[id] = it
	}
	if err := p.list.SetItems(items); err != nil {
		m.failure = m.text.Lookup(kittui.TextError, err.Error())
	}
}

// hostProposalError is the sentence of a failed call to the host service.
func (m *Model) hostProposalError(err error, failed kittui.TextID) string {
	t := m.text
	var remote *control.Error
	switch {
	case errors.Is(err, errNoHostProposals):
		return t.Lookup(textHostPropNoChannel)
	case errors.Is(err, client.ErrNotRunning):
		return t.Lookup(textLogNotRunning)
	case errors.Is(err, client.ErrNoAccess),
		errors.As(err, &remote) && (remote.Code == control.CodeNotAdmin || remote.Code == control.CodeNoAccess):
		return t.Lookup(textLogNoAccess)
	case errors.As(err, &remote):
		return t.Lookup(failed, remote.Message)
	}
	return t.Lookup(failed, err.Error())
}

// hostProposalsView draws the proposals page in width by height cells.
func (m *Model) hostProposalsView(width, height int) string {
	p, t := &m.sys.prop, m.text
	var head []string
	switch {
	case p.err != nil && len(p.items) == 0:
		lines := m.wrapped(detailMargin, kittui.StyleFail, m.hostProposalError(p.err, textHostPropFailed), width)
		return strings.Join(lines[:min(len(lines), height)], "\n")
	case !p.read:
		return m.line(detailMargin, kittui.StyleHint, t.Lookup(textHostPropReading), width)
	case p.err != nil:
		head = m.wrapped(detailMargin, kittui.StyleFail, m.hostProposalError(p.err, textHostPropFailed), width)
	case p.stale:
		head = m.wrapped(detailMargin, kittui.StyleWarn, t.Lookup(textHostPropStale), width)
	}
	if len(head) > 0 {
		head = append(head, "")
	}
	head = head[:min(len(head), max(height-minListRows, 0))]
	view := m.listView(p.list, width, max(height-len(head), 1), len(head))
	return strings.Join(append(head, view), "\n")
}

// hostProposalFooter are the own keys of the proposals page.
func (m *Model) hostProposalFooter(own, common, nav []kittui.Hint) []kittui.Hint {
	p := &m.sys.prop
	if p.list.Searching() {
		return []kittui.Hint{{Action: actEnter, Label: textFilterKeep}, {Action: actBack, Label: textFilterClear}}
	}
	if len(p.items) == 0 {
		return append(own, common...)
	}
	own = append(own, kittui.Hint{Action: actEnter, Label: textKeyDetails},
		kittui.Hint{Action: actSearch, Label: kittui.TextKeySearch, Priority: 1})
	if info, ok := m.selectedHostProposal(); ok && info.State == control.ProposalOpen {
		own = append(own, kittui.Hint{Action: actApprove, Label: textKeyApprove, Priority: 3},
			kittui.Hint{Action: actReject, Label: textKeyReject, Priority: 3})
	}
	if p.list.Query() != "" {
		own = append(own, kittui.Hint{Action: actBack, Label: textFilterClear})
	}
	return append(append(own, common...), nav...)
}

// selectedHostProposal is the proposal under the cursor.
func (m *Model) selectedHostProposal() (control.ProposalInfo, bool) {
	p := &m.sys.prop
	if !p.read {
		return control.ProposalInfo{}, false
	}
	id, ok := p.list.Selected()
	info, found := p.byID[id]
	return info, ok && found
}

// askHostProposal reads the proposal under the cursor again for a view or, with approve, for its approval.
func (m *Model) askHostProposal(approve bool) tea.Cmd {
	info, ok := m.selectedHostProposal()
	if !ok {
		m.warn(m.text.Lookup(textChooseOne))
		return nil
	}
	if approve && info.State != control.ProposalOpen {
		m.warn(m.text.Lookup(textPropNotOpen))
		return nil
	}
	return m.readHostProposal(info.ID, approve)
}

func (m *Model) readHostProposal(id string, approve bool) tea.Cmd {
	if m.hostProps == nil {
		m.failure = m.text.Lookup(kittui.TextError, m.hostProposalError(errNoHostProposals, textHostPropFailed))
		return nil
	}
	p := &m.sys.prop
	p.viewGen++
	gen, h, ctx := p.viewGen, m.hostProps, m.ctx
	return func() tea.Msg {
		res, err := h.ProposalShow(ctx, id)
		return hostProposalMsg{gen: gen, id: id, approve: approve, res: res, err: err}
	}
}

// applyHostProposal takes the read of one proposal: it opens the view or, for an approval, goes on to the
// confirmation.
func (m *Model) applyHostProposal(msg hostProposalMsg) tea.Cmd {
	if msg.gen != m.sys.prop.viewGen {
		return nil
	}
	if msg.err != nil {
		m.failure = m.text.Lookup(kittui.TextError, m.hostProposalError(msg.err, textHostPropFailed))
		return nil
	}
	d := m.hostProposalDetail(msg.res)
	if msg.approve {
		return m.approveHostProposal(d)
	}
	m.openHostProposalView(d)
	return nil
}

// hostProposalDetail is one proposal as it was read, checked with the rules of proposals and worked out into the
// plan the person sees. err is set when the proposal cannot be shown as a change or fails the check.
type hostProposalDetail struct {
	info   control.ProposalInfo
	change control.ProposalHostChange
	// keys are the proposed keys in canonical form, parsed for the view.
	keys []hostchange.Key
	plan hostchange.Plan
	err  error
}

func (m *Model) hostProposalDetail(res control.ProposalShowResult) hostProposalDetail {
	d := hostProposalDetail{info: res.ProposalInfo}
	if res.Kind != control.ProposalKindHostChange {
		d.err = errors.New("this version cannot show a proposal of the kind " + res.Kind)
		return d
	}
	if err := json.Unmarshal(res.Content, &d.change); err != nil {
		d.err = errors.New("the content cannot be read")
		return d
	}
	check := hostproposal.New(m.hostRegistry())
	keys, err := check.Check(d.change.Action, d.change.Args, d.change.Keys)
	if err == nil && !slices.Equal(keys, d.change.Keys) {
		err = errors.New("its SSH keys are not in canonical form")
	}
	if err == nil {
		d.plan, err = check.View(d.change.Action, d.change.Args, keys)
	}
	if err != nil {
		d.err = err
		return d
	}
	for _, line := range keys {
		k, err := hostchange.ParseKey(line)
		if err != nil {
			d.err = err
			return d
		}
		d.keys = append(d.keys, k)
	}
	return d
}

// approvable says the proposal can be approved here: open and passing the check.
func (d hostProposalDetail) approvable() bool {
	return d.err == nil && d.info.State == control.ProposalOpen
}

// openHostProposalView shows the proposal in a sheet; its yes goes on to the approval.
func (m *Model) openHostProposalView(d hostProposalDetail) {
	t := m.text
	question := t.Lookup(textHostPropQuestion, proposalShort(d.info.ID), proposalShort(d.info.SubmitterName))
	body := func(width int) []string { return m.hostProposalLines(d, width) }
	var yes func() tea.Cmd
	answers := []kittui.Answer{{Action: actYes, Label: textKeyApprove}, {Action: actNo, Label: textAnswerClose}}
	if d.approvable() {
		yes = func() tea.Cmd { return m.approveHostProposal(d) }
	}
	consequence := t.Lookup(textHostPropWhat)
	if yes == nil {
		consequence = t.Lookup(textHostPropNotApprovable)
	}
	m.askSheet(textTitleHostProposal, question, consequence, body, answers, yes)
}

// hostProposalLines are the view of a proposal: who proposed it and in which state, the sentence, a red warning
// where the change reaches root, a system account or group, the Docker right or a system path, the exact commands
// dimmed as they would be typed by hand, and the proposed keys. Every text a submitter wrote is escaped.
func (m *Model) hostProposalLines(d hostProposalDetail, width int) []string {
	t, p := m.text, d.info
	state, tone := m.proposalStateWord(p.State)
	lines := m.fieldLines([]field{
		{t.Lookup(textPropFieldSubmitter), proposalShort(p.SubmitterName) + " (" + strconv.Itoa(int(p.SubmitterUID)) + ")", kittui.StyleText},
		{t.Lookup(textPropFieldSubmitted), m.proposalTime(p.Submitted), kittui.StyleText},
		{t.Lookup(textPropFieldExpires), m.proposalTime(p.Expires), kittui.StyleText},
		{t.Lookup(textPropFieldState), state, tone},
	}, width)
	if p.Message != "" {
		style, label := kittui.StyleText, textPropMessage
		switch p.State {
		case control.ProposalFailed:
			style, label = kittui.StyleFail, textPropFailure
		case control.ProposalRejected:
			label = textPropReason
		}
		lines = append(lines, m.wrapped(detailIndent, style, t.Lookup(label, proposalShort(p.Message)), width)...)
	}
	lines = append(lines, "", m.heading(textHostPropHeadChange, width))
	if d.err != nil {
		return append(lines, m.wrapped(detailIndent, kittui.StyleFail, t.Lookup(textHostPropBad, d.err.Error()), width)...)
	}
	lines = append(lines, m.wrapped(detailIndent, kittui.StyleText, d.plan.Sentence, width)...)
	if risk := m.hostProposalRisk(d.change.Action, d.change.Args); risk != "" {
		lines = append(lines, "")
		lines = append(lines, m.wrapped(detailIndent, kittui.StyleFail, m.styles.Glyph(kittui.GlyphWarning)+" "+risk, width)...)
	}
	lines = append(lines, "", m.heading(textHostPropHeadCommands, width))
	for _, c := range d.plan.Commands {
		lines = append(lines, m.wrapped(detailIndent, kittui.StyleHint, "$ "+c.Display(), width)...)
	}
	if len(d.keys) > 0 {
		lines = append(lines, "", m.heading(textHostPropHeadKeys, width))
		for _, k := range d.keys {
			comment := k.Comment
			if comment == "" {
				comment = t.Lookup(textNoValue)
			}
			lines = append(lines, m.wrapped(detailIndent, kittui.StyleText, t.Lookup(textHostPropKey, k.Type, comment, k.Fingerprint), width)...)
		}
	}
	if hostproposal.NeedsPassword(d.change.Action) {
		lines = append(lines, "", m.line(detailMargin, kittui.StyleHint, t.Lookup(textHostPropPassword), width))
	}
	return lines
}

// userNamed is the user of the last read with the name; one that is not there yet has an impossible UID, so it is
// neither root nor a system account.
func (m *Model) userNamed(name string) hostadmin.User {
	if u, ok := m.findUser(name); ok {
		return u
	}
	return hostadmin.User{Name: name, UID: -1}
}

// groupNamed is the group of the last read with the name, or one that is only a name.
func (m *Model) groupNamed(name string) hostadmin.Group {
	for _, g := range m.sys.groups {
		if g.Name == name {
			return g
		}
	}
	return hostadmin.Group{Name: name}
}

// hostProposalRisk names why a proposed change is marked red: the Docker right and the access to qhost, which are
// control over the host, and changes of root, system accounts, system groups and system paths, with the sources
// the changes typed by hand use. It is empty for a change of regular accounts, groups and paths only.
func (m *Model) hostProposalRisk(action string, args []string) string {
	if len(args) == 0 {
		return ""
	}
	t := m.text
	var parts []string
	add := func(s string) {
		if s != "" {
			parts = append(parts, s)
		}
	}
	account := func(groups ...string) { add(m.riskOf(m.userNamed(args[0]), groups)) }
	switch action {
	case hostchange.ActionUserSetDocker:
		add(t.Lookup(textRiskSetDocker))
		account()
	case hostchange.ActionUserEnable:
		add(t.Lookup(textRiskEnable))
		account()
	case hostchange.ActionUserCreate:
		var groups []string
		for _, a := range args[1:] {
			if g, ok := strings.CutPrefix(a, "group="); ok {
				groups = append(groups, g)
			}
		}
		if slices.Contains(args[1:], optionDocker) {
			add(t.Lookup(textRiskSetDocker))
		}
		account(groups...)
	case hostchange.ActionUserAddGroups, hostchange.ActionUserRemoveGroups:
		account(args[1:]...)
	case hostchange.ActionGroupCreate, hostchange.ActionGroupDelete, hostchange.ActionGroupRename:
		add(m.groupRisk(m.groupNamed(args[0])))
	case hostchange.ActionGroupSetMembers:
		var names []string
		for _, a := range args[1:] {
			names = append(names, strings.TrimLeft(a, "+-"))
		}
		add(m.membersRisk(m.groupNamed(args[0]), names))
	case hostchange.ActionFolderSetPermissions:
		if hostchange.IsSystemPath(args[0]) {
			add(t.Lookup(textRiskFolderSystem))
		}
	default:
		account()
	}
	return strings.Join(parts, " ")
}

// approveHostProposal goes on to the approval of a proposal: the initial password of a new user is asked first,
// then the confirmation of the change.
func (m *Model) approveHostProposal(d hostProposalDetail) tea.Cmd {
	if !d.approvable() {
		m.warn(m.text.Lookup(textPropNotOpen))
		return nil
	}
	if hostproposal.NeedsPassword(d.change.Action) {
		f := m.newSettingsForm(formProposalPassword, textTitleHostPropPassword, []kittui.FormField{
			maskedField(fieldUserFirst, textFieldInitial, textHintInitial),
			maskedField(fieldUserRepeat, textFieldRepeat, textHintRepeat),
		})
		if f == nil {
			return nil
		}
		f.hprop = &d
		f.form.SetLead(textLeadNewUser)
		return m.openForm(f)
	}
	if problem := m.confirmHostProposal(d, ""); problem != "" {
		m.failure = m.text.Lookup(kittui.TextError, problem)
	}
	return nil
}

// submitProposalPassword checks the two entries of the initial password and asks to apply the proposal.
func (m *Model) submitProposalPassword(f *settingsForm, v map[kittui.FieldID]kittui.FormValue) tea.Cmd {
	if ps := m.secretProblems(v[fieldUserFirst].Text, v[fieldUserRepeat].Text); len(ps) > 0 {
		f.problems(ps...)
		return nil
	}
	if problem := m.confirmHostProposal(*f.hprop, v[fieldUserFirst].Text); problem != "" {
		f.problem(problem, m.text.Lookup(textFormFix))
		return nil
	}
	m.dropForm()
	return nil
}

// confirmHostProposal plans the proposal again and shows the confirmation of applying it; the text of a refusal
// is returned. The initial password goes only to the standard input of the root command as {"password": ...}: the
// keys come from the proposal.
func (m *Model) confirmHostProposal(d hostProposalDetail, password string) string {
	check := hostproposal.New(m.hostRegistry())
	plan, secret, err := check.Plan(d.change.Action, d.change.Args, d.change.Keys, password)
	clear(secret)
	if err != nil {
		return err.Error()
	}
	var stdin []byte
	var mask [][]byte
	if hostproposal.NeedsPassword(d.change.Action) {
		if stdin, err = json.Marshal(struct {
			Password string `json:"password"`
		}{password}); err != nil {
			return err.Error()
		}
		mask = [][]byte{[]byte(password)}
	}
	commands := make([]string, len(plan.Commands))
	for i, c := range plan.Commands {
		commands[i] = c.Display()
	}
	id := d.info.ID
	a := hostAsk{id: d.change.Action, args: d.change.Args, secret: stdin, mask: mask,
		question: m.text.Lookup(textAskHostProposal, proposalShort(id), plan.Sentence),
		risk:     m.hostProposalRisk(d.change.Action, d.change.Args),
		apply:    []string{"apply", "--proposal", id, "--fingerprint", d.info.Fingerprint, "--confirm", "--output", "json"}}
	m.confirmHost(hostPlanned{ask: a, commands: commands, notReversible: plan.NotReversible})
	return ""
}

// askHostProposalReject asks to reject the proposal under the cursor; y runs `proposal reject` through sudo.
func (m *Model) askHostProposalReject() tea.Cmd {
	info, ok := m.selectedHostProposal()
	switch {
	case !ok:
		m.warn(m.text.Lookup(textChooseOne))
		return nil
	case info.State != control.ProposalOpen:
		m.warn(m.text.Lookup(textPropNotOpen))
		return nil
	}
	t, id := m.text, info.ID
	m.askChange(t.Lookup(textAskProposalReject, kittui.Escape(proposalShort(id))), t.Lookup(textWhatHostPropReject), false, false, nil,
		[]string{"qhost proposal reject " + id}, func() tea.Cmd { return m.runHostProposalReject(id) }, nil)
	return nil
}

func (m *Model) runHostProposalReject(id string) tea.Cmd {
	return m.runHost(HostRequest{Args: []string{"proposal", "reject", id}, Terminal: true}, func(out []byte, err error) tea.Cmd {
		if err != nil {
			m.hostFailed(out, err, true)
		} else {
			m.result = m.text.Lookup(textHostPropRejected, kittui.Escape(proposalShort(id)))
		}
		m.sys.prop.stale = true
		return m.showSystemPage()
	})
}
