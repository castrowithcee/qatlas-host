package hostservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/host/auditlog"
	"github.com/castrowithcee/qatlas-host/internal/host/usage"
	"github.com/castrowithcee/qatlas-host/internal/host/usagehistory"
)

var readBase = time.Date(2001, 2, 3, 4, 0, 0, 0, time.UTC)

// seedHistory writes n measurements every 10 seconds from readBase, each with the given mounts, into a new
// database in dir and closes it.
func seedHistory(t *testing.T, dir string, cfg usagehistory.Config, n int, mounts int) {
	t.Helper()
	s, err := usagehistory.Open(filepath.Join(dir, MetricsFile), cfg)
	if err != nil {
		t.Fatal(err)
	}
	var batch []usagehistory.Measurement
	for i := range n {
		cpu := 99.99
		m := usagehistory.Measurement{Time: readBase.Add(time.Duration(i) * usagehistory.RawInterval), Usage: usage.Usage{
			CPUPercent: &cpu, Load: &usage.Load{One: 99.99},
			Memory: &usage.Memory{UsedBytes: 9e15}, Swap: &usage.Memory{UsedBytes: 9e15}}}
		for j := range mounts {
			m.Filesystems = append(m.Filesystems, usage.Filesystem{
				Mount: fmt.Sprintf("/mnt/%s%03d", strings.Repeat("x", 200), j), UsedBytes: 9e15, TotalBytes: 9e15})
		}
		batch = append(batch, m)
	}
	if err := s.Write(context.Background(), batch); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

// runningCollector starts a collector on dir with the real store and ticks that never fire, and returns once
// it has taken its configuration.
func runningCollector(t *testing.T, dir string, cfg usagehistory.Config, hooks historyHooks) *collector {
	t.Helper()
	hooks.Ticker = func(time.Duration) (<-chan time.Time, func()) { return nil, func() {} }
	now := readBase.Add(1500 * usagehistory.RawInterval)
	c := newCollector(slog.New(slog.NewTextHandler(io.Discard, nil)), dir, func() time.Time { return now }, cfg, hooks)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); c.run(ctx) }()
	t.Cleanup(func() { cancel(); <-done })
	for range 500 {
		if _, err := c.read(ctx, usagehistory.Query{From: now.Add(-time.Minute), Resolution: usagehistory.ResolutionAuto}); !errors.Is(err, errHistoryStarting) {
			return c
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the collector never took its configuration")
	return nil
}

func wantNoHistory(t *testing.T, err error, parts ...string) {
	t.Helper()
	var ce *control.Error
	if !errors.As(err, &ce) || ce.Code != control.CodeNoHistory {
		t.Fatalf("err = %v, want code %s", err, control.CodeNoHistory)
	}
	for _, p := range parts {
		if !strings.Contains(ce.Message, p) {
			t.Errorf("message %q lacks %q", ce.Message, p)
		}
	}
}

func TestReadFailsClearlyWhenTheHistoryIsOffOrTheDatabaseRefused(t *testing.T) {
	q := usagehistory.Query{From: readBase, To: readBase.Add(time.Hour), Resolution: usagehistory.ResolutionAuto}
	off := runningCollector(t, t.TempDir(), usagehistory.Config{}, historyHooks{})
	_, err := off.read(context.Background(), q)
	wantNoHistory(t, err, "turned off", "Ask an administrator")

	refused := runningCollector(t, t.TempDir(), usagehistory.DefaultConfig(), historyHooks{
		Open: func(string, usagehistory.Config) (historyStore, error) {
			return nil, errors.New("file is not a database")
		}})
	_, err = refused.read(context.Background(), q)
	wantNoHistory(t, err, "could not be opened", "file is not a database", "Read the log")
}

func TestReadUsesTheRetentionOfTheRunningConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := usagehistory.DefaultConfig()
	cfg.Raw = 10 * time.Minute
	seedHistory(t, dir, cfg, 10, 1)
	c := runningCollector(t, dir, cfg, historyHooks{})
	now := readBase.Add(1500 * usagehistory.RawInterval)
	// The raw values are older than the retention 10 minutes this collector runs with.
	_, err := c.read(context.Background(), usagehistory.Query{From: readBase, To: readBase.Add(time.Minute), Resolution: usagehistory.ResolutionRaw})
	var qe *usagehistory.QueryError
	if !errors.As(err, &qe) || !strings.Contains(qe.Message, "only keeps the last 10m0s") {
		t.Fatalf("err = %v", err)
	}
	cfg2 := usagehistory.DefaultConfig()
	c.SetConfig(cfg2)
	for range 500 {
		got, err := c.read(context.Background(), usagehistory.Query{From: now.Add(-24 * time.Hour), To: now, Resolution: usagehistory.ResolutionAuto})
		if err == nil && got.Resolution == usagehistory.ResolutionMinute {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the new retention never took effect")
}

func TestUsageHistoryIsReadWithoutRootAndLogged(t *testing.T) {
	e := startCfg(t, func(net.Conn) (uint32, error) { return 4100, nil }, func(_ string, c *Config) {
		h := usagehistory.DefaultConfig()
		c.History = &h
	})
	var res control.HostUsageHistoryResult
	var err error
	for range 500 { // the loop opens the store right after start
		if err = e.do(control.HostUsageHistoryCommand, nil, &res); !isNotOpenYet(err) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil || res.Resolution != usagehistory.ResolutionRaw || len(res.Series) != 4 || len(res.Series[0].Points) != 0 {
		t.Fatalf("res %+v, err %v", res, err)
	}
	wantCode(t, e.do(control.HostUsageHistoryCommand, control.HostUsageHistoryArgs{Resolution: "week"}, nil), control.CodeBadRequest)
	wantCode(t, e.do(control.HostUsageHistoryCommand, control.HostUsageHistoryArgs{From: "yesterday"}, nil), control.CodeBadRequest)
	for _, entry := range e.entries(t) {
		if entry.Entry.Action != auditlog.OperationUsageRead {
			t.Errorf("entry %+v", entry.Entry)
		}
	}
	if got := e.entries(t); len(got) < 3 {
		t.Fatalf("%d entries, want one per request", len(got))
	}
}

func isNotOpenYet(err error) bool {
	var ce *control.Error
	return errors.As(err, &ce) && ce.Code == control.CodeNoHistory && strings.Contains(ce.Message, "not open yet")
}

func TestUsageHistoryOffIsAnErrorNotAnEmptyAnswer(t *testing.T) {
	e := start(t, 4100)
	var err error
	for range 500 {
		if err = e.do(control.HostUsageHistoryCommand, nil, nil); !isNotOpenYet(err) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	wantNoHistory(t, err, "turned off")
}

func TestAnswerStaysUnderTheChannelLimitWithManyFileSystems(t *testing.T) {
	const points = 1500
	for _, mounts := range []int{1, 8, 25} {
		dir := t.TempDir()
		cfg := usagehistory.DefaultConfig()
		cfg.Raw = 24 * time.Hour
		seedHistory(t, dir, cfg, points+1, mounts)
		c := runningCollector(t, dir, cfg, historyHooks{})
		now := readBase.Add(1500 * usagehistory.RawInterval)
		for _, res := range []string{usagehistory.ResolutionRaw, usagehistory.ResolutionAuto} {
			a := control.HostUsageHistoryArgs{From: readBase.Add(usagehistory.RawInterval).Format(time.RFC3339), To: now.Format(time.RFC3339), Resolution: res}
			q, err := historyQuery(a, now)
			if err != nil {
				t.Fatal(err)
			}
			got, err := c.read(context.Background(), q)
			if err != nil {
				var qe *usagehistory.QueryError
				if res == usagehistory.ResolutionRaw && mounts == 25 && errors.As(err, &qe) {
					continue // too much for raw: refused, which is the point
				}
				t.Fatalf("%d mounts, %s: %v", mounts, res, err)
			}
			raw, err := json.Marshal(historyResult(got))
			if err != nil || len(raw) > control.MaxResponseBytes {
				t.Errorf("%d mounts, %s (%s): answer of %d bytes, limit %d, err %v", mounts, res, got.Resolution, len(raw), control.MaxResponseBytes, err)
			}
			if res == usagehistory.ResolutionRaw && mounts <= 8 && got.Resolution != usagehistory.ResolutionRaw {
				t.Errorf("%d mounts: raw answered as %s", mounts, got.Resolution)
			}
		}
	}
}
