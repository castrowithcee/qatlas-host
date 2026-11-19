package cli

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

const approveID = "p-0123456789abcdef"

// approveEnv is a service that holds one proposal and records the order of what the command asks.
type approveEnv struct {
	mu          sync.Mutex
	order       []string
	showCount   int
	fingerprint func(n int) string // fingerprint shown on the n-th read
	inspect     func(n int) control.SourceInspectResult
	inspectN    int
	approveErr  *control.Error
	addErr      *control.Error
	approved    control.ProposalApproveArgs
	result      control.ProposalResultArgs
	rejected    control.ProposalRejectArgs
	inspectURLs []string
	dir         string
	extra       fakeHandlers
}

func (e *approveEnv) note(s string) {
	e.mu.Lock()
	e.order = append(e.order, s)
	e.mu.Unlock()
}

func (e *approveEnv) history() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return strings.Join(e.order, ",")
}

func startApproveEnv(t *testing.T, env *approveEnv) *fakeService {
	t.Helper()
	if env.fingerprint == nil {
		env.fingerprint = func(int) string { return "fp-one" }
	}
	if env.inspect == nil {
		env.inspect = func(int) control.SourceInspectResult { return localInspect(false) }
	}
	handlers := fakeHandlers{
		control.ProposalShowCommand: func(json.RawMessage) any {
			env.mu.Lock()
			env.showCount++
			n := env.showCount
			env.mu.Unlock()
			env.note("show")
			info := proposalInfo(approveID, control.ProposalOpen)
			info.Fingerprint = env.fingerprint(n)
			content, _ := json.Marshal(control.ProposalSourceAdd{Path: env.dir})
			return control.ProposalShowResult{ProposalInfo: info, Content: content}
		},
		control.SourceInspectCommand: func(raw json.RawMessage) any {
			env.mu.Lock()
			env.inspectN++
			n := env.inspectN
			var a control.SourceInspectArgs
			_ = json.Unmarshal(raw, &a)
			env.inspectURLs = append(env.inspectURLs, a.URL)
			env.mu.Unlock()
			env.note("inspect")
			res := env.inspect(n)
			res.Path = a.Path
			if res.Stacks == nil {
				res.Stacks = []control.SourceStack{}
			}
			return res
		},
		control.ProposalApproveCommand: func(raw json.RawMessage) any {
			env.note("approve")
			_ = json.Unmarshal(raw, &env.approved)
			if env.approveErr != nil {
				return env.approveErr
			}
			return proposalInfo(approveID, control.ProposalApproving)
		},
		control.SourceAddCommand: func(raw json.RawMessage) any {
			env.note("add")
			if env.addErr != nil {
				return env.addErr
			}
			var a control.SourceAddArgs
			_ = json.Unmarshal(raw, &a)
			return control.SourceAddResult{ID: "src-1", Path: a.Path, Kind: control.SourceKindLocal, Head: "abc1234"}
		},
		control.ProposalResultCommand: func(raw json.RawMessage) any {
			env.note("result")
			_ = json.Unmarshal(raw, &env.result)
			return proposalInfo(approveID, control.ProposalApplied)
		},
		control.ProposalRejectCommand: func(raw json.RawMessage) any {
			env.note("reject")
			env.rejected = control.ProposalRejectArgs{}
			_ = json.Unmarshal(raw, &env.rejected)
			return proposalInfo(approveID, control.ProposalRejected)
		},
	}
	for k, v := range env.extra {
		handlers[k] = v
	}
	return startFakeService(t, handlers)
}

func TestProposalApproveAndRejectRefuseAnAgentAndNoTerminal(t *testing.T) {
	useFolderHost(t)
	env := &approveEnv{dir: t.TempDir()}
	svc := startApproveEnv(t, env)
	for name, setup := range map[string]func(){
		"agent":       func() { useTerminalStdin(t, "y\ny\n") },
		"no terminal": func() { useStdin(t, strings.NewReader("y\ny\n")) },
	} {
		for _, verb := range []string{"approve", "reject"} {
			setup()
			args := []string{"proposal", verb, approveID}
			if name == "agent" {
				args = append(args, "--agent")
			}
			code, stdout, stderr := run(t, args...)
			if code == 0 || stdout != "" || !strings.Contains(stderr, "terminal") {
				t.Errorf("%s %s: code %d, stdout %q, stderr %q", name, verb, code, stdout, stderr)
			}
		}
	}
	if h := env.history(); h != "" || svc.count(control.SourceAddCommand) != 0 {
		t.Fatalf("the service was asked: %q", h)
	}
}

func TestProposalApproveHasNoYesAndNeedsRoot(t *testing.T) {
	d := useFolderHost(t)
	env := &approveEnv{dir: t.TempDir()}
	startApproveEnv(t, env)
	useTerminalStdin(t, "y\n")
	if code, _, _ := run(t, "proposal", "approve", approveID, "--yes"); code == 0 {
		t.Fatal("approve accepted --yes")
	}
	old := folders.euid
	folders.euid = func() int { return 1000 }
	t.Cleanup(func() { folders.euid = old })
	for _, verb := range []string{"approve", "reject"} {
		useTerminalStdin(t, "y\n")
		code, _, stderr := run(t, "proposal", verb, approveID)
		if code == 0 || !strings.Contains(stderr, "needs root") {
			t.Fatalf("%s: code %d, stderr %q", verb, code, stderr)
		}
	}
	if env.history() != "" || d.dropIn(t) != "" {
		t.Fatalf("something happened: %q", env.history())
	}
}

func TestProposalRejectSendsTheReason(t *testing.T) {
	useFolderHost(t)
	env := &approveEnv{dir: t.TempDir()}
	startApproveEnv(t, env)
	useTerminalStdin(t, "")
	code, stdout, stderr := run(t, "proposal", "reject", approveID, "--reason", "wrong folder", "--output", "compact")
	if code != 0 || env.rejected.ID != approveID || env.rejected.Reason != "wrong folder" || !strings.Contains(stdout, "rejected") {
		t.Fatalf("code %d, sent %+v, stdout %q, stderr %q", code, env.rejected, stdout, stderr)
	}
	useTerminalStdin(t, "")
	if code, _, stderr := run(t, "proposal", "reject", approveID); code != 0 || env.rejected.Reason != "" {
		t.Fatalf("without reason: code %d, sent %+v, stderr %q", code, env.rejected, stderr)
	}
}

func TestProposalShowPrintsTheMessage(t *testing.T) {
	startFakeService(t, fakeHandlers{control.ProposalShowCommand: func(json.RawMessage) any {
		info := proposalInfo(approveID, control.ProposalRejected)
		info.Message = "wrong\x1b[2J folder"
		return control.ProposalShowResult{ProposalInfo: info, Content: json.RawMessage(`{"path":"/srv/x"}`)}
	}})
	code, stdout, stderr := run(t, "proposal", "show", approveID, "--output", "compact")
	if code != 0 || !strings.Contains(stdout, "wrong") || strings.ContainsRune(stdout, 0x1b) {
		t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestProposalApproveAppliesAfterTheApprovalIsAccepted(t *testing.T) {
	d := useFolderHost(t)
	env := &approveEnv{dir: t.TempDir()}
	startApproveEnv(t, env)
	useTerminalStdin(t, "y\n")
	code, stdout, stderr := run(t, "proposal", "approve", approveID, "--output", "json")
	if code != 0 {
		t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	// Read, inspect, last read, inspect again, then the service accepts, then the change, then the result.
	if got, want := env.history(), "show,inspect,show,inspect,approve,add,result"; got != want {
		t.Fatalf("order %q, want %q", got, want)
	}
	if env.approved.Fingerprint != "fp-one" || env.result.Fingerprint != "fp-one" || !env.result.Applied {
		t.Fatalf("approved %+v, result %+v", env.approved, env.result)
	}
	if !strings.Contains(stderr, "Proposal "+approveID) || !strings.Contains(stderr, "Effect:") ||
		!strings.Contains(stderr, "Approve proposal") || !strings.Contains(d.dropIn(t), env.dir) {
		t.Fatalf("stderr %q, drop-in %q", stderr, d.dropIn(t))
	}
}

func TestProposalApproveDeclinedChangesNothing(t *testing.T) {
	d := useFolderHost(t)
	env := &approveEnv{dir: t.TempDir()}
	startApproveEnv(t, env)
	useTerminalStdin(t, "n\n")
	code, _, stderr := run(t, "proposal", "approve", approveID)
	if code == 0 || env.history() != "show,inspect" || d.dropIn(t) != "" || len(d.cmds) != 0 {
		t.Fatalf("code %d, order %q, stderr %q", code, env.history(), stderr)
	}
}

func TestProposalApproveRefusesAChangedProposal(t *testing.T) {
	d := useFolderHost(t)
	env := &approveEnv{dir: t.TempDir(), fingerprint: func(n int) string {
		if n == 1 {
			return "fp-one"
		}
		return "fp-two"
	}}
	startApproveEnv(t, env)
	useTerminalStdin(t, "y\n")
	code, _, stderr := run(t, "proposal", "approve", approveID)
	if code == 0 || !strings.Contains(stderr, "changed since it was shown") || strings.Contains(env.history(), "approve") ||
		strings.Contains(env.history(), "add") || d.dropIn(t) != "" {
		t.Fatalf("code %d, order %q, stderr %q", code, env.history(), stderr)
	}
}

func TestProposalApproveRefusesAChangedEffectAndShowsIt(t *testing.T) {
	d := useFolderHost(t)
	env := &approveEnv{dir: t.TempDir(), inspect: func(n int) control.SourceInspectResult {
		files := []string{"demo/compose.yaml"}
		if n > 1 {
			files = append(files, "demo/new.env")
		}
		return control.SourceInspectResult{Case: control.SourceCaseFiles, PlannedFiles: files,
			Access: &control.SourceAccess{Mode: "0755"}}
	}}
	startApproveEnv(t, env)
	useTerminalStdin(t, "y\n")
	code, _, stderr := run(t, "proposal", "approve", approveID)
	if code == 0 || !strings.Contains(stderr, "effect of the proposal changed") || !strings.Contains(stderr, "demo/new.env") ||
		strings.Contains(env.history(), "approve") || strings.Contains(env.history(), "add") || d.dropIn(t) != "" || len(d.cmds) != 0 {
		t.Fatalf("code %d, order %q, stderr %q", code, env.history(), stderr)
	}
}

func TestProposalApproveRefusedByTheServiceAppliesNothing(t *testing.T) {
	d := useFolderHost(t)
	env := &approveEnv{dir: t.TempDir(), approveErr: &control.Error{Code: control.CodeProposalChanged, Message: "the proposal is not the one that was shown"}}
	startApproveEnv(t, env)
	useTerminalStdin(t, "y\n")
	code, _, stderr := run(t, "proposal", "approve", approveID)
	if code == 0 || !strings.Contains(stderr, "nothing was applied") || strings.Contains(env.history(), "add") || d.dropIn(t) != "" {
		t.Fatalf("code %d, order %q, stderr %q", code, env.history(), stderr)
	}
}

func TestProposalApproveReportsAFailedChange(t *testing.T) {
	useFolderHost(t)
	env := &approveEnv{dir: t.TempDir(), addErr: &control.Error{Code: control.CodeInvalid, Message: "the folder is not a repository"}}
	startApproveEnv(t, env)
	useTerminalStdin(t, "y\n")
	code, _, stderr := run(t, "proposal", "approve", approveID)
	if code == 0 || env.history() != "show,inspect,show,inspect,approve,add,result" || env.result.Applied ||
		!strings.Contains(env.result.Message, "the folder is not a repository") {
		t.Fatalf("code %d, order %q, result %+v, stderr %q", code, env.history(), env.result, stderr)
	}
}

func TestProposalApproveRefusesAnInvalidAddressOverride(t *testing.T) {
	useFolderHost(t)
	env := &approveEnv{dir: t.TempDir()}
	startApproveEnv(t, env)
	useTerminalStdin(t, "y\n")
	code, _, stderr := run(t, "proposal", "approve", approveID, "--url", "https://git.example.test/a.git")
	if code == 0 || strings.Contains(env.history(), "approve") || strings.Contains(env.history(), "inspect") {
		t.Fatalf("code %d, order %q, stderr %q", code, env.history(), stderr)
	}
}

func TestProposalApproveShowsProposedValuesWithoutControlCharacters(t *testing.T) {
	useFolderHost(t)
	env := &approveEnv{dir: t.TempDir(), extra: fakeHandlers{control.ProposalShowCommand: func(json.RawMessage) any {
		info := proposalInfo(approveID, control.ProposalOpen)
		info.SubmitterName = hostile
		content, _ := json.Marshal(control.ProposalSourceAdd{Path: "/srv/" + strings.Repeat("a", 500)})
		return control.ProposalShowResult{ProposalInfo: info, Content: content}
	}}}
	startApproveEnv(t, env)
	useTerminalStdin(t, "n\n")
	_, _, stderr := run(t, "proposal", "approve", approveID)
	for _, r := range stderr {
		if r < 0x20 && r != '\n' && r != '\t' || r == 0x7f || r == 0x202e {
			t.Fatalf("output holds %U:\n%q", r, stderr)
		}
	}
	if !strings.Contains(stderr, "evil") || !strings.Contains(stderr, "more characters") {
		t.Fatalf("stderr %q", stderr)
	}
}

func TestProposalApproveNeverConfirmsAHostKey(t *testing.T) {
	d := useFolderHost(t)
	env := &approveEnv{dir: t.TempDir(), inspect: func(int) control.SourceInspectResult {
		return control.SourceInspectResult{Case: control.SourceCaseClone, Access: &control.SourceAccess{Mode: "0755"}}
	}, extra: fakeHandlers{
		control.ProposalShowCommand: func(json.RawMessage) any {
			content, _ := json.Marshal(control.ProposalSourceAdd{Path: "/srv/example-stacks", URL: "ssh://git.example.test/team/stacks.git"})
			return control.ProposalShowResult{ProposalInfo: proposalInfo(approveID, control.ProposalOpen), Content: content}
		},
		control.RepoProbeCommand: func(json.RawMessage) any {
			return control.RepoProbeResult{Host: "git.example.test", Port: "22", KeyType: "ssh-ed25519", Fingerprint: "SHA256:examplekey"}
		},
	}}
	startApproveEnv(t, env)
	useTerminalStdin(t, "n\n")
	code, _, stderr := run(t, "proposal", "approve", approveID)
	if code == 0 || !strings.Contains(stderr, "SHA256:examplekey") || strings.Contains(env.history(), "approve") || d.dropIn(t) != "" {
		t.Fatalf("code %d, order %q, stderr %q", code, env.history(), stderr)
	}
}

func TestProposalApproveAsksBeforeMakingADeployKey(t *testing.T) {
	d := useFolderHost(t)
	env := &approveEnv{dir: t.TempDir(), inspect: func(int) control.SourceInspectResult {
		return control.SourceInspectResult{Case: control.SourceCaseClone, Access: &control.SourceAccess{Mode: "0755"}}
	}, extra: fakeHandlers{
		control.ProposalShowCommand: func(json.RawMessage) any {
			content, _ := json.Marshal(control.ProposalSourceAdd{Path: "/srv/example-stacks", URL: "ssh://git.example.test/team/stacks.git"})
			return control.ProposalShowResult{ProposalInfo: proposalInfo(approveID, control.ProposalOpen), Content: content}
		},
		control.RepoProbeCommand: func(json.RawMessage) any {
			return control.RepoProbeResult{Host: "git.example.test", Port: "22", KeyType: "ssh-ed25519", Fingerprint: "SHA256:examplekey", Pinned: true}
		},
		control.KeyShowCommand: func(json.RawMessage) any { return &control.Error{Code: control.CodeNoSuchKey, Message: "no key"} },
		control.KeyGenerateCommand: func(json.RawMessage) any {
			t.Error("a key was generated without a yes")
			return control.KeyResult{}
		},
	}}
	startApproveEnv(t, env)
	useTerminalStdin(t, "n\n")
	code, _, stderr := run(t, "proposal", "approve", approveID)
	if code == 0 || !strings.Contains(stderr, "no deploy key") || strings.Contains(env.history(), "approve") || d.dropIn(t) != "" {
		t.Fatalf("code %d, order %q, stderr %q", code, env.history(), stderr)
	}
}
