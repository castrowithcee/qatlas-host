package cli

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/castrowithcee/qatlas-kit/localipc"

	"github.com/castrowithcee/qatlas-host/internal/channeltest"
	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/host/hostchange"
)

const demoHostProposalID = "h-0123456789abcdef"

func hostProposalInfo(id, state string) control.ProposalInfo {
	info := proposalInfo(id, state)
	info.Kind = control.ProposalKindHostChange
	return info
}

// hostProposalEnv is a host service that holds one proposal of the host and records what the command asks.
type hostProposalEnv struct {
	mu          sync.Mutex
	order       []string
	content     control.ProposalHostChange
	state       string
	fingerprint func(n int) string // fingerprint shown on the n-th read
	shows       int
	approved    control.ProposalApproveArgs
	result      control.ProposalResultArgs
}

func (e *hostProposalEnv) note(s string) {
	e.mu.Lock()
	e.order = append(e.order, s)
	e.mu.Unlock()
}

func (e *hostProposalEnv) history() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return strings.Join(e.order, ",")
}

// startHostProposalEnv starts a deploy service that knows nothing and the host service of env. The caller is root
// in a change environment of invented demo actions (see newChangeFixture); the process has a terminal of its own
// unless a test says otherwise.
func startHostProposalEnv(t *testing.T, env *hostProposalEnv) (*changeFixture, *fakeService) {
	t.Helper()
	f := newChangeFixture(t, 0)
	useControllingTerminal(t, true)
	if env.fingerprint == nil {
		env.fingerprint = func(int) string { return "fp-one" }
	}
	if env.state == "" {
		env.state = control.ProposalOpen
	}
	startFakeService(t, fakeHandlers{})
	svc := startFakeHostService(t, fakeHandlers{
		control.ProposalShowCommand: func(json.RawMessage) any {
			env.mu.Lock()
			env.shows++
			n := env.shows
			env.mu.Unlock()
			env.note("show")
			info := hostProposalInfo(demoHostProposalID, env.state)
			info.Fingerprint = env.fingerprint(n)
			content, _ := json.Marshal(env.content)
			return control.ProposalShowResult{ProposalInfo: info, Content: content}
		},
		control.ProposalApproveCommand: func(raw json.RawMessage) any {
			env.note("approve")
			_ = json.Unmarshal(raw, &env.approved)
			return hostProposalInfo(demoHostProposalID, control.ProposalApproving)
		},
		control.ProposalResultCommand: func(raw json.RawMessage) any {
			env.note("result")
			_ = json.Unmarshal(raw, &env.result)
			return hostProposalInfo(demoHostProposalID, control.ProposalApplied)
		},
	})
	return f, svc
}

func useControllingTerminal(t *testing.T, has bool) {
	t.Helper()
	old := controllingTerminal
	controllingTerminal = func() bool { return has }
	t.Cleanup(func() { controllingTerminal = old })
}

func demoPublicKey(t *testing.T) string {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key))) + " demo@example.test"
}

func TestProposalCommandsGoToTheServiceOfTheKindAndTheID(t *testing.T) {
	var deploySent, hostSent []string
	var mu sync.Mutex
	record := func(list *[]string, what string) {
		mu.Lock()
		*list = append(*list, what)
		mu.Unlock()
	}
	deploy := startFakeService(t, fakeHandlers{
		control.ProposalSubmitCommand: func(json.RawMessage) any {
			record(&deploySent, "submit")
			return proposalInfo("p-0123456789abcdef", control.ProposalOpen)
		},
		control.ProposalShowCommand: func(json.RawMessage) any {
			record(&deploySent, "show")
			return control.ProposalShowResult{ProposalInfo: proposalInfo("p-0123456789abcdef", control.ProposalOpen), Content: json.RawMessage(`{"path":"/srv/x"}`)}
		},
		control.ProposalWithdrawCommand: func(json.RawMessage) any {
			record(&deploySent, "withdraw")
			return proposalInfo("p-0123456789abcdef", control.ProposalWithdrawn)
		},
	})
	host := startFakeHostService(t, fakeHandlers{
		control.ProposalSubmitCommand: func(raw json.RawMessage) any {
			var a control.ProposalSubmitArgs
			_ = json.Unmarshal(raw, &a)
			record(&hostSent, "submit:"+a.Kind)
			return hostProposalInfo(demoHostProposalID, control.ProposalOpen)
		},
		control.ProposalShowCommand: func(json.RawMessage) any {
			record(&hostSent, "show")
			return control.ProposalShowResult{ProposalInfo: hostProposalInfo(demoHostProposalID, control.ProposalOpen),
				Content: json.RawMessage(`{"action":"user.lock","args":["demo-user"]}`)}
		},
		control.ProposalWithdrawCommand: func(json.RawMessage) any {
			record(&hostSent, "withdraw")
			return hostProposalInfo(demoHostProposalID, control.ProposalWithdrawn)
		},
	})
	withStdin(t, strings.NewReader(`{"action":"user.lock","args":["demo-user"]}`))
	if code, stdout, stderr := run(t, "proposal", "submit", "--kind", "host-change"); code != 0 || !strings.Contains(stdout, demoHostProposalID) {
		t.Fatalf("host submit: code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	withStdin(t, strings.NewReader(`{"path":"/srv/stacks"}`))
	if code, _, stderr := run(t, "proposal", "submit", "--kind", "source-add"); code != 0 {
		t.Fatalf("deploy submit: code %d, stderr %q", code, stderr)
	}
	for _, id := range []string{demoHostProposalID, "p-0123456789abcdef"} {
		for _, verb := range []string{"show", "withdraw"} {
			if code, _, stderr := run(t, "proposal", verb, id); code != 0 {
				t.Fatalf("%s %s: code %d, stderr %q", verb, id, code, stderr)
			}
		}
	}
	if got := strings.Join(hostSent, ","); got != "submit:host-change,show,withdraw" || host.count(control.ProposalSubmitCommand) != 1 {
		t.Fatalf("host service got %q", got)
	}
	if got := strings.Join(deploySent, ","); got != "submit,show,withdraw" || deploy.count(control.ProposalSubmitCommand) != 1 {
		t.Fatalf("deploy service got %q", got)
	}
}

func TestProposalShowOfTheHostPrintsSentenceCommandsAndKeys(t *testing.T) {
	newChangeFixture(t, 1000)
	key := demoPublicKey(t)
	startFakeService(t, fakeHandlers{})
	startFakeHostService(t, fakeHandlers{control.ProposalShowCommand: func(json.RawMessage) any {
		content, _ := json.Marshal(control.ProposalHostChange{Action: "demo.set", Args: []string{"alpha", "new"}})
		return control.ProposalShowResult{ProposalInfo: hostProposalInfo(demoHostProposalID, control.ProposalOpen), Content: content}
	}})
	code, stdout, stderr := run(t, "proposal", "show", demoHostProposalID, "--output", "json")
	if code != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	m := decode(t, stdout)
	if m["sentence"] != "Set alpha to new." || m["commands"].([]any)[0] != "demo-set alpha new" || m["kind"] != "host-change" {
		t.Fatalf("%v", m)
	}
	if details := keyDetails([]string{key}); len(details) != 1 || !strings.HasPrefix(details[0], "ssh-ed25519 SHA256:") ||
		!strings.HasSuffix(details[0], " demo@example.test") {
		t.Fatalf("key details %q", details)
	}
}

func TestProposalRejectOfTheHostGoesToTheHostServiceAndApproveIsRefused(t *testing.T) {
	useFolderHost(t)
	var rejected control.ProposalRejectArgs
	startFakeService(t, fakeHandlers{})
	startFakeHostService(t, fakeHandlers{control.ProposalRejectCommand: func(raw json.RawMessage) any {
		_ = json.Unmarshal(raw, &rejected)
		return hostProposalInfo(demoHostProposalID, control.ProposalRejected)
	}})
	useTerminalStdin(t, "")
	if code, _, stderr := run(t, "proposal", "reject", demoHostProposalID, "--reason", "not now"); code != 0 || rejected.Reason != "not now" {
		t.Fatalf("reject: code %d, sent %+v, stderr %q", code, rejected, stderr)
	}
	useTerminalStdin(t, "y\n")
	code, _, stderr := run(t, "proposal", "approve", demoHostProposalID)
	if code == 0 || !strings.Contains(stderr, "host apply --proposal") {
		t.Fatalf("approve: code %d, stderr %q", code, stderr)
	}
}

func TestProposalListMergesBothServicesNewestFirst(t *testing.T) {
	at := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	startFakeService(t, fakeHandlers{control.ProposalListCommand: func(json.RawMessage) any {
		return control.ProposalListResult{Proposals: []control.ProposalInfo{proposalInfo("p-aaaaaaaaaaaaaaaa", control.ProposalOpen)}}
	}})
	startFakeHostService(t, fakeHandlers{control.ProposalListCommand: func(json.RawMessage) any {
		newer, older := hostProposalInfo("h-bbbbbbbbbbbbbbbb", control.ProposalOpen), hostProposalInfo("h-cccccccccccccccc", control.ProposalApplied)
		newer.Submitted, older.Submitted = at.Add(time.Hour), at.Add(-time.Hour)
		return control.ProposalListResult{Proposals: []control.ProposalInfo{newer, older}}
	}})
	code, stdout, stderr := run(t, "proposal", "list", "--output", "json")
	var rows []map[string]any
	if err := json.Unmarshal([]byte(stdout), &rows); code != 0 || err != nil || len(rows) != 3 || stderr != "" {
		t.Fatalf("code %d, stdout %q, stderr %q: %v", code, stdout, stderr, err)
	}
	for i, want := range []struct{ id, service string }{
		{"h-bbbbbbbbbbbbbbbb", "host"}, {"p-aaaaaaaaaaaaaaaa", "deploy"}, {"h-cccccccccccccccc", "host"},
	} {
		if rows[i]["id"] != want.id || rows[i]["service"] != want.service {
			t.Fatalf("row %d = %v", i, rows[i])
		}
	}
}

func TestProposalListNamesAServiceThatDoesNotAnswer(t *testing.T) {
	missing := func(t *testing.T) {
		ep := channelMissing(t)
		useEndpoint(t, ep)
	}
	listHost := fakeHandlers{control.ProposalListCommand: func(json.RawMessage) any {
		return control.ProposalListResult{Proposals: []control.ProposalInfo{hostProposalInfo(demoHostProposalID, control.ProposalOpen)}}
	}}
	for name, tc := range map[string]struct {
		setup  func(t *testing.T)
		id     string
		notice string
	}{
		"host not running": {func(t *testing.T) {
			startFakeService(t, fakeHandlers{control.ProposalListCommand: func(json.RawMessage) any {
				return control.ProposalListResult{Proposals: []control.ProposalInfo{proposalInfo("p-aaaaaaaaaaaaaaaa", control.ProposalOpen)}}
			}})
		}, "p-aaaaaaaaaaaaaaaa", "the host service is not running"},
		"deploy not running": {func(t *testing.T) {
			missing(t)
			startFakeHostService(t, listHost)
		}, demoHostProposalID, "the deploy service is not running"},
		"deploy not set up": {func(t *testing.T) {
			missing(t)
			useDeploySetupRoot(t, t.TempDir())
			startFakeHostService(t, listHost)
		}, demoHostProposalID, "the deploy service is not set up"},
	} {
		t.Run(name, func(t *testing.T) {
			tc.setup(t)
			code, stdout, stderr := run(t, "proposal", "list", "--output", "json")
			var rows []map[string]any
			if err := json.Unmarshal([]byte(stdout), &rows); code != 0 || err != nil || len(rows) != 1 || rows[0]["id"] != tc.id {
				t.Fatalf("code %d, stdout %q, stderr %q: %v", code, stdout, stderr, err)
			}
			if !strings.Contains(stderr, tc.notice) || !strings.Contains(stderr, "not listed") {
				t.Fatalf("stderr %q", stderr)
			}
		})
	}
	t.Run("neither", func(t *testing.T) {
		missing(t)
		code, stdout, stderr := run(t, "proposal", "list", "--output", "json")
		if code != ExitServiceNotRunning || stdout != "" || !strings.Contains(stderr, "neither") {
			t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
		}
	})
}

// channelMissing is an endpoint where nothing listens.
func channelMissing(t *testing.T) localipc.ServiceEndpoint {
	t.Helper()
	ep := channeltest.Endpoint(t)
	ep.Dir = filepath.Join(ep.Dir, "missing")
	return ep
}

func TestHostApplyProposalAppliesAndKeepsTheResult(t *testing.T) {
	env := &hostProposalEnv{content: control.ProposalHostChange{Action: "demo.set", Args: []string{"alpha", "new"}}}
	f, _ := startHostProposalEnv(t, env)
	useStdin(t, strings.NewReader(""))
	code, out, errOut := run(t, "host", "apply", "--proposal", demoHostProposalID, "--fingerprint", "fp-one", "--confirm", "--output", "json")
	if code != 0 {
		t.Fatalf("code %d, stderr %q", code, errOut)
	}
	m := decode(t, out)
	if m["result"] != "applied" || m["action"] != "demo.set" || m["sentence"] != "Set alpha to new." || m["proposal_id"] != demoHostProposalID ||
		m["journal_id"] == "" || m["reversible"] != true {
		t.Fatalf("report %v", m)
	}
	if !strings.Contains(errOut, "Set alpha to new.") || !strings.Contains(errOut, "demo-set alpha new") {
		t.Fatalf("the sentence and the commands were not shown: %q", errOut)
	}
	if h := env.history(); h != "show,show,approve,result" {
		t.Fatalf("order %q", h)
	}
	if env.approved.Fingerprint != "fp-one" || !env.result.Applied || env.result.Fingerprint != "fp-one" || env.result.ID != demoHostProposalID {
		t.Fatalf("approved %+v, result %+v", env.approved, env.result)
	}
	if strings.Join(f.w.calls, ",") != "demo-set alpha new" || f.w.values["alpha"] != "new" || len(f.audited) != 1 || f.audited[0].Action != "demo.set" {
		t.Fatalf("executed %v, audited %+v", f.w.calls, f.audited)
	}
}

func TestHostApplyProposalAsksAtTheTerminalWithoutAFingerprint(t *testing.T) {
	env := &hostProposalEnv{content: control.ProposalHostChange{Action: "demo.set", Args: []string{"alpha", "new"}}}
	f, _ := startHostProposalEnv(t, env)
	useTerminalStdin(t, "n\n")
	if code, out, _ := run(t, "host", "apply", "--proposal", demoHostProposalID, "--confirm"); code == 0 || out != "" {
		t.Fatalf("declined: code %d, out %q", code, out)
	}
	if h := env.history(); h != "show" || len(f.w.calls) != 0 {
		t.Fatalf("declined: order %q, executed %v", h, f.w.calls)
	}
	useTerminalStdin(t, "y\n")
	if code, _, errOut := run(t, "host", "apply", "--proposal", demoHostProposalID, "--confirm"); code != 0 {
		t.Fatalf("approved: code %d, stderr %q", code, errOut)
	}
	if f.w.values["alpha"] != "new" || !env.result.Applied {
		t.Fatalf("not applied: %v, %+v", f.w.values, env.result)
	}
	// Without a terminal on standard input there is nobody to ask.
	useStdin(t, strings.NewReader("y\n"))
	if code, _, errOut := run(t, "host", "apply", "--proposal", demoHostProposalID, "--confirm"); code == 0 || !strings.Contains(errOut, "--fingerprint") {
		t.Fatalf("no terminal on stdin: code %d, stderr %q", code, errOut)
	}
}

func TestHostApplyProposalRefusesAChangedProposal(t *testing.T) {
	for name, tc := range map[string]struct {
		fingerprint func(n int) string
		history     string
	}{
		"changed before it was shown": {func(int) string { return "fp-two" }, "show"},
		"changed after it was shown": {func(n int) string {
			if n == 1 {
				return "fp-one"
			}
			return "fp-two"
		}, "show,show"},
	} {
		t.Run(name, func(t *testing.T) {
			env := &hostProposalEnv{content: control.ProposalHostChange{Action: "demo.set", Args: []string{"alpha", "new"}}, fingerprint: tc.fingerprint}
			f, _ := startHostProposalEnv(t, env)
			useStdin(t, strings.NewReader(""))
			code, out, errOut := run(t, "host", "apply", "--proposal", demoHostProposalID, "--fingerprint", "fp-one", "--confirm", "--output", "json")
			if code == 0 || out != "" || !strings.Contains(errOut, "not the one that was shown") {
				t.Fatalf("code %d, out %q, stderr %q", code, out, errOut)
			}
			if h := env.history(); h != tc.history || len(f.w.calls) != 0 || len(f.audited) != 0 {
				t.Fatalf("order %q, executed %v", h, f.w.calls)
			}
		})
	}
}

func TestHostApplyProposalRefusesWhatMayNotBeProposedOrIsNotOpen(t *testing.T) {
	for name, env := range map[string]*hostProposalEnv{
		"not open":     {content: control.ProposalHostChange{Action: "demo.set", Args: []string{"alpha", "new"}}, state: control.ProposalRejected},
		"bad argument": {content: control.ProposalHostChange{Action: "demo.set", Args: []string{"alpha"}}},
		"set-admin":    {content: control.ProposalHostChange{Action: "user.set-admin", Args: []string{"demo-user", "on"}}},
		"secret":       {content: control.ProposalHostChange{Action: "demo.login"}},
	} {
		t.Run(name, func(t *testing.T) {
			f, _ := startHostProposalEnv(t, env)
			useStdin(t, strings.NewReader("demo-secret\n"))
			code, out, _ := run(t, "host", "apply", "--proposal", demoHostProposalID, "--fingerprint", "fp-one", "--confirm")
			if code == 0 || out != "" || env.history() != "show" || len(f.w.calls) != 0 {
				t.Fatalf("code %d, out %q, order %q, executed %v", code, out, env.history(), f.w.calls)
			}
		})
	}
}

func TestHostApplyProposalNeedsAPersonRootAndConfirm(t *testing.T) {
	env := &hostProposalEnv{content: control.ProposalHostChange{Action: "demo.set", Args: []string{"alpha", "new"}}}
	f, _ := startHostProposalEnv(t, env)
	base := []string{"host", "apply", "--proposal", demoHostProposalID, "--fingerprint", "fp-one"}
	useStdin(t, strings.NewReader(""))
	if code, _, errOut := run(t, append(base, "--confirm", "--agent")...); code == 0 || !strings.Contains(errOut, "not for an agent") {
		t.Errorf("agent: code %d, stderr %q", code, errOut)
	}
	useControllingTerminal(t, false)
	if code, _, errOut := run(t, append(base, "--confirm")...); code == 0 || !strings.Contains(errOut, "needs a terminal") {
		t.Errorf("no terminal: code %d, stderr %q", code, errOut)
	}
	useControllingTerminal(t, true)
	code, out, errOut := run(t, base...)
	if code != 2 || out != "" || !strings.Contains(errOut, "--confirm") {
		t.Errorf("no --confirm: code %d, stdout %q, stderr %q", code, out, errOut)
	}
	if strings.Count(errOut, "Set alpha to new.") != 1 || strings.Count(errOut, "demo-set alpha new") != 1 {
		t.Errorf("the sentence and the commands must appear once: %q", errOut)
	}
	if h := env.history(); h != "show" || len(f.w.calls) != 0 {
		t.Errorf("the preview only reads the proposal: order %q, executed %v", h, f.w.calls)
	}
	env.mu.Lock()
	env.order = nil
	env.mu.Unlock()
	if code, _, _ := run(t, append(base, "--confirm", "demo.set")...); code != 2 {
		t.Errorf("an action next to --proposal: code %d", code)
	}
	if code, _, _ := run(t, "host", "apply", "demo.set", "alpha", "new", "--fingerprint", "fp-one", "--confirm"); code != 2 {
		t.Errorf("--fingerprint without --proposal: code %d", code)
	}
	if code, _, _ := run(t, "host", "apply", "--proposal", "p-0123456789abcdef", "--fingerprint", "fp-one", "--confirm"); code != 2 {
		t.Errorf("a proposal of the deploy service: code %d", code)
	}
	hostEnv.euid = func() int { return 1000 }
	if code, _, errOut := run(t, append(base, "--confirm")...); code == 0 || !strings.Contains(errOut, "must run as root") {
		t.Errorf("not root: code %d, stderr %q", code, errOut)
	}
	if h := env.history(); h != "" || len(f.w.calls) != 0 {
		t.Fatalf("something happened: order %q, executed %v", h, f.w.calls)
	}
}

func TestHostApplyProposalReportsARefusalOnTheProposal(t *testing.T) {
	env := &hostProposalEnv{content: control.ProposalHostChange{Action: "demo.guard"}}
	f, _ := startHostProposalEnv(t, env)
	useStdin(t, strings.NewReader(""))
	code, out, errOut := run(t, "host", "apply", "--proposal", demoHostProposalID, "--fingerprint", "fp-one", "--confirm", "--output", "json")
	m := decode(t, out)
	if code == 0 || m["result"] != "refused" || m["proposal_id"] != demoHostProposalID || !strings.Contains(errOut, "change-refused") {
		t.Fatalf("code %d, report %v, stderr %q", code, m, errOut)
	}
	if env.history() != "show,show,approve,result" || env.result.Applied || env.result.Message != "refused: demo-user is the last administrator" {
		t.Fatalf("order %q, result %+v", env.history(), env.result)
	}
	if len(f.w.calls) != 0 {
		t.Fatalf("executed %v", f.w.calls)
	}
}

// A proposed user gets the proposed keys and the initial password the person enters on standard input, which
// shows nowhere. The invented host has no such user afterwards, so the change does not stand; what matters is
// what ran.
func TestHostApplyProposalOfAUserTakesThePasswordFromStdinAndTheKeysFromTheProposal(t *testing.T) {
	key := demoPublicKey(t)
	env := &hostProposalEnv{content: control.ProposalHostChange{Action: hostchange.ActionUserCreate, Args: []string{"demo-new"}, Keys: []string{key}}}
	f, _ := startHostProposalEnv(t, env)
	dir := t.TempDir()
	reg := hostchange.NewRegistry()
	if err := hostchange.RegisterUserActions(reg, hostchange.UserHost{Accounts: hostEnv.accounts(true),
		HomeBase: filepath.Join(dir, "home"), MailDir: filepath.Join(dir, "mail"), Proc: filepath.Join(dir, "proc")}); err != nil {
		t.Fatal(err)
	}
	changeEnv.registry = func() *hostchange.Registry { return reg }
	initial := "demo-initial-" + "7"
	base := []string{"host", "apply", "--proposal", demoHostProposalID, "--confirm", "--output", "json"}

	// The password comes on standard input, so the run must be bound to a reviewed fingerprint.
	useStdin(t, strings.NewReader(string(proposalInput(t, initial, nil))))
	if code, _, errOut := run(t, base...); code == 0 || !strings.Contains(errOut, "--fingerprint") {
		t.Fatalf("without fingerprint: code %d, stderr %q", code, errOut)
	}
	// Keys on standard input are refused: they come from the proposal.
	useStdin(t, strings.NewReader(string(proposalInput(t, initial, []string{key}))))
	if code, _, errOut := run(t, append(base, "--fingerprint", "fp-one")...); code == 0 || !strings.Contains(errOut, "come from the proposal") {
		t.Fatalf("keys on stdin: code %d, stderr %q", code, errOut)
	}
	if strings.Contains(env.history(), "approve") || len(f.w.calls) != 0 {
		t.Fatalf("something happened: %q, %v", env.history(), f.w.calls)
	}

	useStdin(t, strings.NewReader(string(proposalInput(t, initial, nil))+"\n"))
	_, out, errOut := run(t, append(base, "--fingerprint", "fp-one")...)
	if !strings.Contains(env.history(), "approve,result") || env.result.ID != demoHostProposalID {
		t.Fatalf("order %q, stderr %q", env.history(), errOut)
	}
	if !strings.Contains(strings.Join(f.w.stdins, "|"), "demo-new:"+initial+"\n") {
		t.Fatalf("the password did not reach chpasswd: %q", f.w.stdins)
	}
	if !strings.Contains(strings.Join(f.w.stdins, "|"), key) {
		t.Fatalf("the proposed key was not added: %q", f.w.stdins)
	}
	all := out + errOut + strings.Join(f.w.calls, "") + env.result.Message
	files, _ := os.ReadDir(f.dir)
	for _, de := range files {
		b, _ := os.ReadFile(filepath.Join(f.dir, de.Name()))
		all += string(b)
	}
	if strings.Contains(all, initial) {
		t.Fatal("the password leaked into the output, an argument, the journal or the result")
	}
}

// proposalInput is the standard input of an approval: the initial password and, where set, keys.
func proposalInput(t *testing.T, initial string, keys []string) []byte {
	t.Helper()
	b, err := json.Marshal(hostchange.CreateInput{Password: initial, Keys: keys})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestHostApplyProposalPreviewAsksNothingAndTakesNoApproval(t *testing.T) {
	env := &hostProposalEnv{content: control.ProposalHostChange{Action: "demo.set", Args: []string{"alpha", "new"}}}
	f, _ := startHostProposalEnv(t, env)
	// A terminal that would be asked, and standard input that must stay unread.
	useTerminalStdin(t, "y\n")
	code, out, errOut := run(t, "host", "apply", "--proposal", demoHostProposalID)
	if code != 2 || out != "" || !strings.Contains(errOut, "--confirm") || !strings.Contains(errOut, "Fingerprint of the proposal: fp-one") {
		t.Fatalf("code %d, stdout %q, stderr %q", code, out, errOut)
	}
	if strings.Contains(errOut, "Apply proposal") {
		t.Errorf("a question was asked: %q", errOut)
	}
	if h := env.history(); h != "show" || len(f.w.calls) != 0 || len(f.audited) != 0 || f.w.values["alpha"] != "old" {
		t.Errorf("order %q, executed %v, audited %v", h, f.w.calls, f.audited)
	}
}
