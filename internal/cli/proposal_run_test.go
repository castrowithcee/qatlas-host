package cli

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/host/hostchange"
)

// fakeShell records the commands it is asked to run and answers with the exit codes it was given.
type fakeShell struct {
	mu    sync.Mutex
	ran   []string
	exits map[string]int
	out   map[string]string
	limit time.Duration
}

func (s *fakeShell) RunShell(_ context.Context, command string, timeout time.Duration) hostchange.ShellResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ran = append(s.ran, command)
	s.limit = timeout
	return hostchange.ShellResult{Command: command, Ran: true, ExitCode: s.exits[command], Output: s.out[command]}
}

func (s *fakeShell) commands() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.Join(s.ran, "|")
}

// commandRunEnv is a host service that holds one proposal of free commands and records what the command asks.
type commandRunEnv struct {
	mu          sync.Mutex
	order       []string
	enabled     bool
	commands    []string
	fingerprint func(n int) string
	shows       int
	approved    control.ProposalApproveArgs
	result      control.ProposalResultArgs
}

func (e *commandRunEnv) note(s string) {
	e.mu.Lock()
	e.order = append(e.order, s)
	e.mu.Unlock()
}

func (e *commandRunEnv) history() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return strings.Join(e.order, ",")
}

func startCommandRunEnv(t *testing.T, euid int, env *commandRunEnv, sh *fakeShell) *changeFixture {
	t.Helper()
	f := newChangeFixture(t, euid)
	useControllingTerminal(t, true)
	changeEnv.shell = sh
	if env.fingerprint == nil {
		env.fingerprint = func(int) string { return "fp-one" }
	}
	startFakeService(t, fakeHandlers{})
	startFakeHostService(t, fakeHandlers{
		control.HostCommandsStatusCommand: func(json.RawMessage) any {
			env.note("status")
			return control.HostCommandsResult{Enabled: env.enabled}
		},
		control.ProposalShowCommand: func(json.RawMessage) any {
			env.mu.Lock()
			env.shows++
			n := env.shows
			env.mu.Unlock()
			env.note("show")
			info := hostProposalInfo(demoHostProposalID, control.ProposalOpen)
			info.Kind = control.ProposalKindCommand
			info.Fingerprint = env.fingerprint(n)
			content, _ := json.Marshal(control.ProposalCommand{Commands: env.commands, Reason: "invented reason"})
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
	return f
}

func TestProposalRunIsRefusedWhileTheSwitchIsOff(t *testing.T) {
	env := &commandRunEnv{commands: []string{"touch /srv/example/a"}}
	sh := &fakeShell{}
	f := startCommandRunEnv(t, 0, env, sh)
	code, _, stderr := run(t, "proposal", "run", demoHostProposalID, "--fingerprint", "fp-one", "--confirm")
	if code == 0 || !strings.Contains(stderr, "sudo qhost proposal commands enable") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if sh.commands() != "" || env.history() != "status" || journalFiles(t, f.dir) != 0 {
		t.Fatalf("ran %q, history %q", sh.commands(), env.history())
	}
}

func TestProposalRunNeedsRoot(t *testing.T) {
	env := &commandRunEnv{enabled: true, commands: []string{"true"}}
	sh := &fakeShell{}
	startCommandRunEnv(t, 1000, env, sh)
	code, _, stderr := run(t, "proposal", "run", demoHostProposalID, "--fingerprint", "fp-one", "--confirm")
	if code == 0 || !strings.Contains(stderr, "must run as root") || sh.commands() != "" || env.history() != "" {
		t.Fatalf("code %d, stderr %q, ran %q, history %q", code, stderr, sh.commands(), env.history())
	}
}

func TestProposalRunWithoutConfirmShowsThePlanAndRunsNothing(t *testing.T) {
	env := &commandRunEnv{enabled: true, commands: []string{"touch /srv/example/a", "echo hi\x1b[2J‮"}}
	sh := &fakeShell{}
	f := startCommandRunEnv(t, 0, env, sh)
	code, stdout, stderr := run(t, "proposal", "run", demoHostProposalID)
	if code != 2 || stdout != "" || !strings.Contains(stderr, "--confirm") {
		t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	for _, want := range []string{commandProposalNotice, "touch /srv/example/a", "invented reason", "fp-one", "echo hi�[2J�"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("%q is missing from %q", want, stderr)
		}
	}
	if strings.ContainsAny(stderr, "\x1b‮") {
		t.Errorf("a control character reached the terminal: %q", stderr)
	}
	if sh.commands() != "" || strings.Contains(env.history(), "approve") || journalFiles(t, f.dir) != 0 || len(f.audited) != 0 {
		t.Fatalf("ran %q, history %q", sh.commands(), env.history())
	}
}

func TestProposalRunRefusesAnotherFingerprintAndAChangedProposal(t *testing.T) {
	env := &commandRunEnv{enabled: true, commands: []string{"true"}}
	sh := &fakeShell{}
	startCommandRunEnv(t, 0, env, sh)
	code, _, stderr := run(t, "proposal", "run", demoHostProposalID, "--fingerprint", "fp-other", "--confirm")
	if code == 0 || !strings.Contains(stderr, "nothing ran") || sh.commands() != "" || strings.Contains(env.history(), "approve") {
		t.Fatalf("code %d, stderr %q, ran %q, history %q", code, stderr, sh.commands(), env.history())
	}
	// The proposal differs between the first look and the last one.
	env.fingerprint = func(n int) string { return map[bool]string{true: "fp-one", false: "fp-two"}[n%2 == 1] }
	env.shows = 0
	code, _, stderr = run(t, "proposal", "run", demoHostProposalID, "--fingerprint", "fp-one", "--confirm")
	if code == 0 || !strings.Contains(stderr, "nothing ran") || sh.commands() != "" || strings.Contains(env.history(), "approve") {
		t.Fatalf("changed: code %d, stderr %q, ran %q, history %q", code, stderr, sh.commands(), env.history())
	}
}

func TestProposalRunRunsInOrderAndKeepsJournalAndLog(t *testing.T) {
	env := &commandRunEnv{enabled: true, commands: []string{"first", "second"}}
	sh := &fakeShell{out: map[string]string{"first": "one\n"}}
	f := startCommandRunEnv(t, 0, env, sh)
	code, stdout, stderr := run(t, "proposal", "run", demoHostProposalID, "--fingerprint", "fp-one", "--confirm", "--timeout", "7s", "--output", "json")
	if code != 0 {
		t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if sh.commands() != "first|second" || sh.limit != 7*time.Second {
		t.Fatalf("ran %q with limit %v", sh.commands(), sh.limit)
	}
	if got := env.history(); got != "status,show,show,approve,result" || env.approved.Fingerprint != "fp-one" || !env.result.Applied {
		t.Fatalf("history %q, approved %+v, result %+v", got, env.approved, env.result)
	}
	m := decode(t, stdout)
	list, _ := m["commands"].([]any)
	if m["result"] != "applied" || m["proposal_id"] != demoHostProposalID || len(list) != 2 || list[0].(map[string]any)["output"] != "one\n" {
		t.Fatalf("%v", m)
	}
	journal, err := hostchange.OpenJournal(f.dir, changeEnv.ownerUID)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := journal.Get(m["journal_id"].(string))
	if err != nil || entry.Action != hostchange.ActionProposalRun || entry.Reversible || entry.NotReversible == "" || entry.SudoUID != 1000 ||
		strings.Join(entry.Commands, "|") != "first|second" || entry.SubmitterUID != 1000 || entry.SubmitterName != "demo-admin" || len(entry.Exits) != 2 || entry.Result != hostchange.ResultApplied {
		t.Fatalf("entry %+v, %v", entry, err)
	}
	if len(f.audited) != 1 || f.audited[0].ProposalID != demoHostProposalID || f.audited[0].SudoUID != 1000 || len(f.audited[0].Commands) != 0 ||
		strings.Contains(f.audited[0].Sentence, "first") {
		t.Fatalf("audit %+v", f.audited)
	}
	// The entry cannot be undone, with or without --confirm.
	for _, args := range [][]string{{"host", "undo", entry.ID, "--confirm"}, {"host", "undo", entry.ID}} {
		code, _, stderr = run(t, args...)
		if code == 0 || !strings.Contains(stderr, "cannot be undone") {
			t.Fatalf("%v: code %d, stderr %q", args, code, stderr)
		}
	}
	if sh.commands() != "first|second" {
		t.Fatalf("the undo ran something: %q", sh.commands())
	}
}

func TestProposalRunStopsAtTheFirstFailure(t *testing.T) {
	env := &commandRunEnv{enabled: true, commands: []string{"first", "second", "third"}}
	sh := &fakeShell{exits: map[string]int{"second": 3}}
	f := startCommandRunEnv(t, 0, env, sh)
	code, stdout, stderr := run(t, "proposal", "run", demoHostProposalID, "--fingerprint", "fp-one", "--confirm", "--output", "json")
	if code != 1 {
		t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if sh.commands() != "first|second" {
		t.Fatalf("ran %q", sh.commands())
	}
	m := decode(t, stdout)
	list := m["commands"].([]any)
	third := list[2].(map[string]any)
	if m["result"] != "failed" || list[1].(map[string]any)["exit_code"] != float64(3) || third["ran"] != false {
		t.Fatalf("%v", m)
	}
	if env.result.Applied || !strings.Contains(env.result.Message, "command 2") {
		t.Fatalf("result %+v", env.result)
	}
	journal, _ := hostchange.OpenJournal(f.dir, changeEnv.ownerUID)
	entry, err := journal.Get(m["journal_id"].(string))
	if err != nil || entry.Result != hostchange.ResultFailed || len(entry.Exits) != 2 || entry.Exits[1] != 3 {
		t.Fatalf("entry %+v, %v", entry, err)
	}
}

func TestProposalRunTextReportMarksCommandsThatDidNotRun(t *testing.T) {
	env := &commandRunEnv{enabled: true, commands: []string{"first", "second"}}
	sh := &fakeShell{exits: map[string]int{"first": 1}, out: map[string]string{"first": "bad\x1b[2J\nline"}}
	startCommandRunEnv(t, 0, env, sh)
	code, stdout, _ := run(t, "proposal", "run", demoHostProposalID, "--fingerprint", "fp-one", "--confirm")
	if code != 1 || !strings.Contains(stdout, "exit code 1") || !strings.Contains(stdout, "not run") || strings.ContainsRune(stdout, '\x1b') {
		t.Fatalf("code %d, stdout %q", code, stdout)
	}
}

func TestProposalRunChecksTheTimeout(t *testing.T) {
	env := &commandRunEnv{enabled: true, commands: []string{"true"}}
	sh := &fakeShell{}
	startCommandRunEnv(t, 0, env, sh)
	for _, d := range []string{"0s", "-1s", "2h"} {
		if code, _, _ := run(t, "proposal", "run", demoHostProposalID, "--fingerprint", "fp-one", "--confirm", "--timeout", d); code == 0 {
			t.Errorf("--timeout %s was accepted", d)
		}
	}
	if sh.commands() != "" {
		t.Fatalf("ran %q", sh.commands())
	}
}

func TestProposalRunIsNotForAnAgentOrAnotherKind(t *testing.T) {
	env := &commandRunEnv{enabled: true, commands: []string{"true"}}
	sh := &fakeShell{}
	startCommandRunEnv(t, 0, env, sh)
	if code, _, stderr := run(t, "proposal", "run", demoHostProposalID, "--fingerprint", "fp-one", "--confirm", "--agent"); code == 0 ||
		!strings.Contains(stderr, "not for an agent") || sh.commands() != "" {
		t.Fatalf("agent: code %d, stderr %q", code, stderr)
	}
	if code, _, _ := run(t, "proposal", "run", "p-0123456789abcdef", "--fingerprint", "fp-one", "--confirm"); code == 0 || sh.commands() != "" {
		t.Fatalf("a proposal of the deploy service was accepted: code %d", code)
	}
}

func TestApprovingACommandProposalPointsToRun(t *testing.T) {
	env := &commandRunEnv{enabled: true, commands: []string{"true"}}
	sh := &fakeShell{}
	startCommandRunEnv(t, 0, env, sh)
	useFolderHost(t)
	useTerminalStdin(t, "y\n")
	code, _, stderr := run(t, "proposal", "approve", demoHostProposalID)
	if code == 0 || !strings.Contains(stderr, "proposal run") {
		t.Fatalf("approve: code %d, stderr %q", code, stderr)
	}
	code, _, stderr = run(t, "host", "apply", "--proposal", demoHostProposalID, "--fingerprint", "fp-one", "--confirm")
	if code == 0 || !strings.Contains(stderr, "proposal run") {
		t.Fatalf("host apply: code %d, stderr %q", code, stderr)
	}
	if strings.Contains(env.history(), "approve") || sh.commands() != "" {
		t.Fatalf("history %q, ran %q", env.history(), sh.commands())
	}
}

func TestProposalRunNeedsAPersonAtATerminal(t *testing.T) {
	env := &commandRunEnv{enabled: true, commands: []string{"true"}}
	sh := &fakeShell{}
	startCommandRunEnv(t, 0, env, sh)
	useControllingTerminal(t, false)
	withStdin(t, strings.NewReader(""))
	for _, args := range [][]string{
		{"proposal", "run", demoHostProposalID, "--fingerprint", "fp-one", "--confirm"},
		{"proposal", "run", demoHostProposalID},
	} {
		code, _, stderr := run(t, args...)
		if code == 0 || !strings.Contains(stderr, "needs a terminal") {
			t.Fatalf("%v: code %d, stderr %q", args, code, stderr)
		}
	}
	if sh.commands() != "" || env.history() != "" {
		t.Fatalf("ran %q, history %q", sh.commands(), env.history())
	}
}
