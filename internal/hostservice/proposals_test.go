package hostservice

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/host/auditlog"
)

// exampleSecret is an invented value that a proposal must refuse to carry.
const exampleSecret = "example-pass"

func publicKey(t *testing.T) string {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key))) + " example@example.test"
}

func submitArgs(t *testing.T, content any) control.ProposalSubmitArgs {
	t.Helper()
	raw, err := json.Marshal(content)
	if err != nil {
		t.Fatal(err)
	}
	return control.ProposalSubmitArgs{Kind: control.ProposalKindHostChange, Content: raw}
}

// startAs starts a host service whose peer is whoever peer holds at the time of a request.
func startAs(t *testing.T, peer *atomic.Uint32) env {
	t.Helper()
	return startWith(t, func(net.Conn) (uint32, error) { return peer.Load(), nil })
}

func TestAnAdministratorProposesAndRootDecides(t *testing.T) {
	var peer atomic.Uint32
	peer.Store(4100)
	e := startAs(t, &peer)
	key := publicKey(t)
	var info control.ProposalInfo
	err := e.do(control.ProposalSubmitCommand, submitArgs(t, control.ProposalHostChange{
		Action: "user.create", Args: []string{"example-new", "docker", "group=example-team"}, Keys: []string{key}}), &info)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(info.ID, control.HostProposalIDPrefix) || info.Kind != control.ProposalKindHostChange ||
		info.State != control.ProposalOpen || info.SubmitterUID != 4100 || info.SubmitterName != "example-admin" {
		t.Fatalf("info = %+v", info)
	}
	var shown control.ProposalShowResult
	if err := e.do(control.ProposalShowCommand, control.ProposalIDArgs{ID: info.ID}, &shown); err != nil {
		t.Fatal(err)
	}
	var content control.ProposalHostChange
	if err := json.Unmarshal(shown.Content, &content); err != nil || content.Action != "user.create" ||
		len(content.Args) != 3 || len(content.Keys) != 1 || content.Keys[0] != key {
		t.Fatalf("content = %s, %v", shown.Content, err)
	}
	var list control.ProposalListResult
	if err := e.do(control.ProposalListCommand, nil, &list); err != nil || len(list.Proposals) != 1 {
		t.Fatalf("list = %+v, %v", list, err)
	}
	// Deciding is root's.
	for _, command := range []string{control.ProposalApproveCommand, control.ProposalRejectCommand, control.ProposalResultCommand} {
		wantCode(t, e.do(command, control.ProposalApproveArgs{ID: info.ID, Fingerprint: info.Fingerprint}, nil), control.CodeNotRoot)
	}
	peer.Store(0)
	var approved control.ProposalInfo
	if err := e.do(control.ProposalApproveCommand, control.ProposalApproveArgs{ID: info.ID, Fingerprint: info.Fingerprint}, &approved); err != nil ||
		approved.State != control.ProposalApproving {
		t.Fatalf("approve = %+v, %v", approved, err)
	}
	var done control.ProposalInfo
	if err := e.do(control.ProposalResultCommand, control.ProposalResultArgs{ID: info.ID, Fingerprint: info.Fingerprint, Applied: true}, &done); err != nil ||
		done.State != control.ProposalApplied {
		t.Fatalf("result = %+v, %v", done, err)
	}

	// The log names the actions, never the proposal or its content.
	want := []string{auditlog.OperationProposalResult, auditlog.OperationProposalApprove, auditlog.OperationControlRefuse,
		auditlog.OperationControlRefuse, auditlog.OperationControlRefuse, auditlog.OperationProposalRead,
		auditlog.OperationProposalRead, auditlog.OperationProposalSubmit}
	got := e.entries(t)
	if len(got) != len(want) {
		t.Fatalf("log = %+v", got)
	}
	for i, r := range got {
		if r.Entry.Action != want[i] {
			t.Errorf("entry %d = %s, want %s", i, r.Entry.Action, want[i])
		}
	}
	files, _ := filepath.Glob(filepath.Join(LogDir(e.stateDir), "*.jsonl"))
	for _, f := range files {
		data, _ := os.ReadFile(f)
		for _, secret := range []string{"example-new", info.ID, key[:40]} {
			if bytes.Contains(data, []byte(secret)) {
				t.Errorf("the log holds %q", secret)
			}
		}
	}
}

func TestAnAdministratorWithdrawsAndRootRejects(t *testing.T) {
	var peer atomic.Uint32
	peer.Store(4100)
	e := startAs(t, &peer)
	ids := make([]string, 2)
	for i := range ids {
		var info control.ProposalInfo
		if err := e.do(control.ProposalSubmitCommand, submitArgs(t, control.ProposalHostChange{
			Action: "user.lock", Args: []string{"example-reader"}}), &info); err != nil {
			t.Fatal(err)
		}
		ids[i] = info.ID
	}
	var info control.ProposalInfo
	if err := e.do(control.ProposalWithdrawCommand, control.ProposalIDArgs{ID: ids[0]}, &info); err != nil || info.State != control.ProposalWithdrawn {
		t.Fatalf("withdraw = %+v, %v", info, err)
	}
	peer.Store(0)
	if err := e.do(control.ProposalRejectCommand, control.ProposalRejectArgs{ID: ids[1], Reason: "not now"}, &info); err != nil ||
		info.State != control.ProposalRejected || info.Message != "not now" {
		t.Fatalf("reject = %+v, %v", info, err)
	}
}

func TestSubmittingRefusesWhatMayNotBeProposed(t *testing.T) {
	e := start(t, 4100)
	key := publicKey(t)
	for name, content := range map[string]any{
		"set-admin":               control.ProposalHostChange{Action: "user.set-admin", Args: []string{"example-reader", "on"}},
		"set-password":            control.ProposalHostChange{Action: "user.set-password", Args: []string{"example-reader"}},
		"create with admin":       control.ProposalHostChange{Action: "user.create", Args: []string{"example-new", "admin"}},
		"unknown action":          control.ProposalHostChange{Action: "user.make-root", Args: []string{"example-reader"}},
		"argument outside schema": control.ProposalHostChange{Action: "user.lock", Args: []string{"example-reader", "--force"}},
		"members of sudo":         control.ProposalHostChange{Action: "group.set-members", Args: []string{"sudo", "+example-reader"}},
		"members of qhost":        control.ProposalHostChange{Action: "group.set-members", Args: []string{"qhost", "-example-admin"}},
		"key that is no key":      control.ProposalHostChange{Action: "user.add-key", Args: []string{"example-reader"}, Keys: []string{"not-a-key"}},
		"password field":          map[string]any{"action": "user.create", "args": []string{"example-new"}, "keys": []string{key}, "password": exampleSecret},
		"unknown field":           map[string]any{"action": "user.lock", "args": []string{"example-reader"}, "commands": []string{"id"}},
		"no object":               []string{"user.lock", "example-reader"},
	} {
		var info control.ProposalInfo
		err := e.do(control.ProposalSubmitCommand, submitArgs(t, content), &info)
		wantCode(t, err, control.CodeInvalid)
		if info.ID != "" {
			t.Fatalf("%s: stored %+v", name, info)
		}
	}
	// A kind of the deploy service is not kept here.
	raw, _ := json.Marshal(control.ProposalSourceAdd{Path: "/srv/example"})
	wantCode(t, e.do(control.ProposalSubmitCommand, control.ProposalSubmitArgs{Kind: control.ProposalKindSourceAdd, Content: raw}, nil), control.CodeInvalid)
	var list control.ProposalListResult
	if err := e.do(control.ProposalListCommand, nil, &list); err != nil || len(list.Proposals) != 0 {
		t.Fatalf("list = %+v, %v", list, err)
	}
}

// The user of the deploy service is in neither the admin group nor the service group, so the host service gives
// it no proposal and takes none from it.
func TestTheDeployServiceUserGetsNoProposal(t *testing.T) {
	var peer atomic.Uint32
	peer.Store(4100)
	e := startAs(t, &peer)
	var info control.ProposalInfo
	if err := e.do(control.ProposalSubmitCommand, submitArgs(t, control.ProposalHostChange{
		Action: "user.lock", Args: []string{"example-reader"}}), &info); err != nil {
		t.Fatal(err)
	}
	peer.Store(4200)
	for command, args := range map[string]any{
		control.ProposalListCommand:     nil,
		control.ProposalShowCommand:     control.ProposalIDArgs{ID: info.ID},
		control.ProposalSubmitCommand:   submitArgs(t, control.ProposalHostChange{Action: "user.lock", Args: []string{"example-reader"}}),
		control.ProposalWithdrawCommand: control.ProposalIDArgs{ID: info.ID},
	} {
		var out json.RawMessage
		wantCode(t, e.do(command, args, &out), control.CodeNotAdmin)
		if out != nil {
			t.Fatalf("%s: result %s", command, out)
		}
	}
	peer.Store(0)
	var list control.ProposalListResult
	if err := e.do(control.ProposalListCommand, nil, &list); err != nil || len(list.Proposals) != 1 || list.Proposals[0].State != control.ProposalOpen {
		t.Fatalf("list = %+v, %v", list, err)
	}
}
