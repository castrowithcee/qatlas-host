package window

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/castrowithcee/qatlas-host/internal/formatnum/formatnumtest"
)

type lockedBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuf) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// hostRig is an instance with a host window file in a temporary folder.
type hostRig struct {
	s     *Scheduler
	clock *fakeClock
	job   *jobRig
	state string
	host  string
	logs  *lockedBuf
}

func newHostRig(t *testing.T, now time.Time) *hostRig {
	t.Helper()
	r := &hostRig{clock: newClock(now), job: newJob(), state: t.TempDir(), host: filepath.Join(t.TempDir(), "etc", "window.json"), logs: &lockedBuf{}}
	s, err := New(Config{StateDir: r.state, HostFile: r.host, HostPoll: 5 * time.Millisecond, Job: r.job.run, Clock: r.clock,
		Logger: slog.New(slog.NewTextHandler(r.logs, nil))})
	if err != nil {
		t.Fatal(err)
	}
	r.s = s
	return r
}

func (r *hostRig) setHost(t *testing.T, c Change) Window {
	t.Helper()
	w, err := SetHost(r.host, "", c)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func TestWithoutAnyWindowTheDefaultApplies(t *testing.T) {
	r := newHostRig(t, monday)
	st, err := r.s.Status()
	if err != nil || st.Window != Default() || st.Origin != OriginHost || st.Warning != "" {
		t.Fatalf("status = %+v, %v", st, err)
	}
}

func TestAnInstanceFollowsTheHostWindowAndSeesAChangeWithoutRestart(t *testing.T) {
	r := newHostRig(t, monday)
	r.setHost(t, Change{Zone: ptr("UTC"), Start: ptr(180), Duration: ptr(time.Hour)})
	st, err := r.s.Status()
	if err != nil || st.Origin != OriginHost || st.Window.Start != 180 || st.Window.Zone != "UTC" {
		t.Fatalf("status = %+v, %v", st, err)
	}
	// the same scheduler, no restart: the next read sees the new file
	r.setHost(t, Change{Start: ptr(300)})
	if st, _ := r.s.Status(); st.Window.Start != 300 || st.Window.Duration != time.Hour || !st.Next.Equal(utc(2026, 11, 2, 5, 0)) {
		t.Fatalf("status after the change = %+v", st)
	}
}

func TestARunningSchedulerOpensForAHostWindowThatAppearsLater(t *testing.T) {
	// noon: the default window is closed in every zone
	r := newHostRig(t, utc(2026, 11, 2, 12, 0))
	start(t, r.s)
	r.clock.nextWait(t)
	r.job.idle(t)
	// the host window opens now: the scheduler wakes without a restart and without a timer
	r.setHost(t, Change{Zone: ptr("UTC"), Start: ptr(720), Duration: ptr(time.Hour)})
	r.job.called(t)
	r.job.done()
}

func TestAnOwnWindowBeatsTheHostWindowUntilItIsReset(t *testing.T) {
	r := newHostRig(t, monday)
	r.setHost(t, Change{Zone: ptr("UTC"), Start: ptr(180)})
	st, err := r.s.Set(Change{Start: ptr(240)})
	if err != nil || st.Origin != OriginOwn || st.Window.Start != 240 || st.Window.Zone != "UTC" {
		t.Fatalf("own = %+v, %v", st, err)
	}
	r.setHost(t, Change{Start: ptr(360)})
	if st, _ := r.s.Status(); st.Origin != OriginOwn || st.Window.Start != 240 {
		t.Fatalf("the host change reached an own window: %+v", st)
	}
	st, err = r.s.Reset()
	if err != nil || st.Origin != OriginHost || st.Window.Start != 360 {
		t.Fatalf("reset = %+v, %v", st, err)
	}
	// the reset instance keeps following the host
	r.setHost(t, Change{Start: ptr(420)})
	if st, _ := r.s.Status(); st.Origin != OriginHost || st.Window.Start != 420 {
		t.Fatalf("after reset = %+v", st)
	}
}

func TestResetWithoutAHostWindowFallsBackToTheDefault(t *testing.T) {
	r := newHostRig(t, monday)
	set(t, r.s, Change{Zone: ptr("UTC"), Start: ptr(500)})
	st, err := r.s.Reset()
	if err != nil || st.Origin != OriginHost || st.Window != Default() {
		t.Fatalf("reset = %+v, %v", st, err)
	}
}

func TestAnOwnWindowStartsFromTheWindowThatApplied(t *testing.T) {
	r := newHostRig(t, monday)
	r.setHost(t, Change{Days: ptr(mustDays(t, "sat")), Zone: ptr("UTC"), Start: ptr(180), Duration: ptr(3 * time.Hour)})
	st := set(t, r.s, Change{Start: ptr(200)})
	if st.Window.Days != mustDays(t, "sat") || st.Window.Duration != 3*time.Hour || st.Window.Start != 200 {
		t.Fatalf("own = %+v", st.Window)
	}
}

func TestAMissingOrBrokenHostFileGivesTheDefaultAndAWarning(t *testing.T) {
	for name, content := range map[string]string{
		"malformed":     `{"schema":1,"days":["mon"]`,
		"invalid value": `{"schema":1,"days":["funday"],"start":"03:00","duration":"2h","zone":"UTC"}`,
		"higher number": `{"schema":2,"days":["mon"],"start":"03:00","duration":"2h","zone":"UTC"}`,
		"no number":     `{"days":["mon"],"start":"03:00","duration":"2h","zone":"UTC"}`,
	} {
		t.Run(name, func(t *testing.T) {
			r := newHostRig(t, monday)
			formatnumtest.Seed(t, r.host, content)
			for range 2 {
				st, err := r.s.Status()
				if err != nil || st.Window != Default() || st.Origin != OriginHost || !strings.Contains(st.Warning, r.host) {
					t.Fatalf("status = %+v, %v", st, err)
				}
			}
			if n := strings.Count(r.logs.String(), "so the default window applies"); n != 1 {
				t.Fatalf("the warning was logged %d times:\n%s", n, r.logs.String())
			}
			// the instance does not crash and still schedules
			if _, err := r.s.Set(Change{Start: ptr(100)}); err != nil {
				t.Fatalf("an own window cannot be set: %v", err)
			}
			formatnumtest.Unchanged(t, r.host, content)
			// the warning goes when the file is fixed
			if err := os.Remove(r.host); err != nil {
				t.Fatal(err)
			}
			if _, err := r.s.Reset(); err != nil {
				t.Fatal(err)
			}
			if st, _ := r.s.Status(); st.Warning != "" {
				t.Fatalf("warning stays: %+v", st)
			}
		})
	}
}

func TestAHostFileWithAnUnknownFormatNumberIsRefusedAndKept(t *testing.T) {
	for name, doc := range formatnumtest.Cases("schema", hostSchema, `"days":["mon"],"start":"03:00","duration":"2h","zone":"UTC"`) {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "etc", "window.json")
			formatnumtest.Seed(t, path, doc)
			_, err := SetHost(path, "", Change{Zone: ptr("UTC")})
			formatnumtest.Refused(t, err, path)
			formatnumtest.Unchanged(t, path, doc)
		})
	}
}

func TestTheHostFileIsWorldReadableAndCarriesItsFormatNumber(t *testing.T) {
	path := filepath.Join(t.TempDir(), "etc", "window.json")
	if _, err := SetHost(path, "", Change{Start: ptr(90)}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o644 {
		t.Fatalf("mode = %v, %v", info, err)
	}
	if dir, err := os.Stat(filepath.Dir(path)); err != nil || dir.Mode().Perm() != 0o755 {
		t.Fatalf("folder mode = %v, %v", dir, err)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), `"schema": 1`) {
		t.Fatalf("no format number: %s", b)
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Fatalf("temporary files stay: %v", entries)
	}
	w, ok, err := ReadHost(path)
	if err != nil || !ok || w.Start != 90 {
		t.Fatalf("read back = %+v, %v, %v", w, ok, err)
	}
}

func TestAnExistingWindowOfTheServiceBecomesTheHostWindow(t *testing.T) {
	state, host := t.TempDir(), filepath.Join(t.TempDir(), "etc", "window.json")
	legacy := `{"schema":1,"days":["sat","sun"],"start":"01:15","duration":"3h","zone":"UTC","last_run":"2026-11-01T01:15:00Z"}`
	formatnumtest.Seed(t, filepath.Join(state, fileName), legacy)

	// the stored window of the old version is read as one that follows the host
	s, err := New(Config{StateDir: state, HostFile: host, Job: newJob().run, Clock: newClock(monday)})
	if err != nil {
		t.Fatal(err)
	}
	if st, err := s.Status(); err != nil || st.Origin != OriginHost || st.Window.Start != 75 || !st.LastRun.Equal(utc(2026, 11, 1, 1, 15)) {
		t.Fatalf("legacy status = %+v, %v", st, err)
	}
	// the first change of the host window starts from it and writes it into the file
	w, err := SetHost(host, state, Change{Duration: ptr(2 * time.Hour)})
	if err != nil || w.Start != 75 || w.Days != mustDays(t, "sat", "sun") || w.Duration != 2*time.Hour {
		t.Fatalf("host window = %+v, %v", w, err)
	}
	if st, _ := s.Status(); st.Origin != OriginHost || st.Window != w {
		t.Fatalf("instance after = %+v", st)
	}
}

func TestAdoptStateWritesTheFileOnceAndOnlyForAWindowThatWasSet(t *testing.T) {
	state, host := t.TempDir(), filepath.Join(t.TempDir(), "etc", "window.json")
	if ok, err := AdoptState(host, state); err != nil || ok {
		t.Fatalf("no state: %v, %v", ok, err)
	}
	formatnumtest.Seed(t, filepath.Join(state, fileName),
		`{"schema":1,"days":["daily"],"start":"03:00","duration":"2h","zone":"local"}`)
	if ok, err := AdoptState(host, state); err != nil || ok {
		t.Fatalf("the default is not adopted: %v, %v", ok, err)
	}
	formatnumtest.Seed(t, filepath.Join(state, fileName),
		`{"schema":1,"days":["mon"],"start":"04:00","duration":"1h","zone":"UTC"}`)
	if ok, err := AdoptState(host, state); err != nil || !ok {
		t.Fatalf("adopt: %v, %v", ok, err)
	}
	if w, ok, err := ReadHost(host); err != nil || !ok || w.Start != 240 {
		t.Fatalf("host = %+v, %v, %v", w, ok, err)
	}
	// an existing host window is never replaced
	formatnumtest.Seed(t, filepath.Join(state, fileName),
		`{"schema":1,"days":["tue"],"start":"05:00","duration":"1h","zone":"UTC"}`)
	if ok, err := AdoptState(host, state); err != nil || ok {
		t.Fatalf("second adopt: %v, %v", ok, err)
	}
	// an own window is not a window of the host
	other := filepath.Join(t.TempDir(), "window.json")
	formatnumtest.Seed(t, filepath.Join(state, fileName),
		`{"schema":2,"own":true,"days":["tue"],"start":"05:00","duration":"1h","zone":"UTC"}`)
	if ok, err := AdoptState(other, state); err != nil || ok {
		t.Fatalf("own window adopted: %v, %v", ok, err)
	}
}

func TestTheStoredWindowOfAnInstanceCarriesTheOriginAndTheFormatNumber(t *testing.T) {
	r := newHostRig(t, monday)
	set(t, r.s, Change{Zone: ptr("UTC")})
	b, err := os.ReadFile(filepath.Join(r.state, fileName))
	if err != nil || !strings.Contains(string(b), `"schema": 2`) || !strings.Contains(string(b), `"own": true`) {
		t.Fatalf("stored = %s, %v", b, err)
	}
	if _, err := r.s.Reset(); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(filepath.Join(r.state, fileName))
	if strings.Contains(string(b), `"own"`) {
		t.Fatalf("the own mark stays after a reset: %s", b)
	}
}
