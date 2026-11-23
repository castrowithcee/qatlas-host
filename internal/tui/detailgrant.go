package tui

import (
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// The Grants section of the stack detail lists the standing grants of the stack, grants one and withdraws one. Both
// writes need root, so they run as `sudo qhost grant …`. The list is read by the deploy service and needs no root.

const formGrant formKind = formAdoptTarget + 1

// grantSetup is what the stack has set up, as the confirmation of a new grant says it.
type grantSetup struct{ backup, check bool }

// grantAdded and grantRemoved are the answers of the two commands.
type (
	grantAdded struct {
		user, action, stack string
		replaced            bool
	}
	grantRemoved struct{ user, action, stack string }
)

type grantsMsg struct {
	stack string
	gen   int
	res   control.GrantListResult
	err   error
}

// readGrants reads the grants of the stack shown; a read under way is not asked twice.
func (m *Model) readGrants() tea.Cmd {
	d := &m.detail
	if d.stack == "" || d.gr.loading {
		return nil
	}
	d.gr.loading = true
	ctx, b, stack, gen := m.engineCtx(d.engine), m.backend, d.stack, d.gen
	return func() tea.Msg {
		res, err := b.GrantList(ctx, stack)
		return grantsMsg{stack: stack, gen: gen, res: res, err: err}
	}
}

func (m *Model) applyGrants(msg grantsMsg) {
	d := &m.detail
	if msg.gen != d.gen || msg.stack != d.stack {
		return
	}
	d.gr.loading = false
	if d.gr.err = msg.err; msg.err != nil {
		d.gr.loaded, d.grants = false, control.GrantListResult{}
		return
	}
	d.gr.loaded, d.grants = true, msg.res
}

// grantActionText is an action of a grant in plain words; an unknown action stands as it is.
func (m *Model) grantActionText(action string) string {
	id, ok := map[string]kittui.TextID{control.GrantActionStackRestart: textGrantActRestart,
		control.GrantActionContainerRestart: textGrantActContainer, control.GrantActionUpdateCheck: textGrantActCheck,
		control.GrantActionUpdateRun: textGrantActUpdate}[action]
	if !ok {
		return action
	}
	return m.text.Lookup(id)
}

func (m *Model) grantExpiryText(expires string) string {
	if expires == "" {
		return m.text.Lookup(textGrantNoExpiry)
	}
	return m.text.Lookup(textGrantUntil, expires)
}

// grantLeadText is the line of the confirmation about how long the grant holds.
func (m *Model) grantLeadText(expires string) string {
	if expires == "" {
		return m.text.Lookup(textGrantLeadNever)
	}
	return m.text.Lookup(textGrantLeadUntil, expires)
}

// grantStateText is "valid" or the reason the grant does not apply.
func (m *Model) grantStateText(g control.GrantInfo) string {
	id, ok := map[string]kittui.TextID{"": textGrantValid, control.GrantExpired: textGrantExpired,
		control.GrantStackMissing: textGrantStackMissing, control.GrantUserChanged: textGrantUserChanged}[g.Invalid]
	if !ok {
		return g.Invalid
	}
	return m.text.Lookup(id)
}

func (m *Model) grantSectionLines(width int) []string {
	t, d := m.text, &m.detail
	if note, ready := m.partNote(d.gr, width); !ready {
		return note
	}
	out := m.wrapped(detailIndent, kittui.StyleHint, t.Lookup(textGrantsNote), width)
	if d.grants.Problem != "" {
		out = append(out, m.wrapped(detailIndent, kittui.StyleWarn, t.Lookup(textGrantsProblem, d.grants.Problem), width)...)
	}
	if len(d.grants.Grants) == 0 {
		return append(out, m.wrapped(detailIndent, kittui.StyleHint, t.Lookup(textGrantsNone), width)...)
	}
	userWidth, actionWidth, expiryWidth := 0, 0, 0
	for _, g := range d.grants.Grants {
		userWidth = max(userWidth, lipgloss.Width(g.User))
		actionWidth = max(actionWidth, lipgloss.Width(m.grantActionText(g.Action)))
		expiryWidth = max(expiryWidth, lipgloss.Width(m.grantExpiryText(g.Expires)))
	}
	for _, g := range d.grants.Grants {
		style := kittui.StyleText
		if g.Invalid != "" {
			style = kittui.StyleWarn
		}
		row := strings.Join([]string{kittui.Pad(g.User, userWidth), kittui.Pad(m.grantActionText(g.Action), actionWidth),
			kittui.Pad(m.grantExpiryText(g.Expires), expiryWidth), m.grantStateText(g)}, "  ")
		out = append(out, m.line(detailIndent, style, row, width))
	}
	return out
}

// grantUsers are the users that can hold a grant: administrators with access to qhost. The System area has read
// them already, else they are read from the account files now.
func (m *Model) grantUsers() []string {
	users := m.sys.users
	if !m.sys.loaded {
		var err error
		if users, err = m.accountsReader().Users(); err != nil {
			return nil
		}
	}
	var names []string
	for _, u := range users {
		if u.Admin && u.Enabled && accountName.MatchString(u.Name) {
			names = append(names, u.Name)
		}
	}
	slices.Sort(names)
	return names
}

// openGrantForm reads the settings of the stack, which say whether it has a backup and a check; the form opens when
// they arrive.
func (m *Model) openGrantForm() tea.Cmd {
	stack, ok := m.detailStack()
	if !ok || m.detailBusy() {
		return nil
	}
	if !m.detail.gr.loaded {
		m.warn(m.text.Lookup(textDetailLoading))
		return nil
	}
	if len(m.grantUsers()) == 0 {
		m.warn(m.text.Lookup(textGrantsNoUsers))
		return nil
	}
	ctx := m.engineCtx(m.stackEngineOf(stack))
	return m.writeCall(opStackSettingsGrant, stack, func(b Backend) (any, error) { return b.StackSettings(ctx, stack) })
}

func (m *Model) grantFormFor(res control.StackSettingsResult) tea.Cmd {
	stack, ok := m.detailStack()
	if !ok || stack != res.Stack || m.set.form != nil || m.modal != nil {
		return nil
	}
	var users, actions []kittui.Choice
	for _, n := range m.grantUsers() {
		users = append(users, kittui.Choice{ID: kittui.ChoiceID(n), Label: dataLabel(n)})
	}
	for _, a := range control.GrantActions {
		actions = append(actions, kittui.Choice{ID: kittui.ChoiceID(a), Label: dataLabel(m.grantActionText(a))})
	}
	pick := func(id string, label, hint kittui.TextID, cs []kittui.Choice) kittui.FormField {
		return kittui.FormField{ID: kittui.FieldID(id), Kind: kittui.FieldChoice, Label: label, Hint: hint, Choices: cs,
			Value: kittui.FormValue{Choice: cs[0].ID}}
	}
	f := m.newSettingsForm(formGrant, textTitleGrant, []kittui.FormField{
		pick("user", textFieldGrantUser, textHintGrantUser, users),
		pick("action", textFieldGrantAction, textHintGrantAction, actions),
		textField("expires", textFieldGrantExpires, textHintGrantExpires, ""),
	})
	if f == nil {
		return nil
	}
	f.stack = stack
	f.setup = &grantSetup{backup: res.Settings != nil && res.Settings.Backup != nil, check: res.Settings != nil && res.Settings.Check != nil}
	return m.openForm(f)
}

func (m *Model) submitGrant(f *settingsForm, v map[kittui.FieldID]kittui.FormValue) tea.Cmd {
	user, action := string(v["user"].Choice), string(v["action"].Choice)
	expires := strings.TrimSpace(v["expires"].Text)
	if expires != "" {
		if _, err := time.Parse(control.GrantExpiresLayout, expires); err != nil {
			f.problems(m.bad("expires", textProblemGrantExpires))
			return nil
		}
	}
	m.askGrantAdd(f.stack, user, action, expires, *f.setup)
	return nil
}

// grantSetupText says in plain words whether the stack has a backup and a check, and for an update what it does.
func (m *Model) grantSetupText(action string, s grantSetup) string {
	id := textGrantSetupNone
	switch {
	case s.backup && s.check:
		id = textGrantSetupBoth
	case s.backup:
		id = textGrantSetupBackup
	case s.check:
		id = textGrantSetupCheck
	}
	text := m.text.Lookup(id)
	if action == control.GrantActionUpdateRun {
		text += " " + m.text.Lookup(textGrantUpdatePush)
	}
	return text
}

func (m *Model) askGrantAdd(stack, user, action, expires string, s grantSetup) {
	engine := m.stackEngineOf(stack)
	args := []string{"grant", "add"}
	if expires != "" {
		args = append(args, "--expires", expires)
	}
	args = m.engineArgs(engine, append(args, "--confirm", "--output", "json", "--", user, action, stack))
	m.askGrantSudo(sudoAsk{title: textTitleGrant,
		question:    m.text.Lookup(textAskGrantAdd, kittui.Escape(user), m.grantActionText(action), stack),
		consequence: m.grantSetupText(action, s), lead: dialogLines([]string{m.grantLeadText(expires)}), args: args},
		opGrantAdd, user, func(r sudoRun) (any, bool) {
			var v struct {
				ID       string `json:"id"`
				Replaced bool   `json:"replaced"`
			}
			if !r.decode(&v) || v.ID == "" {
				return nil, false
			}
			return grantAdded{user: user, action: action, stack: stack, replaced: v.Replaced}, true
		})
}

// askGrantSudo asks with the command and gives the answer of the run to applySetting, as askSetting does without
// root; as root the command runs without sudo.
func (m *Model) askGrantSudo(a sudoAsk, op settingOp, name string, decode func(sudoRun) (any, bool)) {
	if m.settingsBusy() {
		m.warn(m.text.Lookup(textActionBusy))
		return
	}
	var gen int
	a.begin = func() { gen = m.beginSetting() }
	a.done = func(r sudoRun) tea.Cmd {
		msg := settingMsg{gen: gen, op: op, name: name}
		if res, ok := decode(r); ok {
			msg.res = res
		} else {
			msg.err = sudoProblem(r.problem)
		}
		return m.applySetting(msg)
	}
	m.askSudo(a)
}

func (m *Model) openGrantPick() tea.Cmd {
	stack, ok := m.detailStack()
	if !ok || m.detailBusy() {
		return nil
	}
	d := &m.detail
	if !d.gr.loaded {
		m.warn(m.text.Lookup(textDetailLoading))
		return nil
	}
	if len(d.grants.Grants) == 0 {
		m.warn(m.text.Lookup(textGrantsNoneToRemove))
		return nil
	}
	items := make([]kittui.ListItem, len(d.grants.Grants))
	for i, g := range d.grants.Grants {
		items[i] = kittui.ListItem{ID: kittui.ItemID(g.ID), Cells: []string{g.User, m.grantActionText(g.Action), m.grantExpiryText(g.Expires)}}
	}
	m.pick = m.newPicker(pickGrant, stack, textTitleGrantRemove, []kittui.Column{{Title: textColUser}, {Title: textColAction},
		{Title: textColExpires, Flex: true}}, textGrantsNone, items)
	return nil
}

// askGrantRemove asks to withdraw the grant the person chose.
func (m *Model) askGrantRemove(stack, id string) {
	i := slices.IndexFunc(m.detail.grants.Grants, func(g control.GrantInfo) bool { return g.ID == id })
	if i < 0 {
		return
	}
	g := m.detail.grants.Grants[i]
	args := m.engineArgs(m.stackEngineOf(stack), []string{"grant", "remove", "--confirm", "--output", "json", "--", id})
	m.askGrantSudo(sudoAsk{title: textTitleGrantRemove,
		question:    m.text.Lookup(textAskGrantRemove, kittui.Escape(g.User), m.grantActionText(g.Action), stack),
		consequence: m.text.Lookup(textWhatGrantRemove), danger: true, args: args},
		opGrantRemove, id, func(r sudoRun) (any, bool) {
			var v struct {
				Removed bool `json:"removed"`
			}
			if !r.decode(&v) || !v.Removed {
				return nil, false
			}
			return grantRemoved{user: g.User, action: g.Action, stack: stack}, true
		})
}
