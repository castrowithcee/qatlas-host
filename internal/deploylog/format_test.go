package deploylog

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	kit "github.com/castrowithcee/qatlas-kit/auditlog"

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
