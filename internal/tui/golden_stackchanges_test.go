package tui

import (
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// The goldens of the changes of a stack folder: the list of two files and the question of the commit without root,
// the diff and the refusal of a commit for secret-like files as root.

func goldenStackChanges(t *testing.T, prefix string, root bool, setup func(*fakeBackend), keys ...string) {
	t.Helper()
	for name, lang := range goldenLangs {
		for size, width := range goldenSizes {
			t.Run(name+"."+size, func(t *testing.T) {
				b := changesBackend()
				if setup != nil {
					setup(b)
				}
				m := newTestModel(t, lang, b, width, settingsRows)
				uid := 1000
				if root {
					uid = 0
				}
				m.euid, m.human = func() int { return uid }, func() int { return 1000 }
				send(m, append([]string{"up", "="}, keys...)...)
				screen := m.render()
				checkScreen(t, screen, width, settingsRows)
				checkGolden(t, prefix+"."+name+"."+size+".golden", screen+"\n")
			})
		}
	}
}

func TestGoldenStackChangesList(t *testing.T) {
	goldenStackChanges(t, "stackchanges.list", false, nil)
}

func TestGoldenStackChangesDiff(t *testing.T) {
	goldenStackChanges(t, "stackchanges.diff", true, nil, "enter")
}

func TestGoldenStackChangesCommit(t *testing.T) {
	goldenStackChanges(t, "stackchanges.commit", false, nil, ">")
}

func TestGoldenStackChangesSecretsRefused(t *testing.T) {
	goldenStackChanges(t, "stackchanges.secrets", true, func(b *fakeBackend) {
		b.errs = map[string]error{"sourcecommit:" + stackDir: &control.Error{Code: control.CodeSecrets,
			Message: "secret-like files would be pushed: demo-stack/.env (env file with values)"}}
	}, ">", "y")
}
