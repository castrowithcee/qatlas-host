package tui

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-kit/i18n"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// A form with a secret, saved without root, asks with the command without the value and runs it through sudo in
// two steps. The value is on standard input only, once and whole, and gone from the buffer and from every message
// afterwards.

// secretRun checks what the run of a command with a secret did and returns its standard input.
func secretRun(t *testing.T, f *fakeRoot, args []string) []byte {
	t.Helper()
	if len(f.reqs) != 2 || !f.reqs[0].Validate || f.reqs[1].Validate {
		t.Fatalf("requests %+v, want the password check then the command", f.reqs)
	}
	if !slices.Equal(f.reqs[1].Args, args) {
		t.Fatalf("arguments %v, want %v", f.reqs[1].Args, args)
	}
	if len(f.execs) != 1 || len(f.bgs) != 1 || !slices.Equal(f.bgs[0].Args, append([]string{"sudo-fake", "-n", "qhost-fake"}, args...)) ||
		!slices.Equal(f.bgs[0].Env, []string{"LC_ALL=C"}) {
		t.Fatalf("the command was not run once without a prompt: execs %v, background %v", f.execs, f.bgs)
	}
	if len(f.stdins) != 1 {
		t.Fatalf("standard input read %d times", len(f.stdins))
	}
	if secret := f.reqs[1].Secret; len(secret) == 0 || !bytes.Equal(secret, make([]byte, len(secret))) {
		t.Fatalf("the secret was not overwritten: %q", secret)
	}
	return f.stdins[0]
}

func noValueIn(t *testing.T, f *fakeRoot, values ...string) {
	t.Helper()
	for _, req := range f.reqs {
		for _, a := range req.Args {
			for _, v := range values {
				if strings.Contains(a, v) {
					t.Fatalf("the value is in the arguments: %v", req.Args)
				}
			}
		}
	}
	for _, c := range append(slices.Clone(f.execs), f.bgs...) {
		for _, a := range c.Args {
			for _, v := range values {
				if strings.Contains(a, v) {
					t.Fatalf("the value is in a command line: %v", c.Args)
				}
			}
		}
		for _, e := range c.Env {
			for _, v := range values {
				if strings.Contains(e, v) {
					t.Fatalf("the value is in the environment: %v", c.Env)
				}
			}
		}
	}
}

func TestSecretSetWithoutRootHandsTheValueOnStandardInput(t *testing.T) {
	b := settingsBackend()
	m, f := sudoDetailModel(t, i18n.English, b, wideWidth, 40)
	f.report = `{"stack":"demo-stack","key":"SERVICE_TOKEN","stored":true,"replaced":false}`
	send(m, seq(secretFormKeys(), keys("f2"))...)
	args := []string{"stack", "secret", "set", "--output", "json", "--", "demo-stack", "SERVICE_TOKEN"}
	command := "sudo qhost " + strings.Join(args, " ")
	if !shows(m.render(), command) {
		t.Fatalf("the question lacks %q:\n%s", command, m.render())
	}
	noLeak(t, m.render(), hiddenA)
	if len(f.reqs) != 0 {
		t.Fatalf("sudo ran before the answer: %+v", f.reqs)
	}
	send(m, "y")
	if got := string(secretRun(t, f, args)); got != hiddenA {
		t.Fatalf("standard input %q, want the value alone", got)
	}
	noValueIn(t, f, hiddenA)
	if got := b.actions(); len(got) != 0 {
		t.Fatalf("the service was called directly: %v", got)
	}
	contains(t, m, "Secret SERVICE_TOKEN set for the stack demo-stack.")
	noLeak(t, m.render(), hiddenA)
}

func TestSecretFailureTextHoldsNoValue(t *testing.T) {
	m, f := sudoDetailModel(t, i18n.English, settingsBackend(), wideWidth, 40)
	f.stderr = "qhost: invalid: the value " + hiddenA + " is refused\n"
	f.runErr = exitCode(t, "1")
	send(m, seq(secretFormKeys(), keys("f2", "y"))...)
	contains(t, m, "qhost reported: the value <hidden> is refused")
	noLeak(t, m.render(), hiddenA)
	if m.set.form == nil || m.set.busy {
		t.Fatalf("form %v, busy %v", m.set.form, m.set.busy)
	}
}

func TestSecretCancelledRunsNothing(t *testing.T) {
	m, f := sudoDetailModel(t, i18n.English, settingsBackend(), wideWidth, 40)
	send(m, seq(secretFormKeys(), keys("f2", "n"))...)
	if len(f.reqs) != 0 || m.set.busy {
		t.Fatalf("cancelling ran something: %+v", f.reqs)
	}
}

func TestCredentialSetWithoutRootHandsTheLoginOnStandardInput(t *testing.T) {
	b := settingsBackend()
	m, f := sudoDetailModel(t, i18n.English, b, wideWidth, 40)
	f.report = `{"name":"registry.registry.example.test-5000","stored":true}`
	hidden := []string{hiddenA}
	send(m, toCreds...)
	send(m, "a", "down")
	sendHidden(t, m, hidden, typed("Registry.Example.test:5000")...)
	sendHidden(t, m, hidden, "down")
	sendHidden(t, m, hidden, typed("alice")...)
	sendHidden(t, m, hidden, "down")
	sendHidden(t, m, hidden, typed(hiddenA)...)
	sendHidden(t, m, hidden, "f2")
	args := []string{"credential", "set", "--output", "json", "--", "registry.registry.example.test-5000"}
	if command := "sudo qhost " + strings.Join(args, " "); !shows(m.render(), command) {
		t.Fatalf("the question lacks %q:\n%s", command, m.render())
	}
	send(m, "y")
	if got := string(secretRun(t, f, args)); got != "alice:"+hiddenA {
		t.Fatalf("standard input %q", got)
	}
	noValueIn(t, f, hiddenA)
	if len(b.credSets) != 0 {
		t.Fatalf("the service was called directly: %+v", b.credSets)
	}
	contains(t, m, "The credential registry.registry.example.test-5000 is stored.")
}

func TestWebhookChannelWithoutRootHandsBothSecretsOnStandardInput(t *testing.T) {
	b := settingsBackend()
	m, f := sudoDetailModel(t, i18n.English, b, wideWidth, 40)
	f.report = `{"name":"deploy-hook","type":"webhook","events":"succeeded"}`
	hidden := []string{hiddenA, hiddenB}
	send(m, toNotify...)
	send(m, "a")
	sendHidden(t, m, hidden, typed("deploy-hook")...)
	sendHidden(t, m, hidden, "down", "down", "space", "f2")
	sendHidden(t, m, hidden, typed(hiddenA)...)
	sendHidden(t, m, hidden, "down")
	sendHidden(t, m, hidden, typed(hiddenB)...)
	sendHidden(t, m, hidden, "f2")
	args := []string{"notify", "add", "--type", "webhook", "--events", "succeeded", "--output", "json", "--", "deploy-hook"}
	if command := "sudo qhost " + strings.Join(args, " "); !shows(m.render(), command) {
		t.Fatalf("the question lacks %q:\n%s", command, m.render())
	}
	send(m, "y")
	if got := string(secretRun(t, f, args)); got != hiddenA+"\n"+hiddenB {
		t.Fatalf("standard input %q, want both lines", got)
	}
	noValueIn(t, f, hiddenA, hiddenB)
	if len(b.adds) != 0 {
		t.Fatalf("the service was called directly: %+v", b.adds)
	}
	contains(t, m, "The channel deploy-hook is added.")
}

// Each part of a channel is taken out of a message on its own, since a message may quote one of them.
func TestChannelFailureTextHoldsNoPartOfTheSecrets(t *testing.T) {
	m, f := sudoDetailModel(t, i18n.English, settingsBackend(), wideWidth, 40)
	f.stderr = "qhost: invalid: the key " + hiddenB + " is refused\n"
	f.runErr = exitCode(t, "1")
	hidden := []string{hiddenA, hiddenB}
	send(m, toNotify...)
	send(m, "a")
	sendHidden(t, m, hidden, typed("deploy-hook")...)
	sendHidden(t, m, hidden, "down", "down", "space", "f2")
	sendHidden(t, m, hidden, typed(hiddenA)...)
	sendHidden(t, m, hidden, "down")
	sendHidden(t, m, hidden, typed(hiddenB)...)
	sendHidden(t, m, hidden, "f2", "y")
	contains(t, m, "qhost reported: the key <hidden> is refused")
	noLeak(t, m.render(), hiddenA, hiddenB)
}

func TestSMTPChannelWithoutRootHandsOnlyThePasswordOnStandardInput(t *testing.T) {
	b := settingsBackend()
	m, f := sudoDetailModel(t, i18n.English, b, wideWidth, 40)
	f.report = `{"name":"alerts","type":"smtp","events":"","settings":"host=mail.example.test"}`
	hidden := []string{hiddenA}
	send(m, toNotify...)
	send(m, "a")
	send(m, typed("alerts")...)
	send(m, "down", "right", "right", "right", "f2")
	send(m, typed("mail.example.test")...)
	send(m, "down")
	send(m, typed("2525")...)
	send(m, "down", "right", "down")
	send(m, typed("qd")...)
	send(m, "down")
	send(m, typed(hiddenA)...)
	send(m, "down")
	send(m, typed("qhost@example.test")...)
	send(m, "down")
	send(m, typed("ops@example.test, dev@example.test")...)
	sendHidden(t, m, hidden, "f2")
	args := []string{"notify", "add", "--type", "smtp", "--host", "mail.example.test", "--security", "tls", "--user", "qd",
		"--from", "qhost@example.test", "--to", "ops@example.test,dev@example.test", "--port", "2525", "--output", "json", "--", "alerts"}
	if command := "sudo qhost " + strings.Join(args, " "); !shows(m.render(), command) {
		t.Fatalf("the question lacks %q:\n%s", command, m.render())
	}
	send(m, "y")
	if got := string(secretRun(t, f, args)); got != hiddenA {
		t.Fatalf("standard input %q, want the password alone", got)
	}
	noValueIn(t, f, hiddenA)
	contains(t, m, "The channel alerts is added.")
}

// A channel with nothing secret reads nothing, so it runs as any other command, with the password prompt on the
// terminal.
func TestSMTPChannelWithoutUserNeedsNoStandardInput(t *testing.T) {
	m, f := sudoDetailModel(t, i18n.English, settingsBackend(), wideWidth, 40)
	f.report = `{"name":"alerts","type":"smtp","events":""}`
	send(m, toNotify...)
	send(m, "a")
	send(m, typed("alerts")...)
	send(m, "down", "right", "right", "right", "f2")
	send(m, typed("mail.example.test")...)
	send(m, "down", "down", "down", "down", "down")
	send(m, typed("qhost@example.test")...)
	send(m, "down")
	send(m, typed("ops@example.test")...)
	send(m, "f2", "y")
	if len(f.reqs) != 1 || len(f.reqs[0].Secret) != 0 || len(f.execs) != 1 || len(f.bgs) != 0 {
		t.Fatalf("requests %+v, execs %v, background %v", f.reqs, f.execs, f.bgs)
	}
	contains(t, m, "The channel alerts is added.")
}

func TestKeySetWithoutRootHandsTheWholeKeyOnStandardInput(t *testing.T) {
	key := testKeyPEM(t)
	b := keyBackend()
	m, f := sudoDetailModel(t, i18n.English, b, wideWidth, 40)
	f.report = `{"public_key":"ssh-ed25519 AAAAexamplekey qhost","fingerprint":"` + fingerprint + `"}`
	sendKeyChecked(t, m, key, toKey...)
	sendKeyChecked(t, m, key, "e", "f2")
	paste(m, key)
	sendKeyChecked(t, m, key, "f2")
	if m.modal == nil || len(f.reqs) != 0 {
		t.Fatalf("no question before the run:\n%s", m.render())
	}
	args := []string{"key", "set", "--dir", stackDir, "--output", "json"}
	if command := "sudo qhost " + strings.Join(args, " "); !shows(m.render(), command) {
		t.Fatalf("the question lacks %q:\n%s", command, m.render())
	}
	noKeyText(t, m.render(), key)
	send(m, "y")
	if got := string(secretRun(t, f, args)); got != key || !strings.Contains(got, "\n") {
		t.Fatalf("standard input %q, want the whole block %q", got, key)
	}
	for _, line := range strings.Split(key, "\n") {
		if len(line) >= 8 {
			noValueIn(t, f, line)
		}
	}
	if len(b.keyArgs) != 0 {
		t.Fatalf("the service was called directly: %+v", b.keyArgs)
	}
	contains(t, m, "The deploy key is stored.", fingerprint)
	noKeyText(t, m.render(), key)
}

// An existing key is replaced only with --replace, which the red question names, and a line of the key in the
// message of qhost is taken out.
func TestKeySetWithoutRootReplacesOnlyAfterTheQuestionAndScrubsMessages(t *testing.T) {
	key := testKeyPEM(t)
	line := strings.Split(key, "\n")[1]
	b := keyBackend()
	b.key = &control.KeyResult{PublicKey: "ssh-ed25519 AAAAexamplekey qhost", Fingerprint: fingerprint}
	m, f := sudoDetailModel(t, i18n.English, b, wideWidth, 40)
	f.stderr = "qhost: invalid: no key like " + line + "\n"
	f.runErr = exitCode(t, "1")
	sendKeyChecked(t, m, key, toKey...)
	sendKeyChecked(t, m, key, "e", "f2")
	paste(m, key)
	sendKeyChecked(t, m, key, "f2")
	contains(t, m, "Replace the existing deploy key?", "--replace")
	send(m, "y")
	if len(f.reqs) != 2 || !slices.Contains(f.reqs[1].Args, "--replace") {
		t.Fatalf("requests %+v", f.reqs)
	}
	contains(t, m, "qhost reported: no key like <hidden>")
	noKeyText(t, m.render(), key)
}

func TestSecretLinesMasksEachLineOfAMultiLineSecret(t *testing.T) {
	key := testKeyPEM(t)
	var got, want []string
	for _, l := range secretLines([]byte(key)) {
		got = append(got, string(l))
	}
	for _, l := range strings.Split(strings.TrimSpace(key), "\n") {
		if !strings.HasPrefix(l, "-----") {
			want = append(want, l)
		}
	}
	if len(want) == 0 || !slices.Equal(got, want) {
		t.Errorf("key lines %q, want %q", got, want)
	}
	if got := secretLines([]byte("a-single-line-value")); got != nil {
		t.Errorf("a single line gave %q", got)
	}
	if got := secretLines([]byte("one-value\nx")); len(got) != 1 || string(got[0]) != "one-value" {
		t.Errorf("a short line is masked: %q", got)
	}
}
