package cli

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/castrowithcee/qatlas-host/internal/channeltest"
	"github.com/castrowithcee/qatlas-host/internal/diagnose"
	"github.com/castrowithcee/qatlas-host/internal/engine"
	"github.com/castrowithcee/qatlas-host/internal/stacks"
	"github.com/castrowithcee/qatlas-host/internal/userfile"
)

func useHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	old := diagnoseEnv
	diagnoseEnv = func() userfile.Env {
		return userfile.Env{Getenv: func(string) string { return "" }, Euid: os.Geteuid, Egid: os.Getegid,
			Lookup: func(uid int) (userfile.Account, error) {
				return userfile.Account{UID: uid, GID: os.Getgid(), Home: home}, nil
			}}
	}
	t.Cleanup(func() { diagnoseEnv = old })
	return home
}

// readArchive unpacks a package file into its files.
func readArchive(t *testing.T, path string) map[string]string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for tr := tar.NewReader(gz); ; {
		h, err := tr.Next()
		if err == io.EOF {
			return files
		}
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(tr)
		files[h.Name] = string(b)
	}
}

func diagnoseJSON(t *testing.T, args ...string) map[string]any {
	t.Helper()
	code, stdout, stderr := run(t, append([]string{"diagnose", "--output", "json"}, args...)...)
	var res map[string]any
	if err := json.Unmarshal([]byte(stdout), &res); code != 0 || err != nil {
		t.Fatalf("diagnose: %d %q %q %v", code, stdout, stderr, err)
	}
	return res
}

func TestDiagnoseWithoutRunningServiceWritesAPackageWithTheReason(t *testing.T) {
	home := useHome(t)
	useEndpoint(t, channeltest.Endpoint(t))
	res := diagnoseJSON(t)
	path, _ := res["path"].(string)
	if filepath.Dir(path) != filepath.Join(home, "qhost") || res["complete"] != false || !strings.Contains(res["notice"].(string), "host name") {
		t.Fatalf("%v", res)
	}
	files := readArchive(t, path)
	if !strings.Contains(files["services.txt"], "not running") || !strings.Contains(files["version.txt"], "qhost") {
		t.Fatalf("%q", files["services.txt"])
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("%v %v", info, err)
	}
}

func TestDiagnoseOutIsNewAndNeverReplaces(t *testing.T) {
	home := useHome(t)
	startPlainService(t)
	out := filepath.Join(home, "package.tar.gz")
	res := diagnoseJSON(t, "--out", out)
	if res["path"] != out {
		t.Fatalf("%v", res)
	}
	files := readArchive(t, out)
	if !strings.Contains(files["service-log.txt"], "service started") || !strings.Contains(files["services.txt"], "9.9.9") {
		t.Fatalf("%q", files["service-log.txt"])
	}
	before, _ := os.ReadFile(out)
	if code, stdout, stderr := run(t, "diagnose", "--out", out); code == 0 || stdout != "" || !strings.Contains(stderr, "exists") {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	if after, _ := os.ReadFile(out); string(after) != string(before) {
		t.Fatal("the existing file changed")
	}
	link := filepath.Join(home, "link")
	if err := os.Symlink(filepath.Join(home, "elsewhere"), link); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := run(t, "diagnose", "--out", link); code == 0 {
		t.Fatal("a link was followed")
	}
	if _, err := os.Stat(filepath.Join(home, "elsewhere")); err == nil {
		t.Fatal("the link target was created")
	}
	if code, _, _ := run(t, "diagnose", "extra"); code != 2 {
		t.Fatalf("code %d", code)
	}
}

func TestDiagnoseTableNamesThePathAndWarns(t *testing.T) {
	home := useHome(t)
	startPlainService(t)
	out := filepath.Join(home, "t.tar.gz")
	code, stdout, _ := run(t, "diagnose", "--out", out)
	if code != 0 || !strings.Contains(stdout, out) || !strings.Contains(stdout, "host name and paths") {
		t.Fatalf("%d %q", code, stdout)
	}
}

// leakyDeployer fails a run and writes the secrets into its log and its error once armed, so that runs
// before the secrets are set stay harmless.
type leakyDeployer struct {
	okDeployer
	armed   *atomic.Bool
	secrets []string
}

func (d leakyDeployer) Pull(_ context.Context, st stacks.Stack, log *engine.Log) error {
	if !d.armed.Load() {
		return nil
	}
	log.Step("pull with " + strings.Join(d.secrets, " and "))
	return errors.New("pull failed for " + strings.Join(d.secrets, " "))
}

func TestDiagnosePackageHoldsNoSecretValue(t *testing.T) {
	home := useHome(t)
	registryToken := "demo-registry-token-not-real"
	old := testDeployer
	var armed atomic.Bool
	testDeployer = leakyDeployer{armed: &armed, secrets: []string{fakeSecretValue, registryToken}}
	t.Cleanup(func() { testDeployer = old })
	var log syncBuffer
	startSecretService(t, &log)
	if code, _, stderr := runWithStdin(t, fakeSecretValue, "stack", "secret", "set", "plain-stack", "DB_PASSWORD"); code != 0 {
		t.Fatalf("secret: %s", stderr)
	}
	if code, _, stderr := runWithStdin(t, registryToken, "credential", "set", "registry.example"); code != 0 {
		t.Fatalf("credential: %s", stderr)
	}
	settle(t)
	armed.Store(true)
	if code, _, stderr := run(t, "stack", "deploy", "plain-stack"); code != 0 {
		t.Fatalf("deploy: %s", stderr)
	}
	var failed string
	for deadline := time.Now().Add(10 * time.Second); failed == ""; time.Sleep(20 * time.Millisecond) {
		_, stdout, _ := run(t, "stack", "runs", "--output", "json")
		var rows []map[string]any
		_ = json.Unmarshal([]byte(stdout), &rows)
		for _, r := range rows {
			if r["result"] == engine.ResultFailed || r["result"] == engine.ResultRolledBack {
				failed = r["run"].(string)
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("no failed run: %s", stdout)
		}
	}
	res := diagnoseJSON(t, "--out", filepath.Join(home, "p.tar.gz"))
	files := readArchive(t, res["path"].(string))
	for _, part := range diagnose.Parts {
		if files[part+".txt"] == "" || files[part+".json"] == "" {
			t.Errorf("part %s is missing", part)
		}
	}
	for name, content := range files {
		for _, secret := range []string{fakeSecretValue, registryToken} {
			if strings.Contains(content, secret) {
				t.Errorf("%s holds the secret value %q: %s", name, secret, content)
			}
		}
	}
	if !strings.Contains(files["run-logs.json"], failed) || !strings.Contains(files["run-logs.txt"], "pull with") {
		t.Errorf("the log of the failed run %s is missing:\n%s", failed, files["run-logs.txt"])
	}
}
