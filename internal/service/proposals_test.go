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
	for _, to := range []string{control.ProposalApplied, control.ProposalRejected} {
		if err := h.p.resolve(info.ID, to); codeOf(t, err) != control.CodeInvalid {
			t.Fatalf("resolve an expired proposal as %s = %v", to, err)
		}
	}
	if err := h.withdraw(admin, info.ID); codeOf(t, err) != control.CodeInvalid {
		t.Fatalf("withdraw an expired proposal = %v", err)
	}
	if got, _ := h.show(info.ID); got.State != control.ProposalExpired {
		t.Fatalf("state = %s", got.State)
	}
}

func TestProposalResolveOnlyFromOpen(t *testing.T) {
	h := newProposalHarness(t)
	a, _ := h.submit(admin, control.ProposalKindSourceAdd, okSrc)
	if err := h.p.resolve(a.ID, control.ProposalApplied); err != nil {
		t.Fatal(err)
	}
	if got, _ := h.show(a.ID); got.State != control.ProposalApplied {
		t.Fatalf("state = %s", got.State)
	}
	if err := h.p.resolve(a.ID, control.ProposalRejected); codeOf(t, err) != control.CodeInvalid {
		t.Fatalf("resolve twice = %v", err)
	}
	if h.p.resolve(a.ID, control.ProposalWithdrawn) == nil || h.p.resolve(a.ID, control.ProposalOpen) == nil {
		t.Fatal("resolve accepted a state other than applied or rejected")
	}
	b, _ := h.submit(admin, control.ProposalKindSourceAdd, okSrc)
	if err := h.p.resolve(b.ID, control.ProposalRejected); err != nil {
		t.Fatal(err)
	}
	if err := h.withdraw(admin, b.ID); codeOf(t, err) != control.CodeInvalid {
		t.Fatalf("withdraw of a rejected proposal = %v", err)
	}
}

func TestProposalSubmitNeedsAPeer(t *testing.T) {
	h := newProposalHarness(t)
	if _, err := h.p.handleSubmit(context.Background(), json.RawMessage(`{}`)); codeOf(t, err) != control.CodeInternal {
		t.Fatalf("without a peer = %v", err)
	}
}

func TestProposalPrunesOldClosedProposals(t *testing.T) {
	h := newProposalHarness(t)
	old, _ := h.submit(admin, control.ProposalKindSourceAdd, okSrc)
	h.now = h.now.Add(proposalKeep + time.Hour)
	if _, err := h.submit(admin, control.ProposalKindSourceAdd, okSrc); err != nil {
		t.Fatal(err)
	}
	if _, err := h.show(old.ID); codeOf(t, err) != control.CodeNotFound {
		t.Fatalf("old expired proposal = %v", err)
	}
}
