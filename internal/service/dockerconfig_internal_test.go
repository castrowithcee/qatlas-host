package service

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupDockerConfig(t *testing.T, state, exe string) string {
	t.Helper()
	var buf bytes.Buffer
	if err := prepareDockerConfig(state, exe, slog.New(slog.NewTextHandler(&buf, nil))); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestDockerConfigIsCreated(t *testing.T) {
	state := t.TempDir()
	exe := filepath.Join(t.TempDir(), "qhost")
	setupDockerConfig(t, state, exe)
	dir := filepath.Join(state, "docker")
	if fi, err := os.Stat(dir); err != nil || fi.Mode().Perm() != 0o700 {
		t.Fatalf("folder: %v %v", fi, err)
	}
	cfg := filepath.Join(dir, "config.json")
	if fi, err := os.Stat(cfg); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("config.json: %v %v", fi, err)
	}
	if got, _ := os.ReadFile(cfg); string(got) != `{"credsStore":"qhost"}` {
		t.Errorf("config.json = %q", got)
	}
	if got, err := os.Readlink(filepath.Join(dir, "bin", "docker-credential-qhost")); err != nil || got != exe {
		t.Errorf("link = %q, %v", got, err)
	}
}

func TestDockerConfigSecondStartChangesNothing(t *testing.T) {
	state := t.TempDir()
	exe := filepath.Join(t.TempDir(), "qhost")
	setupDockerConfig(t, state, exe)
	link := filepath.Join(state, "docker", "bin", "docker-credential-qhost")
	cfg := filepath.Join(state, "docker", "config.json")
	before, _ := os.Lstat(cfg)
	linkBefore, _ := os.Lstat(link)
	if logged := setupDockerConfig(t, state, exe); logged != "" {
		t.Errorf("second start logged %q", logged)
	}
	after, _ := os.Lstat(cfg)
	linkAfter, _ := os.Lstat(link)
	if !os.SameFile(before, after) || !before.ModTime().Equal(after.ModTime()) || !os.SameFile(linkBefore, linkAfter) {
		t.Error("second start rewrote a file")
	}
	if _, err := os.Stat(cfg + ".bak"); err == nil {
		t.Error("second start left a backup")
	}
}

func TestDockerConfigKeepsADifferingFileAsBackup(t *testing.T) {
	state := t.TempDir()
	dir := filepath.Join(state, "docker")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(dir, "config.json")
	const old = `{"auths":{"registry.example.invalid":{"auth":"ZGVtbzpkZW1v"}}}`
	if err := os.WriteFile(cfg, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	logged := setupDockerConfig(t, state, filepath.Join(t.TempDir(), "qhost"))
	if got, _ := os.ReadFile(cfg + ".bak"); string(got) != old {
		t.Errorf("backup = %q", got)
	}
	if got, _ := os.ReadFile(cfg); string(got) != `{"credsStore":"qhost"}` {
		t.Errorf("config.json = %q", got)
	}
	if !strings.Contains(logged, "config.json.bak") || strings.Contains(logged, "ZGVtbzpkZW1v") || strings.Contains(logged, "auths") {
		t.Errorf("log = %q", logged)
	}
	if fi, _ := os.Stat(dir); fi.Mode().Perm() != 0o700 {
		t.Errorf("folder mode = %v", fi.Mode().Perm())
	}
}

func TestDockerConfigCorrectsAWrongLink(t *testing.T) {
	state := t.TempDir()
	exe := filepath.Join(t.TempDir(), "qhost")
	setupDockerConfig(t, state, filepath.Join(t.TempDir(), "old"))
	setupDockerConfig(t, state, exe)
	if got, _ := os.Readlink(filepath.Join(state, "docker", "bin", "docker-credential-qhost")); got != exe {
		t.Errorf("link = %q", got)
	}
}

func TestDockerConfigDoesNotFollowLinks(t *testing.T) {
	t.Run("folder", func(t *testing.T) {
		state, other := t.TempDir(), t.TempDir()
		if err := os.Symlink(other, filepath.Join(state, "docker")); err != nil {
			t.Fatal(err)
		}
		if err := prepareDockerConfig(state, "/x/qhost", slog.Default()); err == nil {
			t.Error("a link in place of the folder was accepted")
		}
		if entries, _ := os.ReadDir(other); len(entries) != 0 {
			t.Errorf("wrote through the link: %v", entries)
		}
	})
	t.Run("config.json", func(t *testing.T) {
		state, other := t.TempDir(), t.TempDir()
		dir := filepath.Join(state, "docker")
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(other, "victim")
		if err := os.WriteFile(target, []byte("keep"), 0o600); err != nil {
			t.Fatal(err)
		}
		cfg := filepath.Join(dir, "config.json")
		if err := os.Symlink(target, cfg); err != nil {
			t.Fatal(err)
		}
		setupDockerConfig(t, state, "/x/qhost")
		if got, _ := os.ReadFile(target); string(got) != "keep" {
			t.Errorf("wrote through the link: %q", got)
		}
		if fi, _ := os.Lstat(cfg); !fi.Mode().IsRegular() {
			t.Error("config.json is still a link")
		}
		if fi, err := os.Lstat(cfg + ".bak"); err != nil || fi.Mode().IsRegular() {
			t.Errorf("the link was not kept as backup: %v", err)
		}
	})
}
