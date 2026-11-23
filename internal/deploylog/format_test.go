package deploylog

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	kit "github.com/castrowithcee/qatlas-kit/auditlog"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/formatnum"
)

func dayBytes(t *testing.T, dir string) []byte {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if len(files) != 1 {
		t.Fatalf("want one day file, got %v", files)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestNewLinesCarryTheFormatNumber(t *testing.T) {
	dir := t.TempDir()
	logger, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := logger.Append(Fields{ClientUID: 1000, Command: "demo.run"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(dayBytes(t, dir)), `"format":1`) {
		t.Error("the line has no format number")
	}
}

func TestUnknownOrMissingNumberIsRefusedAndFileKept(t *testing.T) {
	for name, format := range map[string]int{"higher": EntryFormat + 1, "missing": 0} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			raw, err := kit.New(kit.Options[Entry]{
				Dir: dir, LockName: lockName, Validate: func(Entry) error { return nil },
				RetentionDays: RetentionDays, Now: func() time.Time { return time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC) },
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := raw.Append(Entry{Format: format, Result: ResultSuccess}); err != nil {
				t.Fatal(err)
			}
			before := dayBytes(t, dir)
			if _, err := Read(dir, Query{Limit: 10}); !formatnum.Is(err) {
				t.Errorf("Read: %v", err)
			}
			if _, err := Verify(dir); !formatnum.Is(err) {
				t.Errorf("Verify: %v", err)
			}
			if !bytes.Equal(before, dayBytes(t, dir)) {
				t.Error("the day file changed")
			}
		})
	}
}

// releasedEntry is Entry as released before the grant member existed. Such a version decodes a line leniently,
// chains on the stored bytes and validates only what it appends, so the optional member keeps format 1.
type releasedEntry struct {
	Format     int     `json:"format"`
	ClientUID  *uint32 `json:"client_uid,omitempty"`
	Command    string  `json:"command,omitempty"`
	Class      string  `json:"class,omitempty"`
	Scope      string  `json:"scope,omitempty"`
	Stack      string  `json:"stack,omitempty"`
	Result     string  `json:"result,omitempty"`
	ErrorCode  string  `json:"error_code,omitempty"`
	DurationMS int64   `json:"duration_ms,omitempty"`
	Origin     string  `json:"origin,omitempty"`
}

func TestAReleasedVersionReadsAndChainsOnAnEntryWithAGrant(t *testing.T) {
	dir := t.TempDir()
	logger, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := logger.Append(Fields{ClientUID: 4100, Command: "stack.restart", Class: control.ClassChange, Scope: control.ScopeStack, Stack: "demo-stack", Grant: "g-0000abcd"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(dayBytes(t, dir)), `"grant":"g-0000abcd"`) {
		t.Fatal("the line does not name the grant")
	}
	released, err := kit.New(kit.Options[releasedEntry]{
		Dir: dir, LockName: lockName, RetentionDays: RetentionDays, TornLine: kit.TornLineDocument,
		Validate: func(e releasedEntry) error {
			if e.Format != EntryFormat {
				return formatnum.Check("released", "format", e.Format, EntryFormat)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	page, err := released.Read(kit.Query[releasedEntry]{Limit: 10})
	if err != nil || len(page.Records) != 1 || page.Records[0].Entry.Format != EntryFormat || page.Records[0].Entry.Stack != "demo-stack" {
		t.Fatalf("released read = %+v, %v", page, err)
	}
	if rep, err := released.Verify(nil); err != nil || rep.Broken {
		t.Fatalf("released verify = %+v, %v", rep, err)
	}
	if err := released.Append(releasedEntry{Format: EntryFormat, Command: "status", Result: ResultSuccess}); err != nil {
		t.Fatal(err)
	}
	if rep, err := Verify(dir); err != nil || rep.Broken {
		t.Fatalf("verify = %+v, %v", rep, err)
	}
	got, err := Read(dir, Query{Limit: 10})
	if err != nil || len(got.Records) != 2 || got.Records[0].Entry.Grant != "" || got.Records[1].Entry.Grant != "g-0000abcd" {
		t.Fatalf("read = %+v, %v", got, err)
	}
}

func TestAGrantOfAnotherFormIsRefused(t *testing.T) {
	for _, id := range []string{"x", "g-0000ABCD", "g-0000abcd\n", "g-0000abc"} {
		if err := (Entry{Format: EntryFormat, Result: ResultSuccess, Origin: control.OriginUnknown, Grant: id}).Validate(); err == nil {
			t.Errorf("grant %q passed", id)
		}
	}
	if err := (Entry{Format: EntryFormat, Result: ResultSuccess, Origin: control.OriginUnknown, Grant: "g-0000abcd"}).Validate(); err != nil {
		t.Error(err)
	}
}
