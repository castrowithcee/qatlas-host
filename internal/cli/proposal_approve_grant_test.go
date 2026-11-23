package cli

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// grantEnv is a fake deploy service holding one grant proposal. refuse, when set, answers grant.add; uid is the
// user ID the name has today; replaced says an equal grant exists.
type grantEnv struct {
	mu       sync.Mutex
	order    []string
	content  control.ProposalGrant
	refuse   *control.Error
	uid      uint32
	replaced bool
	added    []control.GrantAddArgs
	result   control.ProposalResultArgs
}

func (e *grantEnv) note(s string) {
	e.mu.Lock()
	e.order = append(e.order, s)
	e.mu.Unlock()
}

func (e *grantEnv) history() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return strings.Join(e.order, ",")
}

func (e *grantEnv) confirmed() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.added)
}

func startGrantEnv(t *testing.T, env *grantEnv) {
	t.Helper()
	if env.content.User == "" {
		env.content = control.ProposalGrant{User: "demo-admin", UID: 4100, Action: control.GrantActionStackRestart,
			Stack: "web", Expires: "2999-12-31", Reason: "restart after the import"}
		env.uid = 4100
	}
	startFakeService(t, fakeHandlers{
		control.ProposalShowCommand: func(json.RawMessage) any {
			env.note("show")
			info := proposalInfo(approveID, control.ProposalOpen)
			info.Kind = control.ProposalKindGrant
			info.Fingerprint = "fp-one"
			content, _ := json.Marshal(env.content)
			return control.ProposalShowResult{ProposalInfo: info, Content: content}
		},
		control.StackSettingsGetCommand: func(json.RawMessage) any {
			env.note("settings")
			containers := true
			return control.StackSettingsResult{Stack: "web", Settings: &control.StackSettings{Version: 1,
				Backup: &control.StackBackupSettings{}, Check: &control.StackCheckSettings{Containers: &containers}}}
		},
		control.GrantAddCommand: func(raw json.RawMessage) any {
			var a control.GrantAddArgs
			_ = json.Unmarshal(raw, &a)
			if env.refuse != nil {
				env.note("add-refused")
				return env.refuse
			}
			env.mu.Lock()
			if a.Confirm {
				env.added = append(env.added, a)
			}
			env.mu.Unlock()
			env.note("add")
			return control.GrantAddResult{Grant: control.Grant{ID: "g-0123abcd", User: a.User, UID: env.uid, Action: a.Action,
				Stack: a.Stack, Expires: a.Expires}, Applied: a.Confirm, Replaced: env.replaced, Settings: true, Backup: true}
		},
		control.ProposalApproveCommand: func(json.RawMessage) any {
			env.note("approve")
			return proposalInfo(approveID, control.ProposalApproving)
		},
		control.ProposalResultCommand: func(raw json.RawMessage) any {
			env.note("result")
			_ = json.Unmarshal(raw, &env.result)
			return proposalInfo(approveID, control.ProposalApplied)
		},
	})
}

func approveGrantReviewed(t *testing.T, extra ...string) (int, string, string) {
	t.Helper()
	useReviewedRun(t, "")
	args := append([]string{"proposal", "approve", approveID, "--fingerprint", "fp-one", "--confirm", "--output", "json"}, extra...)
	return run(t, args...)
}

func TestGrantApproveAddsExactlyOneGrantAndMarksTheProposalApplied(t *testing.T) {
	useFolderHost(t)
	env := &grantEnv{}
	startGrantEnv(t, env)
	code, stdout, stderr := approveGrantReviewed(t)
	if code != 0 {
		t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if got, want := env.history(), "show,add,show,add,approve,add,result"; got != want {
		t.Fatalf("order %q, want %q", got, want)
	}
	a := env.added[0]
	if env.confirmed() != 1 || a.User != "demo-admin" || a.Action != "stack.restart" || a.Stack != "web" || a.Expires != "2999-12-31" || !env.result.Applied {
		t.Fatalf("added %+v, result %+v", env.added, env.result)
	}
	got := decodeObject(t, stdout)
	if got["proposal"] != approveID || got["id"] != "g-0123abcd" || got["user"] != "demo-admin" || got["replaced"] != false {
		t.Fatalf("result %v", got)
	}
	if _, ok := got["requested_expires"]; ok {
		t.Fatalf("an unchanged expiry is named: %v", got)
	}
}

func TestGrantApproveWithoutConfirmShowsThePlanAndAppliesNothing(t *testing.T) {
	useFolderHost(t)
	env := &grantEnv{}
	startGrantEnv(t, env)
	useReviewedRun(t, "")
	code, stdout, stderr := run(t, "proposal", "approve", approveID, "--fingerprint", "fp-one")
	if code != 2 || stdout != "" || env.confirmed() != 0 || strings.Contains(env.history(), "approve") {
		t.Fatalf("code %d, order %q, stdout %q, stderr %q", code, env.history(), stdout, stderr)
	}
	for _, want := range []string{"demo-admin", "stack.restart", "web", "2999-12-31", "backup: set up", "restart after the import"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr lacks %q:\n%s", want, stderr)
		}
	}
}

func TestGrantApproveRefusals(t *testing.T) {
	for _, tc := range []struct {
		name  string
		env   *grantEnv
		extra []string
		want  string
	}{
		{"stack gone", &grantEnv{refuse: control.NewError(control.CodeNoSuchStack, "this instance has no such stack")}, nil, "no such stack"},
		{"user without access", &grantEnv{refuse: control.NewError(control.CodeUserNoAccess, "grants are for users with access to the service only")}, nil, "users with access"},
		{"name has another uid", &grantEnv{uid: 4999, content: control.ProposalGrant{User: "demo-admin", UID: 4100, Action: "stack.restart", Stack: "web", Reason: "r"}}, nil, "another user ID"},
		{"equal grant exists", &grantEnv{replaced: true}, nil, "would replace it"},
		{"wrong fingerprint", &grantEnv{}, []string{"--fingerprint", "fp-other"}, "not the one that was shown"},
		{"later expiry", &grantEnv{}, []string{"--expires", "3000-01-01"}, "later than the requested"},
		{"bad expiry", &grantEnv{}, []string{"--expires", "tomorrow"}, "YYYY-MM-DD"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			useFolderHost(t)
			startGrantEnv(t, tc.env)
			useReviewedRun(t, "")
			args := []string{"proposal", "approve", approveID, "--fingerprint", "fp-one", "--confirm"}
			code, stdout, stderr := run(t, append(args, tc.extra...)...)
			if code == 0 || stdout != "" || !strings.Contains(stderr, tc.want) {
				t.Fatalf("code %d, stdout %q, stderr %q, want %q", code, stdout, stderr, tc.want)
			}
			if tc.env.confirmed() != 0 || strings.Contains(tc.env.history(), "approve") {
				t.Fatalf("something was applied: %q", tc.env.history())
			}
		})
	}
}

func TestGrantApproveNeedsRoot(t *testing.T) {
	useFolderHost(t)
	env := &grantEnv{}
	startGrantEnv(t, env)
	folders.euid = func() int { return 1000 }
	code, _, stderr := approveGrantReviewed(t)
	if code == 0 || !strings.Contains(stderr, "needs root") || env.history() != "" {
		t.Fatalf("code %d, order %q, stderr %q", code, env.history(), stderr)
	}
}

func TestGrantApproveTakesAnEarlierExpiryAndNamesIt(t *testing.T) {
	useFolderHost(t)
	env := &grantEnv{}
	startGrantEnv(t, env)
	code, stdout, stderr := approveGrantReviewed(t, "--expires", "2999-06-30")
	if code != 0 || env.confirmed() != 1 || env.added[0].Expires != "2999-06-30" {
		t.Fatalf("code %d, added %+v, stderr %q", code, env.added, stderr)
	}
	if got := decodeObject(t, stdout); got["expires"] != "2999-06-30" || got["requested_expires"] != "2999-12-31" {
		t.Fatalf("result %v", got)
	}
	if !strings.Contains(stderr, "You set the expiry to 2999-06-30") {
		t.Errorf("the changed expiry is not shown:\n%s", stderr)
	}
}

func TestGrantApproveWithoutRequestedExpiryTakesAnyDate(t *testing.T) {
	useFolderHost(t)
	env := &grantEnv{content: control.ProposalGrant{User: "demo-admin", UID: 4100, Action: "stack.restart", Stack: "web", Reason: "r"}, uid: 4100}
	startGrantEnv(t, env)
	code, _, stderr := approveGrantReviewed(t, "--expires", "3000-01-01")
	if code != 0 || env.confirmed() != 1 || env.added[0].Expires != "3000-01-01" {
		t.Fatalf("code %d, added %+v, stderr %q", code, env.added, stderr)
	}
}

func TestExpiresIsRefusedForAnotherKind(t *testing.T) {
	useFolderHost(t)
	env := &windowEnv{content: control.WindowSetArgs{Days: []string{"sat"}}}
	startWindowEnv(t, env)
	useReviewedRun(t, "")
	code, _, stderr := run(t, "proposal", "approve", approveID, "--fingerprint", "fp-one", "--confirm", "--expires", "2999-01-01")
	if code == 0 || !strings.Contains(stderr, "kind grant only") || strings.Contains(env.history(), "approve") {
		t.Fatalf("code %d, order %q, stderr %q", code, env.history(), stderr)
	}
}

func TestProposalShowMasksTheReasonOfAGrantAndShowsBackupAndCheck(t *testing.T) {
	useFolderHost(t)
	env := &grantEnv{content: control.ProposalGrant{User: "demo-admin", UID: 4100, Action: "stack.restart", Stack: "web",
		Reason: "ok\x1b[2J\u202e now"}, uid: 4100}
	startGrantEnv(t, env)
	code, stdout, stderr := run(t, "proposal", "show", approveID)
	if code != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if strings.ContainsAny(stdout, "\x1b\u202e") {
		t.Fatalf("stdout holds a control character: %q", stdout)
	}
	for _, want := range []string{"untrusted_reason", `"ok`, "demo-admin", "stack.restart", "web", "backup", "set up"} {
		if !strings.Contains(strings.ToLower(stdout), want) {
			t.Errorf("stdout lacks %q:\n%s", want, stdout)
		}
	}
}
