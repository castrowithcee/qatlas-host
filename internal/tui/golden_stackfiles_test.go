package tui

import (
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// The goldens of the browser over a stack folder: the folder, secrets.env masked and uncovered, and the row of a
// folder with uncommitted changes. Only the uncovered screen shows the invented values.

func goldenStackFiles(t *testing.T, prefix string, setup func() *fakeBackend, revealed bool, keys ...string) {
	t.Helper()
	for name, lang := range goldenLangs {
		for size, width := range goldenSizes {
			t.Run(name+"."+size, func(t *testing.T) {
				m := newTestModel(t, lang, setup(), width, settingsRows)
				fakeStackFiles(m)
				send(m, keys...)
				screen := m.render()
				checkScreen(t, screen, width, settingsRows)
				if !revealed {
					noLeak(t, screen, stackSecretA, stackSecretB)
				}
				checkGolden(t, prefix+"."+name+"."+size+".golden", screen+"\n")
			})
		}
	}
}

// toStackSecrets opens the files of the first folder and there demo-stack/secrets.env.
var toStackSecrets = seq(keys("up", "Q", "/"), typed("demo-stack"), keys("enter", "enter", "/"), typed("secrets.env"),
	keys("enter", "enter"))

func TestGoldenStackFiles(t *testing.T) {
	goldenStackFiles(t, "stackfiles.browser", stackFilesBackend, false, "up", "Q")
}

func TestGoldenStackFilesSecretsMasked(t *testing.T) {
	goldenStackFiles(t, "stackfiles.secrets.masked", stackFilesBackend, false, toStackSecrets...)
}

func TestGoldenStackFilesSecretsRevealed(t *testing.T) {
	goldenStackFiles(t, "stackfiles.secrets.revealed", stackFilesBackend, true, seq(toStackSecrets, keys("V"))...)
}

func TestGoldenStackFilesUncommitted(t *testing.T) {
	goldenStackFiles(t, "stackfiles.uncommitted", func() *fakeBackend {
		b := stackFilesBackend()
		b.changes = map[string][]control.SourceChange{stackDir: {{Path: "demo-stack/.env", Kind: control.ChangeModified, Stack: "demo-stack"}}}
		return b
	}, false, "up")
}
