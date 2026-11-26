package tui

import (
	"slices"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-kit/i18n"
	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/host/hostchange"
)

// toGroup shows the groups page and marks the group name in its list.
func toGroup(t *testing.T, m *Model, name string) {
	t.Helper()
	if m.sys.page != pageGroups {
		m.setZone(kittui.ZoneSubtabs)
		send(m, "right", "down")
	}
	if m.sys.page != pageGroups || !m.sys.groupList.Select(kittui.ItemID(name)) {
		t.Fatalf("no row for the group %s", name)
	}
}

// lastTail are the last n arguments of the last command asked for.
func lastTail(t *testing.T, f *fakeRoot, n int) []string {
	t.Helper()
	got := f.lastArgs(t)
	if len(got) < n {
		t.Fatalf("arguments %q", got)
	}
	return got[len(got)-n:]
}

func TestGroupCreateChecksTheNameAndRunsTheAction(t *testing.T) {
	m, f := systemModel(t, i18n.English, wideWidth, 40)
	toGroup(t, m, "media")
	send(m, seq(keys("n"), typed("media"), keys("f2"))...)
	if m.modal != nil || !strings.Contains(m.render(), "This group exists already.") {
		t.Fatalf("a taken name was not refused:\n%s", m.render())
	}
	send(m, "esc", "y")
	send(m, seq(keys("n"), typed("Crew"), keys("f2"))...)
	if m.modal != nil || !strings.Contains(m.render(), "Enter a name of lower-case letters") {
		t.Fatalf("a bad name was not refused:\n%s", m.render())
	}
	send(m, "esc", "y")
	send(m, seq(keys("n"), typed("crew"), keys("f2"))...)
	if screen := flat(m.render()); !strings.Contains(screen, "Create the group crew.") || !strings.Contains(screen, "groupadd") {
		t.Fatalf("the question:\n%s", screen)
	}
	send(m, "y")
	if got := lastTail(t, f, 2); !slices.Equal(got, []string{hostchange.ActionGroupCreate, "crew"}) {
		t.Fatalf("arguments %q", got)
	}
}

// One form adds and removes several members at once, and one confirmation covers it.
func TestGroupMembersAddsAndRemovesSeveralAtOnce(t *testing.T) {
	m, f := systemModel(t, i18n.English, wideWidth, 40)
	toGroup(t, m, "media")
	send(m, "e")
	if m.set.form == nil || m.set.form.kind != formGroupMembers {
		t.Fatalf("no members form:\n%s", m.render())
	}
	// Add offers ada, ben, daemon, nobody and root; Remove offers cleo.
	send(m, "space", "right", "space", "down", "space", "f2")
	screen := flat(m.render())
	for _, want := range []string{"Add ada, ben to the group media.", "Remove cleo from the", "gpasswd -a ada media", "gpasswd -d cleo media"} {
		if !strings.Contains(screen, want) {
			t.Errorf("the question lacks %q:\n%s", want, screen)
		}
	}
	if strings.Contains(screen, "Packages or services may depend") {
		t.Fatalf("a change of regular accounts in a regular group asks in red:\n%s", screen)
	}
	send(m, "y")
	if got := lastTail(t, f, 5); !slices.Equal(got, []string{hostchange.ActionGroupSetMembers, "media", "+ada", "+ben", "-cleo"}) {
		t.Fatalf("arguments %q", got)
	}
}

func TestGroupMembersNeedsATick(t *testing.T) {
	m, f := systemModel(t, i18n.English, wideWidth, 40)
	toGroup(t, m, "media")
	send(m, "e", "f2")
	if m.modal != nil || !strings.Contains(m.render(), "Tick at least one user.") || len(f.reqs) != 0 {
		t.Fatalf("an empty change was not refused:\n%s", m.render())
	}
}

// In sudo, a member without qhost leaves only sudo, and nobody takes sudo from themselves.
func TestSudoMembersFollowTheAdminRules(t *testing.T) {
	m, f := systemModel(t, i18n.English, wideWidth, 40)
	toGroup(t, m, "sudo")
	send(m, "e")
	// Add offers cleo, daemon, nobody and root; Remove offers ada and ben.
	send(m, "down", "right", "space", "f2")
	screen := flat(m.render())
	if !strings.Contains(screen, "Administrators gain or lose access to qhost") || !strings.Contains(screen, "gpasswd -d ben sudo") ||
		strings.Contains(screen, "gpasswd -d ben qhost") {
		t.Fatalf("the question:\n%s", screen)
	}
	send(m, "y")
	if got := lastTail(t, f, 3); !slices.Equal(got, []string{hostchange.ActionGroupSetMembers, "sudo", "-ben:without-qhost"}) {
		t.Fatalf("arguments %q", got)
	}
	send(m, "esc")
	toGroup(t, m, "sudo")
	send(m, "e", "down", "space", "f2")
	if m.modal != nil || !strings.Contains(m.render(), "You cannot take administrator rights from yourself.") {
		t.Fatalf("taking sudo from oneself was offered:\n%s", m.render())
	}
}

func TestSystemGroupMembersAskInRed(t *testing.T) {
	m, _ := systemModel(t, i18n.English, wideWidth, 40)
	toGroup(t, m, "docker")
	send(m, "e", "space", "f2")
	screen := flat(m.render())
	for _, want := range []string{"Members of the group docker use Docker", "Packages or services may depend on a system group"} {
		if !strings.Contains(screen, want) {
			t.Errorf("the question lacks %q:\n%s", want, screen)
		}
	}
}

func TestGroupRenameAsksInRedAndNamesTheConsequence(t *testing.T) {
	m, f := systemModel(t, i18n.English, wideWidth, 40)
	toGroup(t, m, "media")
	send(m, "r")
	if m.set.form == nil || m.set.form.kind != formGroupRename || !strings.Contains(m.render(), "Rename media") {
		t.Fatalf("no rename form:\n%s", m.render())
	}
	send(m, seq(typed("studio"), keys("f2"))...)
	screen := flat(m.render())
	for _, want := range []string{"Rename the group media to studio.", "groupmod -n studio media", "they stop working until they are"} {
		if !strings.Contains(screen, want) {
			t.Errorf("the question lacks %q:\n%s", want, screen)
		}
	}
	send(m, "y")
	if got := lastTail(t, f, 3); !slices.Equal(got, []string{hostchange.ActionGroupRename, "media", "studio"}) {
		t.Fatalf("arguments %q", got)
	}
}

func TestProtectedAndPrimaryGroupsAreLeftAlone(t *testing.T) {
	m, f := systemModel(t, i18n.English, wideWidth, 40)
	for _, tt := range []struct{ group, key, want string }{
		{"sudo", "r", "sudo keeps its name"},
		{"qhost", "d", "qhost is never deleted"},
		{"ada", "d", "ada is the primary group of ada and cannot be deleted."},
	} {
		toGroup(t, m, tt.group)
		send(m, tt.key)
		if m.modal != nil || m.set.form != nil || !strings.Contains(m.render(), tt.want) {
			t.Errorf("%s on %s: want %q:\n%s", tt.key, tt.group, tt.want, m.render())
		}
	}
	if len(f.reqs) != 0 {
		t.Fatalf("%d commands were asked for", len(f.reqs))
	}
}

func TestGroupDeleteAsks(t *testing.T) {
	m, f := systemModel(t, i18n.English, wideWidth, 40)
	toGroup(t, m, "backup-ops")
	send(m, "d")
	if screen := flat(m.render()); !strings.Contains(screen, "Delete the group backup-ops.") || !strings.Contains(screen, "groupdel backup-ops") {
		t.Fatalf("the question:\n%s", screen)
	}
	send(m, "n")
	if len(f.reqs) != 0 {
		t.Fatal("a cancelled delete asked for a command")
	}
	toGroup(t, m, "backup-ops")
	send(m, "d", "y")
	if got := lastTail(t, f, 2); !slices.Equal(got, []string{hostchange.ActionGroupDelete, "backup-ops"}) {
		t.Fatalf("arguments %q", got)
	}
}

// The members of a group are changed from the user too: several groups are left at once.
func TestUserLeavesSeveralGroupsAtOnce(t *testing.T) {
	m, f := systemModel(t, i18n.English, wideWidth, 40)
	for i := range m.sys.users {
		if m.sys.users[i].Name == "ben" {
			m.sys.users[i].Groups = append(m.sys.users[i].Groups, "backup-ops", "media")
		}
	}
	m.fillSystem()
	toUser(t, m, "ben")
	pickChangeRow(t, m, chgRemoveGroups)
	send(m, "space", "right", "space", "f2", "y")
	if got := lastTail(t, f, 4); !slices.Equal(got, []string{hostchange.ActionUserRemoveGroups, "ben", "backup-ops", "media"}) {
		t.Fatalf("arguments %q", got)
	}
}
