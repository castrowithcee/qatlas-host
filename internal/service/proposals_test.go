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
	"github.com/castrowithcee/qatlas-host/internal/proposals"
)

type proposalHarness struct {
	t     *testing.T
	p     *proposals.Store
	hooks *proposalHooks
	dir   string
	now   time.Time
}

func newProposalHarness(t *testing.T) *proposalHarness {
	t.Helper()
	dir := t.TempDir()
	passwd := filepath.Join(dir, "passwd")
	if err := os.WriteFile(passwd, []byte(inventedUsers), 0o600); err != nil {
		t.Fatal(err)
	}
	h := &proposalHarness{t: t, now: time.Date(2030, 1, 10, 12, 0, 0, 0, time.UTC)}
	h.hooks = &proposalHooks{log: slog.New(slog.DiscardHandler)}
	h.p = proposals.New(filepath.Join(dir, "state"), passwd, func() time.Time { return h.now }, h.hooks.log, h.hooks.normalize)
	h.dir = filepath.Join(dir, "state", "proposals")
	return h
}

func (h *proposalHarness) submit(uid uint32, kind, content string) (control.ProposalInfo, error) {
	h.t.Helper()
	raw, _ := json.Marshal(control.ProposalSubmitArgs{Kind: kind, Content: json.RawMessage(content)})
	v, err := h.p.Submit(control.WithPeer(context.Background(), uid), raw)
	if err != nil {
		return control.ProposalInfo{}, err
	}
	return v.(control.ProposalInfo), nil
}

func (h *proposalHarness) withdraw(uid uint32, id string) error {
	raw, _ := json.Marshal(control.ProposalIDArgs{ID: id})
	_, err := h.p.Withdraw(control.WithPeer(context.Background(), uid), raw)
	return err
}

func (h *proposalHarness) show(id string) (control.ProposalShowResult, error) {
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
	v, err := h.p.List(context.Background(), nil)
	if err != nil || len(v.(control.ProposalListResult).Proposals) != 3 {
		t.Fatalf("list = %+v, %v", v, err)
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
	if _, err := h.p.Submit(control.WithPeer(context.Background(), admin),
		json.RawMessage(`{"kind":"source-add","content":{"path":"/srv/x"},"extra":1}`)); codeOf(t, err) != control.CodeBadRequest {
		t.Fatalf("unknown request field = %v", err)
	}
	if _, err := os.Stat(h.dir); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("a refused proposal left a store behind: %v", err)
	}
}
