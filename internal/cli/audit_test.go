package cli

import (
	"bytes"
	"encoding/json"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/channeltest"
	"github.com/castrowithcee/qatlas-host/internal/hostservice"
)

func TestAuditListsTheActionLogNewestFirst(t *testing.T) {
	startHost(t, true)
	run(t, "audit", "--limit", "1")
	code, stdout, stderr := run(t, "audit", "--output", "json", "--action", "log.read")
	if code != 0 || stderr != "" {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(stdout), &rows); err != nil {
		t.Fatalf("invalid JSON %q: %v", stdout, err)
	}
	// The listing itself is logged once it is answered, so it shows only the request before it.
	if len(rows) != 1 || rows[0]["action"] != "log.read" || rows[0]["result"] != "success" || rows[0]["client_uid"] == nil || rows[0]["seq"] != 1.0 {
		t.Fatalf("rows = %v", rows)
	}
	if code, stdout, _ := run(t, "audit", "--result", "failure"); code != 0 || strings.Contains(stdout, "log.read") {
		t.Fatalf("failures: code %d, %q", code, stdout)
	}
	_, stdout, _ = run(t, "audit", "--output", "json")
	rows = nil
	if err := json.Unmarshal([]byte(stdout), &rows); err != nil || len(rows) != 3 || rows[0]["seq"] != 3.0 || rows[2]["seq"] != 1.0 {
		t.Fatalf("not newest first: %q, %v", stdout, err)
	}
}

func TestAuditChecksItsFlags(t *testing.T) {
	startHost(t, true)
	for _, args := range [][]string{
		{"audit", "--result", "maybe"}, {"audit", "--limit", "0"}, {"audit", "--limit", "501"}, {"audit", "--since", "yesterday"},
	} {
		if code, _, stderr := run(t, args...); code != 2 || !strings.Contains(stderr, "invalid-request") {
			t.Errorf("%v: code %d, stderr %q", args, code, stderr)
		}
	}
	if code, _, stderr := run(t, "audit", "--action", "a\tb"); code != 2 || !strings.Contains(stderr, "invalid-request") {
		t.Errorf("a control character: code %d, stderr %q", code, stderr)
	}
}

func TestAuditVerifyReportsAChangedLine(t *testing.T) {
	state := startHost(t, true)
	for range 3 {
		run(t, "audit")
	}
	code, stdout, stderr := run(t, "audit", "verify")
	if code != 0 || !strings.Contains(stdout, "intact") || strings.Contains(stdout, "broken") {
		t.Fatalf("intact log: code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	files, _ := filepath.Glob(filepath.Join(hostservice.LogDir(state), "*.jsonl"))
	if len(files) != 1 {
		t.Fatalf("log files = %v", files)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.SplitAfter(data, []byte("\n"))
	lines[1] = bytes.Replace(lines[1], []byte(`"action":"log.read"`), []byte(`"action":"control.status"`), 1)
	if err := os.WriteFile(files[0], bytes.Join(lines, nil), 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr = run(t, "audit", "verify")
	if code != 1 || !strings.Contains(stdout, "broken") || !strings.Contains(stdout, "entry with sequence 3 (line 3) does not chain") ||
		!strings.Contains(stderr, "the action log of the host service is broken") {
		t.Fatalf("changed log: code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestAuditIsReservedForAdministrators(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root is always served")
	}
	state := startHost(t, false)
	for _, args := range [][]string{{"audit"}, {"audit", "verify"}} {
		code, stdout, stderr := run(t, args...)
		if code != 1 || stdout != "" || !strings.Contains(stderr, "qhost is reserved for administrators (sudo)") || !strings.Contains(stderr, "permission") {
			t.Errorf("%v: code %d, stdout %q, stderr %q", args, code, stdout, stderr)
		}
	}
	// Once an administrator, the user sees the refusals (also of the readiness probe) in the order of the columns,
	// which the scenario relies on.
	me, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	group := filepath.Join(filepath.Dir(state), "group")
	if err := os.WriteFile(group, []byte("qhost:x:990:"+me.Username+"\nsudo:x:27:"+me.Username+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, _ := run(t, "--output", "json", "audit", "--action", "control.refuse", "--result", "failure")
	want := `"client_uid":` + me.Uid + `,"action":"control.refuse","result":"failure","error_code":"not-admin"`
	if code != 0 || strings.Count(strings.NewReplacer(" ", "", "\n", "").Replace(stdout), want) < 2 {
		t.Fatalf("code %d, refusals %q", code, stdout)
	}
}

func TestAuditWithoutTheHostService(t *testing.T) {
	useHostEndpoint(t, channeltest.Endpoint(t))
	code, _, stderr := run(t, "audit")
	if code != ExitServiceNotRunning || !strings.Contains(stderr, "the host service is not running") || !strings.Contains(stderr, "systemctl start qhost") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
}

func TestAuditShowsTheActionLogOfTheDeployService(t *testing.T) {
	startService(t, func(net.Conn) (uint32, error) { return 0, nil })
	if code, _, stderr := run(t, "credential", "list"); code != 0 {
		t.Fatalf("credential list: code %d, stderr %q", code, stderr)
	}
	code, stdout, stderr := run(t, "audit", "--service", "deploy", "--output", "json")
	if code != 0 || stderr != "" {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(stdout), &rows); err != nil {
		t.Fatalf("invalid JSON %q: %v", stdout, err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %v", rows)
	}
	r := rows[0]
	if r["service"] != "deploy" || r["engine"] != "system" || r["command"] != "credential.list" || r["class"] != "read" ||
		r["result"] != "success" || r["origin"] != "cli" || r["client_uid"] != 0.0 {
		t.Fatalf("row = %v", r)
	}
	code, stdout, stderr = run(t, "audit", "verify", "--service", "deploy", "--output", "json")
	if code != 0 || !strings.Contains(stdout, `"service":"deploy"`) || !strings.Contains(stdout, `"chain":"intact"`) {
		t.Fatalf("verify: code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if code, stdout, _ := run(t, "audit", "--service", "deploy", "--action", "status", "--result", "failure", "--output", "json"); code != 0 || strings.TrimSpace(stdout) != "[]" {
		t.Fatalf("filtered: code %d, stdout %q", code, stdout)
	}
}

func TestAuditChecksServiceAndEngine(t *testing.T) {
	for _, args := range [][]string{
		{"audit", "--service", "other"}, {"audit", "--engine", "system"}, {"audit", "verify", "--engine", "system"},
	} {
		if code, _, stderr := run(t, args...); code != 2 || !strings.Contains(stderr, "usage:") {
			t.Errorf("%v: code %d, stderr %q", args, code, stderr)
		}
	}
}
