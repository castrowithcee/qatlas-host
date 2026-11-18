package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/castrowithcee/qatlas-kit/redact"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

var storeBase = time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)

// putLog writes one record with the given time through the store's handler.
func putLog(t *testing.T, h slog.Handler, at time.Duration, lvl slog.Level, msg string, attrs ...slog.Attr) {
	t.Helper()
	if !h.Enabled(context.Background(), lvl) {
		return
	}
	rec := slog.NewRecord(storeBase.Add(at), lvl, msg, 0)
	rec.AddAttrs(attrs...)
	if err := h.Handle(context.Background(), rec); err != nil {
		t.Fatal(err)
	}
}

func askLog(t *testing.T, s *logService, a control.ServiceLogArgs) control.ServiceLogResult {
	t.Helper()
	raw, _ := json.Marshal(a)
	v, err := s.handle(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	return v.(control.ServiceLogResult)
}

func messages(r control.ServiceLogResult) string {
	var m []string
	for _, e := range r.Entries {
		m = append(m, e.Message)
	}
	return strings.Join(m, ",")
}

func newTestStore(t *testing.T) (*logStore, *logService, *redact.Redactor) {
	t.Helper()
	st := newLogStore(filepath.Join(t.TempDir(), "service-log"))
	sec := &redact.Redactor{}
	return st, &logService{store: st, secrets: sec}, sec
}

func TestLogStoreMasksSecretsInFilesAndAnswers(t *testing.T) {
	const hidden = "example-log-hidden-not-real"
	st, svc, sec := newTestStore(t)
	sec.Add(hidden)
	var stderr bytes.Buffer
	lg := slog.New(redactHandler{next: teeHandler{primary: slog.NewTextHandler(&stderr, nil), store: st.handler()}, r: sec})
	lg.With("token", hidden).WithGroup("grp").Warn("failed with "+hidden, "value", hidden, "other", fmt.Errorf("wrapped %s", hidden))

	files := st.files()
	if len(files) != 1 {
		t.Fatalf("files = %v", files)
	}
	data, err := os.ReadFile(filepath.Join(st.dir, files[0]))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), hidden) || strings.Contains(stderr.String(), hidden) {
		t.Fatalf("a hidden reached a file or stderr: %s", data)
	}
	if !strings.Contains(stderr.String(), "failed with") {
		t.Fatalf("stderr lost the record: %q", stderr.String())
	}
	res := askLog(t, svc, control.ServiceLogArgs{})
	b, _ := json.Marshal(res)
	if len(res.Entries) != 1 || strings.Contains(string(b), hidden) {
		t.Fatalf("answer = %s", b)
	}
	e := res.Entries[0]
	if e.Level != "warn" || e.Source != "qhost" || !strings.HasPrefix(e.Message, "failed with ") {
		t.Fatalf("entry = %+v", e)
	}
	keys := map[string]bool{}
	for _, a := range e.Attrs {
		keys[a.Key] = true
	}
	if !keys["token"] || !keys["grp.value"] || !keys["grp.other"] {
		t.Fatalf("attrs = %+v", e.Attrs)
	}
	// A hidden registered after the line was stored is hidden in the answer too.
	sec.Add("failed")
	if b, _ := json.Marshal(askLog(t, svc, control.ServiceLogArgs{})); strings.Contains(string(b), "failed") {
		t.Fatalf("late hidden in answer: %s", b)
	}
}

func TestLogStoreKeepsItsBoundAndDropsTheOldest(t *testing.T) {
	st, svc, _ := newTestStore(t)
	st.maxBytes, st.maxFiles = 400, 3
	h := st.handler()
	for i := 0; i < 60; i++ {
		putLog(t, h, time.Duration(i)*time.Second, slog.LevelInfo, fmt.Sprintf("line-%02d", i))
	}
	files := st.files()
	if len(files) != 3 {
		t.Fatalf("files = %v, want 3", files)
	}
	var total int64
	for _, f := range files {
		fi, err := os.Stat(filepath.Join(st.dir, f))
		if err != nil {
			t.Fatal(err)
		}
		if fi.Size() > st.maxBytes {
			t.Fatalf("%s holds %d bytes, bound %d", f, fi.Size(), st.maxBytes)
		}
		total += fi.Size()
	}
	res := askLog(t, svc, control.ServiceLogArgs{})
	n := len(res.Entries)
	if n == 0 || n >= 60 || res.Entries[n-1].Message != "line-59" || res.Entries[0].Message == "line-00" {
		t.Fatalf("kept %d entries, from %q to %q", n, res.Entries[0].Message, res.Entries[n-1].Message)
	}
	for i := 1; i < n; i++ {
		if res.Entries[i].Time.Before(res.Entries[i-1].Time) {
			t.Fatal("entries out of order")
		}
	}
}

func TestLogStoreDropsFilesBeyondTheAge(t *testing.T) {
	st, _, _ := newTestStore(t)
	st.maxBytes = 200
	now := time.Now()
	st.now = func() time.Time { return now }
	h := st.handler()
	for i := 0; i < 4; i++ {
		putLog(t, h, 0, slog.LevelInfo, strings.Repeat("x", 150))
	}
	old := st.files()
	if len(old) < 2 {
		t.Fatalf("files = %v", old)
	}
	past := now.Add(-2 * st.maxAge)
	for _, f := range old {
		_ = os.Chtimes(filepath.Join(st.dir, f), past, past)
	}
	putLog(t, h, 0, slog.LevelInfo, strings.Repeat("y", 150))
	putLog(t, h, 0, slog.LevelInfo, strings.Repeat("y", 150))
	for _, f := range st.files() {
		if slicesContains(old, f) {
			t.Fatalf("old file %s kept", f)
		}
	}
}

func slicesContains(l []string, s string) bool {
	for _, v := range l {
		if v == s {
			return true
		}
	}
	return false
}

func TestLogStoreFilesArePrivate(t *testing.T) {
	st, _, _ := newTestStore(t)
	st.maxBytes = 100
	if err := os.MkdirAll(st.dir, 0o755); err != nil {
		t.Fatal(err)
	}
	h := st.handler()
	for i := 0; i < 3; i++ {
		putLog(t, h, 0, slog.LevelInfo, "a line that forces a rotation")
	}
	di, err := os.Stat(st.dir)
	if err != nil || di.Mode().Perm() != 0o700 {
		t.Fatalf("dir = %v, %v", di, err)
	}
	if len(st.files()) < 2 {
		t.Fatal("no rotation")
	}
	for _, f := range st.files() {
		fi, err := os.Stat(filepath.Join(st.dir, f))
		if err != nil || fi.Mode().Perm() != 0o600 {
			t.Fatalf("%s = %v, %v", f, fi, err)
		}
	}
}

func TestServiceLogFiltersByTimeLevelAndTail(t *testing.T) {
	st, svc, _ := newTestStore(t)
	h := st.handler()
	putLog(t, h, 1*time.Second, slog.LevelDebug, "d1") // below the handler's level: not stored
	putLog(t, h, 2*time.Second, slog.LevelInfo, "i1")
	putLog(t, h, 3*time.Second, slog.LevelWarn, "w1")
	putLog(t, h, 4*time.Second, slog.LevelError, "e1")
	putLog(t, h, 5*time.Second, slog.LevelInfo, "i2")

	for _, c := range []struct {
		a    control.ServiceLogArgs
		want string
	}{
		{control.ServiceLogArgs{}, "i1,w1,e1,i2"},
		{control.ServiceLogArgs{Level: "debug"}, "i1,w1,e1,i2"},
		{control.ServiceLogArgs{Level: "warn"}, "w1,e1"},
		{control.ServiceLogArgs{Level: "error"}, "e1"},
		{control.ServiceLogArgs{Since: storeBase.Add(4 * time.Second)}, "e1,i2"},
		{control.ServiceLogArgs{Tail: 2}, "e1,i2"},
		{control.ServiceLogArgs{Tail: 2, Level: "warn"}, "w1,e1"},
		{control.ServiceLogArgs{Tail: 9}, "i1,w1,e1,i2"},
		{control.ServiceLogArgs{Source: "qhost", Level: "info", Tail: 1}, "i2"},
	} {
		if got := messages(askLog(t, svc, c.a)); got != c.want {
			t.Errorf("%+v = %q, want %q", c.a, got, c.want)
		}
	}
	for _, a := range []control.ServiceLogArgs{
		{Level: "loud"}, {Tail: -1}, {Tail: control.ServiceLogMaxTail + 1}, {Source: "other"}, {Cursor: "nope"},
		{Tail: 1, Cursor: "00000000000000000001.jsonl:0"},
	} {
		raw, _ := json.Marshal(a)
		if _, err := svc.handle(context.Background(), raw); err == nil {
			t.Errorf("%+v accepted", a)
		}
	}
}

func TestServiceLogCursorContinuesWithoutGapsAcrossRotation(t *testing.T) {
	st, svc, _ := newTestStore(t)
	st.maxBytes, st.maxFiles = 300, 50
	h := st.handler()
	first := askLog(t, svc, control.ServiceLogArgs{Tail: 5})
	if len(first.Entries) != 0 {
		t.Fatalf("entries = %+v", first.Entries)
	}
	var seen []string
	cur := first.Cursor
	for i := 0; i < 40; i++ {
		putLog(t, h, time.Duration(i)*time.Second, slog.LevelInfo, fmt.Sprintf("m%02d", i))
		if i%7 == 0 {
			r := askLog(t, svc, control.ServiceLogArgs{Cursor: cur})
			for _, e := range r.Entries {
				seen = append(seen, e.Message)
			}
			cur = r.Cursor
		}
	}
	r := askLog(t, svc, control.ServiceLogArgs{Cursor: cur})
	for _, e := range r.Entries {
		seen = append(seen, e.Message)
	}
	if len(seen) != 40 {
		t.Fatalf("saw %d entries: %v", len(seen), seen)
	}
	for i, m := range seen {
		if m != fmt.Sprintf("m%02d", i) {
			t.Fatalf("entry %d = %s", i, m)
		}
	}
	if again := askLog(t, svc, control.ServiceLogArgs{Cursor: r.Cursor}); len(again.Entries) != 0 || again.Cursor != r.Cursor {
		t.Fatalf("again = %+v", again)
	}
}

func TestServiceLogMasksControlCharactersAndBoundsTheAnswer(t *testing.T) {
	st, svc, _ := newTestStore(t)
	h := st.handler()
	putLog(t, h, 0, slog.LevelWarn, "esc \x1b[31mred\x1b[0m", slog.String("k\x07", "v\x00al\nue"))
	e := askLog(t, svc, control.ServiceLogArgs{}).Entries[0]
	if e.Message != "esc �[31mred�[0m" || e.Attrs[0].Key != "k�" || e.Attrs[0].Value != "v�al�ue" {
		t.Fatalf("entry = %+v", e)
	}
	big := strings.Repeat("z", 100<<10)
	for i := 0; i < 20; i++ {
		putLog(t, h, time.Duration(i+1)*time.Second, slog.LevelInfo, big, slog.String("a", big))
	}
	res := askLog(t, svc, control.ServiceLogArgs{})
	b, _ := json.Marshal(res)
	if len(b) > control.MaxResponseBytes*3/4 || len(res.Entries[1].Message) > logMessageBytes+4 {
		t.Fatalf("answer of %d bytes", len(b))
	}
}

func TestServiceLogHandlerLogsFromManyGoroutines(t *testing.T) {
	st, svc, _ := newTestStore(t)
	st.maxBytes = 2 << 10
	st.maxFiles = 100
	lg := slog.New(teeHandler{primary: slog.NewTextHandler(&bytes.Buffer{}, nil), store: st.handler()}).With("k", "v").WithGroup("g")
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				lg.Info("m", "n", i)
			}
		}()
	}
	wg.Wait()
	res := askLog(t, svc, control.ServiceLogArgs{})
	if len(res.Entries) != 400 {
		t.Fatalf("entries = %d, want 400", len(res.Entries))
	}
	if a := res.Entries[0].Attrs; len(a) != 2 || a[0].Key != "g.n" || a[1].Key != "k" {
		t.Fatalf("attrs = %+v", a)
	}
}

func TestTeeHandlerKeepsStderrWhenTheStoreFails(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	st := newLogStore(filepath.Join(blocker, "service-log")) // the folder cannot be created
	var stderr bytes.Buffer
	lg := slog.New(teeHandler{primary: slog.NewTextHandler(&stderr, nil), store: st.handler()})
	lg.Info("still here")
	lg.Error("and here")
	if !strings.Contains(stderr.String(), "still here") || !strings.Contains(stderr.String(), "and here") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}
