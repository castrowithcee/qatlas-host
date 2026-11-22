package cli

import (
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/castrowithcee/qatlas-host/internal/channeltest"
	"github.com/castrowithcee/qatlas-host/internal/window"
)

func objectOf(t *testing.T, out string) map[string]any {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output = %q: %v", out, err)
	}
	return got
}

// useHostWindow lets the command act as root on a host window file in a temporary folder and returns its path.
func useHostWindow(t *testing.T) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "etc", "window.json")
	oldFile, oldEuid := windowHostFile, windowEuid
	windowHostFile, windowEuid = func() string { return file }, func() int { return 0 }
	t.Cleanup(func() { windowHostFile, windowEuid = oldFile, oldEuid })
	return file
}

func ptr[T any](v T) *T { return &v }

func windowRowsOf(t *testing.T, out string) []map[string]any {
	t.Helper()
	var got []map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output = %q: %v", out, err)
	}
	return got
}

func TestWindowShowListsTheInstanceWithItsOrigin(t *testing.T) {
	useHostWindow(t)
	startService(t, func(net.Conn) (uint32, error) { return 0, nil })
	code, out, stderr := run(t, "window", "show", "--output", "json")
	if code != 0 {
		t.Fatalf("show: %d %s", code, stderr)
	}
	rows := windowRowsOf(t, out)
	if len(rows) != 1 {
		t.Fatalf("rows = %v", rows)
	}
	got := rows[0]
	if got["engine"] != "system" || got["origin"] != "host" || got["days"] != "mon,tue,wed,thu,fri,sat,sun" ||
		got["start"] != "03:00" || got["duration"] != "2h" || got["zone"] != "local" {
		t.Fatalf("show = %v", got)
	}
	next, err := time.Parse(time.RFC3339, got["next_opening"].(string))
	if h, m, _ := next.In(time.Local).Clock(); err != nil || h != 3 || m != 0 || !next.After(time.Now()) {
		t.Fatalf("next_opening = %v, %v", got["next_opening"], err)
	}
	if code, out, _ := run(t, "window", "show"); code != 0 || !strings.Contains(out, "ORIGIN") || !strings.Contains(out, "host") {
		t.Fatalf("show table: %d %q", code, out)
	}
}

func TestWindowSetWithoutAnEngineWritesTheHostFileAndItsInstancesFollow(t *testing.T) {
	file := useHostWindow(t)
	startService(t, func(net.Conn) (uint32, error) { return 0, nil })
	code, out, stderr := run(t, "window", "set", "--start", "04:30", "--zone", "UTC", "--duration", "1h30m", "--output", "json")
	if code != 0 {
		t.Fatalf("set: %d %s", code, stderr)
	}
	got := objectOf(t, out)
	if got["origin"] != "host" || got["days"] != "mon,tue,wed,thu,fri,sat,sun" || got["start"] != "04:30" ||
		got["duration"] != "1h30m" || got["zone"] != "UTC" || !strings.HasSuffix(got["next_opening"].(string), "T04:30:00Z") {
		t.Fatalf("set = %v", got)
	}
	w, ok, err := window.ReadHost(file)
	if err != nil || !ok || w.Start != 270 || w.Zone != "UTC" {
		t.Fatalf("file = %+v, %v, %v", w, ok, err)
	}
	// a partial change keeps the other parts
	if code, out, stderr := run(t, "window", "set", "--days", "sat, Sunday"); code != 0 || !strings.Contains(out, "sat,sun") || !strings.Contains(out, "04:30") {
		t.Fatalf("set days: %d %q %s", code, out, stderr)
	}
	// the service follows the file without a restart
	var shown map[string]any
	for deadline := time.Now().Add(30 * time.Second); ; time.Sleep(100 * time.Millisecond) {
		_, out, _ := run(t, "window", "show", "--output", "json")
		if rows := windowRowsOf(t, out); len(rows) == 1 && rows[0]["days"] == "sat,sun" {
			shown = rows[0]
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the service never followed the file: %s", out)
		}
	}
	if shown["origin"] != "host" || shown["start"] != "04:30" || shown["zone"] != "UTC" {
		t.Fatalf("shown = %v", shown)
	}
	if code, out, _ := run(t, "status", "--output", "json"); code != 0 || objectOf(t, out)["window_next_opening"] == nil {
		t.Fatalf("status: %d %q", code, out)
	}
}

func TestWindowSetWithAnEngineSetsTheOwnWindowAndResetFollowsTheHostAgain(t *testing.T) {
	file := useHostWindow(t)
	startService(t, func(net.Conn) (uint32, error) { return 0, nil })
	if code, _, stderr := run(t, "window", "set", "--start", "05:00", "--zone", "UTC"); code != 0 {
		t.Fatalf("host: %d %s", code, stderr)
	}
	code, out, stderr := run(t, "window", "set", "--engine", "system", "--duration", "30m", "--output", "json")
	if code != 0 {
		t.Fatalf("own: %d %s", code, stderr)
	}
	got := objectOf(t, out)
	if got["engine"] != "system" || got["origin"] != "own" || got["duration"] != "30m" || got["start"] != "05:00" {
		t.Fatalf("own = %v", got)
	}
	// the host file was not touched by the own window
	if w, _, _ := window.ReadHost(file); w.Duration != 2*time.Hour {
		t.Fatalf("host file = %+v", w)
	}
	if _, out, _ := run(t, "window", "show", "--output", "json"); windowRowsOf(t, out)[0]["origin"] != "own" {
		t.Fatalf("show = %s", out)
	}
	code, out, stderr = run(t, "window", "reset", "--engine", "system", "--output", "json")
	if code != 0 {
		t.Fatalf("reset: %d %s", code, stderr)
	}
	if got := objectOf(t, out); got["origin"] != "host" || got["duration"] != "2h" || got["start"] != "05:00" {
		t.Fatalf("reset = %v", got)
	}
}

func TestWindowSetOfTheHostNeedsRoot(t *testing.T) {
	file := useHostWindow(t)
	startService(t, func(net.Conn) (uint32, error) { return 0, nil })
	windowEuid = func() int { return 1000 }
	code, out, stderr := run(t, "window", "set", "--start", "04:30")
	if code == 0 || out != "" || !strings.Contains(stderr, "needs root") {
		t.Fatalf("%d %q %q", code, out, stderr)
	}
	if _, ok, _ := window.ReadHost(file); ok {
		t.Fatal("the file was written without root")
	}
}

func TestWindowSetAdoptsTheWindowTheServiceKeptBefore(t *testing.T) {
	file := useHostWindow(t)
	dir := startServiceLogged(t, func(net.Conn) (uint32, error) { return 0, nil }, io.Discard)
	// the window of an earlier version: stored by the system service, no host file yet
	legacy := `{"schema":1,"days":["fri"],"start":"01:15","duration":"3h","zone":"UTC"}`
	if err := os.WriteFile(filepath.Join(dir, "window.json"), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	code, out, stderr := run(t, "window", "set", "--duration", "1h", "--output", "json")
	if code != 0 {
		t.Fatalf("set: %d %s", code, stderr)
	}
	if got := objectOf(t, out); got["days"] != "fri" || got["start"] != "01:15" || got["duration"] != "1h" {
		t.Fatalf("set = %v", got)
	}
	if w, ok, err := window.ReadHost(file); err != nil || !ok || w.Start != 75 {
		t.Fatalf("host file = %+v, %v, %v", w, ok, err)
	}
}

func TestWindowSetWithADamagedHostFileRefusesAndKeepsIt(t *testing.T) {
	file := useHostWindow(t)
	startService(t, func(net.Conn) (uint32, error) { return 0, nil })
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(`{"schema":9}`), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, stderr := run(t, "window", "set", "--start", "04:30")
	if code == 0 || out != "" || !strings.Contains(stderr, "format number") {
		t.Fatalf("%d %q %q", code, out, stderr)
	}
	if b, _ := os.ReadFile(file); string(b) != `{"schema":9}` {
		t.Fatalf("the file changed: %s", b)
	}
}

func TestWindowResetNeedsAnEngine(t *testing.T) {
	startService(t, func(net.Conn) (uint32, error) { return 0, nil })
	code, out, stderr := run(t, "window", "reset")
	if code != 2 || out != "" || !strings.Contains(stderr, "--engine") {
		t.Fatalf("%d %q %q", code, out, stderr)
	}
}

func TestWindowSetErrors(t *testing.T) {
	file := useHostWindow(t)
	startService(t, func(net.Conn) (uint32, error) { return 0, nil })
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"window", "set"}, "at least one"},
		{[]string{"window", "set", "--start", "25:00"}, "HH:MM"},
		{[]string{"window", "set", "--days", "funday"}, "weekday"},
		{[]string{"window", "set", "--days", ""}, "weekday"},
		{[]string{"window", "set", "--duration", "13h"}, "duration"},
		{[]string{"window", "set", "--zone", "Nowhere/Atlantis"}, "zone"},
		{[]string{"window", "set", "--zone", ""}, "--zone needs a value"},
		{[]string{"window", "set", "--engine", "system", "--start", "25:00"}, "HH:MM"},
		{[]string{"window", "show", "extra"}, ""},
	} {
		code, out, stderr := run(t, tc.args...)
		if code != 2 || out != "" || !strings.Contains(stderr, tc.want) {
			t.Fatalf("%v: %d %q %q", tc.args, code, out, stderr)
		}
	}
	if _, ok, _ := window.ReadHost(file); ok {
		t.Fatal("a refused change wrote the host file")
	}
	if code, out, _ := run(t, "window", "show", "--output", "json"); code != 0 || windowRowsOf(t, out)[0]["start"] != "03:00" {
		t.Fatalf("refused changes were stored: %d %q", code, out)
	}
}

func TestWindowWithoutRunningService(t *testing.T) {
	useEndpoint(t, channeltest.Endpoint(t))
	if code, _, stderr := run(t, "window", "show"); code != ExitServiceNotRunning || !strings.Contains(stderr, "not running") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
}
