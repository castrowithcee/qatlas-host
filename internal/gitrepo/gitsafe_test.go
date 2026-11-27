package gitrepo

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The tests cannot make root own a folder, so the expected owner is the current user; the rule is the same.
func init() { enforceGitSafe = func() bool { return false } }

// safeGitFolder is a folder with a .git that passes the check, and a git that counts its calls.
func safeGitFolder(t *testing.T) (*Repo, string) {
	t.Helper()
	enforceGitSafe = func() bool { return true }
	gitOwnerUID = func() uint32 { return uint32(os.Getuid()) }
	t.Cleanup(func() {
		enforceGitSafe = func() bool { return false }
		gitOwnerUID = func() uint32 { return 0 }
	})
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	for _, d := range []string{"", "info", "hooks"} {
		if err := os.MkdirAll(filepath.Join(gitDir, d), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(filepath.Join(gitDir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{"config", "info/attributes"} {
		if err := os.WriteFile(filepath.Join(gitDir, f), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	calls := filepath.Join(t.TempDir(), "calls")
	script := filepath.Join(t.TempDir(), "git")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho x >> "+calls+"\necho main\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	r, err := Open(Config{Local: true, test: testOptions{gitPath: script}}, dir)
	if err != nil {
		t.Fatal(err)
	}
	return r, calls
}

func gitCalls(calls string) int {
	b, _ := os.ReadFile(calls)
	return strings.Count(string(b), "x")
}

func TestSafeGitFolderRunsGit(t *testing.T) {
	r, calls := safeGitFolder(t)
	if _, err := r.run(context.Background(), true, maxStdout, "status"); err != nil {
		t.Fatal(err)
	}
	if gitCalls(calls) != 1 || r.CheckGit() != nil {
		t.Fatalf("calls = %d", gitCalls(calls))
	}
}

func TestMissingGitPasses(t *testing.T) {
	r, _ := safeGitFolder(t)
	if err := os.RemoveAll(filepath.Join(r.dir, ".git")); err != nil {
		t.Fatal(err)
	}
	if err := r.CheckGit(); err != nil {
		t.Fatal(err)
	}
}

type unsafeCase struct {
	name   string
	break_ func(t *testing.T, r *Repo)
	reason string
}

func unsafeCases() []unsafeCase {
	return []unsafeCase{
		{"wrong owner", func(t *testing.T, r *Repo) { gitOwnerUID = func() uint32 { return uint32(os.Getuid()) + 1 } }, ".git is not owned by root"},
		{"group-writable .git", chmod(".git", 0o775), ".git can be written"},
		{"group-writable config", chmod(".git/config", 0o664), ".git/config can be written"},
		{"others-writable info/attributes", chmod(".git/info/attributes", 0o646), ".git/info/attributes can be written"},
		{"group-writable hooks", chmod(".git/hooks", 0o775), ".git/hooks can be written"},
		{"group-writable info", chmod(".git/info", 0o775), ".git/info can be written"},
		{"group-writable config.worktree", func(t *testing.T, r *Repo) {
			write(t, filepath.Join(r.dir, ".git", "config.worktree"), 0o664)
		}, ".git/config.worktree can be written"},
		{"group-writable linked config.worktree", func(t *testing.T, r *Repo) {
			p := filepath.Join(r.dir, ".git", "worktrees", "w1")
			if err := os.MkdirAll(p, 0o755); err != nil {
				t.Fatal(err)
			}
			write(t, filepath.Join(p, "config.worktree"), 0o664)
		}, ".git/worktrees/w1/config.worktree can be written"},
		{".git is a link", func(t *testing.T, r *Repo) {
			real := filepath.Join(t.TempDir(), "real")
			if err := os.Rename(filepath.Join(r.dir, ".git"), real); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(real, filepath.Join(r.dir, ".git")); err != nil {
				t.Fatal(err)
			}
		}, ".git is a link"},
		{"info/attributes is a link", func(t *testing.T, r *Repo) {
			p := filepath.Join(r.dir, ".git", "info", "attributes")
			if err := os.Remove(p); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink("/dev/null", p); err != nil {
				t.Fatal(err)
			}
		}, ".git/info/attributes is a link"},
		{".git is a file", func(t *testing.T, r *Repo) {
			p := filepath.Join(r.dir, ".git")
			if err := os.RemoveAll(p); err != nil {
				t.Fatal(err)
			}
			write(t, p, 0o644)
		}, ".git is not a folder"},
	}
}

func TestUnsafeGitRunsNoGit(t *testing.T) {
	for _, c := range unsafeCases() {
		t.Run(c.name, func(t *testing.T) {
			r, calls := safeGitFolder(t)
			c.break_(t, r)
			_, err := r.run(context.Background(), true, maxStdout, "status")
			reason, ok := UnsafeReason(err)
			if !errors.Is(err, ErrGitUnsafe) || !ok || !strings.Contains(reason, c.reason) {
				t.Fatalf("err = %v", err)
			}
			if _, err := r.Head(context.Background()); !errors.Is(err, ErrGitUnsafe) {
				t.Fatalf("Head err = %v", err)
			}
			if n := gitCalls(calls); n != 0 {
				t.Fatalf("git ran %d times", n)
			}
		})
	}
}

func TestUnsafeGitBlocksPreviewAndInit(t *testing.T) {
	r, calls := safeGitFolder(t)
	if err := os.Chmod(filepath.Join(r.dir, ".git"), 0o775); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenPreview(context.Background(), r.cfg, r.dir, nil); !errors.Is(err, ErrGitUnsafe) {
		t.Fatalf("OpenPreview err = %v", err)
	}
	if _, err := r.CompareRemote(context.Background(), "ssh://git.example.test/demo/stacks.git"); !errors.Is(err, ErrGitUnsafe) {
		t.Fatalf("CompareRemote err = %v", err)
	}
	if n := gitCalls(calls); n != 0 {
		t.Fatalf("git ran %d times", n)
	}
}

func TestOtherUsersAreNotChecked(t *testing.T) {
	r, calls := safeGitFolder(t)
	enforceGitSafe = func() bool { return false }
	if err := os.Chmod(filepath.Join(r.dir, ".git"), 0o777); err != nil {
		t.Fatal(err)
	}
	if _, err := r.run(context.Background(), true, maxStdout, "status"); err != nil || gitCalls(calls) != 1 {
		t.Fatalf("err = %v, calls = %d", err, gitCalls(calls))
	}
}

func chmod(rel string, mode os.FileMode) func(*testing.T, *Repo) {
	return func(t *testing.T, r *Repo) {
		t.Helper()
		if err := os.Chmod(filepath.Join(r.dir, rel), mode); err != nil {
			t.Fatal(err)
		}
	}
}

func write(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, nil, mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}
