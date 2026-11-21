package hookservice

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newSecret makes a fresh secret at run time, so no key-like literal sits in the source.
func newSecret(t *testing.T) string {
	t.Helper()
	b := make([]byte, secretBytes)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func hashOf(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func hookFor(id, stack, instance, secret string) Hook {
	return Hook{ID: id, Stack: stack, Instance: instance, Hash: hashOf(secret)}
}

func writeConfig(t *testing.T, path string, c any, mode os.FileMode) {
	t.Helper()
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	writeRaw(t, path, string(data), mode)
}

func writeRaw(t *testing.T, path, data string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), mode); err != nil {
		t.Fatal(err)
	}
	// WriteFile is subject to the umask and keeps the mode of an existing file.
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func cfgOf(hooks ...Hook) Config {
	return Config{Version: 1, Listen: "127.0.0.1:8080", Hooks: hooks}
}

func TestLoadConfigAcceptsAValidFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	writeConfig(t, path, cfgOf(hookFor("h-0a1b2c3d", "web", "system", newSecret(t))), 0o644)
	c, err := LoadConfig(path, uint32(os.Getuid()))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Hooks) != 1 || c.Hooks[0].Stack != "web" {
		t.Fatalf("config = %+v", c)
	}
}

func TestLoadConfigRefusesWhatItMust(t *testing.T) {
	good := hookFor("h-0a1b2c3d", "web", "system", newSecret(t))
	cases := map[string]struct {
		data string
		mode os.FileMode
	}{
		"group writable":   {mustJSON(t, cfgOf(good)), 0o664},
		"others writable":  {mustJSON(t, cfgOf(good)), 0o646},
		"unknown field":    {`{"version":1,"listen":"127.0.0.1:1","hooks":[],"extra":true}`, 0o644},
		"unknown hook key": {`{"version":1,"listen":"127.0.0.1:1","hooks":[{"id":"h-0a1b2c3d","stack":"a","instance":"system","hash":"` + good.Hash + `","x":1}]}`, 0o644},
		"wrong version":    {`{"version":2,"listen":"127.0.0.1:1","hooks":[]}`, 0o644},
		"no listen":        {`{"version":1,"listen":"","hooks":[]}`, 0o644},
		"trailing data":    {mustJSON(t, cfgOf(good)) + "{}", 0o644},
		"bad id":           {mustJSON(t, cfgOf(Hook{ID: "x", Stack: "a", Instance: "system", Hash: good.Hash})), 0o644},
		"bad stack":        {mustJSON(t, cfgOf(Hook{ID: "h-0a1b2c3d", Stack: "../a", Instance: "system", Hash: good.Hash})), 0o644},
		"bad instance":     {mustJSON(t, cfgOf(Hook{ID: "h-0a1b2c3d", Stack: "a", Instance: "A/b", Hash: good.Hash})), 0o644},
		"bad hash":         {mustJSON(t, cfgOf(Hook{ID: "h-0a1b2c3d", Stack: "a", Instance: "system", Hash: "sha256:00"})), 0o644},
		"repeated id":      {mustJSON(t, cfgOf(good, hookFor(good.ID, "b", "system", newSecret(t)))), 0o644},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			writeRaw(t, path, c.data, c.mode)
			if _, err := LoadConfig(path, uint32(os.Getuid())); err == nil {
				t.Fatal("accepted")
			}
		})
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// The owner check is the production rule with the expected uid injected: a file of this user is refused when the
// expected owner is another uid.
func TestLoadConfigRefusesAnotherOwner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	writeConfig(t, path, cfgOf(), 0o644)
	if _, err := LoadConfig(path, uint32(os.Getuid())+1); err == nil || !strings.Contains(err.Error(), "owned by") {
		t.Fatalf("err = %v", err)
	}
}

func TestLoadConfigRefusesASymlink(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, filepath.Join(dir, "real.json"), cfgOf(), 0o644)
	link := filepath.Join(dir, "config.json")
	if err := os.Symlink("real.json", link); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(link, uint32(os.Getuid())); err == nil {
		t.Fatal("accepted a symlink")
	}
}
