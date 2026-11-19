package tui

import (
	"encoding/json"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"golang.org/x/crypto/ssh"

	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/host/hostadmin"
	"github.com/castrowithcee/qatlas-host/internal/host/hostchange"
)

// Users are created, changed and deleted on System > Users with a, e (or enter) and x. Each form collects one
// change and checks what it can before anything leaves the form; the confirmation follows, and the root command
// checks it all again.

// Limits the root actions enforce too; checking them here only saves a round through sudo.
const (
	maxFormSecret = 256
	maxFormKeys   = 16
)

// The forms of the users page.
const (
	formUserCreate    formKind = formProposalSecrets + 1
	formUserGroups    formKind = formUserCreate + 1
	formUserSecret    formKind = formUserGroups + 1
	formUserKeyAdd    formKind = formUserSecret + 1
	formUserKeyRemove formKind = formUserKeyAdd + 1
	formUserDelete    formKind = formUserKeyRemove + 1
)

// The fields of the user forms. The masked fields carry neutral names; their values are read once, when the
// person saves.
const (
	fieldUserName   = "name"
	fieldUserFirst  = "first"
	fieldUserRepeat = "repeat"
	fieldUserAdmin  = "admin"
	fieldUserDocker = "docker"
	fieldUserGroups = "groups"
	fieldUserKeys   = "keys"
	fieldUserMust   = "must"
	fieldUserKeyOf  = "fingerprint"
	fieldUserHome   = "home"
)

// The values of the yes and no choices of the user forms.
const (
	choiceYes = "yes"
	choiceNo  = "no"
)

// userForm is what a form of the users page is about: the user and, for the groups form, whether it adds.
type userForm struct {
	user hostadmin.User
	add  bool
}

// accountName is the user and group name the root actions accept.
var accountName = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)

func yesNoChoices() []kittui.Choice {
	return []kittui.Choice{choice(choiceNo, textNo), choice(choiceYes, textYes)}
}

func choiceField(id string, label, hint kittui.TextID, choices []kittui.Choice, value string) kittui.FormField {
	return kittui.FormField{ID: kittui.FieldID(id), Kind: kittui.FieldChoice, Label: label, Hint: hint, Choices: choices,
		Value: kittui.FormValue{Choice: kittui.ChoiceID(value)}}
}

// dataLabel turns a name of the host into a choice label: the catalog has no text for it, so it stands as it is,
// with no brace a key placeholder could take.
func dataLabel(s string) kittui.TextID {
	return kittui.TextID(strings.NewReplacer("{", "(", "}", ")").Replace(s))
}

// groupChoices are the further groups a user may be put into, by name: existing and without a switch of their
// own. A new user gets only regular groups.
func (m *Model) groupChoices(withSystem bool, skip func(string) bool) []kittui.Choice {
	var names []string
	for _, g := range m.sys.groups {
		if (withSystem || !g.System) && !fixedGroup(g.Name) && accountName.MatchString(g.Name) && (skip == nil || !skip(g.Name)) {
			names = append(names, g.Name)
		}
	}
	slices.Sort(names)
	choices := make([]kittui.Choice, len(names))
	for i, n := range names {
		choices[i] = kittui.Choice{ID: kittui.ChoiceID(n), Label: dataLabel(n)}
	}
	return choices
}

func (m *Model) userForm(kind formKind, title kittui.TextID, ctx userForm, fields []kittui.FormField) *settingsForm {
	f := m.newSettingsForm(kind, title, fields)
	if f != nil {
		f.user = &ctx
	}
	return f
}

// --- create ---

func (m *Model) userCreateForm() *settingsForm {
	fields := []kittui.FormField{
		textField(fieldUserName, textFieldUserName, textHintUserName, ""),
		maskedField(fieldUserFirst, textFieldInitial, textHintInitial),
		maskedField(fieldUserRepeat, textFieldRepeat, textHintRepeat),
		choiceField(fieldUserAdmin, textFieldAdmin, textHintAdmin, yesNoChoices(), choiceNo),
		choiceField(fieldUserDocker, textFieldDocker, textHintDocker, yesNoChoices(), choiceNo),
	}
	if groups := m.groupChoices(false, nil); len(groups) > 0 {
		fields = append(fields, kittui.FormField{ID: fieldUserGroups, Kind: kittui.FieldMultiple, Label: textFieldGroups,
			Hint: textHintGroups, Choices: groups})
	}
	fields = append(fields, kittui.FormField{ID: fieldUserKeys, Kind: kittui.FieldMaskedBlock, Label: textFieldKeys, Hint: textHintKeys})
	f := m.userForm(formUserCreate, textTitleNewUser, userForm{}, fields)
	if f != nil {
		f.form.SetLead(textLeadNewUser)
	}
	return f
}

// secretProblems checks the two entries of a password pair.
func (m *Model) secretProblems(first, repeat string) []kittui.FormProblem {
	switch {
	case first == "":
		return []kittui.FormProblem{m.bad(fieldUserFirst, textProblemSecretEmpty)}
	case len(first) > maxFormSecret || !utf8.ValidString(first) || strings.ContainsFunc(first, func(r rune) bool { return r < 0x20 || r == 0x7f }):
		return []kittui.FormProblem{m.bad(fieldUserFirst, textProblemSecretBad, maxFormSecret)}
	case first != repeat:
		return []kittui.FormProblem{m.bad(fieldUserRepeat, textProblemSecretMismatch)}
	}
	return nil
}

// parseKeys checks the public keys of a block, one per line, the way the root action does: a public key in
// authorized_keys format without options. Blank lines are left out.
func parseKeys(block string) (lines []string, problem kittui.TextID) {
	seen := map[string]bool{}
	for _, l := range strings.Split(strings.ReplaceAll(block, "\r\n", "\n"), "\n") {
		if l = strings.TrimSpace(l); l == "" {
			continue
		}
		pub, _, options, rest, err := ssh.ParseAuthorizedKey([]byte(l))
		if err != nil || len(rest) > 0 || len(options) > 0 {
			return nil, textProblemKeyBad
		}
		fp := ssh.FingerprintSHA256(pub)
		if seen[fp] {
			return nil, textProblemKeyTwice
		}
		seen[fp] = true
		lines = append(lines, l)
	}
	if len(lines) > maxFormKeys {
		return nil, textProblemKeysMax
	}
	return lines, ""
}

func (m *Model) submitUserCreate(f *settingsForm, v map[kittui.FieldID]kittui.FormValue) tea.Cmd {
	name := strings.TrimSpace(v[fieldUserName].Text)
	var ps []kittui.FormProblem
	switch _, exists := m.findUser(name); {
	case !accountName.MatchString(name) || name == "root":
		ps = append(ps, m.bad(fieldUserName, textProblemUserName))
	case exists:
		ps = append(ps, m.bad(fieldUserName, textProblemUserTaken))
	}
	ps = append(ps, m.secretProblems(v[fieldUserFirst].Text, v[fieldUserRepeat].Text)...)
	keys, problem := parseKeys(v[fieldUserKeys].Text)
	if problem != "" {
		ps = append(ps, m.bad(fieldUserKeys, problem, maxFormKeys))
	}
	if len(ps) > 0 {
		f.problems(ps...)
		return nil
	}
	admin, docker := v[fieldUserAdmin].Choice == choiceYes, v[fieldUserDocker].Choice == choiceYes
	args := []string{name}
	if admin {
		args = append(args, "admin")
	}
	if docker {
		args = append(args, "docker")
	}
	var risky bool
	for _, g := range v[fieldUserGroups].Selected {
		args = append(args, "group="+string(g))
		risky = risky || m.systemGroup(string(g))
	}
	secret, err := json.Marshal(hostchange.CreateInput{Password: v[fieldUserFirst].Text, Keys: keys})
	if err != nil {
		f.problem(err.Error(), m.text.Lookup(textFormFix))
		return nil
	}
	a := hostAsk{id: hostchange.ActionUserCreate, args: args, secret: secret, mask: [][]byte{[]byte(v[fieldUserFirst].Text)},
		question: m.text.Lookup(textAskCreateUser, name)}
	if admin || docker {
		a.note = m.text.Lookup(textHostPowerNote)
	}
	if risky {
		a.risk = m.text.Lookup(textRiskGroup)
	}
	if m.askHost(a, f) {
		m.dropForm()
	}
	return nil
}

// --- change ---

// The changes the choice offers, by the id of its row.
const (
	chgEnable       = "enable"
	chgAdminOn      = "admin-on"
	chgAdminOff     = "admin-off"
	chgDockerOn     = "docker-on"
	chgDockerOff    = "docker-off"
	chgAddGroups    = "add-groups"
	chgRemoveGroups = "remove-groups"
	chgSecret       = "password"
	chgAddKey       = "add-key"
	chgRemoveKey    = "remove-key"
	chgLock         = "lock"
	chgUnlock       = "unlock"
)

var changeTexts = map[string]kittui.TextID{
	chgEnable: textChgEnable, chgAdminOn: textChgAdminOn, chgAdminOff: textChgAdminOff, chgDockerOn: textChgDockerOn,
	chgDockerOff: textChgDockerOff, chgAddGroups: textChgAddGroups, chgRemoveGroups: textChgRemoveGroups,
	chgSecret: textChgSecret, chgAddKey: textChgAddKey, chgRemoveKey: textChgRemoveKey, chgLock: textChgLock,
	chgUnlock: textChgUnlock,
}

// changesFor are the changes the user offers, in a fixed order.
func (m *Model) changesFor(u hostadmin.User) []string {
	var ids []string
	if u.Admin && !u.Enabled {
		ids = append(ids, chgEnable)
	}
	if u.Admin {
		ids = append(ids, chgAdminOff)
	} else {
		ids = append(ids, chgAdminOn)
	}
	if u.Docker {
		ids = append(ids, chgDockerOff)
	} else {
		ids = append(ids, chgDockerOn)
	}
	if len(m.groupChoices(true, func(g string) bool { return g == u.PrimaryGroup || slices.Contains(u.Groups, g) })) > 0 {
		ids = append(ids, chgAddGroups)
	}
	if len(furtherGroupsOf(u)) > 0 {
		ids = append(ids, chgRemoveGroups)
	}
	ids = append(ids, chgSecret, chgAddKey)
	if u.SSHKeys == nil || len(u.SSHKeys) > 0 {
		ids = append(ids, chgRemoveKey)
	}
	// An unread lock state offers both; a password lock without the end of the account offers both too, so it
	// can be completed or released.
	fully := u.Locked != nil && *u.Locked && u.Expired != nil && *u.Expired
	clean := u.Locked != nil && !*u.Locked && (u.Expired == nil || !*u.Expired)
	if !fully {
		ids = append(ids, chgLock)
	}
	if !clean {
		ids = append(ids, chgUnlock)
	}
	return ids
}

// openChanges opens the choice of a change for the marked user.
func (m *Model) openChanges() tea.Cmd {
	u, ok := m.selectedUser()
	if !ok {
		return nil
	}
	list, err := kittui.NewList(kittui.ListSpec{Columns: []kittui.Column{{Title: textColChange}}, Keys: listKeys(),
		Search: kittui.SearchByKey, Wrap: true, Empty: textUsersEmpty}, m.keys, m.text)
	if err != nil {
		m.failure = m.text.Lookup(kittui.TextError, err.Error())
		return nil
	}
	var items []kittui.ListItem
	for _, id := range m.changesFor(u) {
		items = append(items, kittui.ListItem{ID: kittui.ItemID(id), Cells: []string{m.text.Lookup(changeTexts[id])}})
	}
	if err := list.SetItems(items); err != nil {
		m.failure = m.text.Lookup(kittui.TextError, err.Error())
		return nil
	}
	list.SetFocused(true)
	m.sys.changes, m.sys.changeFor = list, u
	m.setZone(kittui.ZoneRows)
	return nil
}

// riskOf names why a change of the user and of the groups it involves asks in red: root, a system account or a
// system group. It is empty for a change of regular accounts and groups only.
func (m *Model) riskOf(u hostadmin.User, groups []string) string {
	var parts []string
	if u.UID == 0 || u.Name == "root" {
		parts = append(parts, m.text.Lookup(textRiskRoot))
	} else if u.System {
		parts = append(parts, m.text.Lookup(textRiskAccount))
	}
	if slices.ContainsFunc(groups, m.systemGroup) {
		parts = append(parts, m.text.Lookup(textRiskGroup))
	}
	return strings.Join(parts, " ")
}

// selfGuard gives the early message of the rules the root action enforces for the person behind the interface
// and for the last administrator; empty where none applies.
func (m *Model) selfGuard(u hostadmin.User, self kittui.TextID, admin bool) string {
	if uid := m.humanUID(); uid != 0 && uid == u.UID {
		return m.text.Lookup(self)
	}
	if !admin || !u.Admin || !u.Enabled {
		return ""
	}
	for _, o := range m.sys.users {
		if o.Name != u.Name && o.Admin && o.Enabled && (o.Expired == nil || !*o.Expired) && (o.Locked == nil || !*o.Locked) {
			return ""
		}
	}
	return m.text.Lookup(textLastAdmin, u.Name)
}

// pickChange runs the change chosen for the user: a form where it needs input, else the confirmation.
func (m *Model) pickChange(id string) tea.Cmd {
	s := m.sys
	u := s.changeFor
	s.changes = nil
	t := m.text
	a := hostAsk{args: []string{u.Name}, risk: m.riskOf(u, nil)}
	switch id {
	case chgEnable:
		a.id, a.question, a.self = hostchange.ActionUserEnable, t.Lookup(textAskEnable, u.Name), u.UID == m.humanUID()
	case chgAdminOn:
		a.id, a.args, a.question, a.note = hostchange.ActionUserSetAdmin, append(a.args, "on"), t.Lookup(textAskAdminOn, u.Name), t.Lookup(textHostPowerNote)
		a.self = u.UID == m.humanUID()
	case chgAdminOff:
		if msg := m.selfGuard(u, textSelfAdmin, true); msg != "" {
			m.warn(msg)
			return nil
		}
		a.id, a.args, a.question = hostchange.ActionUserSetAdmin, append(a.args, "off"), t.Lookup(textAskAdminOff, u.Name)
		if !u.Enabled {
			a.args = append(a.args, "without-qhost")
		}
	case chgDockerOn:
		a.id, a.args, a.question, a.note = hostchange.ActionUserSetDocker, append(a.args, "on"), t.Lookup(textAskDockerOn, u.Name), t.Lookup(textHostPowerNote)
	case chgDockerOff:
		a.id, a.args, a.question = hostchange.ActionUserSetDocker, append(a.args, "off"), t.Lookup(textAskDockerOff, u.Name)
	case chgLock:
		if msg := m.selfGuard(u, textSelfLock, true); msg != "" {
			m.warn(msg)
			return nil
		}
		a.id, a.question, a.note = hostchange.ActionUserLock, t.Lookup(textAskLock, u.Name), t.Lookup(textLockStays)
	case chgUnlock:
		a.id, a.question = hostchange.ActionUserUnlock, t.Lookup(textAskUnlock, u.Name)
	case chgAddGroups, chgRemoveGroups:
		return m.openForm(m.userGroupsForm(u, id == chgAddGroups))
	case chgSecret:
		return m.openForm(m.userForm(formUserSecret, textTitleUserSecret, userForm{user: u}, []kittui.FormField{
			maskedField(fieldUserFirst, textFieldNewSecret, textHintNewSecret),
			maskedField(fieldUserRepeat, textFieldRepeat, textHintRepeat),
			choiceField(fieldUserMust, textFieldMust, textHintMust, []kittui.Choice{choice(choiceYes, textYes), choice(choiceNo, textNo)}, choiceYes),
		}))
	case chgAddKey:
		return m.openForm(m.userForm(formUserKeyAdd, textTitleAddKey, userForm{user: u}, []kittui.FormField{
			{ID: fieldUserKeys, Kind: kittui.FieldMaskedBlock, Label: textFieldKey, Hint: textHintKey},
		}))
	case chgRemoveKey:
		if u.SSHKeys == nil {
			return m.readUserKeys(u, func(u hostadmin.User) tea.Cmd { return m.openForm(m.userKeyRemoveForm(u)) })
		}
		return m.openForm(m.userKeyRemoveForm(u))
	default:
		return nil
	}
	m.askHost(a, nil)
	return nil
}

func (m *Model) userGroupsForm(u hostadmin.User, add bool) *settingsForm {
	title, label, hint := textTitleAddGroups, textFieldAddTo, textHintAddTo
	var choices []kittui.Choice
	if add {
		choices = m.groupChoices(true, func(g string) bool { return g == u.PrimaryGroup || slices.Contains(u.Groups, g) })
	} else {
		title, label, hint = textTitleRemoveGroups, textFieldRemoveFrom, textHintRemoveFrom
		for _, g := range furtherGroupsOf(u) {
			choices = append(choices, kittui.Choice{ID: kittui.ChoiceID(g), Label: dataLabel(g)})
		}
	}
	return m.userForm(formUserGroups, title, userForm{user: u, add: add}, []kittui.FormField{
		{ID: fieldUserGroups, Kind: kittui.FieldMultiple, Label: label, Hint: hint, Choices: choices},
	})
}

func (m *Model) userKeyRemoveForm(u hostadmin.User) *settingsForm {
	if len(u.SSHKeys) == 0 {
		m.warn(m.text.Lookup(textNoKeys, u.Name))
		return nil
	}
	var choices []kittui.Choice
	for _, k := range u.SSHKeys {
		choices = append(choices, kittui.Choice{ID: kittui.ChoiceID(k.Fingerprint), Label: dataLabel(strings.TrimSpace(k.Fingerprint + " " + k.Comment))})
	}
	return m.userForm(formUserKeyRemove, textTitleRemoveKey, userForm{user: u}, []kittui.FormField{
		choiceField(fieldUserKeyOf, textFieldKeyOf, textHintKeyOf, choices, string(choices[0].ID)),
	})
}

func (m *Model) submitUserGroups(f *settingsForm, v map[kittui.FieldID]kittui.FormValue) tea.Cmd {
	u, t := f.user.user, m.text
	var groups []string
	for _, g := range v[fieldUserGroups].Selected {
		groups = append(groups, string(g))
	}
	if len(groups) == 0 {
		f.problems(m.bad(fieldUserGroups, textProblemTickOne))
		return nil
	}
	a := hostAsk{id: hostchange.ActionUserAddGroups, args: append([]string{u.Name}, groups...),
		question: t.Lookup(textAskAddGroups, u.Name, strings.Join(groups, ", ")), risk: m.riskOf(u, groups)}
	if !f.user.add {
		a.id, a.question = hostchange.ActionUserRemoveGroups, t.Lookup(textAskRemoveGroups, u.Name, strings.Join(groups, ", "))
	}
	if m.askHost(a, f) {
		m.dropForm()
	}
	return nil
}

func (m *Model) submitUserSecret(f *settingsForm, v map[kittui.FieldID]kittui.FormValue) tea.Cmd {
	u := f.user.user
	if ps := m.secretProblems(v[fieldUserFirst].Text, v[fieldUserRepeat].Text); len(ps) > 0 {
		f.problems(ps...)
		return nil
	}
	a := hostAsk{id: hostchange.ActionUserSetPassword, args: []string{u.Name}, secret: []byte(v[fieldUserFirst].Text),
		question: m.text.Lookup(textAskSecret, u.Name), risk: m.riskOf(u, nil)}
	if v[fieldUserMust].Choice == choiceYes {
		a.args, a.question = append(a.args, "must-change"), m.text.Lookup(textAskSecretMust, u.Name)
	}
	if m.askHost(a, f) {
		m.dropForm()
	}
	return nil
}

func (m *Model) submitUserKeyAdd(f *settingsForm, v map[kittui.FieldID]kittui.FormValue) tea.Cmd {
	u := f.user.user
	keys, problem := parseKeys(v[fieldUserKeys].Text)
	if problem == "" && len(keys) != 1 {
		problem = textProblemKeyOne
	}
	if problem != "" {
		f.problems(m.bad(fieldUserKeys, problem, maxFormKeys))
		return nil
	}
	a := hostAsk{id: hostchange.ActionUserAddKey, args: []string{u.Name}, secret: []byte(keys[0]),
		question: m.text.Lookup(textAskAddKey, u.Name), risk: m.riskOf(u, nil)}
	if m.askHost(a, f) {
		m.dropForm()
	}
	return nil
}

func (m *Model) submitUserKeyRemove(f *settingsForm, v map[kittui.FieldID]kittui.FormValue) tea.Cmd {
	u, fp := f.user.user, string(v[fieldUserKeyOf].Choice)
	a := hostAsk{id: hostchange.ActionUserRemoveKey, args: []string{u.Name, fp},
		question: m.text.Lookup(textAskRemoveKey, fp, u.Name), risk: m.riskOf(u, nil)}
	if m.askHost(a, f) {
		m.dropForm()
	}
	return nil
}

// --- delete ---

// The choices for the home of a deleted user.
const (
	homeKeep   = "keep"
	homeDelete = "delete"
)

// openUserDelete asks whether the home of the marked user goes too; F2 leads to the red confirmation. Keeping
// the home is the default and the only delete that can be undone.
func (m *Model) openUserDelete() tea.Cmd {
	u, ok := m.selectedUser()
	if !ok {
		return nil
	}
	if msg := m.selfGuard(u, textSelfDelete, true); msg != "" {
		m.warn(msg)
		return nil
	}
	f := m.userForm(formUserDelete, textTitleDeleteUser, userForm{user: u}, []kittui.FormField{
		choiceField(fieldUserHome, textFieldHome, textHintHome, []kittui.Choice{choice(homeKeep, textHomeKeep), choice(homeDelete, textHomeDelete)}, homeKeep),
	})
	if f != nil {
		f.form.SetLead(textLeadDeleteUser)
	}
	return m.openForm(f)
}

func (m *Model) submitUserDelete(f *settingsForm, v map[kittui.FieldID]kittui.FormValue) tea.Cmd {
	u, t := f.user.user, m.text
	a := hostAsk{id: hostchange.ActionUserDelete, args: []string{u.Name}, question: t.Lookup(textAskDeleteKeep, u.Name),
		risk: m.riskOf(u, nil), danger: true, note: t.Lookup(textDeleteKeeps)}
	if v[fieldUserHome].Choice == homeDelete {
		a.args, a.question, a.note = append(a.args, "with-home"), t.Lookup(textAskDeleteHome, u.Name), ""
		a.final = strings.TrimSpace(m.homeRiskText(u) + " " + t.Lookup(textDeleteFinal, u.Home))
	}
	if m.askHost(a, f) {
		m.dropForm()
	}
	return nil
}

// homeRiskText says each way deleting the home of u reaches beyond its own files, in one plain sentence each. It
// reads only what the user running the interface may read.
func (m *Model) homeRiskText(u hostadmin.User) string {
	t := m.text
	risks, err := hostchange.HomeRisks(m.accountsReader(), u.Name, u.Home)
	var parts []string
	for _, r := range risks {
		who := strings.Join(r.Who, ", ")
		switch r.Kind {
		case hostchange.HomeSystemPath:
			parts = append(parts, t.Lookup(textHomeSystem, u.Home))
		case hostchange.HomeHoldsHome:
			parts = append(parts, t.Lookup(textHomeHolds, u.Home, who))
		case hostchange.HomeLink:
			parts = append(parts, t.Lookup(textHomeLink, u.Home))
		case hostchange.HomeNotDirectory:
			parts = append(parts, t.Lookup(textHomeNotDir, u.Home))
		case hostchange.HomeForeignOwner:
			parts = append(parts, t.Lookup(textHomeOwner, u.Home, who))
		}
	}
	if err != nil {
		parts = append(parts, t.Lookup(textHomeUnread, u.Home, err.Error()))
	}
	return strings.Join(parts, " ")
}
