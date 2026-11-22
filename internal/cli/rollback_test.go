package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRollbackNeedsExactlyOneDataChoice(t *testing.T) {
	const fakeRun = "20240101T000000Z-0123abcd"
	startUpdate(t, okDeployer{})
	for _, args := range [][]string{
		{"stack", "rollback", "demo-stack", fakeRun, "--yes"},
		{"stack", "rollback", "demo-stack", fakeRun, "--restore-data", "--keep-data", "--yes"},
	} {
		if code, _, stderr := run(t, args...); code == 0 || !strings.Contains(stderr, "exactly one of --restore-data and --keep-data") {
			t.Fatalf("%v: %d %q", args, code, stderr)
		}
	}
	if code, _, stderr := run(t, "stack", "rollback", "demo-stack", "--keep-data"); code == 0 || !strings.Contains(stderr, "exactly two arguments") {
		t.Fatalf("one argument: %d %q", code, stderr)
	}
}

func TestRollbackWithoutTerminalNeedsYes(t *testing.T) {
	startUpdate(t, okDeployer{})
	code, out, stderr := run(t, "stack", "update", "demo-stack", "--output", "json")
	if code != 0 {
		t.Fatalf("update: %d %s", code, stderr)
	}
	id, _ := rowsOf(t, out)[0]["run"].(string)

	code, stdout, stderr := run(t, "stack", "rollback", "demo-stack", id, "--keep-data", "--agent")
	if code == 0 || stdout != "" || !strings.Contains(stderr, "--yes") {
		t.Fatalf("without --yes: %d %q %q", code, stdout, stderr)
	}
	if code, out, _ := run(t, "stack", "update", "demo-stack"); code != 0 || !strings.Contains(out, "current") {
		t.Fatalf("a refused return changed the stack: %d %q", code, out)
	}
}

func TestRollbackReturnsAndPinsTheStack(t *testing.T) {
	startUpdate(t, okDeployer{})
	code, out, stderr := run(t, "stack", "update", "demo-stack", "--output", "json")
	if code != 0 {
		t.Fatalf("update: %d %s", code, stderr)
	}
	id, _ := rowsOf(t, out)[0]["run"].(string)

	code, out, stderr = run(t, "stack", "rollback", "demo-stack", id, "--keep-data", "--yes", "--output", "json")
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); code != 0 || err != nil {
		t.Fatalf("rollback: %d %q %q %v", code, out, stderr, err)
	}
	if got["result"] != "succeeded" || got["pinned"] != true || got["update_run"] != id || got["data"] != "keep" ||
		got["versions"] != "DEMO_APP_VERSION 1.3.0 -> 1.2.3" || !strings.Contains(got["note"].(string), "qhost stack unpin demo-stack") {
		t.Fatalf("result = %v", got)
	}
	if code, _, stderr := run(t, "stack", "update", "demo-stack"); code == 0 || !strings.Contains(stderr, "pinned") {
		t.Fatalf("update of the pinned stack: %d %q", code, stderr)
	}
	if code, _, stderr := run(t, "stack", "rollback", "demo-stack", id, "--keep-data", "--yes"); code == 0 || !strings.Contains(stderr, "already has the versions") {
		t.Fatalf("second return: %d %q", code, stderr)
	}
	if code, _, stderr := run(t, "stack", "rollback", "demo-stack", got["run"].(string), "--keep-data", "--yes"); code == 0 || !strings.Contains(stderr, "not a successful update") {
		t.Fatalf("return run as target: %d %q", code, stderr)
	}
}
