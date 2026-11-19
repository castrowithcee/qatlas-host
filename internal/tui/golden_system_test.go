package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	kittui "github.com/castrowithcee/qatlas-kit/tui"
)

// The goldens of the System area: its lists, the forms of the users and groups pages and the questions, in 80x24 and 120x40,
// in English and German. The masked fields hold typed values and a pasted key; goldenSystem fails if one shows.

func goldenSystem(t *testing.T, prefix string, steps func(t *testing.T, m *Model)) {
	t.Helper()
	for name, lang := range goldenLangs {
		for size, dim := range goldenFormSizes {
			t.Run(name+"."+size, func(t *testing.T) {
				width, rows := dim[0], dim[1]
				m, _ := systemModel(t, lang, width, rows)
				steps(t, m)
				screen := m.render()
				checkScreen(t, screen, width, rows)
				noLeak(t, screen, hiddenA, hiddenB, demoKey(t, 7))
				checkGolden(t, prefix+"."+name+"."+size+".golden", screen+"\n")
			})
		}
	}
}

// fillCreate types a new user dora with an initial password, Admin on and a pasted key.
func fillCreate(t *testing.T, m *Model) {
	send(m, seq(keys("a"), typed("dora"), keys("down"), typed(hiddenA), keys("down"), typed(hiddenA), keys("down", "right", "tab", "tab", "tab"))...)
	m.Update(tea.PasteMsg{Content: demoKey(t, 7)})
}

func TestGoldenSystemUsers(t *testing.T) {
	goldenSystem(t, "system.users", func(*testing.T, *Model) {})
}

func TestGoldenSystemGroups(t *testing.T) {
	goldenSystem(t, "system.groups", func(_ *testing.T, m *Model) {
		m.setZone(kittui.ZoneSubtabs)
		send(m, "right", "down")
	})
}

func TestGoldenSystemCreateForm(t *testing.T) {
	goldenSystem(t, "system.user.create", fillCreate)
}

func TestGoldenSystemCreateProblem(t *testing.T) {
	goldenSystem(t, "system.user.create.problem", func(_ *testing.T, m *Model) {
		send(m, seq(keys("a"), typed("cleo"), keys("down"), typed(hiddenA), keys("down"), typed(hiddenB), keys("f2"))...)
	})
}

func TestGoldenSystemCreateConfirm(t *testing.T) {
	goldenSystem(t, "system.user.create.confirm", func(t *testing.T, m *Model) {
		fillCreate(t, m)
		send(m, "f2")
	})
}

func TestGoldenSystemChanges(t *testing.T) {
	goldenSystem(t, "system.user.changes", func(t *testing.T, m *Model) {
		toUser(t, m, "cleo")
		send(m, "e")
	})
}

func TestGoldenSystemSecretForm(t *testing.T) {
	goldenSystem(t, "system.user.secret", func(t *testing.T, m *Model) {
		toUser(t, m, "cleo")
		pickChangeRow(t, m, chgSecret)
		send(m, typed(hiddenB)...)
	})
}

func TestGoldenSystemAdminConfirm(t *testing.T) {
	goldenSystem(t, "system.user.admin.confirm", func(t *testing.T, m *Model) {
		toUser(t, m, "cleo")
		pickChangeRow(t, m, chgAdminOn)
	})
}

func TestGoldenSystemDeleteForm(t *testing.T) {
	goldenSystem(t, "system.user.delete", func(t *testing.T, m *Model) {
		toUser(t, m, "cleo")
		send(m, "x")
	})
}

func TestGoldenSystemDeleteHomeConfirm(t *testing.T) {
	goldenSystem(t, "system.user.delete.confirm", func(t *testing.T, m *Model) {
		toUser(t, m, "cleo")
		send(m, "x", "right", "f2")
	})
}

func TestGoldenSystemRootConfirm(t *testing.T) {
	goldenSystem(t, "system.root.confirm", func(t *testing.T, m *Model) {
		toUser(t, m, "root")
		pickChangeRow(t, m, chgLock)
	})
}

func TestGoldenSystemResult(t *testing.T) {
	goldenSystem(t, "system.result", func(t *testing.T, m *Model) {
		toUser(t, m, "cleo")
		pickChangeRow(t, m, chgLock)
		send(m, "y")
	})
}

func TestGoldenSystemGroupCreateForm(t *testing.T) {
	goldenSystem(t, "system.group.create", func(t *testing.T, m *Model) {
		toGroup(t, m, "media")
		send(m, seq(keys("a"), typed("crew"))...)
	})
}

func TestGoldenSystemGroupMembersForm(t *testing.T) {
	goldenSystem(t, "system.group.members", func(t *testing.T, m *Model) {
		toGroup(t, m, "media")
		send(m, "e", "space", "right", "space", "down", "space")
	})
}

func TestGoldenSystemGroupMembersConfirm(t *testing.T) {
	goldenSystem(t, "system.group.members.confirm", func(t *testing.T, m *Model) {
		toGroup(t, m, "media")
		send(m, "e", "space", "right", "space", "down", "space", "f2")
	})
}

func TestGoldenSystemGroupSystemConfirm(t *testing.T) {
	goldenSystem(t, "system.group.system.confirm", func(t *testing.T, m *Model) {
		toGroup(t, m, "docker")
		send(m, "e", "space", "f2")
	})
}

func TestGoldenSystemGroupRenameForm(t *testing.T) {
	goldenSystem(t, "system.group.rename", func(t *testing.T, m *Model) {
		toGroup(t, m, "media")
		send(m, seq(keys("n"), typed("studio"))...)
	})
}

func TestGoldenSystemGroupRenameConfirm(t *testing.T) {
	goldenSystem(t, "system.group.rename.confirm", func(t *testing.T, m *Model) {
		toGroup(t, m, "media")
		send(m, seq(keys("n"), typed("studio"), keys("f2"))...)
	})
}

func TestGoldenSystemGroupDeleteConfirm(t *testing.T) {
	goldenSystem(t, "system.group.delete.confirm", func(t *testing.T, m *Model) {
		toGroup(t, m, "backup-ops")
		send(m, "x")
	})
}
