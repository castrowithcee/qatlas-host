package tui

import (
	"testing"

	kittui "github.com/castrowithcee/qatlas-kit/tui"
)

// The goldens of adding a stack folder, adding a remote and adopting a foreign project in an interface without
// root: the questions before the probe and the comparison, which ask the git server, the last questions with the
// exact command, and the message when the command refused because the state changed.

// sudoFolderRows is the height of these goldens; the last question lists the command below the changes.
const sudoFolderRows = 40

// goldenSudoFolder renders the screen after steps on sudoFolders without root, sudo printing what replies gives.
func goldenSudoFolder(t *testing.T, prefix string, replies func(t *testing.T, b *fakeBackend) map[string]string, steps func(m *Model, f *fakeRoot)) {
	t.Helper()
	for name, lang := range goldenLangs {
		for size, width := range goldenSizes {
			t.Run(name+"."+size, func(t *testing.T) {
				b := sudoFolders()
				m, f := sudoFolderModel(t, lang, b, width, sudoFolderRows, replies(t, b))
				steps(m, f)
				screen := m.render()
				checkScreen(t, screen, width, sudoFolderRows)
				checkGolden(t, prefix+"."+name+"."+size+".golden", screen+"\n")
			})
		}
	}
}

// cloneReplies are the answers of the commands of a clone into the missing cloneDir.
func cloneReplies(t *testing.T, b *fakeBackend) map[string]string {
	return map[string]string{"source inspect": inspectJSON(t, b.inspects[cloneDir], true, nil), "repo probe": probeJSON, "source add": cloneJSON}
}

// remoteReplies are the answers of the commands of adding a remote to localDir.
func remoteReplies(t *testing.T, _ *fakeBackend) map[string]string {
	return map[string]string{"repo probe": probeJSON, "source compare": compareJSON(t, cmpDiff)}
}

// localReplies are the answers of the commands of adding the local repoDir.
func localReplies(t *testing.T, b *fakeBackend) map[string]string {
	return map[string]string{"source inspect": inspectJSON(t, b.inspects[repoDir], false, nil)}
}

func TestGoldenSudoFolderProbe(t *testing.T) {
	goldenSudoFolder(t, "stacks.sudo.probe", cloneReplies, func(m *Model, _ *fakeRoot) {
		send(m, seq(addKeys(cloneDir), keys("y"), typed(repoURL), keys("f2"))...)
	})
}

func TestGoldenSudoFolderClone(t *testing.T) {
	goldenSudoFolder(t, "stacks.sudo.clone", cloneReplies, func(m *Model, _ *fakeRoot) {
		send(m, seq(addKeys(cloneDir), keys("y"), typed(repoURL), keys("f2", "y", "y"))...)
	})
}

func TestGoldenSudoFolderLocal(t *testing.T) {
	goldenSudoFolder(t, "stacks.sudo.local", localReplies, func(m *Model, _ *fakeRoot) { send(m, addKeys(repoDir)...) })
}

func TestGoldenSudoFolderCompare(t *testing.T) {
	goldenSudoFolder(t, "stacks.sudo.compare", remoteReplies, func(m *Model, _ *fakeRoot) { send(m, sudoRemoteKeys()...) })
}

func TestGoldenSudoFolderRemote(t *testing.T) {
	// The command stands at the end of the comparison; the narrow screen scrolls there.
	goldenSudoFolder(t, "stacks.sudo.remote", remoteReplies, func(m *Model, _ *fakeRoot) {
		send(m, seq(sudoRemoteKeys(), keys("y", "end"))...)
	})
}

func TestGoldenSudoFolderStateChanged(t *testing.T) {
	goldenSudoFolder(t, "stacks.sudo.statechanged", localReplies, func(m *Model, f *fakeRoot) {
		send(m, addKeys(repoDir)...)
		f.stderr = "qhost: state-changed: the folder changed since it was inspected; nothing was changed\n"
		f.runErr = exitCode(t, "1")
		send(m, "y")
	})
}

// The last question of adopting a foreign project without root, with its command.
func TestGoldenSudoAdopt(t *testing.T) {
	for name, lang := range goldenLangs {
		for size, dim := range map[string][2]int{"narrow": {ctNarrow, ctWideRows}, "wide": {ctWide, ctWideRows}} {
			t.Run(name+"."+size, func(t *testing.T) {
				b := sameFiles()
				m, _ := sudoFolderModel(t, lang, b, dim[0], dim[1], map[string]string{
					"source inspect": inspectJSON(t, b.inspects[foreignDir], false, nil)})
				send(m, "5")
				if !m.ctList.Select(kittui.ItemID("legacy-blog-db-1")) {
					t.Fatal("the container of the foreign project is not in the list")
				}
				send(m, "a")
				screen := m.render()
				checkScreen(t, screen, dim[0], dim[1])
				checkGolden(t, "containers.adopt.sudo."+name+"."+size+".golden", screen+"\n")
			})
		}
	}
}
