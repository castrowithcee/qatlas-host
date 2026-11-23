package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/castrowithcee/qatlas-host/internal/admins"
	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/grants"
)

// grantProposalHarness gives the proposal harness a grant service over its clock and user database.
func grantProposalHarness(t *testing.T) (*proposalHarness, *grants.Store) {
	t.Helper()
	h := newProposalHarness(t)
	state := filepath.Dir(h.dir)
	store, err := grants.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	h.hooks.grants = &grantService{
		store: store, users: admins.Database{Passwd: filepath.Join(filepath.Dir(state), "passwd")},
		now: func() time.Time { return h.now }, log: h.hooks.log,
		stackKnown: func(n string) bool { return n == "demo-stack" },
	}
	return h, store
}

func TestGrantProposalIsStoredForThePeerOnly(t *testing.T) {
	h, _ := grantProposalHarness(t)
	info, err := h.submit(other, control.ProposalKindGrant,
		`{"action":"stack.restart","stack":"demo-stack","expires":"2999-12-31","reason":"restart after deploys"}`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := h.show(info.ID)
	if err != nil {
		t.Fatal(err)
	}
	var c control.ProposalGrant
	if err := json.Unmarshal(got.Content, &c); err != nil {
		t.Fatal(err)
	}
	want := control.ProposalGrant{User: "example-primary", UID: other, Action: "stack.restart", Stack: "demo-stack",
		Expires: "2999-12-31", Reason: "restart after deploys"}
	if c != want || got.State != control.ProposalOpen {
		t.Fatalf("proposal = %+v, content %+v", got, c)
	}
	// The fingerprint covers the user: the same content from another user is another proposal.
	again, err := h.submit(admin, control.ProposalKindGrant,
		`{"action":"stack.restart","stack":"demo-stack","expires":"2999-12-31","reason":"restart after deploys"}`)
	if err != nil || again.Fingerprint == info.Fingerprint {
		t.Fatalf("fingerprints %q and %q, %v", info.Fingerprint, again.Fingerprint, err)
	}
}

func TestGrantProposalRefusals(t *testing.T) {
	h, store := grantProposalHarness(t)
	if _, _, err := store.Upsert(control.Grant{User: "example-primary", UID: other, Action: "update.check", Stack: "demo-stack"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Upsert(control.Grant{User: "example-primary", UID: other, Action: "stack.restart", Stack: "demo-stack", Expires: "2020-01-01"}); err != nil {
		t.Fatal(err)
	}
	const ok = `"action":"stack.restart","stack":"demo-stack","reason":"why"`
	cases := map[string]struct{ content, code string }{
		"unknown action":    {`{"action":"stack.stop","stack":"demo-stack","reason":"why"}`, control.CodeGrantAction},
		"unknown stack":     {`{"action":"stack.restart","stack":"nope-stack","reason":"why"}`, control.CodeNoSuchStack},
		"bad stack name":    {`{"action":"stack.restart","stack":"../x","reason":"why"}`, control.CodeInvalid},
		"no reason":         {`{"action":"stack.restart","stack":"demo-stack"}`, control.CodeInvalid},
		"blank reason":      {`{"action":"stack.restart","stack":"demo-stack","reason":"  "}`, control.CodeInvalid},
		"long reason":       {`{"action":"stack.restart","stack":"demo-stack","reason":"` + strings.Repeat("x", 501) + `"}`, control.CodeInvalid},
		"past expiry":       {`{` + ok + `,"expires":"2020-01-01"}`, control.CodeInvalid},
		"bad expiry":        {`{` + ok + `,"expires":"soon"}`, control.CodeInvalid},
		"other user":        {`{` + ok + `,"user":"example-admin"}`, control.CodeInvalid},
		"other uid":         {`{` + ok + `,"uid":4100}`, control.CodeInvalid},
		"equal valid grant": {`{"action":"update.check","stack":"demo-stack","reason":"why"}`, control.CodeInvalid},
	}
	for name, c := range cases {
		_, err := h.submit(other, control.ProposalKindGrant, c.content)
		if err == nil || codeOf(t, err) != c.code {
			t.Errorf("%s: err = %v, want code %s", name, err, c.code)
		}
	}
	if _, err := os.Stat(h.dir); err == nil {
		if entries, _ := os.ReadDir(h.dir); len(entries) > 0 {
			t.Errorf("refused proposals left %d files", len(entries))
		}
	}
	// An expired equal grant does not block; a grant of another user does not either.
	if _, err := h.submit(other, control.ProposalKindGrant, `{`+ok+`}`); err != nil {
		t.Errorf("expired equal grant blocked: %v", err)
	}
	if _, err := h.submit(admin, control.ProposalKindGrant, `{"action":"update.check","stack":"demo-stack","reason":"why"}`); err != nil {
		t.Errorf("grant of another user blocked: %v", err)
	}
}
