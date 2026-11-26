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
	send(m, seq(keys("n"), typed("dora"), keys("down"), typed(hiddenA), keys("down"), typed(hiddenA), keys("down", "right", "tab", "tab", "tab"))...)
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
		send(m, seq(keys("n"), typed("cleo"), keys("down"), typed(hiddenA), keys("down"), typed(hiddenB), keys("f2"))...)
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
		send(m, "d")
	})
}

func TestGoldenSystemDeleteHomeConfirm(t *testing.T) {
	goldenSystem(t, "system.user.delete.confirm", func(t *testing.T, m *Model) {
		toUser(t, m, "cleo")
		send(m, "d", "right", "f2")
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
		send(m, seq(keys("n"), typed("crew"))...)
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
		send(m, seq(keys("r"), typed("studio"))...)
	})
}

func TestGoldenSystemGroupRenameConfirm(t *testing.T) {
	goldenSystem(t, "system.group.rename.confirm", func(t *testing.T, m *Model) {
		toGroup(t, m, "media")
		send(m, seq(keys("r"), typed("studio"), keys("f2"))...)
	})
}

func TestGoldenSystemGroupDeleteConfirm(t *testing.T) {
	goldenSystem(t, "system.group.delete.confirm", func(t *testing.T, m *Model) {
		toGroup(t, m, "backup-ops")
		send(m, "d")
	})
}

// The file browser of System > Folders over the invented tree: a folder, a YAML file in colour, a binary file and
// the question before a read through sudo.
func TestGoldenSystemBrowser(t *testing.T) {
	goldenSystem(t, "system.browser", func(t *testing.T, m *Model) {
		fakeHostFolders(m, 42)
		toFolderPage(t, m)
		browseTo(t, m, "/etc")
	})
}

func TestGoldenSystemPreviewYAML(t *testing.T) {
	goldenSystem(t, "system.preview.yaml", func(t *testing.T, m *Model) {
		m.profile = kittui.ProfileANSI16
		m.restyle()
		fakeHostFolders(m, 42)
		toFolderPage(t, m)
		browseTo(t, m, "/etc/demo-app/app.yaml")
	})
}

func TestGoldenSystemPreviewBinary(t *testing.T) {
	goldenSystem(t, "system.preview.binary", func(t *testing.T, m *Model) {
		fakeHostFolders(m, 42)
		toFolderPage(t, m)
		browseTo(t, m, "/etc/demo-app/logo.bin")
	})
}

func TestGoldenSystemReadSudo(t *testing.T) {
	goldenSystem(t, "system.read.sudo", func(t *testing.T, m *Model) {
		fakeHostFolders(m, 42)
		toFolderPage(t, m)
		browseTo(t, m, "/etc/secret-demo")
	})
}

func TestGoldenSystemFolder(t *testing.T) {
	goldenSystem(t, "system.folder", func(t *testing.T, m *Model) {
		fakeHostFolders(m, 42)
		toFolderPage(t, m)
		showFolder(t, m, "/srv/media")
	})
}

func TestGoldenSystemFolderForm(t *testing.T) {
	goldenSystem(t, "system.folder.form", func(t *testing.T, m *Model) {
		fakeHostFolders(m, 42)
		toFolderPage(t, m)
		showFolder(t, m, "/srv/media")
		send(m, "e", "down", "down", "left")
	})
}

func TestGoldenSystemFolderRecursiveConfirm(t *testing.T) {
	goldenSystem(t, "system.folder.recursive.confirm", func(t *testing.T, m *Model) {
		fakeHostFolders(m, 42)
		toFolderPage(t, m)
		showFolder(t, m, "/srv/media")
		changeBelow(m)
	})
}

func TestGoldenSystemFolderSystemConfirm(t *testing.T) {
	goldenSystem(t, "system.folder.system.confirm", func(t *testing.T, m *Model) {
		fakeHostFolders(m, 42)
		toFolderPage(t, m)
		showFolder(t, m, "/etc/demo")
		send(m, "e", "f2")
	})
}

// changesRead opens the changes page and reads the invented journal through the fake sudo.
func changesRead(t *testing.T, m *Model) {
	toChangesPage(t, m)
	f := &fakeRoot{report: demoJournal}
	m.root = f.hook()
	readChanges(t, m, f)
}

func TestGoldenSystemChangesHint(t *testing.T) {
	goldenSystem(t, "system.changes.hint", toChangesPage)
}

func TestGoldenSystemChangesList(t *testing.T) {
	goldenSystem(t, "system.changes", changesRead)
}

func TestGoldenSystemUndoConfirm(t *testing.T) {
	goldenSystem(t, "system.undo.confirm", func(t *testing.T, m *Model) {
		changesRead(t, m)
		send(m, "enter")
	})
}

func TestGoldenSystemNoUndo(t *testing.T) {
	goldenSystem(t, "system.undo.none", func(t *testing.T, m *Model) {
		changesRead(t, m)
		send(m, "down", "enter")
	})
}

func TestGoldenSystemLog(t *testing.T) {
	goldenSystem(t, "system.log", func(t *testing.T, m *Model) {
		m.hostLog = demoHostLog()
		toLogPage(t, m)
	})
}
