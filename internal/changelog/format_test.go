package changelog

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/castrowithcee/qatlas-host/internal/formatnum/formatnumtest"
)

func TestUnknownFormatNumberIsKept(t *testing.T) {
	members := `"repo":"example-org/example-app","fetched_at":"2026-03-01T12:00:00Z","releases":[]`
	for name, doc := range formatnumtest.Cases("version", cacheVersion, members) {
		t.Run(name, func(t *testing.T) {
			f := newFake(t, standardReleases())
			dir := filepath.Join(t.TempDir(), "cache")
			c := f.client(Options{CacheDir: dir})
			path := c.cachePath(testSource.Repo)
			formatnumtest.Seed(t, path, doc)

			if _, ok := c.readCache(testSource.Repo); ok {
				t.Fatal("a cache with an unknown format number was read")
			}
			if err := unreadableFormatError(path); err == nil {
				t.Fatal("the format number was not refused")
			} else {
				formatnumtest.Refused(t, err, path)
			}
			// Asking again fetches but never replaces the file, and cleaning never removes it.
			c.Changelogs(context.Background(), testSource, "1.2.3", "1.4.0")
			old := time.Now().Add(-48 * time.Hour)
			if err := os.Chtimes(path, old, old); err != nil {
				t.Fatal(err)
			}
			if n := CleanCache(dir, time.Hour, time.Now()); n != 0 {
				t.Fatalf("removed %d", n)
			}
			formatnumtest.Unchanged(t, path, doc)
		})
	}
}
