package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/deploysetup"
)

// refused checks that a command without a terminal and without --yes names --yes, prints nothing and fails.
func refused(t *testing.T, args ...string) {
	t.Helper()
	for _, extra := range [][]string{nil, {"--agent"}} {
		code, stdout, stderr := run(t, append(args, extra...)...)
		if code == 0 || stdout != "" || !strings.Contains(stderr, "--yes") {
			t.Fatalf("%v %v: %d %q %q", args, extra, code, stdout, stderr)
		}
	}
}

func TestSourceDiscardWithoutTerminalNeedsYes(t *testing.T) {
	var seen []json.RawMessage
	svc := startFakeService(t, withStatus(changesHandlers(&seen), []string{"/srv/stacks"}, "web"))
	refused(t, "source", "discard", "/srv/stacks", "web/.env")
	if svc.count(control.SourceDiscardCommand) != 0 {
		t.Fatal("a refused discard called the service")
	}
}

func TestSecretDeleteWithoutTerminalNeedsYes(t *testing.T) {
	startSecretService(t, &syncBuffer{})
	if code, _, stderr := runWithStdin(t, fakeSecretValue+"\n", "stack", "secret", "set", "plain-stack", "DB_PASSWORD"); code != 0 {
		t.Fatalf("set: %d %q", code, stderr)
	}
	refused(t, "stack", "secret", "delete", "plain-stack", "DB_PASSWORD")
	if _, stdout, _ := run(t, "stack", "secret", "list", "plain-stack"); !strings.Contains(stdout, "DB_PASSWORD") {
		t.Fatalf("a refused delete removed the secret: %q", stdout)
	}
}

func TestCredentialDeleteWithoutTerminalNeedsYes(t *testing.T) {
	startService(t, allow)
	if code, _, _ := runWithStdin(t, demoToken, "credential", "set", "registry"); code != 0 {
		t.Fatal("set failed")
	}
	refused(t, "credential", "delete", "registry")
	if _, stdout, _ := run(t, "credential", "list"); !strings.Contains(stdout, "registry") {
		t.Fatalf("a refused delete removed the credential: %q", stdout)
	}
}

func TestNotifyRemoveWithoutTerminalNeedsYes(t *testing.T) {
	startService(t, allow)
	hook := newReceiver(t, 204)
	if code, _, stderr := runWithStdin(t, hook.URL+"/a\n"+demoHookKey+"\n", "notify", "add", "one", "--type", "webhook"); code != 0 {
		t.Fatalf("add: %d %q", code, stderr)
	}
	refused(t, "notify", "remove", "one")
	if _, stdout, _ := run(t, "notify", "list"); !strings.Contains(stdout, "one") {
		t.Fatalf("a refused remove removed the channel: %q", stdout)
	}
}

func TestHookRemoveWithoutTerminalNeedsYes(t *testing.T) {
	root := hookFixture(t, map[string]string{"web": "webhook"})
	id, _, _ := addHook(t, "web")
	refused(t, "hook", "remove", id)
	if cfg, _ := deploysetup.HookConfig(root); len(cfg.Hooks) != 1 {
		t.Fatalf("a refused remove changed the hooks: %+v", cfg)
	}
}
