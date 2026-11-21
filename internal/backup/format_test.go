package backup

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/castrowithcee/qatlas-host/internal/formatnum/formatnumtest"
)

func TestUnknownFormatNumberIsRefusedAndKept(t *testing.T) {
	const id = "20260101T000000Z-00000000"
	members := `"id":"` + id + `","stack":"demo","created":"2026-01-01T00:00:00Z","items":[]`
	for name, doc := range formatnumtest.Cases("schema", metaSchema, members) {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t, "version: 1\nbackup:\n  paths: [data/db]\n  retention: {keep: 1}\n")
			meta := filepath.Join(e.root, "demo", id, metaFile)
			formatnumtest.Seed(t, meta, doc)

			list, err := e.m.List("demo")
			if err != nil || len(list) != 1 || !list[0].Incomplete {
				t.Fatalf("list = %+v, %v", list, err)
			}
			formatnumtest.Refused(t, list[0].Unsupported, meta)
			if ok, err := e.m.Exists("demo", id); err != nil || ok {
				t.Fatalf("exists = %v, %v", ok, err)
			}
			_, err = e.m.Restore(context.Background(), e.stack(), id, e.log)
			formatnumtest.Refused(t, err, meta)

			// Retention that removes every older backup and sweeps incomplete ones leaves it alone.
			for range 3 {
				e.now = e.now.Add(time.Hour)
				if _, err := e.create(); err != nil {
					t.Fatal(err)
				}
			}
			if removed, err := e.m.retain("demo", 1, 0, ""); err != nil {
				t.Fatalf("removed %v, %v", removed, err)
			}
			formatnumtest.Unchanged(t, meta, doc)
		})
	}
}
