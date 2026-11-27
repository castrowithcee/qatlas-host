package hostservice

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/castrowithcee/qatlas-host/internal/host/usage"
	"github.com/castrowithcee/qatlas-host/internal/host/usagehistory"
)

// fakeStore records what the collector does to it.
type fakeStore struct {
	mu      sync.Mutex
	batches [][]usagehistory.Measurement
	pruned  []time.Time
	closed  bool
	failing bool
	ctxErr  error // the error of the context of the last write
}

func (f *fakeStore) Write(ctx context.Context, b []usagehistory.Measurement) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ctxErr = ctx.Err()
	if f.failing {
		return errors.New("disk full")
	}
	f.batches = append(f.batches, b)
	return nil
}

func (f *fakeStore) Prune(_ context.Context, now time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pruned = append(f.pruned, now)
	return nil
}

func (f *fakeStore) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

func (f *fakeStore) writes() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.batches)
}

// rig drives a collector with ticks it sends by hand and a clock it sets.
type rig struct {
	c       *collector
	store   *fakeStore
	mu      sync.Mutex // guards opened and clock, which the loop reads while the test moves on
	opened  []usagehistory.Config
	measure chan time.Time
	write   chan time.Time
	prune   chan time.Time
	clock   time.Time
	logs    *bytes.Buffer
	cancel  context.CancelFunc
	done    chan struct{}
	reads   chan struct{}
	// waitRead is false while the history is off, when a measure tick causes no read.
	waitRead bool
}

func newRig(t *testing.T, cfg usagehistory.Config) *rig {
	t.Helper()
	r := &rig{store: &fakeStore{}, measure: make(chan time.Time), write: make(chan time.Time), prune: make(chan time.Time),
		clock: time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC), logs: &bytes.Buffer{}, done: make(chan struct{}), reads: make(chan struct{}, 100), waitRead: cfg.Enabled}
	chans := map[time.Duration]chan time.Time{measureEvery: r.measure, writeEvery: r.write, pruneEvery: r.prune}
	cpu := 42.0
	hooks := historyHooks{
		Ticker: func(d time.Duration) (<-chan time.Time, func()) { return chans[d], func() {} },
		Read: func(context.Context) (usage.Usage, error) {
			r.reads <- struct{}{}
			return usage.Usage{CPUPercent: &cpu}, nil
		},
		Open: func(_ string, c usagehistory.Config) (historyStore, error) {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.opened = append(r.opened, c)
			return r.store, nil
		},
	}
	r.c = newCollector(slog.New(slog.NewTextHandler(r.logs, nil)), t.TempDir(), r.now, cfg, hooks)
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	go func() { defer close(r.done); r.c.run(ctx) }()
	t.Cleanup(r.stop)
	if cfg.Enabled {
		r.waitOpened(t, 1) // the loop takes its first configuration before any tick
	}
	return r
}

func (r *rig) now() time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.clock
}

func (r *rig) advance(d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clock = r.clock.Add(d)
}

func (r *rig) stop() { r.cancel(); <-r.done }

// tick sends n ticks and returns once the loop has handled each. A measurement is known to be timed when
// its read starts; any other tick is followed by a write tick on an empty buffer, which the loop takes only
// after it finished the first.
func (r *rig) tick(ch chan time.Time, n int) {
	for range n {
		ch <- r.now()
		if ch == r.measure && r.waitRead {
			<-r.reads
		} else {
			r.write <- r.now()
		}
		r.advance(measureEvery)
	}
}

func TestWritesAtMostOncePerMinute(t *testing.T) {
	r := newRig(t, usagehistory.DefaultConfig())
	r.tick(r.measure, 6)
	r.tick(r.measure, 6) // twelve measurements, no write tick yet
	if n := r.store.writes(); n != 0 {
		t.Fatalf("%d writes before the minute ended", n)
	}
	r.tick(r.write, 1)
	r.tick(r.measure, 6)
	r.tick(r.write, 1)
	r.tick(r.write, 1) // an empty buffer writes nothing
	r.stop()
	if n := r.store.writes(); n != 2 || len(r.store.batches[0]) != 12 || len(r.store.batches[1]) != 6 {
		t.Fatalf("batches %v", r.store.batches)
	}
}

func TestBufferIsWrittenAtStopWithItsOwnContext(t *testing.T) {
	r := newRig(t, usagehistory.DefaultConfig())
	r.tick(r.measure, 3)
	r.stop()
	if len(r.store.batches) != 1 || len(r.store.batches[0]) != 3 || !r.store.closed || r.store.ctxErr != nil {
		t.Fatalf("batches %v closed %v ctx error %v", r.store.batches, r.store.closed, r.store.ctxErr)
	}
}

func TestNothingIsFilledAfterARestart(t *testing.T) {
	r := newRig(t, usagehistory.DefaultConfig())
	r.tick(r.measure, 2)
	r.stop()
	first := r.store.batches[0]
	r2 := newRig(t, usagehistory.DefaultConfig())
	r2.advance(r.now().Add(time.Hour).Sub(r2.now()))
	r2.tick(r2.measure, 2)
	r2.stop()
	second := r2.store.batches[0]
	if len(first) != 2 || len(second) != 2 || second[0].Time.Sub(first[1].Time) < time.Hour {
		t.Fatalf("first %v second %v", first, second)
	}
}

func TestFailedWriteDropsBufferAndThrottlesTheMessage(t *testing.T) {
	r := newRig(t, usagehistory.DefaultConfig())
	r.store.failing = true
	for range 3 {
		r.tick(r.measure, 1)
		r.tick(r.write, 1)
	}
	r.tick(r.measure, 1)
	r.store.mu.Lock()
	r.store.failing = false
	r.store.mu.Unlock()
	r.tick(r.write, 1)
	r.stop()
	if len(r.store.batches) != 1 || len(r.store.batches[0]) != 1 {
		t.Fatalf("a failed batch was kept: %v", r.store.batches)
	}
	if n := strings.Count(r.logs.String(), "level=WARN"); n != 1 {
		t.Fatalf("%d warnings within the hour, want 1:\n%s", n, r.logs)
	}
}

func TestWarningComesAgainAfterAnHourWithTheCount(t *testing.T) {
	r := newRig(t, usagehistory.DefaultConfig())
	r.store.failing = true
	for range 3 {
		r.tick(r.measure, 1)
		r.tick(r.write, 1)
	}
	r.advance(warnEvery)
	r.tick(r.measure, 1)
	r.tick(r.write, 1)
	r.stop()
	logs := r.logs.String()
	if strings.Count(logs, "level=WARN") != 2 || !strings.Contains(logs, "failures_since_last_message=2") {
		t.Fatalf("logs:\n%s", logs)
	}
}

func TestPruneRunsOnItsTick(t *testing.T) {
	r := newRig(t, usagehistory.DefaultConfig())
	at := r.now()
	r.tick(r.prune, 1)
	r.stop()
	if len(r.store.pruned) != 1 || !r.store.pruned[0].Equal(at) {
		t.Fatalf("pruned at %v, want %v", r.store.pruned, at)
	}
}

func TestDisabledHistoryNeitherMeasuresNorOpens(t *testing.T) {
	r := newRig(t, usagehistory.Config{})
	r.tick(r.measure, 3)
	r.tick(r.write, 1)
	r.stop()
	if len(r.opened) != 0 || r.store.writes() != 0 {
		t.Fatalf("opened %v, writes %d", r.opened, r.store.writes())
	}
}

func (r *rig) waitOpened(t *testing.T, n int) {
	t.Helper()
	for range 400 {
		r.mu.Lock()
		got := len(r.opened)
		r.mu.Unlock()
		if got >= n {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("the store was not opened %d times", n)
}

func TestNewConfigReachesTheRunningLoop(t *testing.T) {
	r := newRig(t, usagehistory.Config{})
	r.tick(r.measure, 1) // ignored: off
	first := usagehistory.DefaultConfig()
	r.c.SetConfig(first)
	r.waitOpened(t, 1)
	r.waitRead = true
	r.tick(r.measure, 2)
	second := first
	second.Minutes = 2 * time.Hour
	r.c.SetConfig(second)
	r.waitOpened(t, 2)
	r.stop()
	if len(r.opened) != 2 || r.opened[0] != first || r.opened[1] != second || !r.store.closed {
		t.Fatalf("opened %v", r.opened)
	}
	if len(r.store.batches) < 1 || len(r.store.batches[0]) != 2 {
		t.Fatalf("the buffer was not written before the store was reopened: %v", r.store.batches)
	}
}

func TestServiceRunsAndLeavesAForeignDatabaseAlone(t *testing.T) {
	foreign := bytes.Repeat([]byte("not the history of this program, "), 8)
	var path string
	e := startCfg(t, func(net.Conn) (uint32, error) { return 0, nil }, func(state string, c *Config) {
		path = filepath.Join(state, MetricsFile)
		if err := os.MkdirAll(state, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, foreign, 0o600); err != nil {
			t.Fatal(err)
		}
		def := usagehistory.DefaultConfig()
		c.History = &def
		c.history.Ticker = func(time.Duration) (<-chan time.Time, func()) { return nil, func() {} }
	})
	if err := e.status(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, foreign) {
		t.Fatalf("the foreign file changed or is gone: %v", err)
	}
}
