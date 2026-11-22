package gitrepo

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscardRestoresTakesBackAndRemovesOnlyTheNamedPaths(t *testing.T) {
	ctx := context.Background()
	r := localFixture(t)
	writeTree(t, r.dir, map[string]string{"web/.env": "APP_VERSION=2.0.0\n", "web/new.txt": "new\n", "web/other.txt": "other\n",
		"staged.txt": "staged\n"})
	fixtureGit(t, r.dir, "add", "staged.txt")
	if err := os.Remove(filepath.Join(r.dir, "db/compose.yaml")); err != nil {
		t.Fatal(err)
	}

	if err := r.Discard(ctx, []string{"web/.env", "db/compose.yaml", "web/new.txt", "staged.txt"}); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(r.dir, "web/.env")); got != "APP_VERSION=1.0.0\n" {
		t.Fatalf(".env = %q", got)
	}
	if got := readFile(t, filepath.Join(r.dir, "db/compose.yaml")); got != "a\n" {
		t.Fatalf("compose.yaml = %q", got)
	}
	for _, gone := range []string{"web/new.txt", "staged.txt"} {
		if _, err := os.Stat(filepath.Join(r.dir, gone)); !os.IsNotExist(err) {
			t.Errorf("%s still exists: %v", gone, err)
		}
	}
	// a file that was not named stays
	if got := readFile(t, filepath.Join(r.dir, "web/other.txt")); got != "other\n" {
		t.Fatalf("other.txt = %q", got)
	}
	if got := fixtureGit(t, r.dir, "status", "--porcelain", "--untracked-files=all"); got != "?? web/other.txt" {
		t.Fatalf("status = %q", got)
	}
}

func TestDiscardRefusesAnythingButExactChangesAndChangesNothing(t *testing.T) {
	ctx := context.Background()
	r := localFixture(t)
	writeTree(t, r.dir, map[string]string{"web/.env": "APP_VERSION=2.0.0\n", "web/sub/a.txt": "a\n", "web/sub/b.txt": "b\n", ".gitignore": "*.log\n",
		"web/x.log": "log\n"})
	fixtureGit(t, r.dir, "add", ".gitignore")
	fixtureGit(t, r.dir, "commit", "-q", "-m", "ignore")
	for _, p := range []string{"web", "web/sub", "web/*", "web/sub/*.txt", ":(top)web/sub/a.txt", "web/x.log", "README.md", "../outside", "/etc/passwd",
		"web/../web/sub/a.txt", "-f", "", "web/missing", ".git/config", "web/.env\x00"} {
		err := r.Discard(ctx, []string{"web/.env", p})
		if err == nil {
			t.Errorf("discard %q succeeded", p)
		}
		if _, _, derr := r.Diff(ctx, p); derr == nil {
			t.Errorf("diff %q succeeded", p)
		}
	}
	if got := readFile(t, filepath.Join(r.dir, "web/.env")); got != "APP_VERSION=2.0.0\n" {
		t.Fatalf(".env was changed: %q", got)
	}
	if err := r.Discard(ctx, []string{"web/sub"}); !errors.Is(err, ErrNotChanged) {
		t.Fatalf("folder: %v", err)
	}
	for _, f := range []string{"web/sub/a.txt", "web/sub/b.txt", "web/x.log"} {
		if _, err := os.Stat(filepath.Join(r.dir, f)); err != nil {
			t.Errorf("%s was removed: %v", f, err)
		}
	}
}

func TestDiscardNeverFollowsALink(t *testing.T) {
	ctx := context.Background()
	r := localFixture(t)
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "victim.txt"), []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(r.dir, "web/link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "victim.txt"), filepath.Join(r.dir, "web/file-link")); err != nil {
		t.Fatal(err)
	}
	// a path beyond the link is no change
	if err := r.Discard(ctx, []string{"web/link/victim.txt"}); err == nil {
		t.Fatal("a path beyond a link was accepted")
	}
	// the link itself is a new file and goes, not its target
	if err := r.Discard(ctx, []string{"web/link", "web/file-link"}); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(outside, "victim.txt")); got != "keep\n" {
		t.Fatalf("target = %q", got)
	}
	if _, err := os.Lstat(filepath.Join(r.dir, "web/link")); !os.IsNotExist(err) {
		t.Fatalf("link still exists: %v", err)
	}
}

func TestDiffOfModifiedDeletedAndNewFiles(t *testing.T) {
	ctx := context.Background()
	r := localFixture(t)
	writeTree(t, r.dir, map[string]string{"web/.env": "APP_VERSION=2.0.0\n", "web/new.txt": "fresh\n"})
	if err := os.Remove(filepath.Join(r.dir, "db/compose.yaml")); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string][]string{
		"web/.env":        {"-APP_VERSION=1.0.0", "+APP_VERSION=2.0.0"},
		"db/compose.yaml": {"deleted file", "-a"},
		"web/new.txt":     {"new file", "+fresh"},
	} {
		text, truncated, err := r.Diff(ctx, path)
		if err != nil || truncated {
			t.Fatalf("diff %s = %v, %v", path, truncated, err)
		}
		for _, w := range want {
			if !strings.Contains(text, w) {
				t.Errorf("diff of %s lacks %q:\n%s", path, w, text)
			}
		}
	}
}

func TestDiffIsCutAtTheLimit(t *testing.T) {
	ctx := context.Background()
	r := localFixture(t)
	big := strings.Repeat("é", MaxDiffBytes)
	writeTree(t, r.dir, map[string]string{"web/big.txt": big})
	text, truncated, err := r.Diff(ctx, "web/big.txt")
	if err != nil || !truncated || len(text) > MaxDiffBytes {
		t.Fatalf("len = %d, truncated = %v, err = %v", len(text), truncated, err)
	}
	if !strings.HasSuffix(text, "é") {
		t.Fatalf("the cut is not on a character boundary")
	}
}
