package tui

import (
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// The goldens of the questions of the forms with a secret in an interface without root: the sentence, the command
// without the value and the answers. The masked values are typed in; noLeak fails if one shows.

func goldenSecretSudo(t *testing.T, prefix string, setup func() *fakeBackend, steps func(m *Model, key string)) {
	t.Helper()
	for name, lang := range goldenLangs {
		for size, dim := range goldenFormSizes {
			t.Run(name+"."+size, func(t *testing.T) {
				m := newTestModel(t, lang, setup(), dim[0], dim[1])
				m.euid = func() int { return 1000 }
				f := &fakeRoot{}
				m.root = f.hook()
				key := testKeyPEM(t)
				steps(m, key)
				screen := m.render()
				checkScreen(t, screen, dim[0], dim[1])
				noLeak(t, screen, hiddenA, hiddenB)
				noKeyText(t, screen, key)
				checkGolden(t, prefix+"."+name+"."+size+".golden", screen+"\n")
			})
		}
	}
}

func TestGoldenSudoSecretQuestion(t *testing.T) {
	goldenSecretSudo(t, "detail.secret.sudo", settingsBackend, func(m *Model, _ string) { send(m, seq(secretFormKeys(), keys("f2"))...) })
}

func TestGoldenSudoCredentialQuestion(t *testing.T) {
	goldenSecretSudo(t, "settings.creds.sudo", settingsBackend, func(m *Model, _ string) { send(m, seq(registryFormKeys(), keys("f2"))...) })
}

func TestGoldenSudoNotifyAddQuestion(t *testing.T) {
	goldenSecretSudo(t, "settings.notify.sudo", settingsBackend, func(m *Model, _ string) {
		send(m, toNotify...)
		send(m, "a")
		send(m, typed("deploy-hook")...)
		send(m, "down", "down", "space", "f2")
		send(m, typed(hiddenA)...)
		send(m, "down")
		send(m, typed(hiddenB)...)
		send(m, "f2")
	})
}

func TestGoldenSudoKeySetQuestion(t *testing.T) {
	goldenSecretSudo(t, "stacks.key.sudo", oneFolderBackend, func(m *Model, key string) {
		send(m, seq(toKey, keys("e", "f2"))...)
		paste(m, key)
		send(m, "f2")
	})
}

func TestGoldenSudoKeyReplaceQuestion(t *testing.T) {
	goldenSecretSudo(t, "stacks.key.replace.sudo", func() *fakeBackend {
		b := oneFolderBackend()
		b.key = &control.KeyResult{PublicKey: "ssh-ed25519 AAAAexamplekey qhost", Fingerprint: fingerprint}
		return b
	}, func(m *Model, key string) {
		send(m, seq(toKey, keys("e", "f2"))...)
		paste(m, key)
		send(m, "f2")
	})
}
