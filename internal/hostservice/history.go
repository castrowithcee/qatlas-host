package hostservice

import (
	"context"
	"log/slog"
	"path/filepath"
	"sync"
	"time"

	"github.com/castrowithcee/qatlas-host/internal/host/usage"
	"github.com/castrowithcee/qatlas-host/internal/host/usagehistory"
)

const (
	// MetricsFile is the history database inside the state directory.
	MetricsFile = "metrics.db"

	measureEvery = usagehistory.RawInterval
	writeEvery   = time.Minute
	pruneEvery   = time.Hour
	// flushTimeout bounds the last write at stop, which runs after ctx has ended.
	flushTimeout = 5 * time.Second
	// warnEvery is the shortest time between two log lines about the same kind of failure.
	warnEvery = time.Hour
)

// historyStore is the part of *usagehistory.Store the collector uses.
type historyStore interface {
	Write(ctx context.Context, batch []usagehistory.Measurement) error
	Prune(ctx context.Context, now time.Time) error
	Read(ctx context.Context, now time.Time, q usagehistory.Query) (usagehistory.Result, error)
	Close() error
}

// historyHooks replace the outside world of the collector. A zero field means the real one; tests set them.
type historyHooks struct {
	// Ticker returns the ticks every d and the function that stops them.
	Ticker func(d time.Duration) (<-chan time.Time, func())
	Read   func(ctx context.Context) (usage.Usage, error)
	Open   func(path string, cfg usagehistory.Config) (historyStore, error)
}

func realTicker(d time.Duration) (<-chan time.Time, func()) {
	t := time.NewTicker(d)
	return t.C, t.Stop
}

func realOpen(path string, cfg usagehistory.Config) (historyStore, error) {
	return usagehistory.Open(path, cfg)
}

// collector measures the host every measureEvery, keeps the measurements in a buffer, writes it once a
// minute and prunes once an hour. Its history is expendable: every failure is logged, throttled, and the
// service goes on.
type collector struct {
	log   *slog.Logger
	path  string
	now   func() time.Time
	hooks historyHooks
	cfgs  chan usagehistory.Config

	// readMu guards reader, the store as readers see it, and readErr, why there is none. A reader holds the
	// read lock for its query, so the loop closes the store only after the last one is done.
	readMu  sync.RWMutex
	reader  historyStore
	readErr error

	store    historyStore
	buffer   []usagehistory.Measurement
	lastWarn map[string]time.Time
	missed   map[string]int
}

func newCollector(log *slog.Logger, stateDir string, now func() time.Time, cfg usagehistory.Config, hooks historyHooks) *collector {
	if hooks.Ticker == nil {
		hooks.Ticker = realTicker
	}
	if hooks.Read == nil {
		hooks.Read = usage.NewSource().Read
	}
	if hooks.Open == nil {
		hooks.Open = realOpen
	}
	c := &collector{log: log, path: filepath.Join(stateDir, MetricsFile), now: now, hooks: hooks,
		cfgs: make(chan usagehistory.Config, 1), lastWarn: map[string]time.Time{}, missed: map[string]int{}}
	c.cfgs <- cfg
	c.readErr = errHistoryStarting
	return c
}

// SetConfig hands the running collector a new configuration. It takes effect at the next turn of the loop:
// the buffer is written, and the store is reopened with the new retention, or closed when disabled. Only the
// newest pending configuration counts.
func (c *collector) SetConfig(cfg usagehistory.Config) {
	for {
		select {
		case c.cfgs <- cfg:
			return
		default:
			select {
			case <-c.cfgs:
			default:
			}
		}
	}
}

// run works until ctx ends, then writes the buffer once more and closes the store.
func (c *collector) run(ctx context.Context) {
	measure, stopMeasure := c.hooks.Ticker(measureEvery)
	write, stopWrite := c.hooks.Ticker(writeEvery)
	prune, stopPrune := c.hooks.Ticker(pruneEvery)
	defer stopMeasure()
	defer stopWrite()
	defer stopPrune()
	enabled := false
	for {
		select {
		case <-ctx.Done():
			c.close()
			return
		case cfg := <-c.cfgs:
			c.close()
			enabled = c.reopen(cfg)
		case <-measure:
			if enabled {
				c.measure(ctx)
			}
		case <-write:
			c.flush(ctx)
		case <-prune:
			if c.store != nil {
				c.warn("prune", "prune the usage history", c.store.Prune(ctx, c.now()))
			}
		}
	}
}

// reopen opens the store for cfg and reports whether the history is on.
func (c *collector) reopen(cfg usagehistory.Config) bool {
	if !cfg.Enabled {
		c.setReader(nil, errHistoryOff)
		return false
	}
	store, err := c.hooks.Open(c.path, cfg)
	if err != nil {
		c.warn("open", "usage history is off, the service goes on without it", err)
		c.setReader(nil, historyOpenError(err))
		return false
	}
	c.store = store
	c.setReader(store, nil)
	return true
}

func (c *collector) measure(ctx context.Context) {
	at := c.now().UTC()
	u, err := c.hooks.Read(ctx)
	if err != nil && ctx.Err() != nil {
		return // stopping, not a failure
	}
	if c.warn("read", "measure the host load", err) == nil {
		c.buffer = append(c.buffer, usagehistory.Measurement{Time: at, Usage: u})
	}
}

// flush writes the buffer in one transaction. A failed write drops the buffer, so a full disk cannot grow
// memory.
func (c *collector) flush(ctx context.Context) {
	if c.store == nil || len(c.buffer) == 0 {
		return
	}
	batch := c.buffer
	c.buffer = nil
	c.warn("write", "write the usage history, the buffered values are dropped", c.store.Write(ctx, batch))
}

// close writes what is buffered with a context of its own and closes the store.
func (c *collector) close() {
	if c.store == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), flushTimeout)
	defer cancel()
	c.flush(ctx)
	c.setReader(nil, errHistoryClosed)
	c.warn("close", "close the usage history", c.store.Close())
	c.store, c.buffer = nil, nil
}

// warn logs err at most once per warnEvery for its kind, the first at once, with the failures since.
func (c *collector) warn(kind, what string, err error) error {
	if err == nil {
		return nil
	}
	now := c.now()
	last, seen := c.lastWarn[kind]
	if !seen || now.Sub(last) >= warnEvery || now.Before(last) {
		c.log.Warn(what, "error", err, "failures_since_last_message", c.missed[kind])
		c.lastWarn[kind], c.missed[kind] = now, 0
		return err
	}
	c.missed[kind]++
	return err
}
