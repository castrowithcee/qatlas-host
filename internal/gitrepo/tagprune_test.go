package gitrepo

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const (
	idOld = "20260101T000000Z-0a1b2c3d"
	idMid = "20260102T000000Z-0a1b2c3d"
	idNew = "20260103T000000Z-0a1b2c3d"
)

// tagNames lists the tags of a repository directory, sorted.
func tagNames(t *testing.T, dir string) []string {
	t.Helper()
	out := fixtureGit(t, dir, "tag", "-l")
	if out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}

// seedTags sets the run tags of web01/web and tags that pruning web01/web must never touch.
func seedTags(t *testing.T, r *Repo, head string) {
	t.Helper()
	ctx := context.Background()
	for _, n := range []string{
		RunTag("web01", "web", idOld), RunTag("web01", "web", idMid), RunTag("web01", "web", idNew),
		RunTag("web01", "web-2", idOld), RunTag("db02", "web", idOld), RunTag("web01", "web", "not-a-run-id"),
	} {
		if err := r.TagCommit(ctx, n, head); err != nil {
			t.Fatal(err)
		}
	}
	fixtureGit(t, r.dir, "tag", "release/1.0")
	fixtureGit(t, r.dir, "push", "-q", "origin", "refs/tags/release/1.0")
}

func TestPruneTagsDeletesOnlyChosenOwnTags(t *testing.T) {
	ctx := context.Background()
	f := newRemote(t)
	r, _ := cloneFixture(t, f)
	head, _ := r.Head(ctx)
	seedTags(t, r, head)
	before := tagNames(t, f.bare)

	var offered []string
	deleted, err := r.PruneTags(ctx, "Web01.example.test", "web", func(ids []string) []string {
		offered = ids
		return []string{idOld, idMid, "20260105T000000Z-ffffffff"}
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(offered, []string{idOld, idMid, idNew}) || !slices.Equal(deleted, []string{idOld, idMid}) {
		t.Fatalf("offered %v, deleted %v", offered, deleted)
	}
	for _, dir := range []string{f.bare, r.dir} {
		got := tagNames(t, dir)
		for _, gone := range []string{RunTag("web01", "web", idOld), RunTag("web01", "web", idMid)} {
			if slices.Contains(got, gone) {
				t.Errorf("%s still has %s", dir, gone)
			}
		}
		if len(got) != len(before)-2 {
			t.Errorf("%s tags = %v, before %v", dir, got, before)
		}
	}
}

func TestPruneTagsRemoteRefusalKeepsTheTag(t *testing.T) {
	ctx := context.Background()
	f := newRemote(t)
	r, _ := cloneFixture(t, f)
	head, _ := r.Head(ctx)
	for _, id := range []string{idOld, idMid} {
		if err := r.TagCommit(ctx, RunTag("web01", "web", id), head); err != nil {
			t.Fatal(err)
		}
	}
	hook := filepath.Join(f.bare, "hooks", "update")
	script := "#!/bin/sh\ncase \"$1\" in refs/tags/*) echo 'tags are protected' >&2; exit 1;; esac\n"
	if err := os.WriteFile(hook, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	deleted, err := r.PruneTags(ctx, "web01", "web", func(ids []string) []string { return ids[:1] })
	if !errors.Is(err, ErrTagRejected) || len(deleted) != 0 {
		t.Fatalf("deleted %v, err %v", deleted, err)
	}
	if len(tagNames(t, f.bare)) != 2 || len(tagNames(t, r.dir)) != 2 {
		t.Fatalf("tags changed: remote %v, local %v", tagNames(t, f.bare), tagNames(t, r.dir))
	}
}

func TestPruneTagsLocalMode(t *testing.T) {
	ctx := context.Background()
	r := localFixture(t)
	head, _ := r.Head(ctx)
	for _, id := range []string{idOld, idNew} {
		if err := r.TagCommit(ctx, RunTag("web01", "web", id), head); err != nil {
			t.Fatal(err)
		}
	}
	deleted, err := r.PruneTags(ctx, "web01", "web", func(ids []string) []string { return ids[:1] })
	if err != nil || !slices.Equal(deleted, []string{idOld}) {
		t.Fatalf("deleted %v, err %v", deleted, err)
	}
	if got := tagNames(t, r.dir); !slices.Equal(got, []string{RunTag("web01", "web", idNew)}) {
		t.Fatalf("tags = %v", got)
	}
}
