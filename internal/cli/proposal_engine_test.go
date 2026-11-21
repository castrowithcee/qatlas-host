package cli

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

const (
	engineProposalID = "p-aaaaaaaaaaaaaaaa"
	otherProposalID  = "p-bbbbbbbbbbbbbbbb"
)

func notFoundProposal(json.RawMessage) any {
	return &control.Error{Code: control.CodeNotFound, Message: "no such proposal"}
}

// holdsProposal answers show, withdraw and list for one open proposal.
func holdsProposal(id string) fakeHandlers {
	info := proposalInfo(id, control.ProposalOpen)
	return fakeHandlers{
		control.ProposalShowCommand: func(json.RawMessage) any {
			return control.ProposalShowResult{ProposalInfo: info, Content: json.RawMessage(`{"path":"/srv/x"}`)}
		},
		control.ProposalWithdrawCommand: func(json.RawMessage) any { return proposalInfo(id, control.ProposalWithdrawn) },
		control.ProposalListCommand: func(json.RawMessage) any {
			return control.ProposalListResult{Proposals: []control.ProposalInfo{info}}
		},
	}
}

func holdsNothing() fakeHandlers {
	return fakeHandlers{
		control.ProposalShowCommand:     notFoundProposal,
		control.ProposalWithdrawCommand: notFoundProposal,
		control.ProposalListCommand:     func(json.RawMessage) any { return control.ProposalListResult{} },
	}
}

func submitHandlers(id string) fakeHandlers {
	return fakeHandlers{control.ProposalSubmitCommand: func(json.RawMessage) any { return proposalInfo(id, control.ProposalOpen) }}
}

func TestProposalSubmitGoesToTheChosenInstance(t *testing.T) {
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice", "bob")
	system := startFakeService(t, submitHandlers(engineProposalID))
	alice := serveFake(t, eps["alice"], submitHandlers(engineProposalID))
	bob := serveFake(t, eps["bob"], submitHandlers(engineProposalID))

	withStdin(t, strings.NewReader(`{"path":"/srv/stacks"}`))
	code, stdout, stderr := run(t, "proposal", "submit", "--kind", "source-add", "--engine", "rootless:alice", "--output", "json")
	if code != 0 || !strings.Contains(stdout, `"engine":"rootless:alice"`) {
		t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if alice.count(control.ProposalSubmitCommand) != 1 || system.count(control.ProposalSubmitCommand) != 0 || bob.count(control.ProposalSubmitCommand) != 0 {
		t.Fatal("the proposal did not reach only the chosen instance")
	}

	// Without --engine the system engine takes it, as for the settings of a whole engine.
	withStdin(t, strings.NewReader(`{"path":"/srv/stacks"}`))
	if code, stdout, stderr = run(t, "proposal", "submit", "--kind", "source-add", "--output", "json"); code != 0 ||
		!strings.Contains(stdout, `"engine":"system"`) || system.count(control.ProposalSubmitCommand) != 1 {
		t.Fatalf("default: code %d, stdout %q, stderr %q", code, stdout, stderr)
	}

	// An unknown instance is refused and nothing is submitted elsewhere.
	withStdin(t, strings.NewReader(`{"path":"/srv/stacks"}`))
	if code, _, _ = run(t, "proposal", "submit", "--kind", "source-add", "--engine", "rootless:carol"); code != 2 {
		t.Fatalf("unknown engine: code %d", code)
	}
	if system.count(control.ProposalSubmitCommand) != 1 || alice.count(control.ProposalSubmitCommand) != 1 {
		t.Fatal("a proposal for an unknown engine was submitted somewhere")
	}
}

func TestProposalSubmitWithSeveralInstancesAndNoSystemServiceNeedsTheEngine(t *testing.T) {
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice", "bob")
	alice := serveFake(t, eps["alice"], submitHandlers(engineProposalID))
	bob := serveFake(t, eps["bob"], submitHandlers(engineProposalID))
	noSystemService(t)

	withStdin(t, strings.NewReader(`{"path":"/srv/stacks"}`))
	code, _, stderr := run(t, "proposal", "submit", "--kind", "source-add")
	if code != 2 || !strings.Contains(stderr, "name the engine") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if alice.count(control.ProposalSubmitCommand)+bob.count(control.ProposalSubmitCommand) != 0 {
		t.Fatal("the proposal was submitted without a chosen engine")
	}
}

func TestProposalSubmitOnAHostWithOnlyAnInstanceUsesIt(t *testing.T) {
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice")
	alice := serveFake(t, eps["alice"], submitHandlers(engineProposalID))
	noSystemService(t)

	withStdin(t, strings.NewReader(`{"path":"/srv/stacks"}`))
	code, stdout, stderr := run(t, "proposal", "submit", "--kind", "source-add", "--output", "json")
	if code != 0 || !strings.Contains(stdout, `"engine":"rootless:alice"`) || alice.count(control.ProposalSubmitCommand) != 1 {
		t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if strings.Contains(stderr, "not set up") {
		t.Fatalf("the deploy area is reported as not set up: %q", stderr)
	}
}

func TestProposalSubmitHostChangeTakesNoEngine(t *testing.T) {
	d := useFolderHost(t)
	engineHost(t, d, "alice")
	system := startFakeService(t, submitHandlers(engineProposalID))
	host := startFakeHostService(t, fakeHandlers{control.ProposalSubmitCommand: func(json.RawMessage) any {
		return proposalInfo("h-0123456789abcdef", control.ProposalOpen)
	}})

	withStdin(t, strings.NewReader(`{"action":"user.create","args":["anna"]}`))
	if code, _, stderr := run(t, "proposal", "submit", "--kind", "host-change", "--engine", "rootless:alice"); code != 2 ||
		!strings.Contains(stderr, "host-change") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	withStdin(t, strings.NewReader(`{"action":"user.create","args":["anna"]}`))
	code, stdout, stderr := run(t, "proposal", "submit", "--kind", "host-change", "--output", "json")
	if code != 0 || host.count(control.ProposalSubmitCommand) != 1 || system.count(control.ProposalSubmitCommand) != 0 ||
		strings.Contains(stdout, `"engine"`) {
		t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestProposalListShowsEveryInstanceWithItsEngine(t *testing.T) {
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice", "bob")
	at := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	listOf := func(id string, age time.Duration) fakeHandlers {
		info := proposalInfo(id, control.ProposalOpen)
		info.Submitted = at.Add(age)
		return fakeHandlers{control.ProposalListCommand: func(json.RawMessage) any {
			return control.ProposalListResult{Proposals: []control.ProposalInfo{info}}
		}}
	}
	startFakeService(t, listOf("p-1111111111111111", 0))
	serveFake(t, eps["alice"], listOf("p-2222222222222222", time.Hour))
	// bob does not run.
	startFakeHostService(t, fakeHandlers{control.ProposalListCommand: func(json.RawMessage) any {
		info := proposalInfo("h-3333333333333333", control.ProposalOpen)
		info.Submitted = at.Add(2 * time.Hour)
		return control.ProposalListResult{Proposals: []control.ProposalInfo{info}}
	}})

	code, stdout, stderr := run(t, "proposal", "list", "--output", "json")
	var rows []map[string]any
	if err := json.Unmarshal([]byte(stdout), &rows); code != 0 || err != nil || len(rows) != 3 {
		t.Fatalf("code %d, stdout %q, stderr %q: %v", code, stdout, stderr, err)
	}
	want := []struct{ id, service, engine string }{
		{"h-3333333333333333", "host", ""}, {"p-2222222222222222", "deploy", "rootless:alice"}, {"p-1111111111111111", "deploy", "system"}}
	for i, w := range want {
		if rows[i]["id"] != w.id || rows[i]["service"] != w.service || rows[i]["engine"] != w.engine {
			t.Fatalf("row %d is %v, want %+v", i, rows[i], w)
		}
	}
	if !strings.Contains(stderr, "warning: the deploy service for the rootless engine of bob is not running; its proposals are not listed") {
		t.Fatalf("no warning about bob: %q", stderr)
	}
}

func TestProposalListOnAHostWithOnlyInstancesIsNotNotSetUp(t *testing.T) {
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice")
	serveFake(t, eps["alice"], holdsProposal(engineProposalID))
	noSystemService(t)

	code, stdout, stderr := run(t, "proposal", "list", "--output", "json")
	var rows []map[string]any
	if err := json.Unmarshal([]byte(stdout), &rows); code != 0 || err != nil || len(rows) != 1 || rows[0]["engine"] != "rootless:alice" {
		t.Fatalf("code %d, stdout %q, stderr %q: %v", code, stdout, stderr, err)
	}
	if strings.Contains(stderr, "not set up") {
		t.Fatalf("the deploy area is reported as not set up: %q", stderr)
	}
}

func TestProposalListWithoutAnyServiceFails(t *testing.T) {
	d := useFolderHost(t)
	engineHost(t, d, "alice", "bob")
	noSystemService(t)
	code, _, stderr := run(t, "proposal", "list")
	if code == 0 || !strings.Contains(stderr, "neither the host service nor any deploy service answers") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
}

func TestProposalLookupFindsTheInstanceThatHoldsTheId(t *testing.T) {
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice")
	system := startFakeService(t, holdsNothing())
	alice := serveFake(t, eps["alice"], holdsProposal(engineProposalID))

	code, stdout, stderr := run(t, "proposal", "show", engineProposalID, "--output", "json")
	if code != 0 || !strings.Contains(stdout, `"engine":"rootless:alice"`) || stderr != "" {
		t.Fatalf("show: code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	code, stdout, stderr = run(t, "proposal", "withdraw", engineProposalID, "--output", "json")
	if code != 0 || !strings.Contains(stdout, "withdrawn") || !strings.Contains(stdout, `"engine":"rootless:alice"`) {
		t.Fatalf("withdraw: code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if alice.count(control.ProposalWithdrawCommand) != 1 || system.count(control.ProposalWithdrawCommand) != 0 {
		t.Fatal("the withdrawal did not reach only the instance that holds the proposal")
	}
}

func TestProposalLookupOfAnUnknownIdNamesTheEngines(t *testing.T) {
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice")
	system := startFakeService(t, holdsNothing())
	alice := serveFake(t, eps["alice"], holdsNothing())

	code, _, stderr := run(t, "proposal", "withdraw", otherProposalID)
	if code == 0 || !strings.Contains(stderr, "no such proposal: "+otherProposalID+" is in none of the engines system, rootless:alice") ||
		!strings.Contains(stderr, "qhost proposal list") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if system.count(control.ProposalWithdrawCommand)+alice.count(control.ProposalWithdrawCommand) != 0 {
		t.Fatal("an unknown proposal was withdrawn somewhere")
	}
}

func TestProposalLookupOfAnIdInTwoInstancesNamesBothAndEngineResolves(t *testing.T) {
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice")
	system := startFakeService(t, holdsProposal(engineProposalID))
	alice := serveFake(t, eps["alice"], holdsProposal(engineProposalID))

	code, _, stderr := run(t, "proposal", "withdraw", engineProposalID)
	if code != 2 || !strings.Contains(stderr, "the proposal "+engineProposalID+" exists in the engines system, rootless:alice") ||
		!strings.Contains(stderr, "--engine") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if system.count(control.ProposalWithdrawCommand)+alice.count(control.ProposalWithdrawCommand) != 0 {
		t.Fatal("an ambiguous proposal was withdrawn")
	}
	before := system.count(control.ProposalShowCommand) + alice.count(control.ProposalShowCommand)
	if code, _, stderr = run(t, "proposal", "withdraw", engineProposalID, "--engine", "rootless:alice"); code != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if alice.count(control.ProposalWithdrawCommand) != 1 || system.count(control.ProposalWithdrawCommand) != 0 {
		t.Fatal("--engine did not choose the instance")
	}
	if system.count(control.ProposalShowCommand)+alice.count(control.ProposalShowCommand) != before {
		t.Fatal("a named engine still asked the services")
	}
}

func TestProposalLookupNeverFallsBackWhenTheHolderCannotBeAsked(t *testing.T) {
	d := useFolderHost(t)
	engineHost(t, d, "alice")
	system := startFakeService(t, holdsNothing())
	// alice does not run: the proposal may be hers.

	code, _, stderr := run(t, "proposal", "withdraw", engineProposalID)
	if code == 0 || !strings.Contains(stderr, "rootless:alice (not running)") || !strings.Contains(stderr, "--engine rootless:alice") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if system.count(control.ProposalWithdrawCommand) != 0 {
		t.Fatal("a proposal that may belong to an unreachable instance was withdrawn in another engine")
	}
}

func TestProposalLookupWarnsAboutAnInstanceThatCannotBeAsked(t *testing.T) {
	d := useFolderHost(t)
	engineHost(t, d, "alice")
	system := startFakeService(t, holdsProposal(engineProposalID))

	code, _, stderr := run(t, "proposal", "withdraw", engineProposalID)
	if code != 0 || system.count(control.ProposalWithdrawCommand) != 1 ||
		!strings.Contains(stderr, "warning: the deploy service of the rootless:alice engine could not be asked (not running)") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
}

func TestProposalLookupOnAHostWithOnlyAnInstance(t *testing.T) {
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice")
	alice := serveFake(t, eps["alice"], holdsProposal(engineProposalID))
	noSystemService(t)

	code, stdout, stderr := run(t, "proposal", "show", engineProposalID, "--output", "json")
	if code != 0 || !strings.Contains(stdout, `"engine":"rootless:alice"`) || strings.Contains(stderr, "not set up") {
		t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if alice.count(control.ProposalShowCommand) != 1 {
		t.Fatal("the instance was not asked")
	}
	if code, _, stderr = run(t, "proposal", "show", engineProposalID, "--engine", control.EngineSystem); code != 2 ||
		!strings.Contains(stderr, "system engine is not set up") {
		t.Fatalf("named system engine: code %d, stderr %q", code, stderr)
	}
}

func TestProposalEngineFlagDoesNotApplyToAProposalOfTheHost(t *testing.T) {
	d := useFolderHost(t)
	engineHost(t, d, "alice")
	startFakeService(t, holdsNothing())
	if code, _, stderr := run(t, "proposal", "show", "h-0123456789abcdef", "--engine", "rootless:alice"); code != 2 ||
		!strings.Contains(stderr, "does not apply to a proposal of the host") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
}

// instanceApprove serves the deploy service of the instance alice with a source-add proposal, beside a system
// service that holds none.
func instanceApprove(t *testing.T, env *approveEnv) (system, alice *fakeService, d *hostDouble) {
	t.Helper()
	d = useFolderHost(t)
	eps := engineHost(t, d, "alice")
	system = startFakeService(t, holdsNothing())
	alice = serveFake(t, eps["alice"], env.handlers())
	return system, alice, d
}

func TestProposalApproveAppliesOnlyInTheInstanceThatStoredIt(t *testing.T) {
	env := &approveEnv{dir: t.TempDir()}
	system, alice, d := instanceApprove(t, env)
	useTerminalStdin(t, "y\n")

	code, stdout, stderr := run(t, "proposal", "approve", approveID, "--output", "json")
	if code != 0 {
		t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "Engine: rootless:alice") {
		t.Fatalf("the approval does not show the engine: %q", stderr)
	}
	if alice.count(control.ProposalApproveCommand) != 1 || alice.count(control.SourceAddCommand) != 1 || alice.count(control.ProposalResultCommand) != 1 {
		t.Fatalf("the instance did not do the whole approval: %v", alice.calls)
	}
	for _, c := range []string{control.ProposalApproveCommand, control.SourceAddCommand, control.ProposalResultCommand, control.SourceInspectCommand} {
		if system.count(c) != 0 {
			t.Fatalf("the system service was asked %s", c)
		}
	}
	// The takeover is the one of source add --engine: the drop-in of the instance, not that of the system service.
	inst := d.instanceList[0]
	if got := d.dropIn(t); got != "" {
		t.Fatalf("the drop-in of the system service was written: %q", got)
	}
	if !strings.Contains(strings.Join(d.cmds, ";"), "restart "+inst.Unit()) {
		t.Fatalf("commands %q", d.cmds)
	}
}

func TestProposalApproveOfAnUnreachableInstanceDoesNotWorkElsewhere(t *testing.T) {
	d := useFolderHost(t)
	engineHost(t, d, "alice")
	env := &approveEnv{dir: t.TempDir()}
	// The system service holds a proposal of that id: it must not take over for the instance that is named.
	system := startFakeService(t, env.handlers())
	useTerminalStdin(t, "y\n")
	code, _, stderr := run(t, "proposal", "approve", approveID, "--engine", "rootless:alice")
	if code == 0 || !strings.Contains(stderr, "rootless engine of alice is not running") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	for _, c := range []string{control.ProposalApproveCommand, control.SourceAddCommand, control.SourceInspectCommand} {
		if system.count(c) != 0 {
			t.Fatalf("the system service was asked %s although the instance was named", c)
		}
	}
}

func TestProposalApproveWarnsAboutContainersInAnotherEngine(t *testing.T) {
	env := &approveEnv{dir: t.TempDir(), inspect: func(int) control.SourceInspectResult {
		res := localInspect(false)
		res.Stacks[0].Conflicts = []control.SourceConflict{{Container: "old-web-1", Project: "oldweb", Engine: "rootless:bob"}}
		return res
	}}
	_, alice, _ := instanceApprove(t, env)
	useTerminalStdin(t, "y\n")

	code, _, stderr := run(t, "proposal", "approve", approveID, "--output", "json")
	if code != 0 {
		t.Fatalf("the warning blocked the approval: code %d, stderr %q", code, stderr)
	}
	for _, want := range []string{
		"warning: containers of this folder run in the engine rootless:bob, but this proposal belongs to the rootless:alice engine",
		"duplicate containers", "reject the proposal and submit it again with --engine rootless:bob", "source add",
	} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr lacks %q: %q", want, stderr)
		}
	}
	if alice.count(control.SourceAddCommand) != 1 {
		t.Fatal("the warning blocked the takeover")
	}
}

func TestProposalApproveDoesNotWarnWithoutContainersInAnotherEngine(t *testing.T) {
	env := &approveEnv{dir: t.TempDir()}
	instanceApprove(t, env)
	useTerminalStdin(t, "y\n")
	code, _, stderr := run(t, "proposal", "approve", approveID, "--output", "json")
	if code != 0 || strings.Contains(stderr, "but this proposal belongs") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
}

func TestProposalApproveReviewedWarnsAndUsesTheInstance(t *testing.T) {
	env := &approveEnv{dir: t.TempDir(), inspect: func(int) control.SourceInspectResult {
		res := localInspect(false)
		res.Stacks[0].Conflicts = []control.SourceConflict{{Container: "old-web-1", Project: "oldweb", Engine: "rootless:bob"}}
		return res
	}}
	system, alice, _ := instanceApprove(t, env)
	useTerminalStdin(t, "")
	code, _, stderr := run(t, "proposal", "approve", approveID, "--fingerprint", "fp-one", "--confirm", "--output", "json")
	if code != 0 || !strings.Contains(stderr, "Engine: rootless:alice") || !strings.Contains(stderr, "but this proposal belongs to the rootless:alice engine") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if alice.count(control.SourceAddCommand) != 1 || system.count(control.SourceAddCommand) != 0 {
		t.Fatal("the reviewed approval did not apply in the instance only")
	}
}

func TestProposalRejectReachesTheInstanceThatStoredIt(t *testing.T) {
	env := &approveEnv{dir: t.TempDir()}
	system, alice, _ := instanceApprove(t, env)
	useTerminalStdin(t, "")
	code, stdout, stderr := run(t, "proposal", "reject", approveID, "--reason", "no", "--output", "json")
	if code != 0 || !strings.Contains(stdout, `"engine":"rootless:alice"`) {
		t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if alice.count(control.ProposalRejectCommand) != 1 || system.count(control.ProposalRejectCommand) != 0 {
		t.Fatal("the rejection did not reach only the instance")
	}
}

func TestProposalApproveOnAHostWithOnlyAnInstanceIsNotNotSetUp(t *testing.T) {
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice")
	env := &approveEnv{dir: t.TempDir()}
	alice := serveFake(t, eps["alice"], env.handlers())
	noSystemService(t)
	useTerminalStdin(t, "y\n")

	code, _, stderr := run(t, "proposal", "approve", approveID, "--output", "json")
	if code != 0 || strings.Contains(stderr, "not set up") || alice.count(control.SourceAddCommand) != 1 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
}
