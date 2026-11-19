package service

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

type proposalHarness struct {
	t   *testing.T
	p   *proposalService
	now time.Time
}

func newProposalHarness(t *testing.T) *proposalHarness {
	t.Helper()
	dir := t.TempDir()
	passwd := filepath.Join(dir, "passwd")
	if err := os.WriteFile(passwd, []byte(inventedUsers), 0o600); err != nil {
		t.Fatal(err)
	}
	h := &proposalHarness{t: t, now: time.Date(2030, 1, 10, 12, 0, 0, 0, time.UTC)}
	h.p = newProposalService(filepath.Join(dir, "state"), passwd, func() time.Time { return h.now }, slog.New(slog.DiscardHandler))
	return h
}

func (h *proposalHarness) submit(uid uint32, kind, content string) (control.ProposalInfo, error) {
	h.t.Helper()
	raw, _ := json.Marshal(control.ProposalSubmitArgs{Kind: kind, Content: json.RawMessage(content)})
	v, err := h.p.handleSubmit(control.WithPeer(context.Background(), uid), raw)
	if err != nil {
		return control.ProposalInfo{}, err
	}
	return v.(control.ProposalInfo), nil
}

func (h *proposalHarness) withdraw(uid uint32, id string) error {
	raw, _ := json.Marshal(control.ProposalIDArgs{ID: id})
	_, err := h.p.handleWithdraw(control.WithPeer(context.Background(), uid), raw)
	return err
}

func (h *proposalHarness) show(id string) (control.ProposalShowResult, error) {
	raw, _ := json.Marshal(control.ProposalIDArgs{ID: id})
	v, err := h.p.handleShow(context.Background(), raw)
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

const (
	admin = 4100
	other = 4101
	okSrc = `{"path":"/srv/example-stacks","url":"ssh://git.example.test/team/stacks.git"}`
)

func TestProposalSubmitStoresAndShows(t *testing.T) {
	h := newProposalHarness(t)
	info, err := h.submit(admin, control.ProposalKindSourceAdd, okSrc)
	if err != nil {
		t.Fatal(err)
	}
	if info.State != control.ProposalOpen || info.SubmitterUID != admin || info.SubmitterName != "example-admin" ||
		len(info.Fingerprint) != 64 || !info.Submitted.Equal(h.now) || !info.Expires.Equal(h.now.Add(14*24*time.Hour)) {
		t.Fatalf("info = %+v", info)
	}
	shown, err := h.show(info.ID)
	if err != nil || shown.ProposalInfo != info || string(shown.Content) != okSrc {
		t.Fatalf("show = %+v, %v", shown, err)
	}
	// The same content of another submitter has the same fingerprint; another content has another one.
	again, _ := h.submit(other, control.ProposalKindSourceAdd, okSrc)
	diff, _ := h.submit(other, control.ProposalKindSourceAdd, `{"path":"/srv/example-other"}`)
	if again.Fingerprint != info.Fingerprint || diff.Fingerprint == info.Fingerprint {
		t.Fatal("the fingerprint does not follow the content")
	}
	v, err := h.p.handleList(context.Background(), nil)
	if err != nil || len(v.(control.ProposalListResult).Proposals) != 3 {
		t.Fatalf("list = %+v, %v", v, err)
	}
	// The files are private.
	for path, want := range map[string]fs.FileMode{h.p.dir: 0o700, h.p.path(info.ID): 0o600} {
		if st, err := os.Stat(path); err != nil || st.Mode().Perm() != want {
			t.Fatalf("%s: %v, %v", path, st, err)
		}
	}
	if entries, _ := os.ReadDir(h.p.dir); len(entries) != 3 {
		t.Fatalf("leftover files: %v", entries)
	}
}

func TestProposalSubmitRefusals(t *testing.T) {
	h := newProposalHarness(t)
	for name, tc := range map[string]struct{ kind, content string }{
		"unknown field":      {control.ProposalKindSourceAdd, `{"path":"/srv/example-stacks","command":"true"}`},
		"secret field":       {control.ProposalKindSourceAdd, `{"path":"/srv/example-stacks","token":"example"}`},
		"host key field":     {control.ProposalKindSourceAdd, `{"path":"/srv/example-stacks","fingerprint":"SHA256:example"}`},
		"unknown kind":       {"run-command", `{"path":"/srv/example-stacks"}`},
		"relative path":      {control.ProposalKindSourceAdd, `{"path":"example-stacks"}`},
		"unclean path":       {control.ProposalKindSourceAdd, `{"path":"/srv/../etc"}`},
		"path with control":  {control.ProposalKindSourceAdd, `{"path":"/srv/a\u001b[2Jb"}`},
		"empty path":         {control.ProposalKindSourceAdd, `{}`},
		"null content":       {control.ProposalKindSourceAdd, `null`},
		"not an object":      {control.ProposalKindSourceAdd, `"/srv/example-stacks"`},
		"trailing data":      {control.ProposalKindSourceAdd, `{"path":"/srv/example-stacks"} {}`},
		"local url":          {control.ProposalKindSourceAdd, `{"path":"/srv/example-stacks","url":"/srv/repo"}`},
		"option url":         {control.ProposalKindSourceAdd, `{"path":"/srv/example-stacks","url":"-oProxyCommand=x"}`},
		"https url":          {control.ProposalKindSourceAdd, `{"path":"/srv/example-stacks","url":"https://git.example.test/a.git"}`},
		"kind with controls": {"a\u001b[31mb", `{}`},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := h.submit(admin, tc.kind, tc.content)
			if codeOf(t, err) != control.CodeInvalid {
				t.Fatalf("err = %v", err)
			}
			if strings.ContainsRune(err.Error(), 0x1b) {
				t.Fatalf("error holds a control character: %q", err)
			}
		})
	}
	// An unknown field of the request itself is refused as well.
	if _, err := h.p.handleSubmit(control.WithPeer(context.Background(), admin),
		json.RawMessage(`{"kind":"source-add","content":{"path":"/srv/x"},"extra":1}`)); codeOf(t, err) != control.CodeBadRequest {
		t.Fatalf("unknown request field = %v", err)
	}
	if _, err := os.Stat(h.p.dir); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("a refused proposal left a store behind: %v", err)
	}
}

func TestProposalSubmitSizeLimit(t *testing.T) {
	h := newProposalHarness(t)
	big := `{"path":"/srv/` + strings.Repeat("a", control.ProposalMaxBytes) + `"}`
	if _, err := h.submit(admin, control.ProposalKindSourceAdd, big); codeOf(t, err) != control.CodeTooLarge {
		t.Fatalf("oversized = %v", err)
	}
	// Garbage of that size is refused for its size before it is decoded.
	if _, err := h.p.handleSubmit(control.WithPeer(context.Background(), admin),
		json.RawMessage(strings.Repeat("{", control.ProposalMaxBytes+1))); codeOf(t, err) != control.CodeTooLarge {
		t.Fatalf("oversized garbage = %v", err)
	}
}

func TestProposalOpenLimitIsPerSubmitter(t *testing.T) {
	h := newProposalHarness(t)
	var first control.ProposalInfo
	for i := 0; i < control.ProposalMaxOpen; i++ {
		info, err := h.submit(admin, control.ProposalKindSourceAdd, okSrc)
		if err != nil {
			t.Fatalf("proposal %d: %v", i+1, err)
		}
		if i == 0 {
			first = info
		}
	}
	if _, err := h.submit(admin, control.ProposalKindSourceAdd, okSrc); codeOf(t, err) != control.CodeProposalLimit {
		t.Fatalf("21st = %v", err)
	}
	if _, err := h.submit(other, control.ProposalKindSourceAdd, okSrc); err != nil {
		t.Fatalf("another submitter = %v", err)
	}
	// Withdrawing one makes room; an expired one does not count.
	if err := h.withdraw(admin, first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.submit(admin, control.ProposalKindSourceAdd, okSrc); err != nil {
		t.Fatalf("after a withdrawal = %v", err)
	}
	h.now = h.now.Add(control.ProposalLifetime)
	if _, err := h.submit(admin, control.ProposalKindSourceAdd, okSrc); err != nil {
		t.Fatalf("after expiry = %v", err)
	}
}

func TestProposalWithdraw(t *testing.T) {
	h := newProposalHarness(t)
	info, _ := h.submit(admin, control.ProposalKindSourceAdd, okSrc)
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
	rootInfo, _ := h.submit(admin, control.ProposalKindSourceAdd, okSrc)
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
	h := newProposalHarness(t)
	info, _ := h.submit(admin, control.ProposalKindSourceAdd, okSrc)
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

func (h *proposalHarness) approve(id, fingerprint string) error {
	raw, _ := json.Marshal(control.ProposalApproveArgs{ID: id, Fingerprint: fingerprint})
	_, err := h.p.handleApprove(context.Background(), raw)
	return err
}

func (h *proposalHarness) reject(id, reason string) error {
	raw, _ := json.Marshal(control.ProposalRejectArgs{ID: id, Reason: reason})
	_, err := h.p.handleReject(context.Background(), raw)
	return err
}

func (h *proposalHarness) result(id, fingerprint string, applied bool, message string) error {
	raw, _ := json.Marshal(control.ProposalResultArgs{ID: id, Fingerprint: fingerprint, Applied: applied, Message: message})
	_, err := h.p.handleResult(context.Background(), raw)
	return err
}

func (h *proposalHarness) state(id string) control.ProposalShowResult {
	h.t.Helper()
	got, err := h.show(id)
	if err != nil {
		h.t.Fatal(err)
	}
	return got
}

func TestProposalApproveChecksTheFingerprintFirst(t *testing.T) {
	h := newProposalHarness(t)
	a, _ := h.submit(admin, control.ProposalKindSourceAdd, okSrc)
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

func TestProposalApprovalLocksTheProposal(t *testing.T) {
	h := newProposalHarness(t)
	a, _ := h.submit(admin, control.ProposalKindSourceAdd, okSrc)
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

func TestProposalResultNeedsAnApproval(t *testing.T) {
	h := newProposalHarness(t)
	a, _ := h.submit(admin, control.ProposalKindSourceAdd, okSrc)
	if err := h.result(a.ID, a.Fingerprint, true, ""); codeOf(t, err) != control.CodeInvalid {
		t.Fatalf("result of an open proposal = %v", err)
	}
	if got := h.state(a.ID); got.State != control.ProposalOpen {
		t.Fatalf("state = %s", got.State)
	}
}

func TestProposalFailedResultKeepsAReadableMessage(t *testing.T) {
	h := newProposalHarness(t)
	a, _ := h.submit(admin, control.ProposalKindSourceAdd, okSrc)
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
	b, _ := h.submit(admin, control.ProposalKindSourceAdd, okSrc)
	_ = h.approve(b.ID, b.Fingerprint)
	_ = h.result(b.ID, b.Fingerprint, false, "")
	if got := h.state(b.ID); got.State != control.ProposalFailed || got.Message == "" {
		t.Fatalf("an empty failure message = %+v", got)
	}
}

func TestProposalRejectKeepsTheReasonForTheSubmitter(t *testing.T) {
	h := newProposalHarness(t)
	a, _ := h.submit(admin, control.ProposalKindSourceAdd, okSrc)
	if err := h.reject(a.ID, "wrong folder\x1b]0;x\x07\n"+strings.Repeat("y", 1000)); err != nil {
		t.Fatal(err)
	}
	got := h.state(a.ID)
	if got.State != control.ProposalRejected || !strings.HasPrefix(got.Message, "wrong folder ]0;x yyy") ||
		strings.ContainsAny(got.Message, "\x1b\x07\n") || len([]rune(got.Message)) > 303 {
		t.Fatalf("state %s, message %q", got.State, got.Message)
	}
	b, _ := h.submit(admin, control.ProposalKindSourceAdd, okSrc)
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

func TestProposalApprovalThatNeverFinishesDoesNotBlock(t *testing.T) {
	h := newProposalHarness(t)
	a, _ := h.submit(admin, control.ProposalKindSourceAdd, okSrc)
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
	if _, err := h.submit(admin, control.ProposalKindSourceAdd, okSrc); err != nil {
		t.Fatal(err)
	}
}

func TestProposalApprovingDoesNotExpire(t *testing.T) {
	h := newProposalHarness(t)
	a, _ := h.submit(admin, control.ProposalKindSourceAdd, okSrc)
	h.now = a.Expires.Add(-time.Minute)
	_ = h.approve(a.ID, a.Fingerprint)
	h.now = a.Expires.Add(time.Minute)
	if got := h.state(a.ID); got.State == control.ProposalExpired {
		t.Fatalf("state = %s", got.State)
	}
}
