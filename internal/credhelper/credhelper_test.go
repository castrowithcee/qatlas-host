package credhelper_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/credentials"
	"github.com/castrowithcee/qatlas-host/internal/credhelper"
)

const demoLogin = "demo-user:demo-token-0000"

func newStore(t *testing.T) (*credentials.Store, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "state")
	s, err := credentials.Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	return s, dir
}

func run(t *testing.T, dir, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	env := func(k string) string {
		if k == credhelper.StateDirEnv {
			return dir
		}
		return ""
	}
	code := credhelper.Run(args, strings.NewReader(stdin), &out, &errb, env)
	if strings.Contains(errb.String(), "demo-token") || strings.Contains(errb.String(), "demo-user") {
		t.Errorf("stderr leaks the login: %q", errb.String())
	}
	return code, out.String(), errb.String()
}

func TestGet(t *testing.T) {
	s, dir := newStore(t)
	for name, login := range map[string]string{
		"registry.ghcr.io":           demoLogin,
		"registry.docker.io":         "hub-user:hub-token-0000",
		"registry.host.example-5000": "local-user:local-token-0000",
	} {
		if err := s.Set(name, login); err != nil {
			t.Fatal(err)
		}
	}
	for server, user := range map[string]string{
		"ghcr.io":                     "demo-user",
		"https://ghcr.io":             "demo-user",
		"GHCR.IO":                     "demo-user",
		"https://index.docker.io/v1/": "hub-user",
		"registry-1.docker.io":        "hub-user",
		"host.example:5000":           "local-user",
	} {
		code, out, _ := run(t, dir, server+"\n", "get")
		var got struct{ ServerURL, Username, Secret string }
		if code != 0 || json.Unmarshal([]byte(out), &got) != nil || got.Username != user ||
			!strings.HasSuffix(got.Secret, "-0000") || got.ServerURL != server {
			t.Errorf("get %q = %d %q", server, code, out)
		}
	}
}

func TestGetNotFound(t *testing.T) {
	_, dir := newStore(t)
	code, out, _ := run(t, dir, "ghcr.io", "get")
	if code != 1 || out != "credentials not found in native keychain\n" {
		t.Errorf("got %d %q", code, out)
	}
	// A state directory that does not exist yet holds no login either.
	code, out, _ = run(t, filepath.Join(t.TempDir(), "none"), "ghcr.io", "get")
	if code != 1 || !strings.HasPrefix(out, "credentials not found") {
		t.Errorf("missing dir: %d %q", code, out)
	}
}

func TestGetFailsClosed(t *testing.T) {
	s, dir := newStore(t)
	if err := s.Set("registry.ghcr.io", "no-colon-here"); err != nil {
		t.Fatal(err)
	}
	code, out, errText := run(t, dir, "ghcr.io", "get")
	if code != 1 || out != "" || errText == "" || strings.Contains(errText, "no-colon") {
		t.Errorf("malformed: %d %q %q", code, out, errText)
	}
	if err := s.Set("registry.docker.io", demoLogin); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(dir, "credentials"), 0o755); err != nil {
		t.Fatal(err)
	}
	code, out, _ = run(t, dir, "docker.io", "get")
	if code != 1 || out != "" {
		t.Errorf("unsafe dir: %d %q", code, out)
	}
	for _, d := range []string{"", "relative/state"} {
		if code, out, _ = run(t, d, "ghcr.io", "get"); code != 1 || out != "" {
			t.Errorf("dir %q: %d %q", d, code, out)
		}
	}
	if code, out, _ = run(t, dir, strings.Repeat("a", 5000), "get"); code != 1 || out != "" {
		t.Errorf("long input: %d %q", code, out)
	}
	if code, _, _ = run(t, dir, "", "get"); code != 1 {
		t.Errorf("empty input: %d", code)
	}
}

func TestListStoreEraseAndWritesNothing(t *testing.T) {
	s, dir := newStore(t)
	if err := s.Set("registry.ghcr.io", demoLogin); err != nil {
		t.Fatal(err)
	}
	if code, out, _ := run(t, dir, "", "list"); code != 0 || out != "{}\n" {
		t.Errorf("list: %d %q", code, out)
	}
	payload := `{"ServerURL":"ghcr.io","Username":"x-user"}`
	for _, cmd := range []string{"store", "erase", "bogus"} {
		code, out, errText := run(t, dir, payload, cmd)
		if code != 1 || out != "" || errText == "" || strings.Contains(errText, "x-user") {
			t.Errorf("%s: %d %q %q", cmd, code, out, errText)
		}
	}
	if code, _, _ := run(t, dir, "", "store", "extra"); code != 1 {
		t.Errorf("extra args: %d", code)
	}
	if v, err := s.Value("registry.ghcr.io"); err != nil || v != demoLogin {
		t.Errorf("store changed: %q %v", v, err)
	}
	entries, err := s.List()
	if err != nil || len(entries) != 1 {
		t.Errorf("entries %v %v", entries, err)
	}
}
