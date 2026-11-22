package gitrepo

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTagHostAndRunTag(t *testing.T) {
	cases := map[string]string{
		"web01":              "web01",
		"Web01.example.test": "web01",
		"my host_1":          "my-host_1",
		"--a  b--":           "a-b",
		"--a..b--":           "a",
		"":                   "host",
		".example.test":      "host",
		"äöü":                "host",
	}
	for in, want := range cases {
		if got := TagHost(in); got != want {
			t.Errorf("TagHost(%q) = %q, want %q", in, got, want)
		}
	}
	if got := RunTag("Web01.example.test", "blog", "20260101T000000Z-0a1b2c3d"); got != "qhost/web01/blog/20260101T000000Z-0a1b2c3d" {
		t.Errorf("RunTag = %q", got)
	}
}

func TestTagCommitPushesAndRepeats(t *testing.T) {
	ctx := context.Background()
	f := newRemote(t)
	r, _ := cloneFixture(t, f)
	head, _ := r.Head(ctx)
	name := RunTag("web01", "web", "20260101T000000Z-0a1b2c3d")
	for range 2 { // the second call is a repetition and no error
		if err := r.TagCommit(ctx, name, head); err != nil {
			t.Fatal(err)
		}
	}
	if got := fixtureGit(t, f.bare, "rev-parse", "refs/tags/"+name+"^{commit}"); got != head {
		t.Fatalf("remote tag = %s, want %s", got, head)
	}
	// A tag that is already on the remote at the same commit is also fine from a fresh copy.
	fixtureGit(t, r.dir, "tag", "-d", name)
	if err := r.TagCommit(ctx, name, head); err != nil {
		t.Fatal(err)
	}
	// Fetch does not bring in tags of others.
	other := f.commit(map[string]string{"web/compose.yaml": "b\n"})
	fixtureGit(t, f.bare, "tag", "qhost/other/web/20260102T000000Z-ffffffff", other)
	if err := r.Fetch(ctx); err != nil {
		t.Fatal(err)
	}
	if got := fixtureGit(t, r.dir, "tag", "-l", "qhost/other/*"); got != "" {
		t.Fatalf("fetch brought tags: %q", got)
	}
}

func TestTagCommitDifferentCommitIsAnError(t *testing.T) {
	ctx := context.Background()
	f := newRemote(t)
	r, _ := cloneFixture(t, f)
	first, _ := r.Head(ctx)
	next := f.commit(map[string]string{"web/compose.yaml": "b\n"})
	if err := r.Fetch(ctx); err != nil {
		t.Fatal(err)
	}
	if err := r.FastForward(ctx, next); err != nil {
		t.Fatal(err)
	}
	name := RunTag("web01", "web", "20260101T000000Z-0a1b2c3d")
	if err := r.TagCommit(ctx, name, first); err != nil {
		t.Fatal(err)
	}
	if err := r.TagCommit(ctx, name, next); !errors.Is(err, ErrTagMoved) {
		t.Fatalf("err = %v, want ErrTagMoved", err)
	}
	if got := fixtureGit(t, f.bare, "rev-parse", "refs/tags/"+name+"^{commit}"); got != first {
		t.Fatal("the remote tag moved")
	}
}

func TestTagCommitRejectedByRemote(t *testing.T) {
	ctx := context.Background()
	f := newRemote(t)
	hook := filepath.Join(f.bare, "hooks", "update")
	script := "#!/bin/sh\ncase \"$1\" in refs/tags/*) echo 'tags are protected' >&2; exit 1;; esac\n"
	if err := os.WriteFile(hook, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	r, _ := cloneFixture(t, f)
	head, _ := r.Head(ctx)
	name := RunTag("web01", "web", "20260101T000000Z-0a1b2c3d")
	err := r.TagCommit(ctx, name, head)
	if !errors.Is(err, ErrTagRejected) {
		t.Fatalf("err = %v, want ErrTagRejected", err)
	}
	if got := fixtureGit(t, f.bare, "tag", "-l"); got != "" {
		t.Fatalf("remote has tags: %q", got)
	}
}

func TestTagCommitOtherFailuresAreNotRejections(t *testing.T) {
	ctx := context.Background()
	f := newRemote(t)
	r, _ := cloneFixture(t, f)
	head, _ := r.Head(ctx)
	name := RunTag("web01", "web", "20260101T000000Z-0a1b2c3d")
	for _, bad := range []string{"web01/web", "qhost/a..b/x", "qhost/a b/x", "qhost/x.lock"} {
		if err := r.TagCommit(ctx, bad, head); err == nil {
			t.Errorf("accepted the tag name %q", bad)
		}
	}
	if err := r.TagCommit(ctx, name, "main"); err == nil {
		t.Error("accepted a non-id")
	}
	if err := r.TagCommit(ctx, name, strings.Repeat("0", 40)); err == nil {
		t.Error("accepted an unknown commit")
	}
	fixtureGit(t, r.dir, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "missing.git"))
	if err := r.TagCommit(ctx, name, head); err == nil || errors.Is(err, ErrTagRejected) {
		t.Fatalf("err = %v", err)
	}
}

func TestTagCommitLocalMode(t *testing.T) {
	ctx := context.Background()
	r := localFixture(t)
	head, _ := r.Head(ctx)
	name := RunTag("web01", "web", "20260101T000000Z-0a1b2c3d")
	for range 2 {
		if err := r.TagCommit(ctx, name, head); err != nil {
			t.Fatal(err)
		}
	}
	if got := fixtureGit(t, r.dir, "rev-parse", "refs/tags/"+name+"^{commit}"); got != head {
		t.Fatalf("tag = %s, want %s", got, head)
	}
	next := localCommit(t, r.dir, map[string]string{"db/compose.yaml": "b\n"})
	if err := r.TagCommit(ctx, name, next); !errors.Is(err, ErrTagMoved) {
		t.Fatalf("err = %v, want ErrTagMoved", err)
	}
}
