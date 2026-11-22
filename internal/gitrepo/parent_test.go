package gitrepo

import (
	"context"
	"testing"
)

func TestParentOfAndFileAt(t *testing.T) {
	ctx := context.Background()
	f := newRemote(t)
	r, _ := cloneFixture(t, f)
	first, _ := r.Head(ctx)
	second := f.commit(map[string]string{"web/.env": "APP_VERSION=1.0.1\n", "web/compose.yaml": "b\n"})
	if err := r.Fetch(ctx); err != nil {
		t.Fatal(err)
	}

	if got, err := r.ParentOf(ctx, second); err != nil || got != first {
		t.Fatalf("ParentOf = %q, %v; want %s", got, err, first)
	}
	if _, err := r.ParentOf(ctx, first); err == nil {
		t.Fatal("the first commit has no parent")
	}
	for _, bad := range []string{"HEAD", "main", second[:12], "--all"} {
		if _, err := r.ParentOf(ctx, bad); err == nil {
			t.Fatalf("ParentOf accepted %q", bad)
		}
	}

	if got, err := r.FileAt(ctx, second, "web/compose.yaml"); err != nil || string(got) != "b\n" {
		t.Fatalf("FileAt second = %q, %v", got, err)
	}
	if got, err := r.FileAt(ctx, first, "web/compose.yaml"); err != nil || string(got) != "a\n" {
		t.Fatalf("FileAt first = %q, %v", got, err)
	}
	if got, err := r.FileAt(ctx, first, "web/.env"); err != nil || got != nil {
		t.Fatalf("a file the commit lacks = %q, %v; want nil", got, err)
	}
	for _, bad := range []string{"web", "../web/compose.yaml", "/web/compose.yaml", ".git/config", "web/../db/compose.yaml"} {
		if _, err := r.FileAt(ctx, second, bad); err == nil {
			t.Fatalf("FileAt accepted path %q", bad)
		}
	}
	if _, err := r.FileAt(ctx, "HEAD", "web/compose.yaml"); err == nil {
		t.Fatal("FileAt accepted a ref name")
	}
}
