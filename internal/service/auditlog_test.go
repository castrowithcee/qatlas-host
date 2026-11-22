package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/castrowithcee/qatlas-kit/localipc"

	"github.com/castrowithcee/qatlas-host/internal/channeltest"
	"github.com/castrowithcee/qatlas-host/internal/client"
	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/deploylog"
)

const exampleSecret = "example-secret-value-1234"

// auditEnv is a running service whose peers are the user in uid.
type auditEnv struct {
	ep       localipc.ServiceEndpoint
	stateDir string
	uid      *atomic.Uint32
}

func startAudited(t *testing.T) auditEnv {
	t.Helper()
	group, passwd := writeDatabase(t, inventedGroups, inventedUsers)
	uid := &atomic.Uint32{}
	state := filepath.Join(t.TempDir(), "state")
	ep := runService(t, Config{Version: "x", StateDir: state, GroupFile: group, PasswdFile: passwd,
		PeerCheck: func(net.Conn) (uint32, error) { return uid.Load(), nil }})
	return auditEnv{ep: ep, stateDir: state, uid: uid}
}

// as sends one command as the user uid with the origin claimed (empty claims none) and returns its answer's error.
func (e auditEnv) as(uid uint32, origin, command string, args, result any) error {
	e.uid.Store(uid)
	ctx := context.Background()
	if origin != "" {
		ctx = client.WithOrigin(ctx, origin)
	}
	return client.Do(ctx, e.ep, client.CallTimeout, command, args, result)
}

func (e auditEnv) entries(t *testing.T, args control.HostLogListArgs) control.DeployLogListResult {
	t.Helper()
	var res control.DeployLogListResult
	if err := e.as(0, "", control.HostLogListCommand, args, &res); err != nil {
		t.Fatal(err)
	}
	return res
}

func TestEveryRequestLeavesAnEntryWithItsUser(t *testing.T) {
	e := startAudited(t)
	if err := e.as(4100, control.OriginTUI, control.StatusCommand, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := e.as(4100, "", control.StackRestartCommand, control.StackActionArgs{Stack: "demo-stack"}, nil); codeOf(t, err) != control.CodeNotRoot {
		t.Fatalf("restart by an administrator = %v", err)
	}
	if err := e.as(4102, control.OriginCLI, control.StatusCommand, nil, nil); codeOf(t, err) != control.CodeNoAccess {
		t.Fatalf("status by a user without admin right = %v", err)
	}
	if err := e.as(0, "mystery", control.StackRestartCommand, control.StackActionArgs{Stack: "no-such"}, nil); err == nil {
		t.Fatal("restart of an unknown stack passed")
	}
	got := e.entries(t, control.HostLogListArgs{})
	// Newest first: the four requests above and the status request that waited for the service to serve; the
	// listing itself is logged once it is answered.
	if len(got.Entries) != 5 {
		t.Fatalf("entries = %+v", got.Entries)
	}
	type want struct {
		uid                          uint32
		command, class, scope, stack string
		code, origin                 string
	}
	for i, w := range []want{
		{0, control.StackRestartCommand, "change", "stack", "no-such", "no-repo", control.OriginUnknown},
		// A peer without any access is refused before its request is read, so the entry knows neither command nor origin.
		{4102, "", "", "", "", control.CodeNoAccess, control.OriginUnknown},
		{4100, control.StackRestartCommand, "change", "stack", "demo-stack", control.CodeNotRoot, control.OriginUnknown},
		{4100, control.StatusCommand, "read", "deploy", "", "", control.OriginTUI},
	} {
		en := got.Entries[i]
		if en.ClientUID == nil || *en.ClientUID != w.uid || en.Command != w.command || en.Class != w.class || en.Scope != w.scope ||
			en.Stack != w.stack || en.ErrorCode != w.code || en.Origin != w.origin || en.Time.IsZero() {
			t.Errorf("entry %d = %+v, want %+v", i, en, w)
		}
		if (w.code == "") != (en.Result == deploylog.ResultSuccess) {
			t.Errorf("entry %d result = %q for code %q", i, en.Result, w.code)
		}
	}
}

func TestARefusedPeerLeavesAnEntry(t *testing.T) {
	e := startAudited(t)
	if err := e.as(4999, "", control.StatusCommand, nil, nil); codeOf(t, err) != control.CodeNoAccess {
		t.Fatalf("status by a stranger = %v", err)
	}
	en := e.entries(t, control.HostLogListArgs{}).Entries
	if len(en) != 2 || en[0].ClientUID == nil || *en[0].ClientUID != 4999 || en[0].ErrorCode != control.CodeNoAccess || en[0].Command != "" {
		t.Fatalf("entries = %+v", en)
	}
}

func TestNoEntryHoldsAnArgumentOrASecret(t *testing.T) {
	e := startAudited(t)
	_ = e.as(0, "", control.CredentialSetCommand, control.CredentialSetArgs{Name: "example-token", Value: exampleSecret}, nil)
	// An unknown command, a stack field with free text and extra members of the arguments are not logged either.
	_ = e.as(0, "", "no.such-"+exampleSecret, nil, nil)
	_ = e.as(0, "", control.StackRestartCommand, map[string]string{"stack": "Bad Stack " + exampleSecret, "extra": exampleSecret}, nil)
	_ = e.as(0, exampleSecret, control.StatusCommand, nil, nil)
	got := e.entries(t, control.HostLogListArgs{})
	if len(got.Entries) != 5 {
		t.Fatalf("entries = %+v", got.Entries)
	}
	if got.Entries[0].Origin != control.OriginUnknown || got.Entries[1].Stack != "" || got.Entries[2].Command != "" {
		t.Fatalf("entries = %+v", got.Entries)
	}
	files, _ := filepath.Glob(filepath.Join(e.stateDir, deploylog.DirName, "*.jsonl"))
	if len(files) == 0 {
		t.Fatal("no log file")
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil || bytes.Contains(data, []byte(exampleSecret)) || bytes.Contains(data, []byte("example-token")) {
			t.Fatalf("log file %s holds an argument: %q, %v", f, data, err)
		}
	}
}

func TestLogListFiltersAndPages(t *testing.T) {
	e := startAudited(t)
	for range 3 {
		_ = e.as(0, "", control.StatusCommand, nil, nil)
	}
	_ = e.as(4102, "", control.StatusCommand, nil, nil)
	if got := e.entries(t, control.HostLogListArgs{Result: "failure"}); len(got.Entries) != 1 {
		t.Fatalf("failed requests = %+v", got.Entries)
	}
	if got := e.entries(t, control.HostLogListArgs{Action: control.StatusCommand, Result: "failure"}); len(got.Entries) != 0 {
		t.Fatalf("failed status requests = %+v", got.Entries)
	}
	// Three requests of the test and the one that waited for the service to serve.
	if got := e.entries(t, control.HostLogListArgs{Action: control.StatusCommand}); len(got.Entries) != 4 {
		t.Fatalf("status requests = %+v", got.Entries)
	}
	var seqs []uint64
	for after := ""; ; {
		page := e.entries(t, control.HostLogListArgs{Limit: 2, After: after})
		for _, en := range page.Entries {
			seqs = append(seqs, en.Seq)
		}
		if after = page.Next; after == "" {
			break
		}
	}
	if len(seqs) < 4 || seqs[len(seqs)-1] != 1 {
		t.Fatalf("pages = %v", seqs)
	}
	for _, bad := range []control.HostLogListArgs{{From: "yesterday"}, {Limit: -1}, {Limit: control.HostLogMaxLimit + 1}, {Result: "maybe"}, {After: "nope"}} {
		if err := e.as(0, "", control.HostLogListCommand, bad, nil); codeOf(t, err) != control.CodeBadRequest {
			t.Errorf("%+v = %v", bad, err)
		}
	}
}

func TestLogVerifyReportsAChangedLine(t *testing.T) {
	e := startAudited(t)
	for range 3 {
		_ = e.as(0, "", control.StatusCommand, nil, nil)
	}
	var res control.HostLogVerifyResult
	if err := e.as(0, "", control.HostLogVerifyCommand, nil, &res); err != nil || !res.Intact || len(res.Days) != 1 {
		t.Fatalf("intact log = %+v, %v", res, err)
	}
	files, _ := filepath.Glob(filepath.Join(e.stateDir, deploylog.DirName, "*.jsonl"))
	if len(files) != 1 {
		t.Fatalf("files = %v", files)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.SplitAfter(data, []byte("\n"))
	lines[1] = bytes.Replace(lines[1], []byte(`"command":"status"`), []byte(`"command":"stacks.list"`), 1)
	if err := os.WriteFile(files[0], bytes.Join(lines, nil), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := e.as(0, "", control.HostLogVerifyCommand, nil, &res); err != nil || res.Intact || len(res.Problems) != 1 {
		t.Fatalf("changed log = %+v, %v", res, err)
	}
}

func TestTheLogIsPrivateToTheServiceUser(t *testing.T) {
	e := startAudited(t)
	_ = e.as(0, "", control.StatusCommand, nil, nil)
	dir := filepath.Join(e.stateDir, deploylog.DirName)
	info, err := os.Stat(dir)
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("dir = %v, %v", info, err)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*"))
	for _, f := range files {
		if fi, err := os.Stat(f); err != nil || fi.Mode().Perm()&0o077 != 0 {
			t.Errorf("%s = %v, %v", f, fi, err)
		}
	}
}

type failingSink struct{ calls atomic.Int32 }

func (f *failingSink) Append(deploylog.Fields) error {
	f.calls.Add(1)
	return errors.New("disk full")
}

func TestARequestRunsOnWhenItsEntryCannotBeWritten(t *testing.T) {
	reg := control.NewRegistry()
	if err := reg.Register("ping", reading(func(context.Context, json.RawMessage) (any, error) { return "pong", nil })); err != nil {
		t.Fatal(err)
	}
	ep := channeltest.Endpoint(t)
	l, err := ep.Listen()
	if err != nil {
		t.Fatal(err)
	}
	var logged bytes.Buffer
	sink := &failingSink{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		serve(ctx, l, reg, allowAll, groupDB{}.authorize, slog.New(slog.NewTextHandler(&logged, nil)), sink)
		close(done)
	}()
	defer func() { cancel(); <-done }()
	var out string
	if err := client.Do(context.Background(), ep, client.CallTimeout, "ping", nil, &out); err != nil || out != "pong" {
		t.Fatalf("ping = %q, %v", out, err)
	}
	if sink.calls.Load() != 1 || !strings.Contains(logged.String(), "cannot write the action log") {
		t.Fatalf("calls = %d, service log = %q", sink.calls.Load(), logged.String())
	}
}

func TestAnAbsentOriginIsUnknown(t *testing.T) {
	e := startAudited(t)
	_ = e.as(0, "", control.StatusCommand, nil, nil)
	if en := e.entries(t, control.HostLogListArgs{}).Entries; len(en) != 2 || en[0].Origin != control.OriginUnknown {
		t.Fatalf("entries = %+v", en)
	}
	// The request format stays compatible: a request without the member is served.
	if resp := rawExchange(t, e.ep, `{"v":1,"command":"status"}`+"\n"); !resp.OK {
		t.Fatalf("answer = %+v", resp)
	}
}
