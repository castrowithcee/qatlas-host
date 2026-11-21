package cli

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/hookservice"
)

func useHookService(t *testing.T, mode os.FileMode) {
	t.Helper()
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.json")
	data, err := json.Marshal(hookservice.Config{Version: 1, Listen: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(cfg, mode); err != nil {
		t.Fatal(err)
	}
	old := hookServiceOptions
	hookServiceOptions = func() hookservice.Options {
		return hookservice.Options{ConfigPath: cfg, RunDir: filepath.Join(dir, "run"), OwnerUID: uint32(os.Getuid())}
	}
	t.Cleanup(func() { hookServiceOptions = old })
}

func TestHookServiceRunEndsCleanlyOnSignal(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGTERM, syscall.SIGINT} {
		t.Run(sig.String(), func(t *testing.T) {
			runEndsOnSignal(t, func(t *testing.T) { useHookService(t, 0o644) }, []string{"hook-service", "run"}, sig)
		})
	}
}

func TestHookServiceRunRefusesAWritableConfiguration(t *testing.T) {
	useHookService(t, 0o666)
	var stderr strings.Builder
	if code := Run([]string{"hook-service", "run"}, io.Discard, &stderr); code == 0 {
		t.Fatalf("started with a world-writable configuration: %s", stderr.String())
	}
}

func TestHookServiceIsListedButItsRunIsHidden(t *testing.T) {
	var stdout strings.Builder
	if code := Run([]string{"--help"}, &stdout, io.Discard); code != 0 || !strings.Contains(stdout.String(), "hook-service") {
		t.Fatalf("code %d, help does not list hook-service: %s", code, stdout.String())
	}
	stdout.Reset()
	if code := Run([]string{"hook-service", "--help"}, &stdout, io.Discard); code != 0 ||
		!strings.Contains(stdout.String(), "setup") || strings.Contains(stdout.String(), "Run the webhook receiver in the foreground") {
		t.Fatalf("code %d, help of hook-service: %s", code, stdout.String())
	}
}
