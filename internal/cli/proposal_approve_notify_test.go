package cli

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

const (
	proposalHookURL = "https://hooks.example.test/demo-proposal-secret-path"
	proposalHookKey = "demo-proposal-signing-key"
)

type notifyApproveEnv struct {
	mu          sync.Mutex
	order       []string
	showN       int
	fingerprint func(n int) string
	existing    func(n int) []control.NotifyChannelInfo
	listN       int
	content     control.ProposalNotifyChannel
	added       control.NotifyAddArgs
	addErr      *control.Error
	result      control.ProposalResultArgs
}

func (e *notifyApproveEnv) note(s string) {
	e.mu.Lock()
	e.order = append(e.order, s)
	e.mu.Unlock()
}

func (e *notifyApproveEnv) history() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return strings.Join(e.order, ",")
}

func startNotifyApproveEnv(t *testing.T, env *notifyApproveEnv) {
	t.Helper()
	if env.fingerprint == nil {
		env.fingerprint = func(int) string { return "fp-one" }
	}
	startFakeService(t, fakeHandlers{
		control.ProposalShowCommand: func(json.RawMessage) any {
			env.mu.Lock()
			env.showN++
			n := env.showN
			env.mu.Unlock()
			env.note("show")
			info := proposalInfo(approveID, control.ProposalOpen)
			info.Kind = control.ProposalKindNotifyChannel
			info.Fingerprint = env.fingerprint(n)
			content, _ := json.Marshal(env.content)
			return control.ProposalShowResult{ProposalInfo: info, Content: content}
		},
		control.NotifyListCommand: func(json.RawMessage) any {
			env.mu.Lock()
			env.listN++
			n := env.listN
			env.mu.Unlock()
			env.note("list")
			res := control.NotifyListResult{Channels: []control.NotifyChannelInfo{}}
			if env.existing != nil {
				res.Channels = env.existing(n)
			}
			return res
		},
		control.ProposalApproveCommand: func(json.RawMessage) any {
			env.note("approve")
			return proposalInfo(approveID, control.ProposalApproving)
		},
		control.NotifyAddCommand: func(raw json.RawMessage) any {
			env.note("add")
			_ = json.Unmarshal(raw, &env.added)
			if env.addErr != nil {
				return env.addErr
			}
			return control.NotifyChannelInfo{Name: env.added.Name, Type: env.added.Type, Events: env.added.Events}
		},
		control.ProposalResultCommand: func(raw json.RawMessage) any {
			env.note("result")
			_ = json.Unmarshal(raw, &env.result)
			return proposalInfo(approveID, control.ProposalApplied)
		},
	})
}

// useHiddenInput answers each hidden prompt with the next value.
func useHiddenInput(t *testing.T, values ...string) {
	t.Helper()
	next := 0
	oldRead := readHidden
	readHidden = func(*os.File) ([]byte, error) {
		v := values[next]
		next++
		return []byte(v), nil
	}
	t.Cleanup(func() { readHidden = oldRead })
}

func webhookContent() control.ProposalNotifyChannel {
	return control.ProposalNotifyChannel{Name: "ops-hook", Type: control.NotifyWebhook, Events: []string{"failed", "recovered"}}
}

func TestNotifyChannelApproveReadsSecretsAndAdds(t *testing.T) {
	useFolderHost(t)
	env := &notifyApproveEnv{content: webhookContent()}
	startNotifyApproveEnv(t, env)
	useTerminalStdin(t, "y\n")
	useHiddenInput(t, proposalHookURL, proposalHookKey)
	code, stdout, stderr := run(t, "proposal", "approve", approveID, "--output", "json")
	if code != 0 {
		t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if got, want := env.history(), "show,list,show,list,approve,add,result"; got != want {
		t.Fatalf("order %q, want %q", got, want)
	}
	if env.added.Secret != proposalHookURL || env.added.Key != proposalHookKey || env.added.Name != "ops-hook" ||
		strings.Join(env.added.Events, ",") != "failed,recovered" || !env.result.Applied || env.result.Fingerprint != "fp-one" {
		t.Fatalf("added %+v, result %+v", env.added, env.result)
	}
	for _, want := range []string{"ops-hook", "webhook", "failed,recovered", "input is hidden", "webhook URL", "signing key"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr lacks %q:\n%s", want, stderr)
		}
	}
	for _, text := range []string{stdout, stderr, env.result.Message} {
		for _, secret := range []string{proposalHookURL, "demo-proposal-secret-path", proposalHookKey} {
			if strings.Contains(text, secret) {
				t.Fatalf("a secret appears in the output: %q", text)
			}
		}
	}
}

func TestNotifyChannelApproveAsksForThePasswordOnlyWithAUser(t *testing.T) {
	smtp := control.ProposalNotifyChannel{Name: "mail", Type: control.NotifySMTP, Host: "mail.example.test", Port: 587,
		Security: "starttls", From: "qhost@example.test", To: []string{"ops@example.test"}, User: "qhost"}
	for _, withUser := range []bool{true, false} {
		useFolderHost(t)
		c := smtp
		var hidden []string
		if withUser {
			hidden = []string{"demo-smtp-password-not-real"}
		} else {
			c.User = ""
		}
		env := &notifyApproveEnv{content: c}
		startNotifyApproveEnv(t, env)
		useTerminalStdin(t, "y\n")
		useHiddenInput(t, hidden...)
		code, _, stderr := run(t, "proposal", "approve", approveID)
		if code != 0 || env.added.Secret != strings.Join(hidden, "") || strings.Contains(stderr, "demo-smtp-password-not-real") {
			t.Fatalf("user %v: code %d, added %+v, stderr %q", withUser, code, env.added, stderr)
		}
		if withUser && !strings.Contains(stderr, "SMTP password") || !withUser && strings.Contains(stderr, "SMTP password") {
			t.Fatalf("user %v: prompts wrong: %q", withUser, stderr)
		}
	}
}

func TestNotifyChannelApproveShowsEveryFieldInFullWithoutControlCharacters(t *testing.T) {
	useFolderHost(t)
	base := "https://bot.example.test/" + strings.Repeat("0123456789", 20)
	env := &notifyApproveEnv{content: control.ProposalNotifyChannel{Name: "chat", Type: control.NotifyTelegram,
		ChatID: "@opschat", BaseURL: base + "\x1b[2J\u202e", Events: []string{"failed"}}}
	startNotifyApproveEnv(t, env)
	useTerminalStdin(t, "n\n")
	_, _, stderr := run(t, "proposal", "approve", approveID)
	if strings.ContainsAny(stderr, "\x1b\u202e") {
		t.Fatalf("stderr holds a control character: %q", stderr)
	}
	flat := strings.NewReplacer("\n", "", " ", "").Replace(stderr)
	if !strings.Contains(flat, base) || strings.Contains(stderr, "more characters") || !strings.Contains(stderr, "@opschat") {
		t.Fatalf("a field is not shown in full:\n%s", stderr)
	}
}

func TestNotifyChannelApproveRefusesATakenName(t *testing.T) {
	useFolderHost(t)
	env := &notifyApproveEnv{content: webhookContent(), existing: func(int) []control.NotifyChannelInfo {
		return []control.NotifyChannelInfo{{Name: "ops-hook", Type: "slack"}}
	}}
	startNotifyApproveEnv(t, env)
	useTerminalStdin(t, "y\n")
	code, _, stderr := run(t, "proposal", "approve", approveID)
	if code == 0 || !strings.Contains(stderr, "exists now") || env.history() != "show,list" {
		t.Fatalf("code %d, order %q, stderr %q", code, env.history(), stderr)
	}
}

func TestNotifyChannelApproveRefusesAChannelThatAppearedBeforeApplying(t *testing.T) {
	useFolderHost(t)
	env := &notifyApproveEnv{content: webhookContent(), existing: func(n int) []control.NotifyChannelInfo {
		if n > 1 {
			return []control.NotifyChannelInfo{{Name: "ops-hook", Type: "slack"}}
		}
		return nil
	}}
	startNotifyApproveEnv(t, env)
	useTerminalStdin(t, "y\n")
	useHiddenInput(t, proposalHookURL, proposalHookKey)
	code, _, stderr := run(t, "proposal", "approve", approveID)
	if code == 0 || strings.Contains(env.history(), "approve") || strings.Contains(env.history(), "add") {
		t.Fatalf("code %d, order %q, stderr %q", code, env.history(), stderr)
	}
	if strings.Contains(stderr, proposalHookURL) || strings.Contains(stderr, proposalHookKey) {
		t.Fatalf("a secret appears: %q", stderr)
	}
}

func TestNotifyChannelApproveRefusesAChangedProposal(t *testing.T) {
	useFolderHost(t)
	env := &notifyApproveEnv{content: webhookContent(), fingerprint: func(n int) string {
		if n == 1 {
			return "fp-one"
		}
		return "fp-two"
	}}
	startNotifyApproveEnv(t, env)
	useTerminalStdin(t, "y\n")
	useHiddenInput(t, proposalHookURL, proposalHookKey)
	code, _, stderr := run(t, "proposal", "approve", approveID)
	if code == 0 || !strings.Contains(stderr, "changed since it was shown") || strings.Contains(env.history(), "approve") {
		t.Fatalf("code %d, order %q, stderr %q", code, env.history(), stderr)
	}
}

func TestNotifyChannelApproveDeclinedAsksForNoSecret(t *testing.T) {
	useFolderHost(t)
	env := &notifyApproveEnv{content: webhookContent()}
	startNotifyApproveEnv(t, env)
	useTerminalStdin(t, "n\n")
	useHiddenInput(t)
	if code, _, _ := run(t, "proposal", "approve", approveID); code == 0 || env.history() != "show,list" {
		t.Fatalf("code %d, order %q", code, env.history())
	}
}

func TestNotifyChannelApproveFailureKeepsNoSecretInTheResult(t *testing.T) {
	useFolderHost(t)
	env := &notifyApproveEnv{content: webhookContent(), addErr: control.NewError(control.CodeInvalid,
		"echoed "+proposalHookURL+" and "+proposalHookKey)}
	startNotifyApproveEnv(t, env)
	useTerminalStdin(t, "y\n")
	useHiddenInput(t, proposalHookURL, proposalHookKey)
	code, stdout, stderr := run(t, "proposal", "approve", approveID)
	if code == 0 || env.history() != "show,list,show,list,approve,add,result" || env.result.Applied || env.result.Message == "" {
		t.Fatalf("code %d, order %q, result %+v", code, env.history(), env.result)
	}
	for _, text := range []string{stdout, stderr, env.result.Message} {
		if strings.Contains(text, proposalHookURL) || strings.Contains(text, proposalHookKey) {
			t.Fatalf("a secret appears: %q", text)
		}
	}
}

func TestNotifyChannelApproveRefusesAnAgentAndUrl(t *testing.T) {
	useFolderHost(t)
	env := &notifyApproveEnv{content: webhookContent()}
	startNotifyApproveEnv(t, env)
	useTerminalStdin(t, "y\n")
	if code, _, _ := run(t, "proposal", "approve", approveID, "--agent"); code == 0 || env.history() != "" {
		t.Fatalf("agent: code %d, order %q", code, env.history())
	}
	useTerminalStdin(t, "y\n")
	if code, _, _ := run(t, "proposal", "approve", approveID, "--url", "ssh://git.example.test/a.git"); code == 0 || strings.Contains(env.history(), "approve") {
		t.Fatalf("url: code %d, order %q", code, env.history())
	}
}
