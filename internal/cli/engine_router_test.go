package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/channeltest"
	"github.com/castrowithcee/qatlas-host/internal/control"
)

// withStatus adds the status answer to handlers, so that the router finds the stack folders and stacks.
func withStatus(h fakeHandlers, sources []string, stacks ...string) fakeHandlers {
	out := fakeHandlers{control.StatusCommand: statusWith(sources, stacks...)}
	for k, v := range h {
		out[k] = v
	}
	return out
}

func deployHandlers() fakeHandlers {
	return fakeHandlers{control.DeployCommand: func(raw json.RawMessage) any {
		var a control.DeployArgs
		_ = json.Unmarshal(raw, &a)
		return control.DeployResult{ID: "20260102T030405Z-0123abcd", Stack: a.Stack, Commit: "abc"}
	}}
}

func credentialHandlers() fakeHandlers {
	return fakeHandlers{
		control.CredentialListCommand: func(json.RawMessage) any { return control.CredentialListResult{} },
		control.CredentialSetCommand:  func(json.RawMessage) any { return struct{}{} },
	}
}

// noSystemService makes the host look as if the deploy service of the system engine was never set up.
func noSystemService(t *testing.T) {
	t.Helper()
	missing := channeltest.Endpoint(t)
	missing.Dir += "/missing"
	useEndpoint(t, missing)
	useDeploySetupRoot(t, t.TempDir())
}

func TestStackCommandReachesTheInstanceThatHasTheStack(t *testing.T) {
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice", "bob")
	system := startFakeService(t, withStatus(deployHandlers(), []string{"/srv/system"}, "web"))
	alice := serveFake(t, eps["alice"], withStatus(deployHandlers(), []string{"/srv/alice"}, "demo"))
	bob := serveFake(t, eps["bob"], withStatus(deployHandlers(), []string{"/srv/bob"}, "other"))

	code, stdout, stderr := run(t, "stack", "deploy", "demo", "--output", "json")
	if code != 0 || !strings.Contains(stdout, `"demo"`) || stderr != "" {
		t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if alice.count(control.DeployCommand) != 1 || system.count(control.DeployCommand) != 0 || bob.count(control.DeployCommand) != 0 {
		t.Fatalf("deploy reached system %d, alice %d, bob %d times",
			system.count(control.DeployCommand), alice.count(control.DeployCommand), bob.count(control.DeployCommand))
	}
}

func TestFolderCommandReachesTheInstanceThatHasTheFolderBound(t *testing.T) {
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice")
	fetch := fakeHandlers{control.RepoFetchCommand: func(json.RawMessage) any { return control.RepoFetchResult{Dir: "/srv/alice"} }}
	system := startFakeService(t, withStatus(fetch, []string{"/srv/system"}, "web"))
	alice := serveFake(t, eps["alice"], withStatus(fetch, []string{"/srv/alice"}, "demo"))

	if code, _, stderr := run(t, "repo", "fetch", "--dir", "/srv/alice"); code != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if alice.count(control.RepoFetchCommand) != 1 || system.count(control.RepoFetchCommand) != 0 {
		t.Fatal("the fetch did not reach only the instance that has the folder bound")
	}
	code, _, stderr := run(t, "repo", "fetch", "--dir", "/srv/unknown")
	if code == 0 || !strings.Contains(stderr, "no source is bound to this folder: /srv/unknown is in none of the engines system, rootless:alice") ||
		!strings.Contains(stderr, "not-found") || !strings.Contains(stderr, "qhost source list") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if alice.count(control.RepoFetchCommand) != 1 || system.count(control.RepoFetchCommand) != 0 {
		t.Fatal("a folder no engine has was fetched somewhere")
	}
}

func TestAmbiguousStackNamesBothEnginesAndEngineResolvesIt(t *testing.T) {
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice")
	system := startFakeService(t, withStatus(deployHandlers(), []string{"/srv/system"}, "demo"))
	alice := serveFake(t, eps["alice"], withStatus(deployHandlers(), []string{"/srv/alice"}, "demo"))

	code, _, stderr := run(t, "stack", "deploy", "demo")
	if code != 2 || !strings.Contains(stderr, "the stack demo exists in the engines system, rootless:alice") ||
		!strings.Contains(stderr, "--engine") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if system.count(control.DeployCommand) != 0 || alice.count(control.DeployCommand) != 0 {
		t.Fatal("an ambiguous stack was deployed")
	}

	before := system.count(control.StatusCommand) + alice.count(control.StatusCommand)
	if code, _, stderr = run(t, "stack", "deploy", "demo", "--engine", "rootless:alice"); code != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if alice.count(control.DeployCommand) != 1 || system.count(control.DeployCommand) != 0 {
		t.Fatal("--engine did not choose the instance")
	}
	if system.count(control.StatusCommand)+alice.count(control.StatusCommand) != before {
		t.Fatal("a named engine still asked the services")
	}
	if code, _, _ = run(t, "stack", "deploy", "demo", "--engine", control.EngineSystem); code != 0 || system.count(control.DeployCommand) != 1 {
		t.Fatalf("--engine system: code %d", code)
	}
}

func TestUnknownEngineIsAUsageErrorWithoutFallback(t *testing.T) {
	d := useFolderHost(t)
	engineHost(t, d, "alice")
	system := startFakeService(t, withStatus(deployHandlers(), nil, "demo"))
	for _, engine := range []string{"rootless:carol", "podman", "rootless:", "rootless:../etc"} {
		if code, _, stderr := run(t, "stack", "deploy", "demo", "--engine", engine); code != 2 {
			t.Errorf("%s: code %d, stderr %q", engine, code, stderr)
		}
	}
	if system.count(control.DeployCommand) != 0 {
		t.Fatal("an unknown engine fell back to the system engine")
	}

	noSystemService(t)
	if code, _, stderr := run(t, "stack", "deploy", "demo", "--engine", control.EngineSystem); code != 2 ||
		!strings.Contains(stderr, "system engine is not set up") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
}

func TestUnreachableInstanceIsAWarningWhenTheStackIsFound(t *testing.T) {
	d := useFolderHost(t)
	engineHost(t, d, "alice")
	system := startFakeService(t, withStatus(deployHandlers(), []string{"/srv/system"}, "demo"))

	code, _, stderr := run(t, "stack", "deploy", "demo")
	if code != 0 || system.count(control.DeployCommand) != 1 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if !strings.Contains(stderr, "warning: the deploy service of the rootless:alice engine could not be asked (not running)") {
		t.Fatalf("no warning about the instance: %q", stderr)
	}
}

func TestStackNotFoundWithAnUnreachableInstanceNamesIt(t *testing.T) {
	d := useFolderHost(t)
	engineHost(t, d, "alice")
	system := startFakeService(t, withStatus(deployHandlers(), []string{"/srv/system"}, "web"))

	code, _, stderr := run(t, "stack", "deploy", "demo")
	if code == 0 || !strings.Contains(stderr, "rootless:alice (not running)") || !strings.Contains(stderr, "--engine rootless:alice") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if system.count(control.DeployCommand) != 0 {
		t.Fatal("a stack that was not found was deployed in another engine")
	}
}

func TestSecretGoesOnlyToTheInstanceThatHasTheStack(t *testing.T) {
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice")
	secret := fakeHandlers{control.SecretSetCommand: func(json.RawMessage) any { return control.SecretResult{Stack: "demo", Key: "TOKEN"} }}
	system := startFakeService(t, withStatus(secret, []string{"/srv/system"}, "web"))
	alice := serveFake(t, eps["alice"], withStatus(secret, []string{"/srv/alice"}, "demo"))

	code, _, stderr := runWithStdin(t, "example-value", "stack", "secret", "set", "demo", "TOKEN")
	if code != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if alice.count(control.SecretSetCommand) != 1 || system.count(control.SecretSetCommand) != 0 {
		t.Fatal("the secret did not reach only the instance of the stack")
	}
	// A stack no engine has: the value reaches nobody.
	if code, _, _ = runWithStdin(t, "example-value", "stack", "secret", "set", "nope", "TOKEN"); code == 0 {
		t.Fatal("a secret of an unknown stack was accepted")
	}
	if alice.count(control.SecretSetCommand) != 1 || system.count(control.SecretSetCommand) != 0 {
		t.Fatal("the secret of an unknown stack reached a service")
	}
}

func TestUpdateRefusesStacksOfDifferentEngines(t *testing.T) {
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice")
	update := fakeHandlers{control.UpdateRunCommand: func(json.RawMessage) any { return control.UpdateRunResult{} }}
	system := startFakeService(t, withStatus(update, nil, "web"))
	alice := serveFake(t, eps["alice"], withStatus(update, nil, "demo"))

	code, _, stderr := run(t, "stack", "update", "web", "demo")
	if code != 2 || !strings.Contains(stderr, "the stack web is in the system engine and the stack demo in the rootless:alice engine") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if system.count(control.UpdateRunCommand) != 0 || alice.count(control.UpdateRunCommand) != 0 {
		t.Fatal("stacks of different engines were updated")
	}
}

func TestRoutingReadsWithoutRoot(t *testing.T) {
	d := useFolderHost(t)
	folders.euid = func() int { return 1000 }
	eps := engineHost(t, d, "alice")
	list := fakeHandlers{control.SecretListCommand: func(json.RawMessage) any { return control.SecretListResult{Keys: []string{"TOKEN"}} }}
	startFakeService(t, withStatus(list, nil, "web"))
	alice := serveFake(t, eps["alice"], withStatus(list, nil, "demo"))

	code, stdout, stderr := run(t, "stack", "secret", "list", "demo")
	if code != 0 || !strings.Contains(stdout, "TOKEN") || alice.count(control.SecretListCommand) != 1 {
		t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestInstanceSettingDefaultsToTheSystemEngine(t *testing.T) {
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice")
	system := startFakeService(t, credentialHandlers())
	alice := serveFake(t, eps["alice"], credentialHandlers())

	if code, _, stderr := run(t, "credential", "list"); code != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if system.count(control.CredentialListCommand) != 1 || alice.count(control.CredentialListCommand) != 0 {
		t.Fatal("without --engine the credentials of the system engine were not meant")
	}
	code, _, stderr := runWithStdin(t, "example-token", "credential", "set", "registry.example", "--engine", "rootless:alice")
	if code != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if alice.count(control.CredentialSetCommand) != 1 || system.count(control.CredentialSetCommand) != 0 {
		t.Fatal("the credential did not reach only the named instance")
	}
}

func TestInstanceSettingWithoutSystemEngineUsesTheOnlyInstance(t *testing.T) {
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice")
	noSystemService(t)
	alice := serveFake(t, eps["alice"], credentialHandlers())

	if code, _, stderr := run(t, "credential", "list"); code != 0 || alice.count(control.CredentialListCommand) != 1 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
}

func TestInstanceSettingWithoutSystemEngineAndTwoInstancesNeedsEngine(t *testing.T) {
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice", "bob")
	noSystemService(t)
	alice := serveFake(t, eps["alice"], credentialHandlers())
	bob := serveFake(t, eps["bob"], credentialHandlers())

	code, _, stderr := runWithStdin(t, "example-token", "credential", "set", "registry.example")
	if code != 2 || !strings.Contains(stderr, "--engine") || !strings.Contains(stderr, "rootless:alice, rootless:bob") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if alice.count(control.CredentialSetCommand)+bob.count(control.CredentialSetCommand) != 0 {
		t.Fatal("a credential reached an instance that was not named")
	}
	if code, _, stderr = runWithStdin(t, "example-token", "credential", "set", "registry.example", "--engine", "rootless:bob"); code != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if bob.count(control.CredentialSetCommand) != 1 || alice.count(control.CredentialSetCommand) != 0 {
		t.Fatal("the credential did not reach only the named instance")
	}
}

func TestOneDeployServiceIsReachedWithoutAsking(t *testing.T) {
	useFolderHost(t)
	system := startFakeService(t, deployHandlers())
	if code, _, stderr := run(t, "stack", "deploy", "demo"); code != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if system.count(control.StatusCommand) != 0 || system.count(control.DeployCommand) != 1 {
		t.Fatal("a host with one deploy service was asked for its stacks")
	}
}

func TestUnknownStackKeepsTheServiceWording(t *testing.T) {
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice")
	system := startFakeService(t, withStatus(deployHandlers(), []string{"/srv/system"}, "web"))
	alice := serveFake(t, eps["alice"], withStatus(deployHandlers(), []string{"/srv/alice"}, "demo"))

	code, _, stderr := run(t, "stack", "deploy", "nope")
	if code == 0 || !strings.Contains(stderr, "no such stack: nope is in none of the engines system, rootless:alice") ||
		!strings.Contains(stderr, "not-found") || !strings.Contains(stderr, "qhost stack list") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if system.count(control.DeployCommand)+alice.count(control.DeployCommand) != 0 {
		t.Fatal("an unknown stack was sent to a deploy service")
	}
}

func keyHandlers() fakeHandlers {
	return fakeHandlers{control.KeyShowCommand: func(json.RawMessage) any {
		return control.KeyResult{PublicKey: "ssh-ed25519 AAAA", Fingerprint: "SHA256:x"}
	}}
}

func TestKeyOfAnUnboundFolderFollowsTheInstanceRule(t *testing.T) {
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice")
	system := startFakeService(t, withStatus(keyHandlers(), []string{"/srv/system"}, "web"))
	alice := serveFake(t, eps["alice"], withStatus(keyHandlers(), []string{"/srv/alice"}, "demo"))

	if code, _, stderr := run(t, "key", "show", "--dir", "/srv/new"); code != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if system.count(control.KeyShowCommand) != 1 || alice.count(control.KeyShowCommand) != 0 {
		t.Fatal("the key of an unbound folder did not follow the system engine")
	}
	if code, _, stderr := run(t, "key", "show", "--dir", "/srv/alice"); code != 0 || alice.count(control.KeyShowCommand) != 1 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if code, _, stderr := run(t, "key", "show", "--dir", "/srv/new", "--engine", "rootless:alice"); code != 0 || alice.count(control.KeyShowCommand) != 2 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
}

func TestKeyOfAnUnboundFolderNeedsEngineWithTwoInstancesAndNoSystemService(t *testing.T) {
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice", "bob")
	noSystemService(t)
	alice := serveFake(t, eps["alice"], withStatus(keyHandlers(), []string{"/srv/alice"}))
	bob := serveFake(t, eps["bob"], withStatus(keyHandlers(), []string{"/srv/bob"}))

	code, _, stderr := run(t, "key", "show", "--dir", "/srv/new")
	if code != 2 || !strings.Contains(stderr, "--engine") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if alice.count(control.KeyShowCommand)+bob.count(control.KeyShowCommand) != 0 {
		t.Fatal("a key request reached an instance that was not named")
	}
}

func TestKeyOfAnUnboundFolderIsNotSentWhenAnInstanceCannotBeAsked(t *testing.T) {
	d := useFolderHost(t)
	engineHost(t, d, "alice")
	system := startFakeService(t, withStatus(keyHandlers(), []string{"/srv/system"}))

	code, _, stderr := run(t, "key", "show", "--dir", "/srv/new")
	if code == 0 || !strings.Contains(stderr, "rootless:alice (not running)") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if system.count(control.KeyShowCommand) != 0 {
		t.Fatal("a key request fell back to another engine")
	}
}
