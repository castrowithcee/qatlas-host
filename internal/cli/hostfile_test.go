package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/host/hostchange"
)

func hostFileTree(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o640); err != nil {
			t.Fatal(err)
		}
	}
	write("demo.conf", "DEMO=1\nline two\n")
	write("big.txt", strings.Repeat("x", hostchange.MaxFileContent+1))
	write("blob.bin", "ab\x00cd")
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/demo-secret-target", filepath.Join(dir, "demo-link")); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestHostFileListsAFolderAndShowsAFile(t *testing.T) {
	useHostFixture(t, 0)
	dir := hostFileTree(t)

	code, out, errOut := run(t, "host", "file", dir, "--output", "json")
	if code != 0 {
		t.Fatalf("code %d err %q", code, errOut)
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("not JSON: %q: %v", out, err)
	}
	byName := map[string]map[string]any{}
	for _, r := range rows {
		byName[r["name"].(string)] = r
	}
	want := map[string]string{"demo.conf": "file", "big.txt": "file", "blob.bin": "file", "sub": "folder", "demo-link": "link"}
	if len(byName) != len(want) {
		t.Fatalf("entries %v", byName)
	}
	for name, kind := range want {
		if byName[name]["kind"] != kind {
			t.Errorf("%s: %v", name, byName[name])
		}
	}
	if r := byName["demo.conf"]; r["size"] != float64(16) || r["mode"] != "0640" || r["owner"] == "" || r["group"] == "" || r["modified"] == "" {
		t.Errorf("demo.conf: %v", r)
	}
	if byName["demo-link"]["target"] != "/etc/demo-secret-target" {
		t.Errorf("link: %v", byName["demo-link"])
	}

	code, out, errOut = run(t, "host", "file", filepath.Join(dir, "demo.conf"), "--output", "json")
	if code != 0 {
		t.Fatalf("code %d err %q", code, errOut)
	}
	m := decode(t, out)
	if m["content"] != "DEMO=1\nline two\n" || m["kind"] != "file" || m["too_large"] != false || m["binary"] != false || m["mode"] != "0640" {
		t.Errorf("file: %v", m)
	}
}

func TestHostFileFlagsLargeAndBinaryFilesWithoutContent(t *testing.T) {
	useHostFixture(t, 0)
	dir := hostFileTree(t)
	for name, flag := range map[string]string{"big.txt": "too_large", "blob.bin": "binary"} {
		code, out, errOut := run(t, "host", "file", filepath.Join(dir, name), "--output", "json")
		if code != 0 {
			t.Fatalf("%s: code %d err %q", name, code, errOut)
		}
		m := decode(t, out)
		if m[flag] != true || m["content"] != "" {
			t.Errorf("%s: %v", name, m)
		}
	}
}

func TestHostFileDoesNotFollowLinks(t *testing.T) {
	useHostFixture(t, 0)
	dir := hostFileTree(t)
	secret := filepath.Join(dir, "sub", "secret.txt")
	if err := os.WriteFile(secret, []byte("demo-secret-1234"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "sub"), filepath.Join(dir, "sublink")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(dir, "filelink")); err != nil {
		t.Fatal(err)
	}

	code, out, _ := run(t, "host", "file", filepath.Join(dir, "filelink"), "--output", "json")
	m := decode(t, out)
	if code != 0 || m["kind"] != "link" || m["target"] != secret || m["content"] != "" || strings.Contains(out, "demo-secret-1234") {
		t.Errorf("link as the path: code %d %v", code, m)
	}
	code, out, errOut := run(t, "host", "file", filepath.Join(dir, "sublink", "secret.txt"), "--output", "json")
	if code == 0 || strings.Contains(out+errOut, "demo-secret-1234") || !strings.Contains(errOut, "link") {
		t.Errorf("link on the way: code %d out %q err %q", code, out, errOut)
	}
}

func TestHostFileNeedsRootAndAbsoluteCleanPath(t *testing.T) {
	useHostFixture(t, 1000)
	dir := hostFileTree(t)
	code, out, errOut := run(t, "host", "file", dir)
	if code == 0 || out != "" || !strings.Contains(errOut, "must run as root") {
		t.Errorf("not root: code %d out %q err %q", code, out, errOut)
	}
	hostEnv.euid = func() int { return 0 }
	for _, p := range []string{"relative/path", dir + "/../x", filepath.Join(dir, "missing")} {
		if code, out, _ = run(t, "host", "file", p); code == 0 || out != "" {
			t.Errorf("%s: code %d out %q", p, code, out)
		}
	}
}

func TestHostFileReadsInEveryFormat(t *testing.T) {
	useHostFixture(t, 0)
	dir := hostFileTree(t)
	for _, format := range []string{"table", "compact"} {
		if code, out, errOut := run(t, "host", "file", dir, "--output", format); code != 0 || !strings.Contains(out, "demo.conf") {
			t.Errorf("%s folder: code %d out %q err %q", format, code, out, errOut)
		}
		if code, out, errOut := run(t, "host", "file", filepath.Join(dir, "demo.conf"), "--output", format); code != 0 || !strings.Contains(out, "line two") {
			t.Errorf("%s file: code %d out %q err %q", format, code, out, errOut)
		}
	}
}
