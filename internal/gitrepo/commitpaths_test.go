package gitrepo

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/stacks"
)

func wantNoCommitTempLeft(t *testing.T) {
	t.Helper()
	if left, _ := filepath.Glob(filepath.Join(os.TempDir(), "qhost-commit-*")); len(left) > 0 {
		t.Fatalf("temporary directories left: %v", left)
	}
}

func TestCommitPathsCommitsExactlyThePathsAndMovesTheBranch(t *testing.T) {
	ctx := context.Background()
	f := newRemote(t)
	r, _ := cloneFixture(t, f)
	head, _ := r.Head(ctx)

	writeTree(t, r.dir, map[string]string{
		"web/compose.yaml": "changed\n",
		"web/extra.txt":    "new\n",
		"README.md":        "foreign edit\n",
		"other.txt":        "staged\n",
	})
	if err := os.Remove(filepath.Join(r.dir, "db", "compose.yaml")); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(r.dir, "web", "extra.txt"), 0o755); err != nil {
		t.Fatal(err)
	}
	fixtureGit(t, r.dir, "add", "other.txt")
	before := fixtureGit(t, r.dir, "status", "--porcelain", "--untracked-files=all")

	id, err := r.CommitPaths(ctx, head, []string{"db/compose.yaml", "web/extra.txt", "web/compose.yaml"}, nil, "Commit web and db\n\x1b[31m")
	if err != nil {
		t.Fatal(err)
	}
	if got := fixtureGit(t, r.dir, "diff", "--name-status", head, id); got != "D\tdb/compose.yaml\nM\tweb/compose.yaml\nA\tweb/extra.txt" {
		t.Fatalf("diff = %q", got)
	}
	if got := fixtureGit(t, r.dir, "ls-tree", id, "web/extra.txt"); !strings.HasPrefix(got, "100755 ") {
		t.Fatalf("mode = %q", got)
	}
	if got := fixtureGit(t, r.dir, "show", "-s", "--format=%an <%ae>|%B", id); got != "qhost <qhost@localhost.invalid>|Commit web and db [31m" {
		t.Fatalf("commit = %q", got)
	}
	if h, _ := r.Head(ctx); h != id {
		t.Fatalf("head = %s, want %s", h, id)
	}
	if got := fixtureGit(t, f.bare, "rev-parse", "main"); got != id {
		t.Fatalf("remote main = %s, want %s", got, id)
	}
	if got := fixtureGit(t, r.dir, "rev-parse", "origin/main"); got != id {
		t.Fatalf("tracking ref = %s, want %s", got, id)
	}
	// Only the unrelated changes are left, as they were.
	if got := fixtureGit(t, r.dir, "status", "--porcelain", "--untracked-files=all"); got != "M README.md\nA  other.txt" {
		t.Fatalf("status = %q (before: %q)", got, before)
	}
	if got := readFile(t, filepath.Join(r.dir, "README.md")); got != "foreign edit\n" {
		t.Fatalf("README = %q", got)
	}
	wantNoCommitTempLeft(t)
}

func TestCommitPathsKeepsTheHostsOwnFilesOut(t *testing.T) {
	ctx := context.Background()
	f := newRemote(t)
	f.commit(map[string]string{"web/mnt/tracked.txt": "a\n"})
	r, _ := cloneFixture(t, f)
	head, _ := r.Head(ctx)
	list := []stacks.Stack{{Name: "web", Folder: "web", BindMounts: []string{"mnt"}}}
	writeTree(t, r.dir, map[string]string{
		"web/secrets.env":     "DEMO_PASSWORD=x\n",
		"web/data/db.bin":     "x\n",
		"web/mnt/new.txt":     "x\n",
		"web/mnt/tracked.txt": "b\n",
		"web/compose.yaml":    "changed\n",
	})
	for _, p := range []string{"web/secrets.env", "web/data/db.bin", "web/mnt/new.txt"} {
		if _, err := r.CommitPaths(ctx, head, []string{"web/compose.yaml", p}, list, "m"); !errors.Is(err, ErrExcluded) {
			t.Fatalf("%s: err = %v, want ErrExcluded", p, err)
		}
	}
	if h, _ := r.Head(ctx); h != head {
		t.Fatal("a refused commit moved the branch")
	}
	// A changed tracked file below a mount is a normal change.
	id, err := r.CommitPaths(ctx, head, []string{"web/mnt/tracked.txt"}, list, "m")
	if err != nil {
		t.Fatal(err)
	}
	if got := fixtureGit(t, r.dir, "diff", "--name-only", head, id); got != "web/mnt/tracked.txt" {
		t.Fatalf("diff = %q", got)
	}
}

func TestCommitPathsRefusals(t *testing.T) {
	ctx := context.Background()
	f := newRemote(t)
	r, _ := cloneFixture(t, f)
	head, _ := r.Head(ctx)
	writeTree(t, r.dir, map[string]string{"web/compose.yaml": "changed\n"})

	if _, err := r.CommitPaths(ctx, strings.Repeat("0", 40), []string{"web/compose.yaml"}, nil, "m"); !errors.Is(err, ErrHeadMoved) {
		t.Fatalf("wrong head: err = %v", err)
	}
	if _, err := r.CommitPaths(ctx, head, []string{"web/compose.yaml", "db/compose.yaml"}, nil, "m"); !errors.Is(err, ErrNotChanged) {
		t.Fatalf("unchanged path: err = %v", err)
	}
	if _, err := r.CommitPaths(ctx, head, []string{"web"}, nil, "m"); !errors.Is(err, ErrNotChanged) {
		t.Fatalf("folder: err = %v", err)
	}
	if _, err := r.CommitPaths(ctx, head, []string{"../x"}, nil, "m"); err == nil {
		t.Fatal("a path outside the working copy went through")
	}
	if _, err := r.CommitPaths(ctx, head, nil, nil, "m"); err == nil {
		t.Fatal("an empty list went through")
	}
	if _, err := r.CommitPaths(ctx, head, []string{"web/compose.yaml"}, nil, " \n"); err == nil {
		t.Fatal("an empty message went through")
	}
	if got := fixtureGit(t, f.bare, "rev-parse", "main"); got != head {
		t.Fatalf("remote main = %s", got)
	}
	// A symbolic link at a changed path is not a regular file.
	if err := os.Symlink("compose.yaml", filepath.Join(r.dir, "web", "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CommitPaths(ctx, head, []string{"web/link"}, nil, "m"); !errors.Is(err, ErrNotRegular) {
		t.Fatalf("link: err = %v", err)
	}
	wantNoCommitTempLeft(t)
}

func TestCommitPathsRefusesASecretBeforeAnythingIsPushed(t *testing.T) {
	ctx := context.Background()
	f := newRemote(t)
	r, _ := cloneFixture(t, f)
	head, _ := r.Head(ctx)
	// The value is made at run time so that no key-like text lives in the repository.
	value := strings.ReplaceAll(t.Name()+t.TempDir(), "/", "")
	writeTree(t, r.dir, map[string]string{"web/.env": "DEMO_PASSWORD=" + value + "\n"})

	_, err := r.CommitPaths(ctx, head, []string{"web/.env"}, nil, "m")
	var secrets *SecretsError
	if !errors.As(err, &secrets) {
		t.Fatalf("err = %v, want a *SecretsError", err)
	}
	if secrets.Scan.Total != 1 || secrets.Scan.Findings[0].Path != "web/.env" || secrets.Scan.Findings[0].Kind != KindEnvSecret {
		t.Fatalf("scan = %+v", secrets.Scan)
	}
	if strings.Contains(err.Error(), value) {
		t.Fatalf("the error holds the value: %v", err)
	}
	if got := fixtureGit(t, f.bare, "rev-parse", "main"); got != head {
		t.Fatalf("remote main = %s", got)
	}
	if h, _ := r.Head(ctx); h != head {
		t.Fatal("the branch moved")
	}
}

func TestCommitPathsDistinguishesTheRefusalsOfThePush(t *testing.T) {
	ctx := context.Background()
	f := newRemote(t)
	r, _ := cloneFixture(t, f)
	head, _ := r.Head(ctx)
	writeTree(t, r.dir, map[string]string{"web/compose.yaml": "changed\n"})

	// The remote moved after the last fetch: a rejection that a retry after a fetch can resolve.
	f.commit(map[string]string{"db/compose.yaml": "moved\n"})
	if _, err := r.CommitPaths(ctx, head, []string{"web/compose.yaml"}, nil, "m"); !errors.Is(err, ErrPushRejected) {
		t.Fatalf("moved remote: err = %v, want ErrPushRejected", err)
	}
	if h, _ := r.Head(ctx); h != head {
		t.Fatal("the branch moved after a rejected push")
	}
	if err := r.Fetch(ctx); err != nil {
		t.Fatal(err)
	}
	if err := r.FastForward(ctx, mustRemoteHead(t, r)); err != nil {
		t.Fatal(err)
	}
	head, _ = r.Head(ctx)

	// A hook of the remote declines the update, as a branch protection does.
	hook := filepath.Join(f.bare, "hooks", "pre-receive")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\necho 'protected branch' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := r.CommitPaths(ctx, head, []string{"web/compose.yaml"}, nil, "m")
	var rejected *RemoteRejectedError
	if !errors.As(err, &rejected) || errors.Is(err, ErrPushRejected) {
		t.Fatalf("err = %v, want a *RemoteRejectedError", err)
	}
	if !strings.Contains(rejected.Reason, "pre-receive hook declined") {
		t.Fatalf("reason = %q", rejected.Reason)
	}
	if rejected.Output != "protected branch" || !strings.HasSuffix(err.Error(), "; remote output: protected branch") {
		t.Fatalf("output = %q, error = %q", rejected.Output, err)
	}
	if h, _ := r.Head(ctx); h != head {
		t.Fatal("the branch moved after a refused push")
	}
	if got := fixtureGit(t, r.dir, "status", "--porcelain"); got != "M web/compose.yaml" {
		t.Fatalf("status = %q", got)
	}
}

func mustRemoteHead(t *testing.T, r *Repo) string {
	t.Helper()
	id, err := r.RemoteHead(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestCommitPathsInLocalMode(t *testing.T) {
	ctx := context.Background()
	r := localFixture(t)
	head, _ := r.Head(ctx)
	writeTree(t, r.dir, map[string]string{"web/.env": "APP_VERSION=2.0.0\n", "README.md": "foreign\n"})
	if err := os.Remove(filepath.Join(r.dir, "db", "compose.yaml")); err != nil {
		t.Fatal(err)
	}

	id, err := r.CommitPaths(ctx, head, []string{"web/.env", "db/compose.yaml"}, nil, "m")
	if err != nil {
		t.Fatal(err)
	}
	if h, _ := r.Head(ctx); h != id {
		t.Fatalf("head = %s, want %s", h, id)
	}
	if got := fixtureGit(t, r.dir, "diff", "--name-status", head, id); got != "D\tdb/compose.yaml\nM\tweb/.env" {
		t.Fatalf("diff = %q", got)
	}
	if got := fixtureGit(t, r.dir, "status", "--porcelain"); got != "M README.md" {
		t.Fatalf("status = %q", got)
	}
	if _, err := r.CommitPaths(ctx, head, []string{"README.md"}, nil, "m"); !errors.Is(err, ErrHeadMoved) {
		t.Fatalf("old head: err = %v", err)
	}
}
