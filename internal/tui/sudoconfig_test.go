package tui

import (
	"slices"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-kit/i18n"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// What the commands of the folders and the Settings area print with --output json.
const (
	reportFetch  = `{"dir":"` + stackDir + `","head":"0123456789abcdef","remote_head":"fedcba9876543210","updates_available":true}`
	reportRemove = `{"id":"srv-stacks-example","path":"` + stackDir + `"}`
	reportWindow = `{"days":"sun","start":"04:30","duration":"2h","zone":"UTC","open":false,"next_opening":"2026-11-01T04:30:00Z"}`
	reportKey    = `{"public_key":"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExampleKeyForTestsOnly qhost",` +
		`"fingerprint":"SHA256:ExampleKeyFingerprintForTestsOnly000000000"}`
)

// keyHost is keyBackend without further folders.
func keyHost() *fakeBackend { return keyBackend() }

// toFolderKeyForm opens the form of the deploy key of the folder and chooses to generate one.
var toFolderKeyForm = seq(toKey, keys("e", "right", "f2"))

// onStackFolder puts the mark on the stack folder row of the Stacks area.
func onStackFolder(t *testing.T) func(*Model) {
	return func(m *Model) {
		send(m, toFolders...)
		onFolder(t, m, stackDir)
	}
}

// Each action of the stack folder row and of the Settings area shows the exact command, runs nothing before y, then
// hands the terminal to sudo with that command and shows its answer. The service is not called, and nothing but the
// command and LC_ALL goes to it.
func TestFolderAndSettingsActionsWithoutRootRunTheirCommand(t *testing.T) {
	for _, tt := range []struct {
		name    string
		backend func() *fakeBackend
		to      func(t *testing.T) func(*Model)
		keys    []string
		report  string
		args    []string
		done    string
	}{
		{"repo fetch", foldersBackend, onStackFolder, keys("F"), reportFetch,
			[]string{"repo", "fetch", "--dir", stackDir, "--output", "json"}, "Fetched " + stackDir + "; the remote has newer commits"},
		{"source remove", foldersBackend, onStackFolder, keys("x"), reportRemove,
			[]string{"source", "remove", "--yes", "--output", "json", "--", stackDir}, "The stack folder " + stackDir + " is released"},
		{"key generate", keyHost, nil, toFolderKeyForm, reportKey,
			[]string{"key", "generate", "--dir", stackDir, "--output", "json"}, "The deploy key is generated."},
		{"window set", settingsBackend, nil, seq(toWindow, keys("e", "tab", "backspace", "backspace", "backspace", "backspace", "backspace"),
			typed("04:30"), keys("f2")), reportWindow,
			[]string{"window", "set", "--start", "04:30", "--output", "json"}, "The maintenance window is changed."},
		{"credential delete", settingsBackend, nil, seq(toCreds, keys("x")), `{"name":"registry.docker.io","deleted":true}`,
			[]string{"credential", "delete", "--output", "json", "--", "registry.docker.io"}, "The credential registry.docker.io is deleted."},
		{"notify remove", settingsBackend, nil, seq(toNotify, keys("down", "x")), `{"name":"ops-mail","removed":true}`,
			[]string{"notify", "remove", "--output", "json", "--", "ops-mail"}, "The channel ops-mail is removed."},
		{"notify test", settingsBackend, nil, seq(toNotify, keys("s")), `{"name":"ops-hook","type":"webhook","sent":true}`,
			[]string{"notify", "test", "--output", "json", "--", "ops-hook"}, "The test message through ops-hook was delivered."},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := tt.backend()
			m, f := sudoDetailModel(t, i18n.English, b, wideWidth, 40)
			f.report = tt.report
			if tt.to != nil {
				tt.to(t)(m)
			}
			send(m, tt.keys...)
			if command := "sudo qhost " + strings.Join(tt.args, " "); !shows(m.render(), command) {
				t.Fatalf("the question lacks %q:\n%s", command, m.render())
			}
			if len(f.reqs) != 0 {
				t.Fatalf("sudo ran before the answer: %+v", f.reqs)
			}
			send(m, "y")
			if got := commandsOf(f); len(got) != 1 || !slices.Equal(got[0], tt.args) {
				t.Fatalf("commands %v, want %v", got, tt.args)
			}
			if len(f.execs) != 1 || len(f.bgs) != 0 {
				t.Fatalf("no terminal handover: execs %d, background %d", len(f.execs), len(f.bgs))
			}
			for _, cmd := range f.built {
				if !slices.Equal(cmd.Env, []string{"LC_ALL=C"}) || cmd.Stdin != nil {
					t.Fatalf("the command carries more than LC_ALL: env %v, stdin %v", cmd.Env, cmd.Stdin)
				}
			}
			for _, call := range b.actions() {
				if strings.HasPrefix(call, "fetch:") || strings.HasPrefix(call, "sourceremove:") || strings.HasPrefix(call, "keygen:") ||
					strings.HasPrefix(call, "creddel:") || strings.HasPrefix(call, "notifyremove:") || strings.HasPrefix(call, "notifytest:") {
					t.Fatalf("the service was called directly: %v", b.actions())
				}
			}
			if len(b.windows) != 0 || len(b.keyArgs) != 0 {
				t.Fatalf("the service was called directly: %+v %+v", b.windows, b.keyArgs)
			}
			contains(t, m, tt.done)
		})
	}
}

// An existing key is replaced only with --replace, which the question names and marks as not undoable.
func TestKeyGenerateWithoutRootReplacesOnlyWithTheFlag(t *testing.T) {
	b := keyBackend()
	b.putKey(stackDir, control.KeyResult{PublicKey: "ssh-ed25519 AAAAExampleOld", Fingerprint: "SHA256:ExampleOld"})
	m, f := sudoDetailModel(t, i18n.English, b, wideWidth, 40)
	f.report = reportKey
	send(m, toFolderKeyForm...)
	contains(t, m, "Replace the existing deploy key?")
	args := []string{"key", "generate", "--dir", stackDir, "--replace", "--output", "json"}
	if !shows(m.render(), "sudo qhost "+strings.Join(args, " ")) {
		t.Fatalf("the question lacks the command:\n%s", m.render())
	}
	if len(f.reqs) != 0 || len(b.keyArgs) != 0 {
		t.Fatalf("something ran before the answer: %+v", f.reqs)
	}
	send(m, "y")
	if got := commandsOf(f); len(got) != 1 || !slices.Equal(got[0], args) {
		t.Fatalf("commands %v, want %v", got, args)
	}
	if len(b.keyArgs) != 0 {
		t.Fatalf("the service was called directly: %+v", b.keyArgs)
	}
	contains(t, m, "The deploy key is generated.")
}

// The message of qhost about a failed action is shown, for each of them.
func TestFolderAndSettingsActionsWithoutRootShowTheMessageOfTheCommand(t *testing.T) {
	for _, tt := range []struct {
		name    string
		backend func() *fakeBackend
		to      func(t *testing.T) func(*Model)
		keys    []string
	}{
		{"repo fetch", foldersBackend, onStackFolder, keys("F", "y")},
		{"source remove", foldersBackend, onStackFolder, keys("x", "y")},
		{"key generate", keyHost, nil, seq(toFolderKeyForm, keys("y"))},
		{"window set", settingsBackend, nil, seq(toWindow, keys("e", "tab", "backspace", "backspace", "backspace", "backspace", "backspace"),
			typed("04:30"), keys("f2", "y"))},
		{"credential delete", settingsBackend, nil, seq(toCreds, keys("x", "y"))},
		{"notify remove", settingsBackend, nil, seq(toNotify, keys("down", "x", "y"))},
		{"notify test", settingsBackend, nil, seq(toNotify, keys("s", "y"))},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m, f := sudoDetailModel(t, i18n.English, tt.backend(), wideWidth, 40)
			f.stderr = "qhost: invalid-request: example refusal of the command\n"
			f.runErr = exitCode(t, "1")
			if tt.to != nil {
				tt.to(t)(m)
			}
			send(m, tt.keys...)
			contains(t, m, "qhost reported: example refusal of the command")
		})
	}
}

// Answering no, or cancelling the password prompt, changes nothing.
func TestFolderAndSettingsActionsWithoutRootCancelledRunNothing(t *testing.T) {
	for name, tt := range map[string]struct {
		backend func() *fakeBackend
		to      func(t *testing.T) func(*Model)
		keys    []string
	}{
		"repo fetch":        {foldersBackend, onStackFolder, keys("F")},
		"source remove":     {foldersBackend, onStackFolder, keys("x")},
		"key generate":      {keyHost, nil, toFolderKeyForm},
		"credential delete": {settingsBackend, nil, seq(toCreds, keys("x"))},
		"notify remove":     {settingsBackend, nil, seq(toNotify, keys("down", "x"))},
		"notify test":       {settingsBackend, nil, seq(toNotify, keys("s"))},
	} {
		t.Run(name, func(t *testing.T) {
			b := tt.backend()
			m, f := sudoDetailModel(t, i18n.English, b, wideWidth, 40)
			if tt.to != nil {
				tt.to(t)(m)
			}
			send(m, tt.keys...)
			send(m, "n")
			if len(f.reqs) != 0 || m.set.busy || len(b.keyArgs) != 0 || len(b.windows) != 0 {
				t.Fatalf("a declined question ran something: %+v", f.reqs)
			}
		})
	}
}

// Goldens of the questions with their command: the folder row, the key, the window and the channel test.
func goldenSudoSettings(t *testing.T, prefix string, setup func() *fakeBackend, ks ...string) {
	t.Helper()
	for lname, lang := range goldenLangs {
		for size, width := range goldenSizes {
			t.Run(lname+"."+size, func(t *testing.T) {
				m, _ := sudoDetailModel(t, lang, setup(), width, settingsRows)
				send(m, ks...)
				screen := m.render()
				checkScreen(t, screen, width, settingsRows)
				checkGolden(t, prefix+"."+lname+"."+size+".golden", screen+"\n")
			})
		}
	}
}

func TestGoldenSudoFolderFetchQuestion(t *testing.T) {
	goldenSudoSettings(t, "stacks.fetch.sudo", oneFolderBackend, seq(toFolders, keys("up", "F"))...)
}

func TestGoldenSudoFolderReleaseQuestion(t *testing.T) {
	goldenSudoSettings(t, "stacks.release.sudo", oneFolderBackend, seq(toFolders, keys("up", "x"))...)
}

func TestGoldenSudoKeyGenerateQuestion(t *testing.T) {
	goldenSudoSettings(t, "stacks.key.generate.sudo", oneFolderBackend, toFolderKeyForm...)
}

func TestGoldenSudoWindowQuestion(t *testing.T) {
	goldenSudoSettings(t, "settings.window.sudo", settingsBackend,
		seq(toWindow, keys("e", "tab", "backspace", "backspace", "backspace", "backspace", "backspace"), typed("04:30"), keys("f2"))...)
}

func TestGoldenSudoNotifyTestQuestion(t *testing.T) {
	goldenSudoSettings(t, "settings.notify.test.sudo", settingsBackend, seq(toNotify, keys("s"))...)
}

func TestGoldenSudoCredentialDeleteQuestion(t *testing.T) {
	goldenSudoSettings(t, "settings.credential.delete.sudo", settingsBackend, seq(toCreds, keys("x"))...)
}
