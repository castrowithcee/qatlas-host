package tui

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/host/hostadmin"
	"github.com/castrowithcee/qatlas-host/internal/host/hostchange"
)

// areaSystem holds the users and groups of the host. It reads them from the account files of the host, not over
// a service, so it works while the deploy service is not set up; it is the last area so the keys of the others
// stay.
const areaSystem kittui.AreaID = "system"

// systemPage is one page of the System area, shown under its sub-tab.
type systemPage string

const (
	pageUsers  systemPage = "users"
	pageGroups systemPage = "groups"
)

type systemEntry struct {
	page  systemPage
	title kittui.TextID
}

// systemPages are the pages in the order of the sub-tabs, with the text of their title.
var systemPages = []systemEntry{
	{pageUsers, textPageUsers},
	{pageGroups, textPageGroups},
}

// systemTabs are the sub-tabs of the System area.
func systemTabs() []kittui.Tab {
	tabs := make([]kittui.Tab, len(systemPages))
	for i, p := range systemPages {
		tabs[i] = kittui.Tab{ID: kittui.TabID(p.page), Title: p.title}
	}
	return tabs
}

// groupRegular is the group key of the regular accounts and groups. System accounts and groups carry no key: the
// list puts that group last, under the title of ListSpec.NoGroup.
const groupRegular = "regular"

// systemState is what the System area holds. page is the sub-tab shown; it stays while the area is left. gen
// separates the answer of a read that was overtaken from the current one.
type systemState struct {
	page            systemPage
	gen             int
	loading, loaded bool
	err             error
	users           []hostadmin.User
	groups          []hostadmin.Group
	userList        *kittui.List
	groupList       *kittui.List
	userByID        map[kittui.ItemID]hostadmin.User
	// busy is set while a change runs in the background after sudo took the password on the terminal.
	busy bool
	// changes is the choice of a change for the user changeFor, nil while none is open.
	changes   *kittui.List
	changeFor hostadmin.User
}

// systemMsg carries a read of the account files.
type systemMsg struct {
	gen    int
	users  []hostadmin.User
	groups []hostadmin.Group
	err    error
}

func newSystem(keys *kittui.Keymap, text kittui.Translator) (*systemState, error) {
	s := &systemState{page: pageUsers, userByID: map[kittui.ItemID]hostadmin.User{}}
	var err error
	s.userList, err = kittui.NewList(kittui.ListSpec{Columns: []kittui.Column{
		{Title: textColName}, {Title: textColUID}, {Title: textColAdmin}, {Title: textColDocker},
		{Title: textColLogin}, {Title: textColGroups, Flex: true, Optional: true}},
		Keys: listKeys(), Search: kittui.SearchByKey, Empty: textUsersEmpty, NoGroup: textGroupSystemUsers}, keys, text)
	if err != nil {
		return nil, err
	}
	s.groupList, err = kittui.NewList(kittui.ListSpec{Columns: []kittui.Column{
		{Title: textColName}, {Title: textColGID}, {Title: textColMembers, Flex: true}},
		Keys: listKeys(), Search: kittui.SearchByKey, Empty: textGroupsEmpty, NoGroup: textGroupSystemGroups}, keys, text)
	if err != nil {
		return nil, err
	}
	for _, l := range []*kittui.List{s.userList, s.groupList} {
		l.SetFocused(true)
		l.SetGrouped(true)
	}
	return s, nil
}

// activeList is the list the System area shows: the choice of a change while it is open, else the list of the
// page.
func (s *systemState) activeList() *kittui.List {
	switch {
	case s.changes != nil:
		return s.changes
	case s.page == pageGroups:
		return s.groupList
	}
	return s.userList
}

// accountsReader is what the area reads: the real account files, privileged for root, unless a test set others.
func (m *Model) accountsReader() *hostadmin.Host {
	if m.accounts != nil {
		return m.accounts
	}
	return hostadmin.NewHost(m.euid() == 0)
}

// hostRegistry describes the changes; it reads nothing from the host.
func (m *Model) hostRegistry() *hostchange.Registry {
	if m.hostReg == nil {
		m.hostReg = hostchange.Default()
	}
	return m.hostReg
}

// loadSystem reads the users and the groups unless a read is under way; force reads also what was read before.
func (m *Model) loadSystem(force bool) tea.Cmd {
	s := m.sys
	if s.loading || s.loaded && !force {
		return nil
	}
	s.loading = true
	s.gen++
	gen, h := s.gen, m.accountsReader()
	return func() tea.Msg {
		users, err := h.Users()
		if err != nil {
			return systemMsg{gen: gen, err: err}
		}
		groups, err := h.Groups()
		return systemMsg{gen: gen, users: users, groups: groups, err: err}
	}
}

// applySystem takes a read. A failed read keeps what was read before.
func (m *Model) applySystem(msg systemMsg) {
	s := m.sys
	if msg.gen != s.gen {
		return
	}
	s.loading, s.err = false, msg.err
	if msg.err != nil {
		return
	}
	s.loaded, s.users, s.groups = true, msg.users, msg.groups
	m.fillSystem()
}

// fillSystem sets the rows of both lists from what was read last.
func (m *Model) fillSystem() {
	s, t := m.sys, m.text
	clear(s.userByID)
	users := make([]kittui.ListItem, 0, len(s.users))
	for _, u := range s.users {
		id := kittui.ItemID(u.Name)
		s.userByID[id] = u
		users = append(users, kittui.ListItem{ID: id, Group: accountGroup(u.System), Cells: []string{
			u.Name, strconv.Itoa(u.UID), m.adminText(u), yesNo(t, u.Docker), m.loginText(u),
			strings.Join(furtherGroupsOf(u), ","),
		}})
	}
	groups := make([]kittui.ListItem, 0, len(s.groups))
	for _, g := range s.groups {
		groups = append(groups, kittui.ListItem{ID: kittui.ItemID(g.Name), Group: accountGroup(g.System), Cells: []string{
			g.Name, strconv.Itoa(g.GID), strings.Join(g.Members, ","),
		}})
	}
	for _, step := range []func() error{
		func() error { return s.userList.SetItems(users) },
		func() error {
			return s.userList.SetGroups([]kittui.ListGroup{{Key: groupRegular, Title: t.Lookup(textGroupRegularUsers)}})
		},
		func() error { return s.groupList.SetItems(groups) },
		func() error {
			return s.groupList.SetGroups([]kittui.ListGroup{{Key: groupRegular, Title: t.Lookup(textGroupRegularGroups)}})
		},
	} {
		if err := step(); err != nil {
			m.failure = t.Lookup(kittui.TextError, err.Error())
			return
		}
	}
}

func accountGroup(system bool) string {
	if system {
		return ""
	}
	return groupRegular
}

// fixedGroup is a group with a switch of its own, never one of the further groups.
func fixedGroup(name string) bool {
	return name == hostadmin.GroupAdmin || name == hostadmin.GroupDocker || name == hostadmin.GroupQhost
}

// furtherGroupsOf are the groups of a user besides its primary group and the groups of the switches.
func furtherGroupsOf(u hostadmin.User) []string {
	var gs []string
	for _, g := range u.Groups {
		if g != u.PrimaryGroup && !fixedGroup(g) {
			gs = append(gs, g)
		}
	}
	slices.Sort(gs)
	return gs
}

// adminText is the Admin cell: an administrator without the group qhost has no access to qhost yet.
func (m *Model) adminText(u hostadmin.User) string {
	switch {
	case u.Admin && !u.Enabled:
		return m.text.Lookup(textAdminNoAccess)
	case u.Admin:
		return m.text.Lookup(textYes)
	}
	return m.text.Lookup(textNo)
}

// loginText says whether the account can log in; only root can read it.
func (m *Model) loginText(u hostadmin.User) string {
	switch {
	case u.Locked == nil:
		return m.text.Lookup(textLoginUnknown)
	case *u.Locked:
		return m.text.Lookup(textLoginLocked)
	case u.Expired != nil && *u.Expired:
		return m.text.Lookup(textLoginExpired)
	}
	return m.text.Lookup(textLoginActive)
}

// selectedUser is the user under the cursor of the users page.
func (m *Model) selectedUser() (hostadmin.User, bool) {
	id, ok := m.sys.userList.Selected()
	u, found := m.sys.userByID[id]
	return u, ok && found
}

// findUser is the user of the last read with the name.
func (m *Model) findUser(name string) (hostadmin.User, bool) {
	for _, u := range m.sys.users {
		if u.Name == name {
			return u, true
		}
	}
	return hostadmin.User{}, false
}

// systemGroup reports whether the group read last is a system group; an unknown group counts as one.
func (m *Model) systemGroup(name string) bool {
	for _, g := range m.sys.groups {
		if g.Name == name {
			return g.System
		}
	}
	return true
}

// selectSystemTab shows the page of a sub-tab and gives the focus to the sub-tab line.
func (m *Model) selectSystemTab(id kittui.TabID) tea.Cmd {
	page := systemPage(id)
	if !slices.ContainsFunc(systemPages, func(e systemEntry) bool { return e.page == page }) {
		return nil
	}
	m.setZone(kittui.ZoneSubtabs)
	m.sys.page, m.sys.changes = page, nil
	return nil
}

// stepSystemTab shows the sub-tab step places after the one shown, with wrap-around.
func (m *Model) stepSystemTab(step int) tea.Cmd {
	for _, a := range areas() {
		if a.ID == areaSystem {
			return m.selectSystemTab(kittui.StepTab(a, kittui.TabID(m.sys.page), step))
		}
	}
	return nil
}

// systemInput is the input focus of the System area.
func (m *Model) systemInput() kittui.InputFocus {
	if f := m.set.form; f != nil {
		return f.input()
	}
	if f := m.focus(); f == kittui.ZoneTabs || f == kittui.ZoneSubtabs {
		return kittui.InputNavigation
	}
	return m.sys.activeList().Input()
}

// systemTyped gives a key to the list shown; enter on a user opens the choice of a change, on a change picks it.
func (m *Model) systemTyped(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	s := m.sys
	open, handled := s.activeList().Update(msg)
	if open == "" {
		return nil, handled
	}
	switch {
	case s.changes != nil:
		return m.pickChange(string(open)), true
	case s.page == pageUsers:
		return m.openChanges(), true
	}
	return nil, handled
}

// systemAction acts on a letter of the System area; handled is false for a key that is none here.
func (m *Model) systemAction(id kittui.ActionID) (tea.Cmd, bool) {
	s := m.sys
	if s.page != pageUsers || s.changes != nil {
		return nil, false
	}
	if s.busy {
		m.warn(m.text.Lookup(textActionBusy))
		return nil, true
	}
	switch id {
	case actAdd:
		return m.openForm(m.userCreateForm()), true
	case actEdit:
		return m.openChanges(), true
	case actRemove:
		return m.openUserDelete(), true
	}
	return nil, false
}

// systemBack is esc in the System area: it closes the choice of a change, clears a search, then leads to the
// sub-tab line.
func (m *Model) systemBack(msg tea.KeyPressMsg) {
	s := m.sys
	switch m.focus() {
	case kittui.ZoneTabs:
	case kittui.ZoneSubtabs:
		m.setZone(kittui.ZoneTabs)
	default:
		if s.changes != nil {
			if _, handled := s.changes.Update(msg); !handled {
				s.changes = nil
			}
			return
		}
		m.back(s.activeList(), msg)
	}
}

// systemFooter is the key line of the System area.
func (m *Model) systemFooter(common, nav []kittui.Hint) []kittui.Hint {
	s := m.sys
	var own []kittui.Hint
	if m.focus() == kittui.ZoneSubtabs {
		own = append(own, kittui.Hint{Action: actLeft, Label: textKeyPrevPage, Priority: 3},
			kittui.Hint{Action: actRight, Label: textKeyNextPage, Priority: 3})
	}
	switch {
	case s.changes != nil:
		if s.changes.Searching() {
			return []kittui.Hint{{Action: actEnter, Label: textFilterKeep}, {Action: actBack, Label: textFilterClear}}
		}
		return append(append([]kittui.Hint{{Action: actEnter, Label: textKeyPick}, {Action: actBack, Label: kittui.TextKeyBack, Priority: 1}},
			common...), nav...)
	case !s.loaded:
		return append(own, common...)
	case s.page == pageUsers:
		own = append(own, kittui.Hint{Action: actAdd, Label: textKeyNewUser, Priority: 3})
		if _, ok := m.selectedUser(); ok {
			own = append(own, kittui.Hint{Action: actEdit, Label: textKeyChange, Priority: 3},
				kittui.Hint{Action: actRemove, Label: textKeyDeleteUser, Priority: 3})
		}
		return m.listFooter(s.userList, len(s.users), textKeyChange, append(own, common...), nav)
	}
	// The groups page has no action of its own yet; enter opens nothing there.
	switch l := s.groupList; {
	case len(s.groups) == 0:
		return append(own, common...)
	case l.Searching():
		return []kittui.Hint{{Action: actEnter, Label: textFilterKeep}, {Action: actBack, Label: textFilterClear}}
	case l.Query() != "":
		own = append(own, kittui.Hint{Action: actBack, Label: textFilterClear})
	}
	return append(append(append([]kittui.Hint{{Action: actSearch, Label: kittui.TextKeySearch, Priority: 1}}, own...), common...), nav...)
}

// systemPosition is the position text of the header.
func (m *Model) systemPosition() string {
	if !m.sys.loaded {
		return ""
	}
	return m.sys.activeList().Position()
}

// systemView draws the System area in width by height cells.
func (m *Model) systemView(width, height int) string {
	s := m.sys
	m.scrollNote = ""
	if !s.loaded {
		var lines []string
		if s.err != nil {
			lines = m.wrapped(detailIndent, kittui.StyleFail, m.text.Lookup(kittui.TextError, m.text.Lookup(textSystemReadFailed, s.err.Error())), width)
		} else {
			lines = []string{m.line(detailIndent, kittui.StyleHint, m.text.Lookup(textSystemLoading), width)}
		}
		return strings.Join(lines[:min(len(lines), height)], "\n")
	}
	if s.changes != nil {
		head := m.line(detailMargin, kittui.StyleActive, m.text.Lookup(textTitleChangeUser, s.changeFor.Name), width) + "\n\n"
		return head + m.listView(s.changes, width, max(height-2, 1), 2)
	}
	if s.page == pageGroups {
		return m.listView(s.groupList, width, height, 0)
	}
	var detail []string
	if u, ok := m.selectedUser(); ok {
		if room := height - minListRows; room >= 3 {
			detail = m.framed(u.Name, "", width, room, func(w int) []string { return m.userDetailLines(u, w) })
		}
	}
	view := m.listView(s.userList, width, height-len(detail), 0)
	return strings.Join(append([]string{view}, detail...), "\n")
}

// userDetailLines describe the marked user below the list. A key is never shown, only its fingerprint.
func (m *Model) userDetailLines(u hostadmin.User, width int) []string {
	t := m.text
	line := func(id kittui.TextID, args ...any) string {
		return m.line(detailIndent, kittui.StyleText, t.Lookup(id, args...), width)
	}
	access := t.Lookup(textAccessNone)
	switch {
	case u.Admin && u.Enabled:
		access = t.Lookup(textAccessAdmin)
	case u.Admin:
		access = t.Lookup(textAccessAdminNoQhost)
	}
	groups := strings.Join(furtherGroupsOf(u), ", ")
	if groups == "" {
		groups = t.Lookup(textNoValue)
	}
	lines := []string{
		line(textDetailHome, u.Home),
		line(textDetailPrimary, u.PrimaryGroup),
		line(textDetailGroups, groups),
		line(textDetailAccess, access),
		line(textDetailLogin, m.loginText(u)),
	}
	switch {
	case u.SSHKeys == nil:
		lines = append(lines, m.line(detailIndent, kittui.StyleHint, t.Lookup(textDetailKeysUnknown), width))
	case len(u.SSHKeys) == 0:
		lines = append(lines, line(textDetailKeys, t.Lookup(textNoValue)))
	default:
		var fps []string
		for _, k := range u.SSHKeys {
			fps = append(fps, k.Fingerprint)
		}
		lines = append(lines, line(textDetailKeys, fmt.Sprintf("%d (%s)", len(u.SSHKeys), strings.Join(fps, ", "))))
	}
	return lines
}

// humanUID is the person behind the interface: under sudo the user that called it, else the user it runs as.
// It is 0 where that is root itself or unknown.
func (m *Model) humanUID() int {
	if m.human != nil {
		return m.human()
	}
	if m.euid() != 0 {
		return os.Getuid()
	}
	uid, err := strconv.Atoi(os.Getenv("SUDO_UID"))
	if err != nil || uid < 0 {
		return 0
	}
	return uid
}
