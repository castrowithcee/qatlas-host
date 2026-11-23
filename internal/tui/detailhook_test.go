package tui

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-kit/i18n"
)

// The Webhooks section of the stack detail. The secret address of a new hook is a made-up value of the fake
// backend; after its one dialog is closed no part of the model may hold it.

const (
	hookSecret    = "FAKE-SECRET-not-a-real-token-0123456789abcd"
	hookRows      = 64
	reportHookAdd = `{"id":"h-a1b2c3d0","stack":"demo-stack","instance":"system","path":"` + fakeHookPath + `","listen":"127.0.0.1:8480"}`
)

var (
	toDetail     = keys("enter")
	hookAddSudo  = []string{"hook", "add", "--output", "json", "--", "demo-stack"}
	hookDelSudo  = []string{"hook", "remove", "--output", "json", "--", "h-a1b2c3d0"}
	oneHook      = map[string][]HookInfo{"demo-stack": {{ID: "h-a1b2c3d0"}}}
	twoHooks     = map[string][]HookInfo{"demo-stack": {{ID: "h-a1b2c3d0"}, {ID: "h-a1b2c3d1"}}}
	hookTrigger  = func(b *fakeBackend, trigger string) *fakeBackend { b.stacks.Stacks[0].Trigger = trigger; return b }
	hookStackFor = func(info HookServiceInfo, hooks map[string][]HookInfo, trigger string) func() *fakeBackend {
		return func() *fakeBackend {
			b := settingsBackend()
			b.hook, b.hooks = info, hooks
			return hookTrigger(b, trigger)
		}
	}
)

// holds reports whether any string or byte slice reachable from v, through unexported fields too, contains needle.
func holds(v reflect.Value, needle string, seen map[uintptr]bool) bool {
	switch v.Kind() {
	case reflect.String:
		return strings.Contains(v.String(), needle)
	case reflect.Pointer, reflect.Map, reflect.Slice:
		if v.IsNil() {
			return false
		}
		if v.Kind() == reflect.Slice && v.Type().Elem().Kind() == reflect.Uint8 {
			b := make([]byte, v.Len())
			for i := range b {
				b[i] = byte(v.Index(i).Uint())
			}
			return strings.Contains(string(b), needle)
		}
		if p := v.Pointer(); seen[p] {
			return false
		} else if v.Kind() != reflect.Slice || v.Len() > 0 {
			seen[p] = true
		}
		switch v.Kind() {
		case reflect.Pointer:
			return holds(v.Elem(), needle, seen)
		case reflect.Map:
			for it := v.MapRange(); it.Next(); {
				if holds(it.Key(), needle, seen) || holds(it.Value(), needle, seen) {
					return true
				}
			}
		default:
			for i := range v.Len() {
				if holds(v.Index(i), needle, seen) {
					return true
				}
			}
		}
	case reflect.Interface:
		return !v.IsNil() && holds(v.Elem(), needle, seen)
	case reflect.Array:
		for i := range v.Len() {
			if holds(v.Index(i), needle, seen) {
				return true
			}
		}
	case reflect.Struct:
		for i := range v.NumField() {
			if holds(v.Field(i), needle, seen) {
				return true
			}
		}
	}
	return false
}

func modelHolds(m *Model, needle string) bool {
	return holds(reflect.ValueOf(m), needle, map[uintptr]bool{})
}

func TestHoldsFindsAStringInAnUnexportedFieldOfTheModel(t *testing.T) {
	m := newTestModel(t, i18n.English, settingsBackend(), wideWidth, 24)
	m.failure = "x " + hookSecret + " y"
	if !modelHolds(m, hookSecret) {
		t.Fatal("the search does not see a string held by the model")
	}
	m.failure = ""
	if modelHolds(m, hookSecret) {
		t.Fatal("the search finds a string that is not held")
	}
}

func TestHookSectionListsTheHooksOfTheStack(t *testing.T) {
	b := hookStackFor(hookRunning, twoHooks, "webhook")()
	m := newTestModel(t, i18n.English, b, wideWidth, hookRows)
	send(m, toDetail...)
	contains(t, m, "Webhooks", "h-a1b2c3d0", "h-a1b2c3d1")
	if strings.Contains(m.render(), "modified") {
		t.Fatalf("a hook shows a time:\n%s", m.render())
	}
	if strings.Contains(m.render(), "Maintenance window") {
		t.Fatalf("the trigger webhook gets the note about the window:\n%s", m.render())
	}
}

func TestHookSectionStates(t *testing.T) {
	for _, tt := range []struct {
		name    string
		backend func() *fakeBackend
		want    string
	}{
		{"no hooks", hookStackFor(hookRunning, nil, "webhook"), "No webhook for this stack."},
		{"no receiver", hookStackFor(HookServiceInfo{}, nil, "webhook"), "No webhook receiver is set up. Set it up under Settings, Webhooks."},
		{"window", hookStackFor(hookRunning, oneHook, "window"), "a hook acts only with the trigger Polling or Webhook only"},
		{"default trigger is the window", hookStackFor(hookRunning, nil, ""), "The update trigger is Maintenance window"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel(t, i18n.English, tt.backend(), wideWidth, hookRows)
			send(m, toDetail...)
			contains(t, m, tt.want)
		})
	}
	m := newTestModel(t, i18n.German, hookStackFor(hookRunning, oneHook, "window")(), wideWidth, hookRows)
	send(m, toDetail...)
	contains(t, m, "h-a1b2c3d0", "Der Auslöser ist Wartungsfenster", "Abfrage oder Nur Webhook")
}

func TestHookSectionErrorLeavesTheOtherSections(t *testing.T) {
	b := hookStackFor(hookRunning, oneHook, "webhook")()
	b.errs = map[string]error{"hooklist:demo-stack": errServiceBusy()}
	m := newTestModel(t, i18n.English, b, wideWidth, hookRows)
	send(m, toDetail...)
	contains(t, m, "Webhooks", "DB_PASSWORD")
	send(m, "h")
	if m.modal != nil {
		t.Fatal("a hook was offered without a read list")
	}
}

func TestHookAddWithoutReceiverOnlyWarns(t *testing.T) {
	b := hookStackFor(HookServiceInfo{}, nil, "webhook")()
	m := newTestModel(t, i18n.English, b, wideWidth, hookRows)
	send(m, toDetail...)
	send(m, "h")
	if m.modal != nil || len(b.actions()) != 0 || m.notice == "" {
		t.Fatalf("modal %v actions %v notice %q", m.modal != nil, b.actions(), m.notice)
	}
}

func TestHookAddAsRootAsksThenShowsTheAddressOnceAndForgetsIt(t *testing.T) {
	b := hookStackFor(hookRunning, nil, "webhook")()
	m := newTestModel(t, i18n.English, b, wideWidth, hookRows)
	m.euid = func() int { return 0 }
	send(m, toDetail...)
	send(m, "h")
	contains(t, m, "Give the stack demo-stack a webhook?")
	if len(b.actions()) != 0 {
		t.Fatalf("the service was called before y: %v", b.actions())
	}
	send(m, "y")
	if got := b.actions(); !slices.Equal(got, []string{"hookadd:demo-stack"}) {
		t.Fatalf("calls %v", got)
	}
	contains(t, m, "http://127.0.0.1:8480", "shown only now and does not appear again", "h-a1b2c3d0")
	if !modelHolds(m, hookSecret) {
		t.Fatal("the open dialog does not hold the address, so the check below proves nothing")
	}
	send(m, "esc")
	assertForgotten(t, m)
	contains(t, m, "h-a1b2c3d0")
}

// assertForgotten fails when the secret is on the screen or anywhere in the model.
func assertForgotten(t *testing.T, m *Model) {
	t.Helper()
	if m.modal != nil {
		t.Fatal("the dialog is still open")
	}
	if strings.Contains(m.render(), "FAKE-SECRET") {
		t.Fatalf("the secret is on the screen:\n%s", m.render())
	}
	if modelHolds(m, hookSecret) || modelHolds(m, "FAKE-SECRET") {
		t.Fatal("the model still holds the secret")
	}
}

func TestHookAddWithoutRootRunsTheCommandThroughSudoAndForgetsTheAddress(t *testing.T) {
	b := hookStackFor(hookRunning, nil, "webhook")()
	m, f := sudoDetailModel(t, i18n.English, b, wideWidth, hookRows)
	f.report = reportHookAdd
	send(m, toDetail...)
	send(m, "h")
	if !shows(m.render(), sudoCommand(hookAddSudo)) {
		t.Fatalf("the question lacks the command:\n%s", m.render())
	}
	if len(f.reqs) != 0 {
		t.Fatalf("sudo ran before the answer: %+v", f.reqs)
	}
	send(m, "y")
	if got := commandsOf(f); len(got) != 1 || !slices.Equal(got[0], hookAddSudo) {
		t.Fatalf("commands %v", got)
	}
	if len(b.actions()) != 0 {
		t.Fatalf("the service was called directly: %v", b.actions())
	}
	contains(t, m, "does not appear again")
	// The address stands whole on one line, so a terminal selection copies it.
	if !strings.Contains(m.render(), "  http://127.0.0.1:8480"+fakeHookPath+" ") {
		t.Fatalf("the address is not on one line:\n%s", m.render())
	}
	send(m, "enter")
	assertForgotten(t, m)
}

func TestHookAddThroughSudoOverwritesThePrintedAnswer(t *testing.T) {
	out := []byte(reportHookAdd)
	res, ok := decodeHookAdd(sudoRun{out: out})
	if !ok || res.(HookAddResult).Path != fakeHookPath {
		t.Fatalf("decoded %v %v", res, ok)
	}
	if strings.Contains(string(out), "FAKE-SECRET") {
		t.Fatalf("the buffer of the run still holds the secret: %q", out)
	}
}

func TestHookAddThroughSudoWithoutAnAnswerFailsWithoutADialog(t *testing.T) {
	b := hookStackFor(hookRunning, nil, "webhook")()
	m, f := sudoDetailModel(t, i18n.English, b, wideWidth, hookRows)
	f.report = `{"removed":true}`
	send(m, toDetail...)
	send(m, "h", "y")
	if m.modal != nil || m.failure == "" {
		t.Fatalf("modal %v failure %q", m.modal != nil, m.failure)
	}
}

func TestHookAddFailureShowsNoSecret(t *testing.T) {
	b := hookStackFor(hookRunning, nil, "webhook")()
	b.errs = map[string]error{"hookadd:demo-stack": errServiceBusy()}
	m := newTestModel(t, i18n.English, b, wideWidth, hookRows)
	m.euid = func() int { return 0 }
	send(m, toDetail...)
	send(m, "h", "y")
	if m.failure == "" || modelHolds(m, "FAKE-SECRET") {
		t.Fatalf("failure %q", m.failure)
	}
}

func TestHookRemoveAsksFirstAndCancelChangesNothing(t *testing.T) {
	b := hookStackFor(hookRunning, twoHooks, "webhook")()
	m := newTestModel(t, i18n.English, b, wideWidth, hookRows)
	m.euid = func() int { return 0 }
	send(m, toDetail...)
	send(m, "U")
	contains(t, m, "Remove a webhook", "h-a1b2c3d0", "h-a1b2c3d1")
	send(m, "down", "enter")
	contains(t, m, "Remove the webhook h-a1b2c3d1 of the stack demo-stack?", "stops working")
	send(m, "n")
	if len(b.actions()) != 0 {
		t.Fatalf("removed without the confirmation: %v", b.actions())
	}
	send(m, "U", "down", "enter", "y")
	if got := b.actions(); !slices.Equal(got, []string{"hookremove:h-a1b2c3d1"}) {
		t.Fatalf("calls %v", got)
	}
	contains(t, m, "h-a1b2c3d0")
	if strings.Contains(m.render(), "│ h-a1b2c3d1") {
		t.Fatalf("the removed hook is still listed:\n%s", m.render())
	}
}

func TestHookRemoveWithoutRootRunsTheCommandThroughSudo(t *testing.T) {
	b := hookStackFor(hookRunning, oneHook, "webhook")()
	m, f := sudoDetailModel(t, i18n.English, b, wideWidth, hookRows)
	f.report = `{"id":"h-a1b2c3d0","removed":true}`
	send(m, toDetail...)
	send(m, "U", "enter")
	if !shows(m.render(), sudoCommand(hookDelSudo)) {
		t.Fatalf("the question lacks the command:\n%s", m.render())
	}
	send(m, "y")
	if got := commandsOf(f); len(got) != 1 || !slices.Equal(got[0], hookDelSudo) {
		t.Fatalf("commands %v", got)
	}
	if len(b.actions()) != 0 {
		t.Fatalf("the service was called directly: %v", b.actions())
	}
	contains(t, m, "The webhook h-a1b2c3d0 was removed.")
}

func TestHookRemoveWithoutHooksOnlyWarns(t *testing.T) {
	m := newTestModel(t, i18n.English, hookStackFor(hookRunning, nil, "webhook")(), wideWidth, hookRows)
	send(m, toDetail...)
	send(m, "U")
	if m.pick != nil || m.modal != nil || m.notice == "" {
		t.Fatal("a choice or question opened without a hook")
	}
}

func TestHookKeysDoNotShareAKeyWithTheDetail(t *testing.T) {
	// A key table with a double key does not build; building the model is the check.
	newTestModel(t, i18n.English, settingsBackend(), wideWidth, 24)
}

func goldenHooks(t *testing.T, prefix string, setup func() *fakeBackend, ks ...string) {
	t.Helper()
	goldenScreens(t, prefix, hookRows, setup, ks...)
}

func goldenHooksSudo(t *testing.T, prefix string, setup func() *fakeBackend, report string, ks ...string) {
	t.Helper()
	for lname, lang := range goldenLangs {
		for size, width := range goldenSizes {
			t.Run(lname+"."+size, func(t *testing.T) {
				m, f := sudoDetailModel(t, lang, setup(), width, hookRows)
				f.report = report
				send(m, ks...)
				screen := m.render()
				checkScreen(t, screen, width, hookRows)
				checkGolden(t, prefix+"."+lname+"."+size+".golden", screen+"\n")
			})
		}
	}
}

func TestGoldenDetailHooks(t *testing.T) {
	goldenHooks(t, "detail.hooks", hookStackFor(hookRunning, twoHooks, "webhook"), toDetail...)
}

func TestGoldenDetailHooksNone(t *testing.T) {
	goldenHooks(t, "detail.hooks.none", hookStackFor(hookRunning, nil, "webhook"), toDetail...)
}

func TestGoldenDetailHooksNoReceiver(t *testing.T) {
	goldenHooks(t, "detail.hooks.noreceiver", hookStackFor(HookServiceInfo{}, nil, "webhook"), toDetail...)
}

func TestGoldenDetailHooksWindow(t *testing.T) {
	goldenHooks(t, "detail.hooks.window", hookStackFor(hookRunning, oneHook, "window"), toDetail...)
}

func TestGoldenDetailHooksAddQuestion(t *testing.T) {
	goldenHooks(t, "detail.hooks.add", hookStackFor(hookRunning, nil, "webhook"), seq(toDetail, keys("h"))...)
}

func TestGoldenDetailHooksAddSudoQuestion(t *testing.T) {
	goldenHooksSudo(t, "detail.hooks.add.sudo", hookStackFor(hookRunning, nil, "webhook"), "", seq(toDetail, keys("h"))...)
}

// The one snapshot that shows the made-up address.
func TestGoldenDetailHooksSecretDialog(t *testing.T) {
	for lname, lang := range goldenLangs {
		for size, width := range goldenSizes {
			t.Run(lname+"."+size, func(t *testing.T) {
				m := newTestModel(t, lang, hookStackFor(hookRunning, nil, "webhook")(), width, hookRows)
				m.euid = func() int { return 0 }
				send(m, seq(toDetail, keys("h", "y"))...)
				screen := m.render()
				checkScreen(t, screen, width, hookRows)
				checkGolden(t, "detail.hooks.secret."+lname+"."+size+".golden", screen+"\n")
			})
		}
	}
}

func TestGoldenDetailHooksSecretClosed(t *testing.T) {
	asRootGolden := func() func() *fakeBackend { return hookStackFor(hookRunning, nil, "webhook") }
	for lname, lang := range goldenLangs {
		for size, width := range goldenSizes {
			t.Run(lname+"."+size, func(t *testing.T) {
				m := newTestModel(t, lang, asRootGolden()(), width, hookRows)
				m.euid = func() int { return 0 }
				send(m, seq(toDetail, keys("h", "y", "esc"))...)
				screen := m.render()
				checkScreen(t, screen, width, hookRows)
				noLeak(t, screen, "FAKE-SECRET")
				checkGolden(t, "detail.hooks.closed."+lname+"."+size+".golden", screen+"\n")
			})
		}
	}
}

func TestGoldenDetailHooksRemovePick(t *testing.T) {
	goldenHooks(t, "detail.hooks.pick", hookStackFor(hookRunning, twoHooks, "webhook"), seq(toDetail, keys("U"))...)
}

func TestGoldenDetailHooksRemoveQuestion(t *testing.T) {
	goldenHooks(t, "detail.hooks.remove", hookStackFor(hookRunning, twoHooks, "webhook"), seq(toDetail, keys("U", "enter"))...)
}

func TestGoldenDetailHooksRemoveSudoQuestion(t *testing.T) {
	goldenHooksSudo(t, "detail.hooks.remove.sudo", hookStackFor(hookRunning, twoHooks, "webhook"), "", seq(toDetail, keys("U", "enter"))...)
}
