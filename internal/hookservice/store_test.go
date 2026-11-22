package hookservice

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewSecretIsWellFormedAndFresh(t *testing.T) {
	a, err := NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := NewSecret()
	if !wellFormed(a) || a == b {
		t.Fatalf("secret %q, second %q", a, b)
	}
	if HashSecret(a) != hashOf(a) {
		t.Fatal("HashSecret differs from the stored form the receiver compares")
	}
}

func TestUpdateConfigAddsAHookTheReceiverAccepts(t *testing.T) {
	f := newFixture(t)
	secret, err := NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	err = UpdateConfig(f.cfg, nil, func(c *Config) error {
		id, err := c.NewID()
		if err != nil {
			return err
		}
		c.Hooks = append(c.Hooks, Hook{ID: id, Stack: "web", Instance: "system", Hash: HashSecret(secret)})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Reload(); err != nil {
		t.Fatal(err)
	}
	if code := f.do(t, "POST", "/hook/"+secret, ""); code != 202 {
		t.Fatalf("code %d", code)
	}
	if !exists(f.marker("system", "web")) {
		t.Fatal("no marker")
	}
	if info, err := os.Stat(f.cfg); err != nil || info.Mode().Perm() != 0o644 {
		t.Fatalf("mode %v %v", info, err)
	}
}

func TestUpdateConfigKeepsFilesItCannotTrust(t *testing.T) {
	dir := t.TempDir()
	called := false
	edit := func(*Config) error { called = true; return nil }
	missing := filepath.Join(dir, "missing.json")
	if err := UpdateConfig(missing, nil, edit); !errors.Is(err, fs.ErrNotExist) || called {
		t.Fatalf("missing: %v, called %v", err, called)
	}
	for name, doc := range map[string]string{
		"damaged": `{"version":1,"listen":"127.0.0.1:8080","hooks":[{"id":"x"}]}`,
		"unknown": `{"version":99,"listen":"127.0.0.1:8080","hooks":[]}`,
		"junk":    `not json`,
	} {
		path := filepath.Join(dir, name+".json")
		writeRaw(t, path, doc, 0o644)
		if err := UpdateConfig(path, nil, edit); err == nil || called {
			t.Fatalf("%s: err %v, called %v", name, err, called)
		}
		if raw, _ := os.ReadFile(path); string(raw) != doc {
			t.Fatalf("%s changed: %q", name, raw)
		}
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 3 {
		t.Fatalf("leftover files: %v", entries)
	}
}

func TestUpdateConfigRefusesAResultThatDoesNotValidate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	writeConfig(t, path, cfgOf(), 0o644)
	before, _ := os.ReadFile(path)
	err := UpdateConfig(path, nil, func(c *Config) error {
		c.Hooks = append(c.Hooks, Hook{ID: "bad", Stack: "web", Instance: "system", Hash: "x"})
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "hook 0") {
		t.Fatalf("err %v", err)
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Fatal("file changed")
	}
}

func TestRemove(t *testing.T) {
	c := cfgOf(hookFor("h-00000001", "a", "system", "x"), hookFor("h-00000002", "b", "system", "y"))
	if !c.Remove("h-00000001") || c.Remove("h-00000001") || len(c.Hooks) != 1 || c.Hooks[0].ID != "h-00000002" {
		t.Fatalf("%+v", c)
	}
}
