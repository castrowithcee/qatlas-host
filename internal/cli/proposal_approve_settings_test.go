package cli

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// settingsEnv is a service holding one stack-settings proposal and the settings that apply to its stack.
type settingsEnv struct {
	mu          sync.Mutex
	order       []string
	showCount   int
	fingerprint func(n int) string
	current     func(n int) *control.StackSettings // nil: the stack has no settings
	getCount    int
	noStack     bool
	noStackFrom int // the stack is gone from this read on; 0 never
	content     control.ProposalStackSettings
	set         control.StackSettingsSetArgs
	result      control.ProposalResultArgs
	setErr      *control.Error
}

func (e *settingsEnv) note(s string) {
	e.mu.Lock()
	e.order = append(e.order, s)
	e.mu.Unlock()
}

func (e *settingsEnv) history() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return strings.Join(e.order, ",")
}

func startSettingsEnv(t *testing.T, env *settingsEnv) *fakeService {
	t.Helper()
	if env.fingerprint == nil {
		env.fingerprint = func(int) string { return "fp-one" }
	}
	if env.current == nil {
		env.current = func(int) *control.StackSettings { return &control.StackSettings{Version: 1} }
	}
	return startFakeService(t, fakeHandlers{
		control.ProposalShowCommand: func(json.RawMessage) any {
			env.mu.Lock()
			env.showCount++
			n := env.showCount
			env.mu.Unlock()
			env.note("show")
			info := proposalInfo(approveID, control.ProposalOpen)
			info.Kind = control.ProposalKindStackSettings
			info.Fingerprint = env.fingerprint(n)
			content, _ := json.Marshal(env.content)
			return control.ProposalShowResult{ProposalInfo: info, Content: content}
		},
		control.StackSettingsGetCommand: func(json.RawMessage) any {
			env.mu.Lock()
			env.getCount++
			n := env.getCount
			env.mu.Unlock()
			env.note("get")
			if env.noStack || (env.noStackFrom > 0 && n >= env.noStackFrom) {
				return control.NewError(control.CodeNoSuchStack, "no such stack in the working copy")
			}
			return control.StackSettingsResult{Stack: env.content.Stack, Settings: env.current(n)}
		},
		control.ProposalApproveCommand: func(raw json.RawMessage) any {
			env.note("approve")
			return proposalInfo(approveID, control.ProposalApproving)
		},
		control.StackSettingsSetCommand: func(raw json.RawMessage) any {
			env.note("set")
			_ = json.Unmarshal(raw, &env.set)
			if env.setErr != nil {
				return env.setErr
			}
			return control.StackSettingsSetResult{Stack: env.set.Stack, Settings: env.set.Settings, Changed: true, Commit: "abc1234"}
		},
		control.ProposalResultCommand: func(raw json.RawMessage) any {
			env.note("result")
			_ = json.Unmarshal(raw, &env.result)
			return proposalInfo(approveID, control.ProposalApplied)
		},
	})
}

func settingsContent(command ...string) control.ProposalStackSettings {
	mode := "automatic"
	return control.ProposalStackSettings{Stack: "web", Settings: control.StackSettings{Version: 1,
		Update: &control.StackUpdateSettings{Mode: mode},
		Backup: &control.StackBackupSettings{Paths: []string{"data"}, Command: command}}}
}

func TestStackSettingsApproveShowsTheDifferenceAndApplies(t *testing.T) {
	useFolderHost(t)
	env := &settingsEnv{content: settingsContent("backup-tool", "--target", "ssh://git.example.test/backups"),
		current: func(int) *control.StackSettings {
			return &control.StackSettings{Version: 1, Update: &control.StackUpdateSettings{Mode: "manual"}}
		}}
	startSettingsEnv(t, env)
	useTerminalStdin(t, "y\n")
	code, stdout, stderr := run(t, "proposal", "approve", approveID, "--output", "json")
	if code != 0 {
		t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if got, want := env.history(), "show,get,show,get,approve,set,result"; got != want {
		t.Fatalf("order %q, want %q", got, want)
	}
	if env.set.Stack != "web" || env.set.Settings.Update.Mode != "automatic" || !env.result.Applied || env.result.Fingerprint != "fp-one" {
		t.Fatalf("set %+v, result %+v", env.set, env.result)
	}
	for _, want := range []string{"update.mode", `"manual"`, `"automatic"`, "!! backup.command[0]", `"backup-tool"`,
		`"ssh://git.example.test/backups"`, "backup command runs on the host", "backup.paths[0]"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr lacks %q:\n%s", want, stderr)
		}
	}
	if !strings.Contains(stdout, "abc1234") {
		t.Errorf("stdout %q", stdout)
	}
}

func TestStackSettingsApproveShowsLongValuesInFullAndWithoutControlCharacters(t *testing.T) {
	useFolderHost(t)
	long := strings.Repeat("0123456789", 60)
	env := &settingsEnv{content: settingsContent("backup-tool", long+"\x1b[2J\u202e")}
	startSettingsEnv(t, env)
	useTerminalStdin(t, "n\n")
	_, _, stderr := run(t, "proposal", "approve", approveID)
	if strings.ContainsAny(stderr, "\x1b\u202e") {
		t.Fatalf("stderr holds a control character: %q", stderr)
	}
	flat := strings.NewReplacer("\n", "", " ", "").Replace(stderr)
	if !strings.Contains(flat, long) || strings.Contains(stderr, "more characters") {
		t.Fatalf("the value is not shown in full:\n%s", stderr)
	}
	for _, line := range strings.Split(stderr, "\n") {
		if strings.HasPrefix(line, "      ") && len([]rune(line)) > 100 {
			t.Fatalf("line not wrapped: %q", line)
		}
	}
}

func TestStackSettingsApproveRefusesAnUnknownStack(t *testing.T) {
	useFolderHost(t)
	env := &settingsEnv{content: settingsContent(), noStack: true}
	startSettingsEnv(t, env)
	useTerminalStdin(t, "y\n")
	code, _, stderr := run(t, "proposal", "approve", approveID)
	if code == 0 || !strings.Contains(stderr, "does not exist") || env.history() != "show,get" {
		t.Fatalf("code %d, order %q, stderr %q", code, env.history(), stderr)
	}
}

func TestStackSettingsApproveRefusesAStackThatVanishedBeforeApplying(t *testing.T) {
	useFolderHost(t)
	env := &settingsEnv{content: settingsContent(), noStackFrom: 2}
	startSettingsEnv(t, env)
	useTerminalStdin(t, "y\n")
	code, _, stderr := run(t, "proposal", "approve", approveID)
	if code == 0 || strings.Contains(env.history(), "approve") || strings.Contains(env.history(), "set") {
		t.Fatalf("code %d, order %q, stderr %q", code, env.history(), stderr)
	}
}

func TestStackSettingsApproveRefusesChangedCurrentSettingsAndShowsThem(t *testing.T) {
	useFolderHost(t)
	env := &settingsEnv{content: settingsContent(), current: func(n int) *control.StackSettings {
		s := &control.StackSettings{Version: 1}
		if n > 1 {
			s.Update = &control.StackUpdateSettings{Mode: "manual", Ceiling: "major"}
		}
		return s
	}}
	startSettingsEnv(t, env)
	useTerminalStdin(t, "y\n")
	code, _, stderr := run(t, "proposal", "approve", approveID)
	if code == 0 || !strings.Contains(stderr, "effect of the proposal changed") || !strings.Contains(stderr, "The effect changed since it was shown") ||
		strings.Contains(env.history(), "approve") || strings.Contains(env.history(), "set") {
		t.Fatalf("code %d, order %q, stderr %q", code, env.history(), stderr)
	}
}

func TestStackSettingsApproveRefusesAChangedProposal(t *testing.T) {
	useFolderHost(t)
	env := &settingsEnv{content: settingsContent(), fingerprint: func(n int) string {
		if n == 1 {
			return "fp-one"
		}
		return "fp-two"
	}}
	startSettingsEnv(t, env)
	useTerminalStdin(t, "y\n")
	code, _, stderr := run(t, "proposal", "approve", approveID)
	if code == 0 || !strings.Contains(stderr, "changed since it was shown") || strings.Contains(env.history(), "approve") {
		t.Fatalf("code %d, order %q, stderr %q", code, env.history(), stderr)
	}
}

func TestStackSettingsApproveDeclinedChangesNothing(t *testing.T) {
	useFolderHost(t)
	env := &settingsEnv{content: settingsContent()}
	startSettingsEnv(t, env)
	useTerminalStdin(t, "n\n")
	if code, _, _ := run(t, "proposal", "approve", approveID); code == 0 || env.history() != "show,get" {
		t.Fatalf("code %d, order %q", code, env.history())
	}
}

func TestStackSettingsApproveReportsAFailedChange(t *testing.T) {
	useFolderHost(t)
	env := &settingsEnv{content: settingsContent(), setErr: control.NewError(control.CodeInvalid, "stack settings: update.tags: unknown variable")}
	startSettingsEnv(t, env)
	useTerminalStdin(t, "y\n")
	code, _, _ := run(t, "proposal", "approve", approveID)
	if code == 0 || env.history() != "show,get,show,get,approve,set,result" || env.result.Applied || env.result.Message == "" {
		t.Fatalf("code %d, order %q, result %+v", code, env.history(), env.result)
	}
}

func TestStackSettingsApproveRefusesAnAgentAndUrl(t *testing.T) {
	useFolderHost(t)
	env := &settingsEnv{content: settingsContent()}
	startSettingsEnv(t, env)
	useTerminalStdin(t, "y\n")
	if code, _, _ := run(t, "proposal", "approve", approveID, "--agent"); code == 0 || env.history() != "" {
		t.Fatalf("agent: code %d, order %q", code, env.history())
	}
	useTerminalStdin(t, "y\n")
	if code, _, _ := run(t, "proposal", "approve", approveID, "--url", "ssh://git.example.test/a.git"); code == 0 || strings.Contains(env.history(), "approve") {
		t.Fatalf("url: code %d, order %q", code, env.history())
	}
}
