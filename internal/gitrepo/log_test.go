package gitrepo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func paths(c Commit) string {
	var s []string
	for _, p := range c.Paths {
		s = append(s, string(p.Kind)+":"+p.Path)
	}
	return strings.Join(s, ",")
}

func TestLogFollowsOnlyTheFirstParentAndShowsAMergeAgainstIt(t *testing.T) {
	ctx := context.Background()
	r := localFixture(t)
	fixtureGit(t, r.dir, "checkout", "-q", "-b", "side")
	side := localCommit(t, r.dir, map[string]string{"side.txt": "s\n"})
	fixtureGit(t, r.dir, "checkout", "-q", "main")
	localCommit(t, r.dir, map[string]string{"web/.env": "APP_VERSION=2.0.0\n"})
	fixtureGit(t, r.dir, "merge", "-q", "--no-ff", "-m", "merge side", "side")

	got, truncated, err := r.Log(ctx, 20)
	if err != nil || truncated {
		t.Fatalf("log: %v truncated=%v", err, truncated)
	}
	if len(got) != 3 || got[0].Subject != "merge side" || got[1].Subject != "change" || got[2].Subject != "change" {
		t.Fatalf("commits = %+v", got)
	}
	for _, c := range got {
		if c.ID == side {
			t.Fatalf("side commit %s listed", side)
		}
	}
	if p := paths(got[0]); p != "new:side.txt" {
		t.Fatalf("merge paths = %q", p)
	}
	if p := paths(got[1]); p != "modified:web/.env" {
		t.Fatalf("paths = %q", p)
	}
	if p := paths(got[2]); p != "new:README.md,new:db/compose.yaml,new:web/.env" {
		t.Fatalf("root paths = %q", p)
	}
	if got[0].Author != "Demo" || got[0].Date.IsZero() || !hexRe.MatchString(got[0].ID) {
		t.Fatalf("header = %+v", got[0])
	}
}

func TestLogRenameIsDeletedAndNewAndLimitApplies(t *testing.T) {
	ctx := context.Background()
	r := localFixture(t)
	fixtureGit(t, r.dir, "mv", "db/compose.yaml", "db/renamed.yaml")
	fixtureGit(t, r.dir, "commit", "-q", "-m", "rename")
	got, _, err := r.Log(ctx, 1)
	if err != nil || len(got) != 1 {
		t.Fatalf("log: %v %+v", err, got)
	}
	if p := paths(got[0]); p != "deleted:db/compose.yaml,new:db/renamed.yaml" {
		t.Fatalf("paths = %q", p)
	}
	if _, _, err := r.Log(ctx, 0); err == nil {
		t.Fatal("limit 0 accepted")
	}
}

func TestLogDeliversControlCharactersAndOddPathsRaw(t *testing.T) {
	ctx := context.Background()
	r := localFixture(t)
	writeTree(t, r.dir, map[string]string{"web/a b\nc.txt": "x\n"})
	fixtureGit(t, r.dir, "add", "-A")
	fixtureGit(t, r.dir, "commit", "-q", "--author=Ev\x1b[31m\x01il <ev@example.test>", "-m", "sub\x1b[2Jject\x01 end")
	got, _, err := r.Log(ctx, 1)
	if err != nil || len(got) != 1 {
		t.Fatalf("log: %v %+v", err, got)
	}
	c := got[0]
	if c.Subject != "sub\x1b[2Jject\x01 end" || c.Author != "Ev\x1b[31m\x01il" || paths(c) != "new:web/a b\nc.txt" {
		t.Fatalf("commit = %q %q %q", c.Subject, c.Author, paths(c))
	}
}

func TestLogCutsAtMaxPaths(t *testing.T) {
	ctx := context.Background()
	r := localFixture(t)
	files := map[string]string{}
	for i := 0; i < MaxLogPaths+5; i++ {
		files[fmt.Sprintf("many/f%03d", i)] = "x\n"
	}
	localCommit(t, r.dir, files)
	localCommit(t, r.dir, map[string]string{"after.txt": "x\n"})
	got, _, err := r.Log(ctx, 2)
	if err != nil || len(got) != 2 {
		t.Fatalf("log: %v %+v", err, got)
	}
	if len(got[1].Paths) != MaxLogPaths || !got[1].PathsTruncated || got[0].PathsTruncated || paths(got[0]) != "new:after.txt" {
		t.Fatalf("paths = %d truncated=%v/%v", len(got[1].Paths), got[1].PathsTruncated, got[0].PathsTruncated)
	}
}

func TestLogOfAnEmptyRepositoryIsEmpty(t *testing.T) {
	empty := filepath.Join(t.TempDir(), "empty")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	fixtureGit(t, empty, "init", "-q", "-b", "main")
	e, err := Open(Config{Local: true}, empty)
	if err != nil {
		t.Fatal(err)
	}
	got, truncated, err := e.Log(context.Background(), 5)
	if err != nil || truncated || len(got) != 0 {
		t.Fatalf("log = %+v %v %v", got, truncated, err)
	}
}

func TestShowRefusesWhatIsNotOnTheFirstParentChain(t *testing.T) {
	ctx := context.Background()
	r := localFixture(t)
	fixtureGit(t, r.dir, "checkout", "-q", "-b", "side")
	side := localCommit(t, r.dir, map[string]string{"side.txt": "s\n"})
	fixtureGit(t, r.dir, "checkout", "-q", "-b", "other", "main")
	other := localCommit(t, r.dir, map[string]string{"other.txt": "o\n"})
	fixtureGit(t, r.dir, "checkout", "-q", "main")
	localCommit(t, r.dir, map[string]string{"web/.env": "APP_VERSION=2.0.0\n"})
	fixtureGit(t, r.dir, "merge", "-q", "--no-ff", "-m", "merge side", "side")

	for _, id := range []string{side, side[:10], other} {
		if _, _, err := r.Show(ctx, id); !errors.Is(err, ErrCommitNotFound) {
			t.Errorf("show %s: %v", id, err)
		}
	}
	for _, id := range []string{"", "-p", "--all", "HEAD", "main", "ABCDEF1", "abc", strings.Repeat("a", 65), "abcdef1\n", "abcdef1 -p"} {
		if _, _, err := r.Show(ctx, id); !errors.Is(err, ErrBadCommitID) {
			t.Errorf("show %q: %v", id, err)
		}
	}
	if _, _, err := r.Show(ctx, strings.Repeat("0", 40)); !errors.Is(err, ErrCommitNotFound) {
		t.Errorf("unknown id: %v", err)
	}
}

func TestShowReturnsTheDiffAgainstTheFirstParentAndCutsABigOne(t *testing.T) {
	ctx := context.Background()
	r := localFixture(t)
	root := fixtureGit(t, r.dir, "rev-list", "--max-parents=0", "HEAD")
	got, truncated, err := r.Show(ctx, root[:12])
	if err != nil || truncated || !strings.Contains(got, "+++ b/web/.env") || !strings.Contains(got, "+APP_VERSION=1.0.0") {
		t.Fatalf("root show: %v %v %q", err, truncated, got)
	}
	big := localCommit(t, r.dir, map[string]string{"big.txt": strings.Repeat("é line of text\n", MaxDiffBytes/8)})
	got, truncated, err = r.Show(ctx, big)
	if err != nil || !truncated || len(got) > MaxDiffBytes || !strings.HasPrefix(got, "diff --git a/big.txt") {
		t.Fatalf("big show: %v truncated=%v len=%d", err, truncated, len(got))
	}
}
