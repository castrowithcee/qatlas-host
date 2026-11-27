package hostservice

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/castrowithcee/qatlas-kit/localipc"

	"github.com/castrowithcee/qatlas-host/internal/channeltest"
	"github.com/castrowithcee/qatlas-host/internal/client"
	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/host/auditlog"
	"github.com/castrowithcee/qatlas-host/internal/host/usagehistory"
)

// An invented group database: example-admin (4100) is an administrator in the service group, example-reader
// (4102) is in the service group only, example-sudoer (4103) in the admin group only, and qhost-deploy (4200),
// the user of the deploy service, is in neither.
const (
	inventedGroups = "sudo:x:27:example-admin,example-sudoer\n" +
		"qhost:x:990:example-admin,example-reader\n"
	inventedUsers = "root:x:0:0:root:/root:/bin/sh\n" +
		"example-admin:x:4100:4100::/nonexistent:/usr/sbin/nologin\n" +
		"example-reader:x:4102:4102::/nonexistent:/usr/sbin/nologin\n" +
		"example-sudoer:x:4103:4103::/nonexistent:/usr/sbin/nologin\n" +
		"qhost-deploy:x:4200:4200::/nonexistent:/usr/sbin/nologin\n"
)

const (
	testJournalID = "20300102T030405.000000Z-0a1b2c"
	testUndoID    = "20300102T030400.000000Z-ffeedd"
)

// env is a running host service whose peer is the user uid. stop ends it and waits; logs is its service log.
type env struct {
	endpoint localipc.ServiceEndpoint
	stateDir string
	group    string
	stop     func()
	logs     *lockedBuffer
}

// lockedBuffer is a service log that the service writes and a test reads at once.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func start(t *testing.T, uid uint32) env {
	t.Helper()
	return startWith(t, func(net.Conn) (uint32, error) { return uid, nil })
}

// startWith starts a host service whose peers are the users peer reports; setup adjusts its configuration.
func startWith(t *testing.T, peer func(net.Conn) (uint32, error), setup ...func(*Config)) env {
	t.Helper()
	return startCfg(t, peer, func(_ string, c *Config) {
		for _, f := range setup {
			f(c)
		}
	})
}

// startCfg is startWith with a last say over the configuration; tweak gets the state directory.
func startCfg(t *testing.T, peer func(net.Conn) (uint32, error), tweak func(stateDir string, c *Config)) env {
	t.Helper()
	dir := t.TempDir()
	e := env{endpoint: channeltest.Endpoint(t), stateDir: filepath.Join(dir, "state"), group: filepath.Join(dir, "group"),
		logs: &lockedBuffer{}}
	passwd := filepath.Join(dir, "passwd")
	if err := os.WriteFile(e.group, []byte(inventedGroups), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(passwd, []byte(inventedUsers), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		Version: "v9.9.9", Endpoint: e.endpoint, StateDir: e.stateDir, GroupFile: e.group, PasswdFile: passwd,
		Now:     func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) },
		History: &usagehistory.Config{},
		peerUID: peer,
	}
	tweak(e.stateDir, &cfg)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, slog.New(slog.NewTextHandler(e.logs, nil)), cfg) }()
	e.stop = sync.OnceFunc(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("run: %v", err)
		}
	})
	t.Cleanup(e.stop)
	channeltest.WaitReady(t, e.endpoint)
	return e
}

func (e env) do(command string, args, result any) error {
	return client.Do(context.Background(), e.endpoint, 5*time.Second, command, args, result)
}

func (e env) status() error {
	var res StatusResult
	err := e.do(control.StatusCommand, nil, &res)
	if err == nil && (res.Version != "v9.9.9" || !res.StartedAt.Equal(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))) {
		return errors.New("unexpected status " + res.Version + " " + res.StartedAt.String())
	}
	return err
}

// entries reads the action log from its files, newest first.
func (e env) entries(t *testing.T) []auditlog.Record {
	t.Helper()
	page, err := auditlog.Read(LogDir(e.stateDir), auditlog.Query{Limit: 1000})
	if err != nil {
		t.Fatal(err)
	}
	return page.Records
}

func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	var remote *control.Error
	if !errors.As(err, &remote) || remote.Code != code {
		t.Fatalf("err = %v, want code %s", err, code)
	}
}

func TestRootGetsTheStatus(t *testing.T) {
	e := start(t, 0)
	if err := e.status(); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(e.stateDir); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("state directory: %v, %v", info, err)
	}
	if info, err := os.Stat(LogDir(e.stateDir)); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("log directory: %v, %v", info, err)
	}
	got := e.entries(t)
	if len(got) != 1 || got[0].Entry.Action != auditlog.OperationControlStatus || got[0].Entry.Result != auditlog.ResultSuccess ||
		got[0].Entry.ClientUID == nil || *got[0].Entry.ClientUID != 0 {
		t.Fatalf("log = %+v", got)
	}
}

func TestAnAdministratorGetsTheStatusWithoutRoot(t *testing.T) {
	e := start(t, 4100)
	if err := e.status(); err != nil {
		t.Fatal(err)
	}
	var out control.HostLogListResult
	if err := e.do(control.HostLogListCommand, nil, &out); err != nil || len(out.Entries) == 0 {
		t.Fatalf("log.list = %+v, %v", out, err)
	}
	var report control.HostLogVerifyResult
	if err := e.do(control.HostLogVerifyCommand, nil, &report); err != nil || !report.Intact {
		t.Fatalf("log.verify = %+v, %v", report, err)
	}
}

func TestAUserWhoIsNoAdministratorIsToldSoAndLearnsNothing(t *testing.T) {
	for _, uid := range []uint32{4102, 4103, 4999} {
		e := start(t, uid)
		for _, command := range []string{control.StatusCommand, control.HostLogListCommand, control.HostRecordCommand, "no.such-command"} {
			var out json.RawMessage
			err := e.do(command, nil, &out)
			wantCode(t, err, control.CodeNotAdmin)
			if out != nil {
				t.Fatalf("user %d, %s: result %s", uid, command, out)
			}
		}
		for _, r := range e.entries(t) {
			en := r.Entry
			if en.Action != auditlog.OperationControlRefuse || en.ErrorCode != control.CodeNotAdmin || en.ClientUID == nil || *en.ClientUID != uid {
				t.Fatalf("user %d: entry %+v", uid, en)
			}
		}
	}
}

func TestAPeerWithoutCredentialsIsRefused(t *testing.T) {
	endpoint := channeltest.Endpoint(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, slog.New(slog.NewTextHandler(io.Discard, nil)), Config{Endpoint: endpoint, History: &usagehistory.Config{},
			StateDir: filepath.Join(t.TempDir(), "state"), peerUID: func(net.Conn) (uint32, error) { return 0, errors.New("unknown") }})
	}()
	defer func() { cancel(); <-done }()
	channeltest.WaitReady(t, endpoint)
	err := client.Do(context.Background(), endpoint, 5*time.Second, control.StatusCommand, nil, nil)
	wantCode(t, err, control.CodeNoAccess)
}

func TestARevokedAdministratorIsRefusedAtTheNextRequestOfAnOpenConnection(t *testing.T) {
	for _, revoke := range []struct{ name, groups string }{
		{"admin group", strings.Replace(inventedGroups, "sudo:x:27:example-admin,", "sudo:x:27:", 1)},
		{"service group", strings.Replace(inventedGroups, "qhost:x:990:example-admin,", "qhost:x:990:", 1)},
	} {
		t.Run(revoke.name, func(t *testing.T) {
			e := start(t, 4100)
			cl, err := client.Dial(context.Background(), e.endpoint)
			if err != nil {
				t.Fatal(err)
			}
			defer cl.Close()
			var st StatusResult
			if err := cl.Call(context.Background(), control.StatusCommand, nil, &st); err != nil || st.Version != "v9.9.9" {
				t.Fatalf("status = %+v, %v", st, err)
			}
			if err := os.WriteFile(e.group, []byte(revoke.groups), 0o600); err != nil {
				t.Fatal(err)
			}
			st = StatusResult{}
			err = cl.Call(context.Background(), control.StatusCommand, nil, &st)
			wantCode(t, err, control.CodeNotAdmin)
			if st.Version != "" {
				t.Fatalf("status after the revocation = %+v", st)
			}
			if got := e.entries(t); got[0].Entry.Action != auditlog.OperationControlRefuse || got[0].Entry.ErrorCode != control.CodeNotAdmin {
				t.Fatalf("newest entry = %+v", got[0].Entry)
			}
		})
	}
}

func TestOtherCommandsAreUnknown(t *testing.T) {
	e := start(t, 0)
	var out json.RawMessage
	wantCode(t, e.do("stack.list", nil, &out), control.CodeUnknownCommand)
	if got := e.entries(t); got[0].Entry.Action != auditlog.OperationControlRequest || got[0].Entry.ErrorCode != control.CodeUnknownCommand {
		t.Fatalf("entry = %+v", got[0].Entry)
	}
}

func TestHostRecordRefusesEveryPeerButRoot(t *testing.T) {
	e := start(t, 4100)
	err := e.do(control.HostRecordCommand, control.HostRecordArgs{Action: "demo.set", Result: control.HostResultApplied, JournalID: testJournalID}, nil)
	wantCode(t, err, control.CodeNotRoot)
	got := e.entries(t)
	if len(got) != 1 || got[0].Entry.Action != auditlog.OperationControlRefuse || got[0].Entry.ErrorCode != control.CodeNotRoot ||
		got[0].Entry.Host != nil || *got[0].Entry.ClientUID != 4100 {
		t.Fatalf("log = %+v", got)
	}
}

func TestHostRecordNamesTheHumanBehindSudo(t *testing.T) {
	e := start(t, 0)
	for _, tc := range []struct{ result, code string }{
		{control.HostResultApplied, ""},
		{control.HostResultFailed, "change-failed"},
		{control.HostResultRestored, "change-restored"},
	} {
		args := control.HostRecordArgs{Action: "demo.set", Result: tc.result, SudoUID: 1000, JournalID: testJournalID, UndoOf: testUndoID}
		if err := e.do(control.HostRecordCommand, args, nil); err != nil {
			t.Fatalf("%s: %v", tc.result, err)
		}
		en := e.entries(t)[0].Entry
		want := auditlog.Host{ActionID: "demo.set", JournalID: testJournalID, UndoOf: testUndoID}
		if en.Action != auditlog.OperationHostChange || en.ClientUID == nil || *en.ClientUID != 1000 || en.Host == nil || *en.Host != want ||
			en.ErrorCode != tc.code {
			t.Fatalf("%s: entry = %+v", tc.result, en)
		}
	}
	var out control.HostLogListResult
	if err := e.do(control.HostLogListCommand, control.HostLogListArgs{Action: auditlog.OperationHostChange, Result: "failure"}, &out); err != nil ||
		len(out.Entries) != 2 || out.Entries[0].Change == nil || out.Entries[0].Change.JournalID != testJournalID {
		t.Fatalf("log.list = %+v, %v", out, err)
	}
}

func TestHostRecordRejectsAFreeFormRequest(t *testing.T) {
	e := start(t, 0)
	for _, args := range []any{
		control.HostRecordArgs{Action: "Demo Set; rm", Result: "applied", JournalID: testJournalID},
		control.HostRecordArgs{Action: "demo.set", Result: "pending", JournalID: testJournalID},
		control.HostRecordArgs{Action: "demo.set", Result: "applied", JournalID: "not-an-id"},
		control.HostRecordArgs{Action: "demo.set", Result: "applied", JournalID: testJournalID, UndoOf: "x"},
		control.HostRecordArgs{Action: strings.Repeat("a", 80) + ".b", Result: "applied", JournalID: testJournalID},
		[]int{1},
	} {
		wantCode(t, e.do(control.HostRecordCommand, args, nil), control.CodeBadRequest)
	}
	for _, r := range e.entries(t) {
		if r.Entry.Host != nil || r.Entry.ClientUID == nil || *r.Entry.ClientUID != 0 || r.Entry.ErrorCode != control.CodeBadRequest {
			t.Fatalf("a rejected record left %+v", r.Entry)
		}
	}
}

// No entry ever holds an argument: neither a filter of the log nor anything sent with an unknown command.
func TestTheLogHoldsNoArgument(t *testing.T) {
	e := start(t, 0)
	const planted = "planted-argument-hostservice"
	_ = e.do(control.HostLogListCommand, control.HostLogListArgs{Action: planted}, nil)
	_ = e.do("no.such-command", map[string]string{"value": planted}, nil)
	_ = e.do(control.HostRecordCommand, map[string]string{"action": planted, "result": "applied", "journal_id": testJournalID}, nil)
	files, _ := filepath.Glob(filepath.Join(LogDir(e.stateDir), "*.jsonl"))
	if len(files) != 1 {
		t.Fatalf("log files = %v", files)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(planted)) || bytes.Count(data, []byte("\n")) != 3 {
		t.Fatalf("log = %s", data)
	}
}

func TestLogListFiltersAndPages(t *testing.T) {
	e := start(t, 0)
	for range 4 {
		if err := e.status(); err != nil {
			t.Fatal(err)
		}
	}
	_ = e.do("no.such-command", nil, nil)
	var all control.HostLogListResult
	if err := e.do(control.HostLogListCommand, nil, &all); err != nil || len(all.Entries) != 5 || all.Next != "" {
		t.Fatalf("all = %+v, %v", all, err)
	}
	if top := all.Entries[0]; top.Action != auditlog.OperationControlRequest || top.ErrorCode != control.CodeUnknownCommand || top.Time.IsZero() {
		t.Fatalf("newest = %+v", top)
	}
	var paged []uint64
	for after := ""; ; {
		var page control.HostLogListResult
		if err := e.do(control.HostLogListCommand, control.HostLogListArgs{Limit: 2, After: after}, &page); err != nil || len(page.Entries) > 2 {
			t.Fatalf("page = %+v, %v", page, err)
		}
		for _, en := range page.Entries {
			paged = append(paged, en.Seq)
		}
		if after = page.Next; after == "" {
			break
		}
	}
	if paged[len(paged)-1] != 1 {
		t.Fatalf("pages end at %v", paged)
	}
	for i := 1; i < len(paged); i++ {
		if paged[i] >= paged[i-1] {
			t.Fatalf("pages overlap or are unordered: %v", paged)
		}
	}
	var statuses control.HostLogListResult
	if err := e.do(control.HostLogListCommand, control.HostLogListArgs{Action: auditlog.OperationControlStatus, Result: "success"}, &statuses); err != nil ||
		len(statuses.Entries) != 4 {
		t.Fatalf("by action = %+v, %v", statuses, err)
	}
	var future control.HostLogListResult
	if err := e.do(control.HostLogListCommand, control.HostLogListArgs{From: "2999-01-01T00:00:00Z"}, &future); err != nil || len(future.Entries) != 0 {
		t.Fatalf("future = %+v, %v", future, err)
	}
	for _, bad := range []control.HostLogListArgs{
		{From: "yesterday"}, {From: "2030-01-02T00:00:00Z", To: "2030-01-01T00:00:00Z"}, {Limit: -1},
		{Limit: control.HostLogMaxLimit + 1}, {Result: "maybe"}, {Action: "a\nb"}, {Action: strings.Repeat("a", 65)}, {After: "nope"},
	} {
		wantCode(t, e.do(control.HostLogListCommand, bad, nil), control.CodeBadRequest)
	}
}

func TestLogVerifyReportsAChangedLine(t *testing.T) {
	e := start(t, 0)
	for range 3 {
		if err := e.status(); err != nil {
			t.Fatal(err)
		}
	}
	files, _ := filepath.Glob(filepath.Join(LogDir(e.stateDir), "*.jsonl"))
	if len(files) != 1 {
		t.Fatalf("log files = %v", files)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.SplitAfter(data, []byte("\n"))
	changed := bytes.Replace(lines[1], []byte(`"client_uid":0`), []byte(`"client_uid":7`), 1)
	if bytes.Equal(changed, lines[1]) {
		t.Fatalf("line 2 has no client: %s", lines[1])
	}
	lines[1] = changed
	if err := os.WriteFile(files[0], bytes.Join(lines, nil), 0o600); err != nil {
		t.Fatal(err)
	}
	var report control.HostLogVerifyResult
	if err := e.do(control.HostLogVerifyCommand, nil, &report); err != nil {
		t.Fatal(err)
	}
	if report.Intact || len(report.Days) != 1 || report.Days[0].Intact || len(report.Problems) != 1 {
		t.Fatalf("report = %+v", report)
	}
	if p := report.Problems[0]; p.Code != "chain-broken" || p.Line != 3 || p.Seq != 3 {
		t.Fatalf("problem = %+v", p)
	}
}
