package cli

import (
	"bytes"
	"encoding/json"
	"net"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/castrowithcee/qatlas-host/internal/channeltest"
)

// lockedBuffer is an output a test reads while a command still writes to it.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

func startPlainService(t *testing.T) {
	t.Helper()
	startService(t, func(net.Conn) (uint32, error) { return 0, nil })
}

func TestServiceLogShowsTheStartOfTheService(t *testing.T) {
	startPlainService(t)
	code, out, stderr := run(t, "service", "log")
	if code != 0 || stderr != "" || !strings.Contains(out, " INFO  service started") {
		t.Fatalf("%d %q %s", code, out, stderr)
	}
	if code, out, _ = run(t, "service", "log", "--level", "warn"); code != 0 || strings.Contains(out, "service started") {
		t.Fatalf("level: %d %q", code, out)
	}
	if code, out, _ = run(t, "service", "log", "--since", "1h", "--tail", "1", "--source", "qhost"); code != 0 || strings.Count(out, "\n") != 1 {
		t.Fatalf("since and tail: %d %q", code, out)
	}
	if code, out, _ = run(t, "service", "log", "--since", "2030-01-01T00:00:00Z"); code != 0 || out != "" {
		t.Fatalf("future since: %d %q", code, out)
	}
}

func TestServiceLogJSON(t *testing.T) {
	startPlainService(t)
	code, out, stderr := run(t, "service", "log", "--output", "json", "--level", "info")
	var first struct{ Time, Level, Source, Message string }
	line, _, _ := strings.Cut(out, "\n")
	if err := json.Unmarshal([]byte(line), &first); code != 0 || err != nil || first.Message != "service started" ||
		first.Level != "info" || first.Source != "qhost" || first.Time == "" {
		t.Fatalf("json: %d %q %s %v", code, out, stderr, err)
	}
}

func TestServiceLogArgumentErrors(t *testing.T) {
	startPlainService(t)
	for _, args := range [][]string{
		{"service", "log", "--level", "loud"},
		{"service", "log", "--since", "never"},
		{"service", "log", "--tail", "99999"},
		{"service", "log", "--tail", "-1"},
		{"service", "log", "extra"},
	} {
		if code, out, stderr := run(t, args...); code == 0 || out != "" || stderr == "" {
			t.Errorf("%v: %d %q %q", args, code, out, stderr)
		}
	}
	if code, _, stderr := run(t, "service", "log", "--source", "other"); code == 0 || stderr == "" {
		t.Errorf("unknown source: %d %q", code, stderr)
	}
}

func TestServiceLogWithoutRunningService(t *testing.T) {
	useEndpoint(t, channeltest.Endpoint(t))
	if code, out, stderr := run(t, "service", "log"); code != ExitServiceNotRunning || out != "" || !strings.Contains(stderr, "not running") {
		t.Fatalf("%d %q %q", code, out, stderr)
	}
}

func TestServiceLogFollowContinuesAtCursorAndEndsOnInterrupt(t *testing.T) {
	old := logsPollInterval
	logsPollInterval = 10 * time.Millisecond
	t.Cleanup(func() { logsPollInterval = old })
	f := sampleEngine()
	startContainers(t, f)

	out, errOut := &lockedBuffer{}, &lockedBuffer{}
	done := make(chan int, 1)
	go func() { done <- Run([]string{"service", "log", "--follow", "--level", "info"}, out, errOut) }()
	waitFor(t, func() bool { return strings.Contains(out.String(), "service started") })
	// A later entry of the service arrives while the command follows.
	if code, _, stderr := run(t, "container", "restart", "lonely", "--yes"); code != 0 {
		t.Fatalf("restart: %d %s", code, stderr)
	}
	waitFor(t, func() bool { return strings.Contains(out.String(), "container action") })
	time.Sleep(50 * time.Millisecond) // more polls: nothing may repeat
	_ = syscall.Kill(os.Getpid(), syscall.SIGINT)
	select {
	case code := <-done:
		got := out.String()
		if code != 0 || errOut.String() != "" || strings.Count(got, "service started") != 1 || strings.Count(got, "container action") != 1 {
			t.Fatalf("%d %q %q", code, got, errOut.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("follow did not end on interrupt")
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		if cond() {
			return
		}
	}
	t.Fatal("condition not reached in time")
}
