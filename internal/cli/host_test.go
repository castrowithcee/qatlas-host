package cli

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/castrowithcee/qatlas-host/internal/host/hostadmin"
)

// useHostFixture writes invented databases and makes the host commands read them as a caller with the given UID.
// The caller 1000 is an administrator, 1001 is not. It returns the public key blob of demo-admin.
func useHostFixture(t *testing.T, uid int) (keyBlob string, h *hostadmin.Host) {
	t.Helper()
	dir := t.TempDir()
	home := filepath.Join(dir, "home", "demo-admin")
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o755); err != nil {
		t.Fatal(err)
	}
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	keyBlob = base64.StdEncoding.EncodeToString(key.Marshal())
	files := map[string]string{
		"passwd.db": "root:x:0:0::/root:/bin/bash\n" +
			"demo-admin:x:1000:1000::" + home + ":/bin/bash\n" +
			"demo-user:x:1001:1001::" + filepath.Join(dir, "home", "demo-user") + ":/bin/bash\n",
		"group.db":  "root:x:0:\nsudo:x:27:demo-admin\nqhost:x:990:demo-admin,demo-user\ndemo-admin:x:1000:\ndemo-user:x:1001:\n",
		"shadow.db": "root:*:19000:::::\ndemo-admin:$6$fake:19000:::::\ndemo-user:!$6$fake:19000:::::\n",
		filepath.Join("home", "demo-admin", ".ssh", "authorized_keys"): "ssh-ed25519 " + keyBlob + " demo-admin@demo-laptop\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	old := hostEnv
	t.Cleanup(func() { hostEnv = old })
	hostEnv = hostEnvironment{
		euid:  func() int { return uid },
		admin: func(u int) (bool, error) { return u == 1000, nil },
		accounts: func(privileged bool) *hostadmin.Host {
			h = hostadmin.NewHost(privileged)
			h.PasswdPath, h.GroupPath, h.ShadowPath = filepath.Join(dir, "passwd.db"), filepath.Join(dir, "group.db"), filepath.Join(dir, "shadow.db")
			return h
		},
	}
	return keyBlob, nil
}

func TestHostRefusesCallersWithoutAdminRights(t *testing.T) {
	useHostFixture(t, 1001)
	for _, args := range [][]string{
		{"host", "users"}, {"host", "user", "demo-user"}, {"host", "groups"}, {"host", "group", "sudo"},
		{"host", "folder", "/"}, {"host", "apply", "demo.set", "--confirm"}, {"host", "journal"},
	} {
		code, out, errOut := run(t, args...)
		if code != 1 || out != "" || !strings.Contains(errOut, "qhost is reserved for administrators (sudo)") {
			t.Errorf("%v: code %d, stdout %q, stderr %q", args, code, out, errOut)
		}
	}
	// The command line is checked first, so a mistyped one shows its usage.
	if code, _, errOut := run(t, "host", "users", "extra"); code != 2 || strings.Contains(errOut, "reserved for administrators") {
		t.Errorf("extra argument: code %d, stderr %q", code, errOut)
	}
}

func TestHostAdminWithoutRootSeesUnknown(t *testing.T) {
	blob, _ := useHostFixture(t, 1000)
	code, out, errOut := run(t, "host", "user", "demo-admin", "--output", "json")
	if code != 0 || errOut != "" {
		t.Fatalf("code %d, stderr %q", code, errOut)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"locked", "expired", "ssh_keys"} {
		if v, ok := got[field]; ok && v != nil {
			t.Errorf("%s = %v, want it unknown", field, v)
		}
	}
	if got["admin"] != true || got["primary_group"] != "demo-admin" || strings.Contains(out, blob) {
		t.Errorf("user = %v", got)
	}
	if _, table, _ := run(t, "host", "user", "demo-admin", "--output", "table"); !strings.Contains(table, "unknown") {
		t.Errorf("table does not say unknown:\n%s", table)
	}
}

func TestHostRootSeesLockedAndKeysButNeverTheKey(t *testing.T) {
	blob, _ := useHostFixture(t, 0)
	for _, format := range []string{"json", "table", "compact"} {
		for _, args := range [][]string{{"host", "user", "demo-admin"}, {"host", "users"}} {
			code, out, errOut := run(t, append(args, "--output", format)...)
			if code != 0 || errOut != "" || strings.Contains(out, blob) || strings.Contains(out, "unknown") {
				t.Fatalf("%v %s: code %d, stderr %q, out %q", args, format, code, errOut, out)
			}
		}
	}
	_, out, _ := run(t, "host", "user", "demo-admin", "--output", "json")
	var got struct {
		Locked *bool              `json:"locked"`
		Keys   []hostadmin.SSHKey `json:"ssh_keys"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil || got.Locked == nil || *got.Locked || len(got.Keys) != 1 ||
		got.Keys[0].Comment != "demo-admin@demo-laptop" || !strings.HasPrefix(got.Keys[0].Fingerprint, "SHA256:") {
		t.Errorf("user = %s (%v)", out, err)
	}
	if _, out, _ = run(t, "host", "user", "demo-user", "--output", "json"); !strings.Contains(out, `"locked":true`) {
		t.Errorf("demo-user not locked: %s", out)
	}
}

func TestHostGroupsAndFolder(t *testing.T) {
	useHostFixture(t, 1000)
	_, out, _ := run(t, "host", "group", "demo-user", "--output", "json")
	if out != `{"name":"demo-user","gid":1001,"members":["demo-user"],"system":false}`+"\n" {
		t.Errorf("group = %s", out)
	}
	if code, out, _ := run(t, "host", "groups", "--output", "json"); code != 0 || !strings.HasPrefix(out, "[{") {
		t.Errorf("groups = %d %s", code, out)
	}
	code, out, _ := run(t, "host", "folder", t.TempDir(), "--output", "json")
	var f map[string]any
	if err := json.Unmarshal([]byte(out), &f); code != 0 || err != nil || f["kind"] != "directory" || f["protected"] != false {
		t.Errorf("folder = %d %v %v", code, f, err)
	}
	if _, out, _ = run(t, "host", "folder", "/etc/../etc", "--output", "json"); !strings.Contains(out, `"protected":true`) {
		t.Errorf("/etc not protected: %s", out)
	}
}

func TestHostUnknownTargets(t *testing.T) {
	useHostFixture(t, 1000)
	for _, args := range [][]string{
		{"host", "user", "nobody-here"}, {"host", "group", "nobody-here"}, {"host", "folder", "/srv/demo-missing-path"},
	} {
		code, out, errOut := run(t, args...)
		if code != 1 || out != "" || !strings.Contains(errOut, "not found") {
			t.Errorf("%v: code %d, stdout %q, stderr %q", args, code, out, errOut)
		}
	}
	if code, _, _ := run(t, "host", "user"); code != 2 {
		t.Errorf("missing argument exits %d, want 2", code)
	}
}
