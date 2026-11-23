package tui

import (
	"slices"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-kit/i18n"
)

// The page of the webhook receiver; the address of a hook is never part of it.
var toHooks = keys("4", "up", "right", "right", "right", "right", "down")

const reportHookSetup = `{"set_up":true,"changed":true,"steps":3,"unit":"qhost-hook.service","listen":"127.0.0.1:8480"}`

func hookBackend(info HookServiceInfo) func() *fakeBackend {
	return func() *fakeBackend {
		b := settingsBackend()
		b.hook = info
		return b
	}
}

var (
	hookRunning = HookServiceInfo{SetUp: true, Running: true, Listen: "127.0.0.1:8480", Hooks: 2}
	hookOpen    = HookServiceInfo{SetUp: true, Running: true, Listen: "0.0.0.0:8480", Hooks: 1}
	hookStopped = HookServiceInfo{SetUp: true, Listen: "127.0.0.1:8480"}
	hookBadFile = HookServiceInfo{SetUp: true, Running: true, Problem: "read /etc/qhost-hook/config.json: invalid character"}
	typeAddress = func(old, addr string) []string {
		return seq(toHooks, keys("e"), slices.Repeat(keys("backspace"), len(old)), typed(addr))
	}
	hookSudoArgs = []string{"hook-service", "setup", "--confirm", "--yes", "--output", "json"}
)

func TestWebhooksIsASubtabNextToTheOthers(t *testing.T) {
	m := newSettingsModel(t, hookBackend(hookRunning)())
	send(m, toHooks...)
	if m.set.page != pageHooks {
		t.Fatalf("page %q", m.set.page)
	}
	contains(t, m, "Webhooks", "running", "Address: 127.0.0.1:8480", "Hooks: 2")
}

func TestWebhooksPageStatesWithoutAndWithTheReceiver(t *testing.T) {
	for _, tt := range []struct {
		name string
		info HookServiceInfo
		want []string
		not  []string
	}{
		{"none", HookServiceInfo{}, []string{"not set up", "127.0.0.1:8480", "set up receiver"}, []string{"Hooks:"}},
		{"stopped", hookStopped, []string{"not running", "Hooks: 0"}, []string{"set up receiver"}},
		{"not local", hookOpen, []string{"Address: 0.0.0.0:8480", "reverse proxy"}, nil},
		{"unreadable", hookBadFile, []string{"cannot be read", "invalid character"}, []string{"Hooks:"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel(t, i18n.English, hookBackend(tt.info)(), 160, 24)
			send(m, toHooks...)
			contains(t, m, tt.want...)
			for _, n := range tt.not {
				if strings.Contains(m.render(), n) {
					t.Fatalf("the screen shows %q:\n%s", n, m.render())
				}
			}
		})
	}
}

func TestWebhooksSetupAsRootCallsTheServiceAfterY(t *testing.T) {
	b := hookBackend(HookServiceInfo{})()
	m := newTestModel(t, i18n.English, b, wideWidth, 24)
	send(m, toHooks...)
	send(m, "a")
	contains(t, m, "Set up the webhook receiver on 127.0.0.1:8480?")
	if got := b.actions(); len(got) != 0 {
		t.Fatalf("the service was called before y: %v", got)
	}
	send(m, "y")
	if got := b.actions(); !slices.Equal(got, []string{"hooksetup:"}) {
		t.Fatalf("calls %v", got)
	}
	contains(t, m, "The webhook receiver listens on 127.0.0.1:8480.", "running", "Hooks: 0")
}

func TestWebhooksSetupWithoutRootShowsTheCommandAndRunsItAfterY(t *testing.T) {
	b := hookBackend(HookServiceInfo{})()
	m, f := sudoDetailModel(t, i18n.English, b, wideWidth, 30)
	f.report = reportHookSetup
	send(m, toHooks...)
	send(m, "a")
	if !shows(m.render(), sudoCommand(hookSudoArgs)) {
		t.Fatalf("the question lacks the command:\n%s", m.render())
	}
	if len(f.reqs) != 0 {
		t.Fatalf("sudo ran before the answer: %+v", f.reqs)
	}
	send(m, "y")
	if got := commandsOf(f); len(got) != 1 || !slices.Equal(got[0], hookSudoArgs) {
		t.Fatalf("commands %v", got)
	}
	if len(b.actions()) != 0 {
		t.Fatalf("the service was called directly: %v", b.actions())
	}
	contains(t, m, "The webhook receiver listens on 127.0.0.1:8480.")
}

func TestWebhooksSetupWithoutRootFailureIsShown(t *testing.T) {
	b := hookBackend(HookServiceInfo{})()
	m, f := sudoDetailModel(t, i18n.English, b, wideWidth, 30)
	f.report = `{"set_up":false}`
	send(m, toHooks...)
	send(m, "a", "y")
	contains(t, m, "not set up")
	if m.failure == "" {
		t.Fatal("no failure shown")
	}
}

func TestWebhooksAddressChangeAsksWithTheProxyNoteForANonLocalAddress(t *testing.T) {
	b := hookBackend(hookRunning)()
	m, f := sudoDetailModel(t, i18n.English, b, 160, 30)
	f.report = `{"set_up":true,"changed":true,"steps":1,"unit":"qhost-hook.service","listen":"0.0.0.0:8481"}`
	send(m, typeAddress("127.0.0.1:8480", "0.0.0.0:8481")...)
	send(m, "f2")
	args := []string{"hook-service", "setup", "--listen", "0.0.0.0:8481", "--confirm", "--yes", "--output", "json"}
	if !shows(m.render(), sudoCommand(args)) {
		t.Fatalf("the question lacks the command:\n%s", m.render())
	}
	contains(t, m, "Let the webhook receiver listen on 0.0.0.0:8481?", "reverse proxy should provide HTTPS")
	send(m, "y")
	if got := commandsOf(f); len(got) != 1 || !slices.Equal(got[0], args) {
		t.Fatalf("commands %v", got)
	}
}

func TestWebhooksAddressChangeForALocalAddressHasNoProxyNote(t *testing.T) {
	m := newTestModel(t, i18n.English, hookBackend(hookRunning)(), 160, 30)
	send(m, typeAddress("127.0.0.1:8480", "127.0.0.1:9001")...)
	send(m, "f2")
	contains(t, m, "Let the webhook receiver listen on 127.0.0.1:9001?")
	if strings.Contains(m.render(), "reverse proxy") {
		t.Fatalf("a local address names the proxy:\n%s", m.render())
	}
}

func TestWebhooksAddressFormRefusesWhatCannotBeUsed(t *testing.T) {
	for name, tt := range map[string]struct{ addr, want string }{
		"no port":       {"example.test", "host:port"},
		"low port":      {"127.0.0.1:80", "1025 to 65535"},
		"same address":  {"127.0.0.1:8480", "listens on this address already"},
		"not an adress": {"bad host:9000", "host:port"},
	} {
		t.Run(name, func(t *testing.T) {
			b := hookBackend(hookRunning)()
			m := newTestModel(t, i18n.English, b, 160, 30)
			send(m, typeAddress("127.0.0.1:8480", tt.addr)...)
			send(m, "f2")
			contains(t, m, tt.want)
			if len(b.actions()) != 0 {
				t.Fatalf("the service was called: %v", b.actions())
			}
		})
	}
}

func TestWebhooksKeysDoNothingWhileTheStateIsNotRead(t *testing.T) {
	b := hookBackend(hookRunning)()
	b.errs = map[string]error{"hookservice:": errServiceBusy()}
	m := newTestModel(t, i18n.English, b, wideWidth, 24)
	send(m, toHooks...)
	send(m, "a", "e")
	if m.modal != nil || m.set.form != nil {
		t.Fatal("a key opened something without a state")
	}
}

func TestGoldenSettingsWebhooksWithoutReceiver(t *testing.T) {
	goldenScreens(t, "settings.hooks.none", settingsRows, hookBackend(HookServiceInfo{}), toHooks...)
}

func TestGoldenSettingsWebhooksWithReceiver(t *testing.T) {
	goldenScreens(t, "settings.hooks", settingsRows, hookBackend(hookRunning), toHooks...)
}

func TestGoldenSettingsWebhooksNonLocalAddress(t *testing.T) {
	goldenScreens(t, "settings.hooks.open", settingsRows, hookBackend(hookOpen), toHooks...)
}

func TestGoldenSettingsWebhooksSetupQuestion(t *testing.T) {
	goldenScreens(t, "settings.hooks.setup", settingsRows, hookBackend(HookServiceInfo{}), seq(toHooks, keys("a"))...)
}

func TestGoldenSettingsWebhooksAddressForm(t *testing.T) {
	goldenScreens(t, "settings.hooks.form", settingsRows, hookBackend(hookRunning), seq(toHooks, keys("e"))...)
}

func TestGoldenSettingsWebhooksAddressQuestion(t *testing.T) {
	goldenScreens(t, "settings.hooks.confirm", settingsRows, hookBackend(hookRunning),
		seq(typeAddress("127.0.0.1:8480", "0.0.0.0:8480"), keys("f2"))...)
}

func TestGoldenSudoWebhooksSetupQuestion(t *testing.T) {
	goldenSudoSettings(t, "settings.hooks.setup.sudo", hookBackend(HookServiceInfo{}), seq(toHooks, keys("a"))...)
}

func TestGoldenSudoWebhooksAddressQuestion(t *testing.T) {
	goldenSudoSettings(t, "settings.hooks.confirm.sudo", hookBackend(hookRunning),
		seq(typeAddress("127.0.0.1:8480", "0.0.0.0:8480"), keys("f2"))...)
}
