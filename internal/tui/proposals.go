package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// areaProposals lists the proposals of the setup that people without root submitted; it is the last area so
// the keys of the others stay.
const areaProposals kittui.AreaID = "proposals"

// proposalShownLimit is how many characters of a proposed value are shown before the rest is only counted.
const proposalShownLimit = 200

// proposalState is the last list of proposals read. A failed read keeps the list read before.
type proposalState struct {
	list   []control.ProposalInfo
	err    error
	loaded bool
	// noted is set once the start notice was decided, so a later refresh never raises it again.
	noted bool
	// deployDone and hostDone are set once the deploy service and the host service answered, or failed to, at
	// the start; the notice waits for both.
	deployDone, hostDone bool
}

// proposalView is one proposal opened from the list, with the effect worked out when it was read.
type proposalView struct {
	open, loading, loaded bool
	id                    string
	// gen tells the answer of the proposal that was left from that of the current one.
	gen        int
	err        error
	info       control.ProposalShowResult
	content    control.ProposalSourceAdd
	contentBad bool
	// needsRoot is set when the effect was not worked out because the interface runs without root.
	needsRoot bool
	flow      *addFlow
	kind      *kindEffect
	effectErr error
	top       int
	total     int
	rows      int
}

// proposalMsg is the answer of reading one proposal and, for an open one, working out its effect.
type proposalMsg struct {
	gen       int
	id        string
	res       control.ProposalShowResult
	err       error
	content   control.ProposalSourceAdd
	bad       bool
	needsRoot bool
	flow      *addFlow
	kind      *kindEffect
	effectErr error
}

func proposalColumns(t kittui.Translator, tones map[string]kittui.TextStyle) []kittui.Column {
	tone := func(cell string) kittui.TextStyle {
		if s, ok := tones[cell]; ok {
			return s
		}
		return kittui.StyleText
	}
	return []kittui.Column{
		{Title: textColKind},
		{Title: textColSubmitter, Flex: true},
		{Title: textColSubmitted},
		{Title: textColExpires, Optional: true},
		{Title: textColState, Tone: tone},
	}
}

func (m *Model) newProposalList(text kittui.Translator) (*kittui.List, error) {
	l, err := kittui.NewList(kittui.ListSpec{Columns: proposalColumns(text, m.propTones), Keys: listKeys(),
		Search: kittui.SearchByKey, Empty: textPropEmpty}, m.keys, text)
	if err == nil {
		l.SetFocused(true)
	}
	return l, err
}

var proposalStateTexts = map[string]kittui.TextID{
	control.ProposalOpen: textPropOpen, control.ProposalApproving: textPropApproving, control.ProposalApplied: textPropApplied,
	control.ProposalFailed: textPropFailed, control.ProposalRejected: textPropRejected, control.ProposalWithdrawn: textPropWithdrawn,
	control.ProposalExpired: textPropExpired,
}

var proposalStateTones = map[string]kittui.TextStyle{
	control.ProposalOpen: kittui.StyleActive, control.ProposalApproving: kittui.StyleWarn, control.ProposalApplied: kittui.StyleOK,
	control.ProposalFailed: kittui.StyleFail, control.ProposalRejected: kittui.StyleHint, control.ProposalWithdrawn: kittui.StyleHint,
	control.ProposalExpired: kittui.StyleHint,
}

// proposalStateWord is the word of a state; a state this version does not know shows as the service wrote it.
func (m *Model) proposalStateWord(state string) (string, kittui.TextStyle) {
	word, tone := proposalShort(state), kittui.StyleText
	if id, ok := proposalStateTexts[state]; ok {
		word = m.text.Lookup(id)
	}
	if s, ok := proposalStateTones[state]; ok {
		tone = s
	}
	return word, tone
}

// proposalKindWord is the word of a kind; a kind this version does not know shows as the service wrote it.
func (m *Model) proposalKindWord(kind string) string {
	switch kind {
	case control.ProposalKindSourceAdd:
		return m.text.Lookup(textPropKindSourceAdd)
	case control.ProposalKindStackSettings:
		return m.text.Lookup(textPropKindStackSettings)
	case control.ProposalKindWindow:
		return m.text.Lookup(textPropKindWindow)
	case control.ProposalKindNotifyChannel:
		return m.text.Lookup(textPropKindChannel)
	}
	return proposalShort(kind)
}

// proposalShort cuts a value of a proposal that is longer than proposalShownLimit characters and counts what
// it left out. The text is escaped where it is drawn.
func proposalShort(s string) string {
	if r := []rune(s); len(r) > proposalShownLimit {
		return string(r[:proposalShownLimit]) + fmt.Sprintf("… (+%d)", len(r)-proposalShownLimit)
	}
	return s
}

func openProposals(list []control.ProposalInfo) int {
	n := 0
	for _, p := range list {
		if p.State == control.ProposalOpen {
			n++
		}
	}
	return n
}

// fillProposals sets the rows of the proposals list from the snapshot read last and, the first time, tells the
// person about open proposals.
func (m *Model) fillProposals() {
	m.prop.list, m.prop.err, m.prop.loaded = m.snap.proposals.Proposals, m.snap.proposalsErr, true
	clear(m.propTones)
	clear(m.propByID)
	items := make([]kittui.ListItem, len(m.prop.list))
	ids := make([]string, len(m.prop.list))
	for i, p := range m.prop.list {
		ids[i] = p.ID
	}
	for i, id := range uniqueIDs(ids) {
		p := m.prop.list[i]
		word, tone := m.proposalStateWord(p.State)
		m.propTones[word] = tone
		items[i] = kittui.ListItem{ID: id, Cells: []string{m.proposalKindWord(p.Kind), proposalShort(p.SubmitterName),
			m.proposalTime(p.Submitted), m.proposalTime(p.Expires), word}}
		m.propByID[id] = p
	}
	if err := m.propList.SetItems(items); err != nil {
		m.prop.err = err
	}
}

// noteProposals tells the person once about the open proposals of both services, when both have answered. A
// service that cannot be reached counts for none, so the other one still counts.
func (m *Model) noteProposals() {
	p := &m.prop
	if p.noted || !p.deployDone || !p.hostDone && m.hostProps != nil {
		return
	}
	p.noted = true
	deploy, host := 0, 0
	if p.loaded && p.err == nil {
		deploy = openProposals(p.list)
	}
	if hp := &m.sys.prop; hp.read && hp.err == nil {
		host = openProposals(hp.items)
	}
	n := deploy + host
	if n == 0 || m.notice != "" {
		return
	}
	var text string
	switch {
	case deploy > 0 && host > 0:
		text = m.text.Lookup(textPropNoticeBoth, n, deploy, areaKey(areaProposals), m.text.Lookup(textAreaProposals),
			host, areaKey(areaSystem), m.text.Lookup(textAreaSystem))
	default:
		id, area := textPropNotice, areaProposals
		if n == 1 {
			id = textPropNoticeOne
		}
		if deploy == 0 {
			area = areaSystem
		}
		text = m.text.Lookup(id, n, areaKey(area))
	}
	m.notice = m.text.Lookup(kittui.TextWarning, text)
}

// areaKey is the key that opens the area id, taken from its place in the tab line.
func areaKey(id kittui.AreaID) string {
	i := slices.IndexFunc(areas(), func(a kittui.Area) bool { return a.ID == id })
	if i < 0 || i >= len(areaKeys) {
		return ""
	}
	return areaKeys[i]
}

func (m *Model) proposalTime(t time.Time) string {
	if t.IsZero() {
		return m.text.Lookup(textNoValue)
	}
	return t.In(m.loc).Format(lastRunLayout)
}

// openProposal shows the proposal id of the list, read at once.
func (m *Model) openProposal(id kittui.ItemID) tea.Cmd {
	p, ok := m.propByID[id]
	if !ok {
		return nil
	}
	m.pv = proposalView{open: true, loading: true, id: p.ID, gen: m.pv.gen + 1}
	return m.readProposal()
}

// readProposal reads the open proposal and, while it is open, works out what applying it would change, with
// the check a stack folder added by hand gets. Nothing changes on the host.
func (m *Model) readProposal() tea.Cmd {
	v := &m.pv
	if !v.open {
		return nil
	}
	v.loading = true
	ctx, b, host, gen, id, root := m.ctx, m.backend, m.folders, v.gen, v.id, m.euid() == 0
	return func() tea.Msg { return loadProposal(ctx, b, host, gen, id, root) }
}

// loadProposal reads the proposal id and, while it is an open one with readable content, works out its effect:
// for the kind source-add only when the interface runs with root, for the kinds stack-settings and window
// always. It only reads.
func loadProposal(ctx context.Context, b Backend, host Folders, gen int, id string, root bool) proposalMsg {
	msg := proposalMsg{gen: gen, id: id}
	if msg.res, msg.err = b.ProposalShow(ctx, id); msg.err != nil {
		return msg
	}
	if isKindProposal(msg.res.Kind) {
		loadKind(ctx, b, &msg)
		return msg
	}
	if msg.res.Kind != control.ProposalKindSourceAdd {
		return msg
	}
	if msg.content, msg.bad = sourceAddContent(msg.res.Content); msg.bad || msg.res.State != control.ProposalOpen {
		return msg
	}
	if !root || host == nil {
		msg.needsRoot = true
		return msg
	}
	res, err := inspectFolder(ctx, b, host, msg.content.Path)
	if err != nil {
		msg.effectErr = err
		return msg
	}
	msg.flow = res.(inspected).flow
	return msg
}

// sourceAddContent reads the content of a proposal of the kind source-add; bad is set for content that is not
// an absolute, clean path.
func sourceAddContent(raw json.RawMessage) (c control.ProposalSourceAdd, bad bool) {
	if err := json.Unmarshal(raw, &c); err != nil {
		return c, true
	}
	return c, !filepath.IsAbs(c.Path) || filepath.Clean(c.Path) != c.Path
}

func (m *Model) applyProposal(msg proposalMsg) {
	v := &m.pv
	if !v.open || msg.gen != v.gen || msg.id != v.id {
		return
	}
	v.loading = false
	if v.err = msg.err; msg.err != nil {
		return
	}
	v.loaded, v.info, v.content, v.contentBad = true, msg.res, msg.content, msg.bad
	v.needsRoot, v.flow, v.effectErr, v.kind = msg.needsRoot, msg.flow, msg.effectErr, msg.kind
}

// proposalsView is the Proposals area: the list, or the proposal chosen in it.
func (m *Model) proposalsView(width, height int) string {
	m.scrollNote = ""
	switch {
	case m.pv.open:
		return m.proposalDetailView(width, height)
	case !m.prop.loaded:
		return m.note(m.text.Lookup(textPropLoading), width)
	case m.prop.err != nil:
		return strings.Join(m.wrapped(2, kittui.StyleFail, m.errorText(m.prop.err), width), "\n")
	}
	return m.listView(m.propList, width, height, 0)
}

func (m *Model) proposalDetailView(width, height int) string {
	lines := m.proposalLines(width)
	v := &m.pv
	v.total, v.rows = len(lines), height
	v.top = max(min(v.top, v.total-v.rows), 0)
	if v.total > v.rows {
		m.scrollNote = fmt.Sprintf("%d/%d", v.top+v.rows, v.total)
	}
	m.textPart(func(steps int) { v.top = wheeled(v.top, v.total, v.rows, steps) }, width, height, 0)
	return strings.Join(lines[v.top:min(v.top+height, len(lines))], "\n")
}

// proposalLines are the content of the open proposal and below it what applying it would change. Every value
// a submitter wrote is escaped and cut; no field is read as a command or a template.
func (m *Model) proposalLines(width int) []string {
	t, v := m.text, &m.pv
	lines := []string{"  " + m.styles.TitleLine(m.proposalKindWord(v.info.Kind), v.id, max(width-2, 1)), ""}
	switch {
	case v.err != nil:
		return append(lines, m.wrapped(detailMargin, kittui.StyleFail, m.errorText(v.err), width)...)
	case !v.loaded:
		return append(lines, m.line(detailMargin, kittui.StyleHint, t.Lookup(textDetailLoading), width))
	}
	p := v.info
	state, tone := m.proposalStateWord(p.State)
	lines = append(lines, m.fieldLines([]field{
		{t.Lookup(textPropFieldSubmitter), fmt.Sprintf("%s (%d)", proposalShort(p.SubmitterName), p.SubmitterUID), kittui.StyleText},
		{t.Lookup(textPropFieldSubmitted), m.proposalTime(p.Submitted), kittui.StyleText},
		{t.Lookup(textPropFieldExpires), m.proposalTime(p.Expires), kittui.StyleText},
		{t.Lookup(textPropFieldState), state, tone},
	}, width)...)
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
	lines = append(lines, "", m.line(detailMargin, kittui.StyleActive, t.Lookup(textPropHeadContent), width))
	if isKindProposal(p.Kind) {
		if v.contentBad {
			return append(lines, m.wrapped(detailIndent, kittui.StyleFail, t.Lookup(textPropContentBad), width)...)
		}
		return m.kindProposalLines(lines, width)
	}
	if p.Kind != control.ProposalKindSourceAdd {
		return append(lines, m.wrapped(detailIndent, kittui.StyleHint, t.Lookup(textPropKindUnknown), width)...)
	}
	if v.contentBad {
		return append(lines, m.wrapped(detailIndent, kittui.StyleFail, t.Lookup(textPropContentBad), width)...)
	}
	address := t.Lookup(textPropNoAddress)
	if v.content.URL != "" {
		address = proposalShort(v.content.URL)
	}
	lines = append(lines, m.wrapped(detailIndent, kittui.StyleText, t.Lookup(textPropPath, proposalShort(v.content.Path)), width)...)
	lines = append(lines, m.wrapped(detailIndent, kittui.StyleText, t.Lookup(textPropAddress, address), width)...)
	if p.State != control.ProposalOpen {
		return lines
	}
	lines = append(lines, "", m.line(detailMargin, kittui.StyleActive, t.Lookup(textPropHeadEffect), width))
	switch {
	case v.needsRoot:
		return append(lines, m.wrapped(detailIndent, kittui.StyleHint, t.Lookup(textPropEffectLater), width)...)
	case v.effectErr != nil:
		text := m.serviceText(v.effectErr)
		if _, host := v.effectErr.(hostError); host {
			text = v.effectErr.Error()
		}
		return append(lines, m.wrapped(detailIndent, kittui.StyleFail, t.Lookup(textPropEffectFailed, text), width)...)
	case v.flow == nil:
		return lines
	}
	return append(lines, m.inspectionLines(v.flow, width)...)
}
