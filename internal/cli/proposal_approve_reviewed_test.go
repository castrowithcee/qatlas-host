package cli

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// useReviewedRun runs the command the way the terminal interface without root does: standard input is no terminal
// and holds stdin, and the process has a terminal of its own.
func useReviewedRun(t *testing.T, stdin string) {
	t.Helper()
	useStdin(t, strings.NewReader(stdin))
	useControllingTerminal(t, true)
}

func decodeObject(t *testing.T, stdout string) map[string]any {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout %q is no JSON object: %v", stdout, err)
	}
	return got
}

func TestReviewedApproveOfEachKindShowsWithoutConfirmAndAppliesWithIt(t *testing.T) {
	for _, tc := range []struct {
		kind    string
		start   func(t *testing.T) (history func() string)
		stdin   string
		applied string
		fields  map[string]any
	}{
		{kind: control.ProposalKindSourceAdd, start: func(t *testing.T) func() string {
			env := &approveEnv{dir: t.TempDir()}
			startApproveEnv(t, env)
			return env.history
		}, applied: "show,inspect,show,inspect,approve,add,result", fields: map[string]any{"id": "src-1", "kind": control.SourceKindLocal}},
		{kind: control.ProposalKindStackSettings, start: func(t *testing.T) func() string {
			env := &settingsEnv{content: settingsContent()}
			startSettingsEnv(t, env)
			return env.history
		}, applied: "show,get,show,get,approve,set,result", fields: map[string]any{"proposal": approveID, "stack": "web", "commit": "abc1234"}},
		{kind: control.ProposalKindWindow, start: func(t *testing.T) func() string {
			env := &windowEnv{content: control.WindowSetArgs{Days: []string{"sat"}, Start: "01:30"}}
			startWindowEnv(t, env)
			return env.history
		}, applied: "show,get,show,get,approve,set,result", fields: map[string]any{"proposal": approveID, "days": "sat", "start": "01:30"}},
		{kind: control.ProposalKindNotifyChannel, start: func(t *testing.T) func() string {
			env := &notifyApproveEnv{content: webhookContent()}
			startNotifyApproveEnv(t, env)
			return env.history
		}, stdin: `{"secret": "` + proposalHookURL + `", "key": "` + proposalHookKey + `"}`,
			applied: "show,list,show,list,approve,add,result", fields: map[string]any{"proposal": approveID, "name": "ops-hook", "type": "webhook"}},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			d := useFolderHost(t)
			history := tc.start(t)
			useReviewedRun(t, tc.stdin)
			code, stdout, stderr := run(t, "proposal", "approve", approveID, "--fingerprint", "fp-one", "--output", "json")
			if code != 2 || stdout != "" || !strings.Contains(stderr, "--confirm") || !strings.Contains(stderr, "Effect") ||
				strings.Contains(history(), "approve") || d.dropIn(t) != "" {
				t.Fatalf("without --confirm: code %d, order %q, stdout %q, stderr %q", code, history(), stdout, stderr)
			}
			before := history()

			useReviewedRun(t, tc.stdin)
			code, stdout, stderr = run(t, "proposal", "approve", approveID, "--fingerprint", "fp-one", "--confirm", "--output", "json")
			if code != 0 {
				t.Fatalf("with --confirm: code %d, stdout %q, stderr %q", code, stdout, stderr)
			}
			if got := strings.TrimPrefix(history(), before+","); got != tc.applied {
				t.Fatalf("order %q, want %q", got, tc.applied)
			}
			got := decodeObject(t, stdout)
			for k, v := range tc.fields {
				if got[k] != v {
					t.Errorf("%s = %v, want %v in %s", k, got[k], v, stdout)
				}
			}
			if strings.Contains(stderr, "[y/N]") {
				t.Errorf("a question was asked:\n%s", stderr)
			}
		})
	}
}

func TestReviewedApproveRefusesAnotherFingerprintBeforeAnything(t *testing.T) {
	d := useFolderHost(t)
	env := &approveEnv{dir: t.TempDir()}
	startApproveEnv(t, env)
	useReviewedRun(t, "")
	code, stdout, stderr := run(t, "proposal", "approve", approveID, "--fingerprint", "fp-other", "--confirm")
	if code != 2 || stdout != "" || !strings.Contains(stderr, "not the one that was shown; nothing was applied") ||
		env.history() != "show" || d.dropIn(t) != "" {
		t.Fatalf("code %d, order %q, stderr %q", code, env.history(), stderr)
	}
}

func TestReviewedApproveRefusesAProposalThatChangesBeforeTheLastLook(t *testing.T) {
	for _, kind := range []string{control.ProposalKindSourceAdd, control.ProposalKindStackSettings} {
		t.Run(kind, func(t *testing.T) {
			d := useFolderHost(t)
			changed := func(n int) string {
				if n == 1 {
					return "fp-one"
				}
				return "fp-two"
			}
			history := func() string { return "" }
			if kind == control.ProposalKindSourceAdd {
				env := &approveEnv{dir: t.TempDir(), fingerprint: changed}
				startApproveEnv(t, env)
				history = env.history
			} else {
				env := &settingsEnv{content: settingsContent(), fingerprint: changed}
				startSettingsEnv(t, env)
				history = env.history
			}
			useReviewedRun(t, "")
			code, _, stderr := run(t, "proposal", "approve", approveID, "--fingerprint", "fp-one", "--confirm")
			if code == 0 || !strings.Contains(stderr, "nothing was applied") || strings.Contains(history(), "approve") || d.dropIn(t) != "" {
				t.Fatalf("code %d, order %q, stderr %q", code, history(), stderr)
			}
		})
	}
}

func TestReviewedSourceAddIsBoundToTheEffectThatWasShown(t *testing.T) {
	d := useFolderHost(t)
	env := &approveEnv{dir: t.TempDir()}
	startApproveEnv(t, env)
	useReviewedRun(t, "")
	_, _, stderr := run(t, "proposal", "approve", approveID, "--fingerprint", "fp-one")
	m := regexp.MustCompile(`Effect fingerprint of the folder: (\S+)`).FindStringSubmatch(stderr)
	if m == nil {
		t.Fatalf("the preview names no effect:\n%s", stderr)
	}
	// The effect is the one 'source inspect' shows, which the terminal interface passes.
	if _, got := inspectJSON(t, env.dir); got["effect"] != m[1] {
		t.Fatalf("preview effect %q, source inspect %v", m[1], got["effect"])
	}

	useReviewedRun(t, "")
	code, stdout, stderr := run(t, "proposal", "approve", approveID, "--fingerprint", "fp-one", "--confirm",
		"--expect-effect", "sha256:another-effect")
	if code == 0 || stdout != "" || !strings.Contains(stderr, "state-changed") || strings.Contains(env.history(), "approve") || d.dropIn(t) != "" {
		t.Fatalf("other effect: code %d, order %q, stderr %q", code, env.history(), stderr)
	}

	useReviewedRun(t, "")
	code, _, stderr = run(t, "proposal", "approve", approveID, "--fingerprint", "fp-one", "--confirm", "--expect-effect", m[1])
	if code != 0 || !strings.Contains(env.history(), "approve,add,result") || !strings.Contains(d.dropIn(t), env.dir) {
		t.Fatalf("same effect: code %d, order %q, stderr %q", code, env.history(), stderr)
	}
}

func TestReviewedSourceAddRefusesAnEffectThatChangesBeforeTheLastLook(t *testing.T) {
	d := useFolderHost(t)
	env := &approveEnv{dir: t.TempDir(), inspect: func(n int) control.SourceInspectResult {
		files := []string{"demo/compose.yaml"}
		if n > 1 {
			files = append(files, "demo/new.env")
		}
		return control.SourceInspectResult{Case: control.SourceCaseFiles, PlannedFiles: files, Access: &control.SourceAccess{Mode: "0755"}}
	}}
	startApproveEnv(t, env)
	useReviewedRun(t, "")
	code, _, stderr := run(t, "proposal", "approve", approveID, "--fingerprint", "fp-one", "--confirm")
	if code == 0 || !strings.Contains(stderr, "state-changed") || !strings.Contains(stderr, "demo/new.env") ||
		strings.Contains(env.history(), "approve") || d.dropIn(t) != "" || len(d.cmds) != 0 {
		t.Fatalf("code %d, order %q, stderr %q", code, env.history(), stderr)
	}
}

func TestReviewedSourceAddOfFilesIsBoundToTheComparison(t *testing.T) {
	for name, tc := range map[string]struct {
		state, commit string
		changed       bool
	}{
		"matches":      {control.SourceRemoteDifferent, remoteCommit, false},
		"state moved":  {control.SourceRemoteEqual, remoteCommit, true},
		"commit moved": {control.SourceRemoteDifferent, "fedcba9876543210fedcba9876543210fedcba98", true},
	} {
		t.Run(name, func(t *testing.T) {
			d := useFolderHost(t)
			tree := newFilesTree(t)
			fake := &remoteFake{compare: differentCompare(control.SourceCaseFiles)}
			extra := fake.handlers(listedFilesInspect())
			delete(extra, control.SourceInspectCommand)
			extra[control.KeyShowCommand] = func(json.RawMessage) any { return control.KeyResult{Fingerprint: "SHA256:demo-deploy-key"} }
			env := &approveEnv{dir: tree.dir, inspect: func(int) control.SourceInspectResult { return listedFilesInspect() }, extra: extra}
			startApproveEnv(t, env)
			before := tree.snapshot(t)
			useReviewedRun(t, "")
			code, stdout, stderr := run(t, "proposal", "approve", approveID, "--url", remoteURL, "--fingerprint", "fp-one", "--confirm",
				"--host-key-fingerprint", remoteKeyPrint, "--expect-state", tc.state, "--expect-commit", tc.commit, "--output", "json")
			if !tc.changed {
				if code != 0 || len(fake.added) != 1 || fake.added[0].ExpectState != tc.state || fake.added[0].Fingerprint != remoteKeyPrint {
					t.Fatalf("code %d, stderr %q, added %+v", code, stderr, fake.added)
				}
				if got := decodeObject(t, stdout); got["url"] != remoteURL || got["state"] != tc.state {
					t.Errorf("stdout %s", stdout)
				}
				return
			}
			if code == 0 || stdout != "" || !strings.Contains(stderr, "state-changed") || !fake.untouched() ||
				strings.Contains(env.history(), "approve") || !equalModes(before, tree.snapshot(t)) || d.dropIn(t) != "" {
				t.Fatalf("code %d, order %q, stderr %q", code, env.history(), stderr)
			}
		})
	}
}

// cloneApproveEnv holds a source-add proposal that clones a repository, with a server whose host key is not pinned.
func cloneApproveEnv(t *testing.T, key func(json.RawMessage) any) *approveEnv {
	t.Helper()
	extra := fakeHandlers{
		control.ProposalShowCommand: func(json.RawMessage) any {
			info := proposalInfo(approveID, control.ProposalOpen)
			info.Fingerprint = "fp-one"
			content, _ := json.Marshal(control.ProposalSourceAdd{Path: "/srv/example-stacks", URL: remoteURL})
			return control.ProposalShowResult{ProposalInfo: info, Content: content}
		},
		control.RepoProbeCommand: func(json.RawMessage) any {
			return control.RepoProbeResult{Host: "git.example.test", Port: "22", KeyType: "ssh-ed25519", Fingerprint: remoteKeyPrint}
		},
		control.KeyShowCommand: key,
		control.KeyGenerateCommand: func(json.RawMessage) any {
			t.Error("a deploy key was generated")
			return control.KeyResult{}
		},
	}
	env := &approveEnv{dir: t.TempDir(), inspect: func(int) control.SourceInspectResult {
		return control.SourceInspectResult{Case: control.SourceCaseClone, Access: &control.SourceAccess{Mode: "0755"}}
	}, extra: extra}
	startApproveEnv(t, env)
	return env
}

func TestReviewedSourceAddNeverAsksForTheHostKey(t *testing.T) {
	d := useFolderHost(t)
	env := cloneApproveEnv(t, func(json.RawMessage) any { return control.KeyResult{Fingerprint: "SHA256:demo-deploy-key"} })
	// A yes at a terminal would trust the key if it were asked.
	useTerminalStdin(t, "y\ny\n")
	code, _, stderr := run(t, "proposal", "approve", approveID, "--fingerprint", "fp-one", "--confirm")
	if code != 2 || !strings.Contains(stderr, "--host-key-fingerprint "+remoteKeyPrint) || strings.Contains(stderr, "Trust this key") ||
		strings.Contains(env.history(), "approve") || d.dropIn(t) != "" {
		t.Fatalf("code %d, order %q, stderr %q", code, env.history(), stderr)
	}

	useReviewedRun(t, "")
	code, _, stderr = run(t, "proposal", "approve", approveID, "--fingerprint", "fp-one", "--confirm",
		"--host-key-fingerprint", "SHA256:another-key")
	if code == 0 || !strings.Contains(stderr, "not the expected") || strings.Contains(env.history(), "approve") || d.dropIn(t) != "" {
		t.Fatalf("other host key: code %d, order %q, stderr %q", code, env.history(), stderr)
	}
}

func TestReviewedSourceAddRefusesAFolderWithoutDeployKey(t *testing.T) {
	d := useFolderHost(t)
	env := cloneApproveEnv(t, func(json.RawMessage) any { return &control.Error{Code: control.CodeNoSuchKey, Message: "no key"} })
	for _, confirm := range []bool{false, true} {
		useReviewedRun(t, "y\n")
		args := []string{"proposal", "approve", approveID, "--fingerprint", "fp-one", "--host-key-fingerprint", remoteKeyPrint}
		if confirm {
			args = append(args, "--confirm")
		}
		code, _, stderr := run(t, args...)
		if code == 0 || !strings.Contains(stderr, "no deploy key") || !strings.Contains(stderr, "qhost key generate") ||
			strings.Contains(env.history(), "approve") || d.dropIn(t) != "" {
			t.Fatalf("confirm %v: code %d, order %q, stderr %q", confirm, code, env.history(), stderr)
		}
	}
}

func TestReviewedNotifyChannelTakesItsSecretsOnlyAsJSONOnStdin(t *testing.T) {
	telegram := control.ProposalNotifyChannel{Name: "ops-bot", Type: control.NotifyTelegram, Events: []string{"failed"}, ChatID: "4242"}
	for name, tc := range map[string]struct {
		content control.ProposalNotifyChannel
		stdin   string
		ok      bool
	}{
		"both parts":           {webhookContent(), `{"secret":"` + proposalHookURL + `","key":"` + proposalHookKey + `"}` + "\n", true},
		"one per line":         {webhookContent(), proposalHookURL + "\n" + proposalHookKey + "\n", false},
		"key missing":          {webhookContent(), `{"secret":"` + proposalHookURL + `"}`, false},
		"unknown field":        {webhookContent(), `{"secret":"` + proposalHookURL + `","key":"` + proposalHookKey + `","url":"x"}`, false},
		"data after":           {webhookContent(), `{"secret":"` + proposalHookURL + `","key":"` + proposalHookKey + `"} {}`, false},
		"empty":                {webhookContent(), "", false},
		"key it does not take": {telegram, `{"secret":"` + proposalHookURL + `","key":"` + proposalHookKey + `"}`, false},
	} {
		t.Run(name, func(t *testing.T) {
			useFolderHost(t)
			env := &notifyApproveEnv{content: tc.content}
			startNotifyApproveEnv(t, env)
			useReviewedRun(t, tc.stdin)
			code, stdout, stderr := run(t, "proposal", "approve", approveID, "--fingerprint", "fp-one", "--confirm", "--output", "json")
			for _, secret := range []string{proposalHookURL, proposalHookKey, "demo-proposal-secret-path"} {
				if strings.Contains(stdout+stderr+env.result.Message, secret) {
					t.Errorf("%q appears in the output or the result", secret)
				}
			}
			if !tc.ok {
				if code != 2 || strings.Contains(env.history(), "approve") {
					t.Fatalf("code %d, order %q, stderr %q", code, env.history(), stderr)
				}
				return
			}
			if code != 0 || env.added.Secret != proposalHookURL || env.added.Key != proposalHookKey || !env.result.Applied {
				t.Fatalf("code %d, added %+v, stderr %q", code, env.added, stderr)
			}
		})
	}
}

func TestReviewedNotifyChannelPreviewReadsNoSecret(t *testing.T) {
	useFolderHost(t)
	env := &notifyApproveEnv{content: webhookContent()}
	startNotifyApproveEnv(t, env)
	stdin := strings.NewReader(`{"secret":"` + proposalHookURL + `","key":"` + proposalHookKey + `"}`)
	useStdin(t, stdin)
	useControllingTerminal(t, true)
	code, _, stderr := run(t, "proposal", "approve", approveID, "--fingerprint", "fp-one")
	if code != 2 || stdin.Len() == 0 || env.history() != "show,list" {
		t.Fatalf("code %d, unread %d, order %q, stderr %q", code, stdin.Len(), env.history(), stderr)
	}
}

func TestReviewedApproveTakesNoSecretAsAnArgument(t *testing.T) {
	useFolderHost(t)
	env := &notifyApproveEnv{content: webhookContent()}
	startNotifyApproveEnv(t, env)
	for _, flag := range []string{"--secret", "--key", "--password"} {
		useReviewedRun(t, "")
		code, stdout, stderr := run(t, "proposal", "approve", approveID, "--fingerprint", "fp-one", "--confirm", flag, proposalHookKey)
		if code == 0 || env.history() != "" || strings.Contains(stdout+stderr, proposalHookKey) {
			t.Fatalf("%s: code %d, order %q, stdout %q, stderr %q", flag, code, env.history(), stdout, stderr)
		}
	}
}

func TestReviewedApproveNeedsAPersonRootAndItsFlags(t *testing.T) {
	d := useFolderHost(t)
	env := &approveEnv{dir: t.TempDir()}
	startApproveEnv(t, env)
	reviewed := []string{"proposal", "approve", approveID, "--fingerprint", "fp-one", "--confirm"}
	for name, tc := range map[string]struct {
		args        []string
		controlling bool
		want        string
	}{
		"agent":                     {append(append([]string{}, reviewed...), "--agent"), true, "not for an agent"},
		"no terminal at all":        {reviewed, false, "needs a terminal"},
		"confirm alone":             {[]string{"proposal", "approve", approveID, "--confirm"}, true, "--confirm belongs to --fingerprint"},
		"expect alone":              {[]string{"proposal", "approve", approveID, "--expect-effect", "sha256:x"}, true, "belong to --fingerprint"},
		"host key alone":            {[]string{"proposal", "approve", approveID, "--host-key-fingerprint", remoteKeyPrint}, true, "belong to --fingerprint"},
		"host key that is no print": {append(append([]string{}, reviewed...), "--host-key-fingerprint", "demo"), true, "must look like SHA256"},
	} {
		t.Run(name, func(t *testing.T) {
			useStdin(t, strings.NewReader(""))
			useControllingTerminal(t, tc.controlling)
			code, stdout, stderr := run(t, tc.args...)
			if code == 0 || stdout != "" || !strings.Contains(stderr, tc.want) {
				t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
			}
		})
	}
	old := folders.euid
	folders.euid = func() int { return 1000 }
	t.Cleanup(func() { folders.euid = old })
	useReviewedRun(t, "")
	if code, _, stderr := run(t, reviewed...); code == 0 || !strings.Contains(stderr, "needs root") {
		t.Fatalf("without root: code %d, stderr %q", code, stderr)
	}
	if env.history() != "" || d.dropIn(t) != "" {
		t.Fatalf("the service was asked: %q", env.history())
	}
}

func TestReviewedApproveRefusesTheFlagsOfSourceAddForOtherKinds(t *testing.T) {
	useFolderHost(t)
	env := &windowEnv{content: control.WindowSetArgs{Start: "01:30"}}
	startWindowEnv(t, env)
	for _, flag := range [][]string{{"--expect-effect", "sha256:x"}, {"--host-key-fingerprint", remoteKeyPrint},
		{"--expect-state", "empty"}, {"--url", remoteURL}} {
		useReviewedRun(t, "")
		args := append([]string{"proposal", "approve", approveID, "--fingerprint", "fp-one", "--confirm"}, flag...)
		code, _, stderr := run(t, args...)
		if code != 2 || !strings.Contains(stderr, "source-add only") || strings.Contains(env.history(), "approve") {
			t.Fatalf("%v: code %d, order %q, stderr %q", flag, code, env.history(), stderr)
		}
	}
}

func TestProposalRejectWithATerminalOfTheProcessPrintsJSON(t *testing.T) {
	useFolderHost(t)
	env := &approveEnv{dir: t.TempDir()}
	startApproveEnv(t, env)
	useReviewedRun(t, "")
	code, stdout, stderr := run(t, "proposal", "reject", approveID, "--reason", "wrong folder", "--output", "json")
	if code != 0 || env.rejected.ID != approveID || env.rejected.Reason != "wrong folder" {
		t.Fatalf("code %d, sent %+v, stderr %q", code, env.rejected, stderr)
	}
	if got := decodeObject(t, stdout); got["id"] != approveID || got["state"] != control.ProposalRejected {
		t.Fatalf("stdout %s", stdout)
	}
	useStdin(t, strings.NewReader(""))
	useControllingTerminal(t, false)
	if code, _, stderr := run(t, "proposal", "reject", approveID, "--output", "json"); code == 0 || !strings.Contains(stderr, "needs a terminal") {
		t.Fatalf("without any terminal: code %d, stderr %q", code, stderr)
	}
}
