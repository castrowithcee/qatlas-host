package usagehistory

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/castrowithcee/qatlas-kit/sqlite"

	"github.com/castrowithcee/qatlas-host/internal/formatnum/formatnumtest"
	"github.com/castrowithcee/qatlas-host/internal/host/usage"
)

// base is a made-up UTC time on an hour boundary.
var base = time.Date(2001, 2, 3, 4, 0, 0, 0, time.UTC)

func sample(at time.Duration, cpu float64, fsUsed uint64) Measurement {
	return Measurement{Time: base.Add(at), Usage: usage.Usage{
		CPUPercent:  &cpu,
		Memory:      &usage.Memory{UsedBytes: 1000},
		Filesystems: []usage.Filesystem{{Mount: "/data", UsedBytes: fsUsed, TotalBytes: 5000}},
	}}
}

func openTemp(t *testing.T, cfg Config) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "metrics.db")
	s, err := Open(path, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, path
}

func write(t *testing.T, s *Store, batch ...Measurement) {
	t.Helper()
	if err := s.Write(context.Background(), batch); err != nil {
		t.Fatal(err)
	}
}

func count(t *testing.T, s *Store, table string) (n int) {
	t.Helper()
	if err := s.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestMigrationCountIsSchemaVersion(t *testing.T) {
	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil || len(entries) != schemaVersion {
		t.Fatalf("%d migrations, err %v, want %d", len(entries), err, schemaVersion)
	}
}

func TestTiersAggregateMeanAndMax(t *testing.T) {
	s, _ := openTemp(t, DefaultConfig())
	m := time.Minute
	write(t, s, sample(0, 10, 100), sample(10*time.Second, 20, 200), sample(20*time.Second, 30, 300), sample(m, 50, 400))
	if n := count(t, s, "minute"); n != 1 {
		t.Fatalf("%d minutes closed, want only the first", n)
	}
	write(t, s, sample(2*m, 70, 500))
	var avg, maxv float64
	var fsAvg float64
	var fsMax, total int64
	if err := s.db.QueryRow("SELECT cpu_percent_avg, cpu_percent_max FROM minute WHERE ts = ?", base.Unix()).Scan(&avg, &maxv); err != nil {
		t.Fatal(err)
	}
	if avg != 20 || maxv != 30 {
		t.Fatalf("minute 0: avg %v max %v, want 20 and 30", avg, maxv)
	}
	if err := s.db.QueryRow("SELECT used_avg, used_max, total FROM minute_fs WHERE ts = ? AND mount = '/data'", base.Unix()).Scan(&fsAvg, &fsMax, &total); err != nil {
		t.Fatal(err)
	}
	if fsAvg != 200 || fsMax != 300 || total != 5000 {
		t.Fatalf("minute 0 file system: %v %v %v", fsAvg, fsMax, total)
	}
	if n := count(t, s, "minute"); n != 2 {
		t.Fatalf("%d minutes, want the two closed ones", n)
	}
}

func TestHoursAggregateMinutes(t *testing.T) {
	s, _ := openTemp(t, DefaultConfig())
	m := time.Minute
	write(t, s, sample(0, 10, 100), sample(10*time.Second, 20, 200), sample(20*time.Second, 30, 300), sample(m, 50, 400), sample(2*m, 70, 500))
	var load any = 1
	if err := s.db.QueryRow("SELECT load_one_avg FROM minute WHERE ts = ?", base.Unix()).Scan(&load); err != nil || load != nil {
		t.Fatalf("a value that was not measured must stay empty: %v %v", load, err)
	}
	write(t, s, sample(time.Hour, 5, 600))
	var avg, maxv float64
	if err := s.db.QueryRow("SELECT cpu_percent_avg, cpu_percent_max FROM hour WHERE ts = ?", base.Unix()).Scan(&avg, &maxv); err != nil {
		t.Fatal(err)
	}
	// Minute means 20, 50 and 70; the hour is the mean of the minutes and the highest minute maximum.
	if avg != (20.0+50+70)/3 || maxv != 70 {
		t.Fatalf("hour: avg %v max %v", avg, maxv)
	}
	if n := count(t, s, "hour_fs"); n != 1 {
		t.Fatalf("%d hour file system rows, want 1", n)
	}
}

func TestGapStaysEmpty(t *testing.T) {
	s, _ := openTemp(t, DefaultConfig())
	write(t, s, sample(0, 10, 1), sample(5*time.Minute, 20, 1), sample(6*time.Minute, 30, 1))
	if n := count(t, s, "minute"); n != 2 {
		t.Fatalf("%d minutes, want only the two with data", n)
	}
	if n := count(t, s, "raw"); n != 3 {
		t.Fatalf("%d raw rows, want 3", n)
	}
}

func TestClockStepBackKeepsStoredRows(t *testing.T) {
	s, _ := openTemp(t, DefaultConfig())
	write(t, s, sample(0, 10, 100), sample(time.Minute, 20, 100), sample(2*time.Minute, 30, 100))
	write(t, s, sample(0, 99, 999), sample(time.Minute, 99, 999), sample(3*time.Minute, 40, 100))
	var cpu, avg float64
	var used int64
	if err := s.db.QueryRow("SELECT cpu_percent FROM raw WHERE ts = ?", base.Unix()).Scan(&cpu); err != nil || cpu != 10 {
		t.Fatalf("raw row replaced: %v %v", cpu, err)
	}
	if err := s.db.QueryRow("SELECT used FROM raw_fs WHERE ts = ?", base.Unix()).Scan(&used); err != nil || used != 100 {
		t.Fatalf("raw file system row replaced: %v %v", used, err)
	}
	if err := s.db.QueryRow("SELECT cpu_percent_avg FROM minute WHERE ts = ?", base.Unix()).Scan(&avg); err != nil || avg != 10 {
		t.Fatalf("minute replaced: %v %v", avg, err)
	}
}

func TestPruneFollowsRetentionPerTier(t *testing.T) {
	cfg := Config{Enabled: true, Raw: time.Hour, Minutes: 2 * time.Hour, Hours: 3 * time.Hour}
	s, _ := openTemp(t, cfg)
	for h := range 6 {
		at := time.Duration(h) * time.Hour
		write(t, s, sample(at, 10, 1), sample(at+30*time.Minute, 20, 1))
	}
	write(t, s, sample(6*time.Hour, 10, 1))
	// Closed: minutes at :00 and :30 of hours 0 to 5, hours 0 to 5.
	now := base.Add(6 * time.Hour)
	if err := s.Prune(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	// Raw keeps 5:00 and later: 5:00, 5:30, 6:00. Minutes keep 4:00 and later: 4:00, 4:30, 5:00, 5:30.
	// Hours keep 3:00 and later: hours 3, 4 and 5.
	for table, want := range map[string]int{"raw": 3, "raw_fs": 3, "minute": 4, "minute_fs": 4, "hour": 3, "hour_fs": 3} {
		if n := count(t, s, table); n != want {
			t.Errorf("%s: %d rows, want %d", table, n, want)
		}
	}
}

func TestOpenRejectsNonPositiveRetention(t *testing.T) {
	if _, err := Open(filepath.Join(t.TempDir(), "m.db"), Config{Enabled: true}); err == nil {
		t.Fatal("a zero retention was accepted")
	}
}

func TestReopenKeepsData(t *testing.T) {
	s, path := openTemp(t, DefaultConfig())
	write(t, s, sample(0, 10, 1))
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s2, err := Open(path, DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if n := count(t, s2, "raw"); n != 1 {
		t.Fatalf("%d raw rows after reopen, want 1", n)
	}
}

// refusedUnchanged checks that Open refuses the file with the format error, leaves its bytes as they were and
// creates no lock file or backup beside it.
func refusedUnchanged(t *testing.T, path string) {
	t.Helper()
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(path)
	listing := func() []string {
		entries, _ := os.ReadDir(dir)
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		return names
	}
	names := listing()
	_, err = Open(path, DefaultConfig())
	formatnumtest.Refused(t, err, path)
	formatnumtest.Unchanged(t, path, string(before))
	if got := listing(); len(got) != len(names) {
		t.Fatalf("the folder changed from %v to %v", names, got)
	}
}

func TestNewerDatabaseIsRefusedUnchanged(t *testing.T) {
	s, path := openTemp(t, DefaultConfig())
	write(t, s, sample(0, 10, 1))
	if _, err := s.db.Exec("INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, 'later', '2099-01-01T00:00:00Z')", schemaVersion+1); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	refusedUnchanged(t, path)
}

func TestForeignDatabaseIsRefusedUnchanged(t *testing.T) {
	dir := t.TempDir()
	foreign := filepath.Join(dir, "foreign.db")
	kit, err := sqlite.Open(foreign, sqlite.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := kit.SQL().Exec("CREATE TABLE notes (id INTEGER PRIMARY KEY, body TEXT); INSERT INTO notes (body) VALUES ('made up')"); err != nil {
		t.Fatal(err)
	}
	if err := kit.Close(); err != nil {
		t.Fatal(err)
	}
	refusedUnchanged(t, foreign)

	plain := filepath.Join(dir, "plain.db")
	formatnumtest.Seed(t, plain, "not a database, only some text that is longer than a header would be, "+
		"padded out so that it is well over one hundred and eight bytes long.")
	refusedUnchanged(t, plain)
}
