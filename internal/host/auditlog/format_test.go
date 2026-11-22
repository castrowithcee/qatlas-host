package auditlog

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

// numberless is an entry as a version before the format number wrote it.
type numberless struct {
	ClientUID *uint32 `json:"client_uid,omitempty"`
	Action    string  `json:"action,omitempty"`
	Result    string  `json:"result,omitempty"`
}

var testDay = time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)

func testClock() time.Time { return testDay }

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
	if err := logger.Append(Fields{Action: OperationLogRead, Result: ResultSuccess}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(dayBytes(t, dir)), `"format":1`) {
		t.Error("the line has no format number")
	}
	if (Entry{Action: OperationLogRead, Result: ResultSuccess}).Validate() == nil {
		t.Error("an entry without a number is accepted")
	}
}

func TestDayFileWithOldAndNewLinesReadsWhole(t *testing.T) {
	dir := t.TempDir()
	old, err := kit.New(kit.Options[numberless]{
		Dir: dir, LockName: lockName, Validate: func(numberless) error { return nil },
		RetentionDays: RetentionDays, Now: testClock,
	})
	if err != nil {
		t.Fatal(err)
	}
	uid := uint32(1000)
	for range 2 {
		if err := old.Append(numberless{ClientUID: &uid, Action: OperationLogRead, Result: ResultSuccess}); err != nil {
			t.Fatal(err)
		}
	}
	kl, err := newLog(dir, testClock)
	if err != nil {
		t.Fatal(err)
	}
	if err := (&Logger{log: kl}).Append(Fields{ClientUID: 1000, Action: OperationLogRead, Result: ResultSuccess}); err != nil {
		t.Fatal(err)
	}
	before := dayBytes(t, dir)

	page, err := Read(dir, Query{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Records) != 3 || page.Records[0].Entry.Format != EntryFormat || page.Records[2].Entry.Format != 0 {
		t.Fatalf("records: %+v", page.Records)
	}
	report, err := Verify(dir)
	if err != nil || report.Broken {
		t.Fatalf("chain: %+v, %v", report, err)
	}
	if !bytes.Equal(before, dayBytes(t, dir)) {
		t.Error("reading changed the day file")
	}
}

func TestUnknownNumberIsRefusedAndFileKept(t *testing.T) {
	dir := t.TempDir()
	future, err := kit.New(kit.Options[Entry]{
		Dir: dir, LockName: lockName, Validate: func(Entry) error { return nil },
		RetentionDays: RetentionDays, Now: testClock,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := future.Append(Entry{Format: EntryFormat + 1, Action: OperationLogRead, Result: ResultSuccess}); err != nil {
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
}
