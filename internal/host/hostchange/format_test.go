package hostchange

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/castrowithcee/qatlas-host/internal/formatnum/formatnumtest"
)

func TestJournalWritesItsFormatNumber(t *testing.T) {
	j, err := OpenJournal(filepath.Join(t.TempDir(), "host", "journal"), os.Getuid())
	if err != nil {
		t.Fatal(err)
	}
	id, err := newID(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := j.Write(Entry{ID: id, Action: "example"}); err != nil {
		t.Fatal(err)
	}
	if e, err := j.Get(id); err != nil || e.Schema != journalSchema {
		t.Fatalf("entry = %+v, %v", e, err)
	}
}

func TestJournalEntryWithoutFormatNumberIsReadAsFormat1AndKept(t *testing.T) {
	j, err := OpenJournal(filepath.Join(t.TempDir(), "host", "journal"), os.Getuid())
	if err != nil {
		t.Fatal(err)
	}
	const id = "20260102T030405.000000Z-0a1b2c"
	doc := formatnumtest.Cases("schema", journalSchema, `"id":"`+id+`","action":"example"`)["missing"]
	path := j.path(id)
	formatnumtest.Seed(t, path, doc)
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if e, err := j.Get(id); err != nil || e.ID != id || e.Action != "example" {
		t.Fatalf("entry = %+v, %v", e, err)
	}
	if list, err := j.List(); err != nil || len(list) != 1 {
		t.Fatalf("list = %v, %v", list, err)
	}
	formatnumtest.Unchanged(t, path, doc)
}

func TestUnknownJournalFormatNumberIsRefusedAndKept(t *testing.T) {
	for name, doc := range formatnumtest.Cases("schema", journalSchema, `"id":"20260102T030405.000000Z-0a1b2c"`) {
		if name == "missing" {
			continue
		}
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "host", "journal")
			j, err := OpenJournal(dir, os.Getuid())
			if err != nil {
				t.Fatal(err)
			}
			const id = "20260102T030405.000000Z-0a1b2c"
			path := j.path(id)
			formatnumtest.Seed(t, path, doc)
			if err := os.Chmod(path, 0o600); err != nil {
				t.Fatal(err)
			}

			_, err = j.Get(id)
			formatnumtest.Refused(t, err, path)
			_, err = j.List()
			formatnumtest.Refused(t, err, path)
			formatnumtest.Refused(t, j.Write(Entry{ID: id, Action: "example"}), path)

			// Newer entries push it beyond the number kept; it is not removed.
			for i := range 3 {
				nid, err := newID(time.Date(2026, 2, 1+i, 0, 0, 0, 0, time.UTC))
				if err != nil {
					t.Fatal(err)
				}
				if err := j.Write(Entry{ID: nid, Action: "example"}); err != nil {
					t.Fatal(err)
				}
			}
			if err := j.Prune(1); err != nil {
				t.Fatal(err)
			}
			formatnumtest.Unchanged(t, path, doc)
		})
	}
}
