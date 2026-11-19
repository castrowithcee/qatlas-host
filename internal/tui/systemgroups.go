package tui

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/host/hostadmin"
	"github.com/castrowithcee/qatlas-host/internal/host/hostchange"
)

// Groups are created, renamed and deleted on System > Groups with a, n and x; e (or enter) adds and removes
// members of the marked group, several at once with one confirmation. Each form checks what it can before
// anything leaves it; the root command checks it all again and stays the authority.

// The forms of the groups page.
const (
	formGroupCreate  formKind = formUserDelete + 1
	formGroupMembers formKind = formGroupCreate + 1
	formGroupRename  formKind = formGroupMembers + 1
)

// The fields of the group forms.
const (
	fieldGroupName   = "name"
	fieldGroupAdd    = "add"
	fieldGroupRemove = "remove"
)

// protectedGroup is a group that is neither renamed nor deleted: administrator rights and access to qhost rest
// on its name.
func protectedGroup(name string) bool {
	return name == hostadmin.GroupAdmin || name == hostadmin.GroupQhost
}

// selectedGroup is the group under the cursor of the groups page.
func (m *Model) selectedGroup() (hostadmin.Group, bool) {
	id, ok := m.sys.groupList.Selected()
	if !ok {
		return hostadmin.Group{}, false
	}
	return m.findGroup(string(id))
}

// findGroup is the group of the last read with the name.
func (m *Model) findGroup(name string) (hostadmin.Group, bool) {
	for _, g := range m.sys.groups {
		if g.Name == name {
			return g, true
		}
	}
	return hostadmin.Group{}, false
}

// groupRisk names why a change of the group asks in red: the group root, docker or a system group. It is empty
// for a regular group.
func (m *Model) groupRisk(g hostadmin.Group) string {
	var parts []string
	switch g.Name {
	case "root":
		parts = append(parts, m.text.Lookup(textRiskGroupRoot))
	case hostadmin.GroupDocker:
		parts = append(parts, m.text.Lookup(textRiskGroupDocker))
	}
	if g.System {
		parts = append(parts, m.text.Lookup(textRiskGroup))
	}
	return strings.Join(parts, " ")
}

// groupNameProblem checks a typed group name the way the root action does.
func (m *Model) groupNameProblem(name string) []kittui.FormProblem {
	switch _, exists := m.findGroup(name); {
	case !accountName.MatchString(name):
		return []kittui.FormProblem{m.bad(fieldGroupName, textProblemGroupName)}
	case exists:
		return []kittui.FormProblem{m.bad(fieldGroupName, textProblemGroupTaken)}
	}
	return nil
}

func (m *Model) groupForm(kind formKind, title kittui.TextID, g hostadmin.Group, lead kittui.TextID, fields []kittui.FormField) *settingsForm {
	f := m.newSettingsForm(kind, title, fields)
	if f != nil {
		f.group, f.titleArg = &g, g.Name
		if lead != "" {
			f.form.SetLead(lead)
		}
	}
	return f
}

// --- create ---

func (m *Model) groupCreateForm() *settingsForm {
	return m.groupForm(formGroupCreate, textTitleNewGroup, hostadmin.Group{}, textLeadNewGroup, []kittui.FormField{
		textField(fieldGroupName, textFieldUserName, textHintUserName, ""),
	})
}

func (m *Model) submitGroupCreate(f *settingsForm, v map[kittui.FieldID]kittui.FormValue) tea.Cmd {
	name := strings.TrimSpace(v[fieldGroupName].Text)
	if ps := m.groupNameProblem(name); len(ps) > 0 {
		f.problems(ps...)
		return nil
	}
	if m.askHost(hostAsk{id: hostchange.ActionGroupCreate, args: []string{name}, question: m.text.Lookup(textAskCreateGroup, name)}, f) {
		m.dropForm()
	}
	return nil
}

// --- members ---

// memberChoices are the users the members form offers for the group: those outside it to add, the listed
// members to remove. A user whose primary group it is is in neither, because that membership cannot change.
func (m *Model) memberChoices(g hostadmin.Group) (add, remove []string) {
	for _, u := range m.sys.users {
		switch {
		case u.PrimaryGroup == g.Name:
		case slices.Contains(g.Members, u.Name):
			remove = append(remove, u.Name)
		default:
			add = append(add, u.Name)
		}
	}
	slices.Sort(add)
	slices.Sort(remove)
	return add, remove
}

func namedChoices(names []string) []kittui.Choice {
	choices := make([]kittui.Choice, len(names))
	for i, n := range names {
		choices[i] = kittui.Choice{ID: kittui.ChoiceID(n), Label: dataLabel(n)}
	}
	return choices
}

// openGroupMembers opens the form that adds and removes members of the marked group in one step. A list with
// nothing to offer is left out of the form.
func (m *Model) openGroupMembers() tea.Cmd {
	g, ok := m.selectedGroup()
	if !ok {
		return nil
	}
	add, remove := m.memberChoices(g)
	if len(add)+len(remove) == 0 {
		m.warn(m.text.Lookup(textNoMemberChange, g.Name))
		return nil
	}
	var fields []kittui.FormField
	if len(add) > 0 {
		fields = append(fields, kittui.FormField{ID: fieldGroupAdd, Kind: kittui.FieldMultiple, Label: textFieldMembersAdd,
			Hint: textHintMembers, Choices: namedChoices(add)})
	}
	if len(remove) > 0 {
		fields = append(fields, kittui.FormField{ID: fieldGroupRemove, Kind: kittui.FieldMultiple, Label: textFieldMembersRemove,
			Hint: textHintMembers, Choices: namedChoices(remove)})
	}
	lead := textLeadMembers
	switch g.Name {
	case hostadmin.GroupAdmin:
		lead = textLeadMembersSudo
	case hostadmin.GroupDocker:
		lead = textLeadMembersDocker
	}
	return m.openForm(m.groupForm(formGroupMembers, textTitleMembers, g, lead, fields))
}

// guardMembers gives the early message of the rules the root action enforces for taking users out of sudo or
// qhost: nobody does it to themselves, and an administrator with access to qhost remains.
func (m *Model) guardMembers(g hostadmin.Group, added, removed []string) string {
	if !protectedGroup(g.Name) || len(removed) == 0 {
		return ""
	}
	uid := m.humanUID()
	for _, u := range m.sys.users {
		if uid != 0 && u.UID == uid && slices.Contains(removed, u.Name) {
			return m.text.Lookup(textSelfAdmin)
		}
	}
	for _, u := range m.sys.users {
		sudo, qhost := u.Admin, u.Enabled
		if slices.Contains(removed, u.Name) {
			qhost = false
			sudo = sudo && g.Name != hostadmin.GroupAdmin
		}
		if slices.Contains(added, u.Name) {
			sudo, qhost = sudo || g.Name == hostadmin.GroupAdmin, true
		}
		if sudo && qhost && (u.Expired == nil || !*u.Expired) && (u.Locked == nil || !*u.Locked) {
			return ""
		}
	}
	return m.text.Lookup(textLastAdmin, strings.Join(removed, ", "))
}

// membersRisk adds to the risk of the group what the users of the change carry: root or a system account.
func (m *Model) membersRisk(g hostadmin.Group, names []string) string {
	parts := []string{m.groupRisk(g)}
	var root, system bool
	for _, n := range names {
		if u, ok := m.findUser(n); ok {
			root = root || u.UID == 0 || u.Name == "root"
			system = system || u.System
		}
	}
	if root {
		parts = append(parts, m.text.Lookup(textRiskRoot))
	}
	if system {
		parts = append(parts, m.text.Lookup(textRiskMembers))
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

func (m *Model) submitGroupMembers(f *settingsForm, v map[kittui.FieldID]kittui.FormValue) tea.Cmd {
	g, t := *f.group, m.text
	var added, removed []string
	for _, id := range v[fieldGroupAdd].Selected {
		added = append(added, string(id))
	}
	for _, id := range v[fieldGroupRemove].Selected {
		removed = append(removed, string(id))
	}
	if len(added)+len(removed) == 0 {
		field := fieldGroupAdd
		if _, ok := v[fieldGroupAdd]; !ok {
			field = fieldGroupRemove
		}
		f.problems(m.bad(field, textProblemTickUser))
		return nil
	}
	if msg := m.guardMembers(g, added, removed); msg != "" {
		f.problem(msg, t.Lookup(textFormFix))
		return nil
	}
	a := hostAsk{id: hostchange.ActionGroupSetMembers, args: []string{g.Name}, risk: m.membersRisk(g, append(slices.Clone(added), removed...))}
	var sentences []string
	if len(added) > 0 {
		sentences = append(sentences, t.Lookup(textAskMembersAdd, strings.Join(added, ", "), g.Name))
	}
	if len(removed) > 0 {
		sentences = append(sentences, t.Lookup(textAskMembersRemove, strings.Join(removed, ", "), g.Name))
	}
	a.question = strings.Join(sentences, " ")
	for _, n := range added {
		a.args = append(a.args, "+"+n)
		if u, ok := m.findUser(n); ok && u.UID == m.humanUID() && protectedGroup(g.Name) {
			a.self = true
		}
	}
	for _, n := range removed {
		// A member of sudo who is not in qhost has no access to qhost to take; the root action cannot decide that
		// itself, because the commands of a plan do not depend on the host.
		if u, ok := m.findUser(n); ok && g.Name == hostadmin.GroupAdmin && !u.Enabled {
			n += ":without-qhost"
		}
		a.args = append(a.args, "-"+n)
	}
	// The risk of the group docker names its power already.
	if g.Name == hostadmin.GroupAdmin {
		a.note = t.Lookup(textMembersSudoNote)
		if len(added) > 0 {
			a.note += " " + t.Lookup(textHostPowerNote)
		}
	}
	if m.askHost(a, f) {
		m.dropForm()
	}
	return nil
}

// --- rename ---

// keepProtected warns why a protected group is left alone and reports whether it did.
func (m *Model) keepProtected(g hostadmin.Group, why kittui.TextID) bool {
	if !protectedGroup(g.Name) {
		return false
	}
	m.warn(m.text.Lookup(why, g.Name))
	return true
}

func (m *Model) openGroupRename() tea.Cmd {
	g, ok := m.selectedGroup()
	if !ok || m.keepProtected(g, textGroupKeepName) {
		return nil
	}
	return m.openForm(m.groupForm(formGroupRename, textTitleRenameGroup, g, textLeadRenameGroup, []kittui.FormField{
		textField(fieldGroupName, textFieldNewName, textHintUserName, ""),
	}))
}

func (m *Model) submitGroupRename(f *settingsForm, v map[kittui.FieldID]kittui.FormValue) tea.Cmd {
	g, name := *f.group, strings.TrimSpace(v[fieldGroupName].Text)
	if ps := m.groupNameProblem(name); len(ps) > 0 {
		f.problems(ps...)
		return nil
	}
	// A rename always asks in red: what addresses the group by its name keeps the old one.
	a := hostAsk{id: hostchange.ActionGroupRename, args: []string{g.Name, name}, question: m.text.Lookup(textAskRenameGroup, g.Name, name),
		risk: strings.TrimSpace(m.text.Lookup(textRiskRename) + " " + m.groupRisk(g))}
	if m.askHost(a, f) {
		m.dropForm()
	}
	return nil
}

// --- delete ---

// askGroupDelete asks to delete the marked group. The delete can be undone, so the question names no loss; the
// confirming answer is red all the same.
func (m *Model) askGroupDelete() tea.Cmd {
	g, ok := m.selectedGroup()
	if !ok || m.keepProtected(g, textGroupKeepDelete) {
		return nil
	}
	var primary []string
	for _, u := range m.sys.users {
		if u.PrimaryGroup == g.Name {
			primary = append(primary, u.Name)
		}
	}
	if len(primary) > 0 {
		m.warn(m.text.Lookup(textGroupPrimary, g.Name, strings.Join(primary, ", ")))
		return nil
	}
	m.askHost(hostAsk{id: hostchange.ActionGroupDelete, args: []string{g.Name}, question: m.text.Lookup(textAskDeleteGroup, g.Name),
		risk: m.groupRisk(g), danger: true}, nil)
	return nil
}
