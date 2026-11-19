package proposals

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

const (
	admin       = 4100
	other       = 4101
	kindExample = "example"
	okContent   = `{"name":"example-one"}`
	passwdFile  = "root:x:0:0:root:/root:/bin/sh\nexample-admin:x:4100:4100::/nonexistent:/usr/sbin/nologin\n"
)

// normalizeExample is the check of the only kind the tests know: an object with the field name.
func normalizeExample(kind string, content json.RawMessage) (any, *control.Error) {
	if kind != kindExample {
		return nil, Invalid("unknown proposal kind %q", Printable(kind))
	}
	var c struct {
		Name string `json:"name"`
	}
	if err := DecodeStrict(content, &c); err != nil {
		return nil, Invalid("the content must be an object with the field name only")
	}
	return c, nil
}

type harness struct {
	t   *testing.T
	p   *Store
	dir string
	now time.Time
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	dir := t.TempDir()
	passwd := filepath.Join(dir, "passwd")
	if err := os.WriteFile(passwd, []byte(passwdFile), 0o600); err != nil {
		t.Fatal(err)
	}
	h := &harness{t: t, now: time.Date(2030, 1, 10, 12, 0, 0, 0, time.UTC)}
	h.p = New(filepath.Join(dir, "state"), passwd, func() time.Time { return h.now }, slog.New(slog.DiscardHandler), normalizeExample)
	h.dir = h.p.dir
	return h
}

func (h *harness) submit(uid uint32, kind, content string) (control.ProposalInfo, error) {
	h.t.Helper()
	raw, _ := json.Marshal(control.ProposalSubmitArgs{Kind: kind, Content: json.RawMessage(content)})
	v, err := h.p.Submit(control.WithPeer(context.Background(), uid), raw)
	if err != nil {
		return control.ProposalInfo{}, err
	}
	return v.(control.ProposalInfo), nil
}

func (h *harness) withdraw(uid uint32, id string) error {
	raw, _ := json.Marshal(control.ProposalIDArgs{ID: id})
	_, err := h.p.Withdraw(control.WithPeer(context.Background(), uid), raw)
	return err
}

func (h *harness) show(id string) (control.ProposalShowResult, error) {
	raw, _ := json.Marshal(control.ProposalIDArgs{ID: id})
	v, err := h.p.Show(context.Background(), raw)
	if err != nil {
		return control.ProposalShowResult{}, err
	}
	return v.(control.ProposalShowResult), nil
}

func codeOf(t *testing.T, err error) string {
	t.Helper()
	var ce *control.Error
	if !errors.As(err, &ce) {
		t.Fatalf("error %v has no fixed answer", err)
	}
	return ce.Code
}

func TestSubmitStoresAndShows(t *testing.T) {
	h := newHarness(t)
	info, err := h.submit(admin, kindExample, okContent)
	if err != nil {
		t.Fatal(err)
	}
	if info.State != control.ProposalOpen || info.SubmitterUID != admin || info.SubmitterName != "example-admin" ||
		len(info.Fingerprint) != 64 || !info.Submitted.Equal(h.now) || !info.Expires.Equal(h.now.Add(14*24*time.Hour)) {
		t.Fatalf("info = %+v", info)
	}
	shown, err := h.show(info.ID)
	if err != nil || shown.ProposalInfo != info || string(shown.Content) != okContent {
		t.Fatalf("show = %+v, %v", shown, err)
	}
	// The same content of another submitter has the same fingerprint; another content has another one.
	again, _ := h.submit(other, kindExample, okContent)
	diff, _ := h.submit(other, kindExample, `{"name":"example-two"}`)
	if again.Fingerprint != info.Fingerprint || diff.Fingerprint == info.Fingerprint {
		t.Fatal("the fingerprint does not follow the content")
	}
	if again.SubmitterName != "uid-4101" {
		t.Fatalf("unknown submitter named %q", again.SubmitterName)
	}
	v, err := h.p.List(context.Background(), nil)
	if err != nil || len(v.(control.ProposalListResult).Proposals) != 3 {
		t.Fatalf("list = %+v, %v", v, err)
	}
	// The files are private.
	for path, want := range map[string]fs.FileMode{h.dir: 0o700, h.p.path(info.ID): 0o600} {
		if st, err := os.Stat(path); err != nil || st.Mode().Perm() != want {
			t.Fatalf("%s: %v, %v", path, st, err)
		}
	}
	if entries, _ := os.ReadDir(h.dir); len(entries) != 3 {
		t.Fatalf("leftover files: %v", entries)
	}
}

func TestSubmitRefusals(t *testing.T) {
	h := newHarness(t)
	for name, tc := range map[string]struct{ kind, content string }{
		"unknown field": {kindExample, `{"name":"x","command":"true"}`},
		"unknown kind":  {"run-command", `{"name":"x"}`},
		"trailing data": {kindExample, `{"name":"x"} {}`},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := h.submit(admin, tc.kind, tc.content); codeOf(t, err) != control.CodeInvalid {
				t.Fatalf("err = %v", err)
			}
		})
	}
	// An unknown field of the request itself is refused as well.
	if _, err := h.p.Submit(control.WithPeer(context.Background(), admin),
		json.RawMessage(`{"kind":"example","content":{"name":"x"},"extra":1}`)); codeOf(t, err) != control.CodeBadRequest {
		t.Fatalf("unknown request field = %v", err)
	}
	if _, err := os.Stat(h.dir); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("a refused proposal left a store behind: %v", err)
	}
}

func (h *harness) approve(id, fingerprint string) error {
	raw, _ := json.Marshal(control.ProposalApproveArgs{ID: id, Fingerprint: fingerprint})
	_, err := h.p.Approve(context.Background(), raw)
	return err
}

func (h *harness) reject(id, reason string) error {
	raw, _ := json.Marshal(control.ProposalRejectArgs{ID: id, Reason: reason})
	_, err := h.p.Reject(context.Background(), raw)
	return err
}

func (h *harness) result(id, fingerprint string, applied bool, message string) error {
	raw, _ := json.Marshal(control.ProposalResultArgs{ID: id, Fingerprint: fingerprint, Applied: applied, Message: message})
	_, err := h.p.Result(context.Background(), raw)
	return err
}

func (h *harness) state(id string) control.ProposalShowResult {
	h.t.Helper()
	got, err := h.show(id)
	if err != nil {
		h.t.Fatal(err)
	}
	return got
}

func TestSubmitSizeLimit(t *testing.T) {
	h := newHarness(t)
	big := `{"name":"` + strings.Repeat("a", control.ProposalMaxBytes) + `"}`
	if _, err := h.submit(admin, kindExample, big); codeOf(t, err) != control.CodeTooLarge {
		t.Fatalf("oversized = %v", err)
	}
	// Garbage of that size is refused for its size before it is decoded.
	if _, err := h.p.Submit(control.WithPeer(context.Background(), admin),
		json.RawMessage(strings.Repeat("{", control.ProposalMaxBytes+1))); codeOf(t, err) != control.CodeTooLarge {
		t.Fatalf("oversized garbage = %v", err)
	}
}

func TestOpenLimitIsPerSubmitter(t *testing.T) {
	h := newHarness(t)
	var first control.ProposalInfo
	for i := 0; i < control.ProposalMaxOpen; i++ {
		info, err := h.submit(admin, kindExample, okContent)
		if err != nil {
			t.Fatalf("proposal %d: %v", i+1, err)
		}
		if i == 0 {
			first = info
		}
	}
	if _, err := h.submit(admin, kindExample, okContent); codeOf(t, err) != control.CodeProposalLimit {
		t.Fatalf("21st = %v", err)
	}
	if _, err := h.submit(other, kindExample, okContent); err != nil {
		t.Fatalf("another submitter = %v", err)
	}
	// Withdrawing one makes room; an expired one does not count.
	if err := h.withdraw(admin, first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.submit(admin, kindExample, okContent); err != nil {
		t.Fatalf("after a withdrawal = %v", err)
	}
	h.now = h.now.Add(control.ProposalLifetime)
	if _, err := h.submit(admin, kindExample, okContent); err != nil {
		t.Fatalf("after expiry = %v", err)
	}
}

func TestWithdraw(t *testing.T) {
	h := newHarness(t)
	info, _ := h.submit(admin, kindExample, okContent)
	if err := h.withdraw(other, info.ID); codeOf(t, err) != control.CodeNotSubmitter {
		t.Fatalf("withdraw of another user's proposal = %v", err)
	}
	if got, _ := h.show(info.ID); got.State != control.ProposalOpen {
		t.Fatalf("state after a refused withdrawal = %s", got.State)
	}
	if err := h.withdraw(admin, info.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := h.show(info.ID); got.State != control.ProposalWithdrawn {
		t.Fatalf("state = %s", got.State)
	}
	if err := h.withdraw(admin, info.ID); codeOf(t, err) != control.CodeInvalid {
		t.Fatalf("second withdrawal = %v", err)
	}
	rootInfo, _ := h.submit(admin, kindExample, okContent)
	if err := h.withdraw(0, rootInfo.ID); err != nil {
		t.Fatalf("root withdrawal = %v", err)
	}
	for _, id := range []string{"p-0000000000000000", "../x", "", strings.Repeat("a", 5000)} {
		if err := h.withdraw(0, id); codeOf(t, err) != control.CodeNotFound {
			t.Fatalf("withdraw %.20q = %v", id, err)
		}
	}
}

func TestExpiredProposalShowsAsExpiredAndCannotBeApplied(t *testing.T) {
	h := newHarness(t)
	info, _ := h.submit(admin, kindExample, okContent)
	h.now = info.Submitted.Add(control.ProposalLifetime - time.Second)
	if got, _ := h.show(info.ID); got.State != control.ProposalOpen {
		t.Fatalf("just before the end = %s", got.State)
	}
	h.now = info.Submitted.Add(control.ProposalLifetime)
	got, err := h.show(info.ID)
	if err != nil || got.State != control.ProposalExpired {
		t.Fatalf("expired = %+v, %v", got, err)
	}
	if err := h.approve(info.ID, info.Fingerprint); codeOf(t, err) != control.CodeInvalid {
		t.Fatalf("approve an expired proposal = %v", err)
	}
	if err := h.reject(info.ID, ""); codeOf(t, err) != control.CodeInvalid {
		t.Fatalf("reject an expired proposal = %v", err)
	}
	if err := h.withdraw(admin, info.ID); codeOf(t, err) != control.CodeInvalid {
		t.Fatalf("withdraw an expired proposal = %v", err)
	}
	if got, _ := h.show(info.ID); got.State != control.ProposalExpired {
		t.Fatalf("state = %s", got.State)
	}
}

func TestApproveChecksTheFingerprintFirst(t *testing.T) {
	h := newHarness(t)
	a, _ := h.submit(admin, kindExample, okContent)
	for _, fp := range []string{"", "0000", strings.ToUpper(a.Fingerprint)} {
		if err := h.approve(a.ID, fp); codeOf(t, err) != control.CodeProposalChanged {
			t.Fatalf("approve with %q = %v", fp, err)
		}
	}
	if got := h.state(a.ID); got.State != control.ProposalOpen {
		t.Fatalf("a wrong fingerprint changed the state to %s", got.State)
	}
	if err := h.approve("p-0000000000000000", a.Fingerprint); codeOf(t, err) != control.CodeNotFound {
		t.Fatalf("unknown id = %v", err)
	}
	if err := h.approve(a.ID, a.Fingerprint); err != nil {
		t.Fatal(err)
	}
	if got := h.state(a.ID); got.State != control.ProposalApproving {
		t.Fatalf("state = %s", got.State)
	}
}

func TestApprovalLocksTheProposal(t *testing.T) {
	h := newHarness(t)
	a, _ := h.submit(admin, kindExample, okContent)
	if err := h.approve(a.ID, a.Fingerprint); err != nil {
		t.Fatal(err)
	}
	if err := h.approve(a.ID, a.Fingerprint); codeOf(t, err) != control.CodeInvalid {
		t.Fatalf("second approval = %v", err)
	}
	if err := h.withdraw(admin, a.ID); codeOf(t, err) != control.CodeInvalid {
		t.Fatalf("withdraw while approving = %v", err)
	}
	if err := h.reject(a.ID, "no"); codeOf(t, err) != control.CodeInvalid {
		t.Fatalf("reject while approving = %v", err)
	}
	if err := h.result(a.ID, "0000", true, ""); codeOf(t, err) != control.CodeProposalChanged {
		t.Fatalf("result with a wrong fingerprint = %v", err)
	}
	if err := h.result(a.ID, a.Fingerprint, true, "ignored"); err != nil {
		t.Fatal(err)
	}
	if got := h.state(a.ID); got.State != control.ProposalApplied || got.Message != "" {
		t.Fatalf("state = %+v", got)
	}
	if err := h.result(a.ID, a.Fingerprint, false, "late"); codeOf(t, err) != control.CodeInvalid {
		t.Fatalf("second result = %v", err)
	}
}

func TestResultNeedsAnApproval(t *testing.T) {
	h := newHarness(t)
	a, _ := h.submit(admin, kindExample, okContent)
	if err := h.result(a.ID, a.Fingerprint, true, ""); codeOf(t, err) != control.CodeInvalid {
		t.Fatalf("result of an open proposal = %v", err)
	}
	if got := h.state(a.ID); got.State != control.ProposalOpen {
		t.Fatalf("state = %s", got.State)
	}
}

func TestFailedResultKeepsAReadableMessage(t *testing.T) {
	h := newHarness(t)
	a, _ := h.submit(admin, kindExample, okContent)
	if err := h.approve(a.ID, a.Fingerprint); err != nil {
		t.Fatal(err)
	}
	msg := "clone failed\x1b[2J\n" + strings.Repeat("x", 2000)
	if err := h.result(a.ID, a.Fingerprint, false, msg); err != nil {
		t.Fatal(err)
	}
	got := h.state(a.ID)
	if got.State != control.ProposalFailed || !strings.HasPrefix(got.Message, "clone failed [2J xxx") ||
		strings.ContainsRune(got.Message, 0x1b) || len([]rune(got.Message)) > 303 {
		t.Fatalf("state %s, message %q", got.State, got.Message)
	}
	b, _ := h.submit(admin, kindExample, okContent)
	_ = h.approve(b.ID, b.Fingerprint)
	_ = h.result(b.ID, b.Fingerprint, false, "")
	if got := h.state(b.ID); got.State != control.ProposalFailed || got.Message == "" {
		t.Fatalf("an empty failure message = %+v", got)
	}
}

func TestRejectKeepsTheReasonForTheSubmitter(t *testing.T) {
	h := newHarness(t)
	a, _ := h.submit(admin, kindExample, okContent)
	if err := h.reject(a.ID, "wrong folder\x1b]0;x\x07\n"+strings.Repeat("y", 1000)); err != nil {
		t.Fatal(err)
	}
	got := h.state(a.ID)
	if got.State != control.ProposalRejected || !strings.HasPrefix(got.Message, "wrong folder ]0;x yyy") ||
		strings.ContainsAny(got.Message, "\x1b\x07\n") || len([]rune(got.Message)) > 303 {
		t.Fatalf("state %s, message %q", got.State, got.Message)
	}
	b, _ := h.submit(admin, kindExample, okContent)
	if err := h.reject(b.ID, ""); err != nil {
		t.Fatal(err)
	}
	if got := h.state(b.ID); got.State != control.ProposalRejected || got.Message != "" {
		t.Fatalf("no reason = %+v", got)
	}
	if err := h.reject(b.ID, "again"); codeOf(t, err) != control.CodeInvalid {
		t.Fatalf("reject twice = %v", err)
	}
	if err := h.approve(b.ID, b.Fingerprint); codeOf(t, err) != control.CodeInvalid {
		t.Fatalf("approve a rejected proposal = %v", err)
	}
	if err := h.withdraw(admin, b.ID); codeOf(t, err) != control.CodeInvalid {
		t.Fatalf("withdraw of a rejected proposal = %v", err)
	}
}

func TestApprovalThatNeverFinishesDoesNotBlock(t *testing.T) {
	h := newHarness(t)
	a, _ := h.submit(admin, kindExample, okContent)
	_ = h.approve(a.ID, a.Fingerprint)
	h.now = h.now.Add(control.ProposalApprovalWindow - time.Second)
	if got := h.state(a.ID); got.State != control.ProposalApproving {
		t.Fatalf("inside the window = %s", got.State)
	}
	h.now = h.now.Add(time.Second)
	got := h.state(a.ID)
	if got.State != control.ProposalFailed || got.Message == "" {
		t.Fatalf("after the window = %+v", got)
	}
	if err := h.approve(a.ID, a.Fingerprint); codeOf(t, err) != control.CodeInvalid {
		t.Fatalf("approve after the window = %v", err)
	}
	// The outcome that happened is still recorded, and the submitter may send a new proposal.
	if err := h.result(a.ID, a.Fingerprint, true, ""); err != nil {
		t.Fatal(err)
	}
	if got := h.state(a.ID); got.State != control.ProposalApplied {
		t.Fatalf("late result = %s", got.State)
	}
	if _, err := h.submit(admin, kindExample, okContent); err != nil {
		t.Fatal(err)
	}
}

func TestApprovingDoesNotExpire(t *testing.T) {
	h := newHarness(t)
	a, _ := h.submit(admin, kindExample, okContent)
	h.now = a.Expires.Add(-time.Minute)
	_ = h.approve(a.ID, a.Fingerprint)
	h.now = a.Expires.Add(time.Minute)
	if got := h.state(a.ID); got.State == control.ProposalExpired {
		t.Fatalf("state = %s", got.State)
	}
}

func TestIDPrefixNamesTheStore(t *testing.T) {
	h := newHarness(t)
	info, err := h.submit(admin, kindExample, okContent)
	if err != nil || !strings.HasPrefix(info.ID, control.DeployProposalIDPrefix) {
		t.Fatalf("default id %q, %v", info.ID, err)
	}
	host := New(filepath.Join(t.TempDir(), "state"), "", func() time.Time { return h.now }, slog.New(slog.DiscardHandler),
		normalizeExample, IDPrefix(control.HostProposalIDPrefix))
	raw, _ := json.Marshal(control.ProposalSubmitArgs{Kind: kindExample, Content: json.RawMessage(okContent)})
	v, err := host.Submit(control.WithPeer(context.Background(), admin), raw)
	if err != nil {
		t.Fatal(err)
	}
	id := v.(control.ProposalInfo).ID
	if !strings.HasPrefix(id, control.HostProposalIDPrefix) || len(id) != len(control.HostProposalIDPrefix)+16 {
		t.Fatalf("host id %q", id)
	}
	// Each store finds only the ids of its own form.
	for store, want := range map[*Store]string{host: info.ID, h.p: id} {
		raw, _ := json.Marshal(control.ProposalIDArgs{ID: want})
		if _, err := store.Show(context.Background(), raw); codeOf(t, err) != control.CodeNotFound {
			t.Errorf("a store found the id %s of the other: %v", want, err)
		}
	}
	defer func() {
		if recover() == nil {
			t.Error("an invalid prefix was accepted")
		}
	}()
	IDPrefix("H_")
}
