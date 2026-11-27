package usagehistory

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"time"
)

// The resolutions of a query. Auto picks the finest one that fits.
const (
	ResolutionAuto   = "auto"
	ResolutionRaw    = "raw"
	ResolutionMinute = "minute"
	ResolutionHour   = "hour"
)

// MetricFilesystem is the used bytes of each file system, one series per mount. The other metrics have one
// series each.
const MetricFilesystem = "filesystem_used_bytes"

const (
	// RawInterval is the time between two raw values, which the service measures at.
	RawInterval = 10 * time.Second
	// MaxPoints is the most points one series of an answer holds.
	MaxPoints = 1500

	// Worst-case encoded size of one point of a raw and of an aggregated series, and of the frame of a series
	// besides its mount name.
	rawPointBytes    = 48
	aggPointBytes    = 80
	seriesFrameBytes = 160
)

// metricColumns maps each metric to its column in the raw table; the aggregated tables add _avg and _max.
var metricColumns = []struct{ name, column string }{
	{"cpu_percent", "cpu_percent"},
	{"load_one", "load_one"},
	{"memory_used_bytes", "memory_used"},
	{"swap_used_bytes", "swap_used"},
}

// Metrics lists the names a query may select.
func Metrics() []string {
	names := []string{MetricFilesystem}
	for _, m := range metricColumns {
		names = append(names, m.name)
	}
	return names
}

// Query asks for the history from From to To. A zero To means now, and To is cut to now. Budget is the most
// bytes the encoded answer may take; 0 means no limit.
type Query struct {
	From, To   time.Time
	Resolution string
	Metrics    []string // empty means all
	Budget     int
}

// Point is one bucket at Time (unix seconds, UTC). A raw point has Mean equal to Max, the value.
type Point struct {
	Time      int64
	Mean, Max float64
}

// Series is the course of one metric; Mount and TotalBytes are set for a file system only, TotalBytes being
// the largest size in the period.
type Series struct {
	Metric     string
	Mount      string
	TotalBytes int64
	Points     []Point
}

// Result is the answer to a query: the resolution used and the series, whose gaps are missing points.
type Result struct {
	Resolution string
	From, To   time.Time
	Series     []Series
}

// QueryError is a query the store cannot answer as asked. Its text says what to change.
type QueryError struct{ Message string }

func (e *QueryError) Error() string { return e.Message }

type tier struct {
	name, table string
	step, keep  time.Duration
	aggregated  bool
}

func (s *Store) tiers() []tier {
	return []tier{
		{ResolutionRaw, "raw", RawInterval, s.cfg.Raw, false},
		{ResolutionMinute, "minute", time.Minute, s.cfg.Minutes, true},
		{ResolutionHour, "hour", time.Hour, s.cfg.Hours, true},
	}
}

// Read answers q from the tier the resolution names, or from the finest one that still keeps the whole period
// at now and fits MaxPoints per series and the budget. It only reads, so it runs next to the writer.
func (s *Store) Read(ctx context.Context, now time.Time, q Query) (Result, error) {
	metrics, err := selectMetrics(q.Metrics)
	if err != nil {
		return Result{}, err
	}
	to := now
	if !q.To.IsZero() && q.To.Before(now) {
		to = q.To
	}
	if !q.From.Before(to) {
		return Result{}, &QueryError{"the period is empty: from must be before to, and not in the future"}
	}
	q.From, q.To = q.From.UTC().Truncate(time.Second), to.UTC().Truncate(time.Second)
	var candidates []tier
	for _, t := range s.tiers() {
		if q.Resolution == ResolutionAuto || q.Resolution == t.name {
			candidates = append(candidates, t)
		}
	}
	if len(candidates) == 0 {
		return Result{}, &QueryError{"the resolution must be auto, raw, minute or hour"}
	}
	var why []string
	for _, t := range candidates {
		mounts, reason, err := s.fits(ctx, now, q, metrics, t)
		if err != nil {
			return Result{}, err
		}
		if reason == "" {
			return s.fetch(ctx, q, metrics, t, mounts)
		}
		why = append(why, reason)
	}
	return Result{}, &QueryError{s.advice(ctx, now, q, metrics, why)}
}

func selectMetrics(names []string) ([]string, error) {
	if len(names) == 0 {
		return Metrics(), nil
	}
	for _, n := range names {
		if !slices.Contains(Metrics(), n) {
			return nil, &QueryError{fmt.Sprintf("unknown metric %q, the metrics are %s", n, strings.Join(Metrics(), ", "))}
		}
	}
	return slices.Compact(slices.Sorted(slices.Values(names))), nil
}

// advice words the refusal of an explicit resolution or of auto: it names the finest resolution that works
// instead, or how to narrow the query.
func (s *Store) advice(ctx context.Context, now time.Time, q Query, metrics []string, why []string) string {
	msg := strings.Join(why, "; ")
	if q.Resolution == ResolutionAuto {
		return "no resolution fits the period: " + msg + "; narrow the period or select fewer metrics"
	}
	for _, t := range s.tiers() {
		if _, reason, err := s.fits(ctx, now, q, metrics, t); err == nil && reason == "" {
			return msg + "; use --resolution " + t.name + " (or auto)"
		}
	}
	return msg + "; narrow the period or select fewer metrics"
}

// fits reports why tier t cannot answer q, empty when it can, and the mounts the answer has.
func (s *Store) fits(ctx context.Context, now time.Time, q Query, metrics []string, t tier) ([]string, string, error) {
	if q.From.Before(now.Add(-t.keep)) {
		return nil, fmt.Sprintf("the %s resolution only keeps the last %s", t.name, t.keep), nil
	}
	points := int(q.To.Sub(q.From)/t.step) + 1
	if points > MaxPoints {
		return nil, fmt.Sprintf("the %s resolution would give %d points per metric, at most %d are returned", t.name, points, MaxPoints), nil
	}
	var mounts []string
	if slices.Contains(metrics, MetricFilesystem) {
		var err error
		if mounts, err = s.mounts(ctx, q, t); err != nil {
			return nil, "", err
		}
	}
	perPoint := aggPointBytes
	if !t.aggregated {
		perPoint = rawPointBytes
	}
	series := len(metrics) - btoi(slices.Contains(metrics, MetricFilesystem))
	size := series * (seriesFrameBytes + points*perPoint)
	for _, m := range mounts {
		size += seriesFrameBytes + 2*len(m) + points*perPoint
	}
	if q.Budget > 0 && size > q.Budget {
		return nil, fmt.Sprintf("the %s resolution would exceed the size of an answer", t.name), nil
	}
	return mounts, "", nil
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (s *Store) mounts(ctx context.Context, q Query, t tier) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT DISTINCT mount FROM "+t.table+"_fs WHERE ts >= ? AND ts <= ? ORDER BY mount",
		q.From.Unix(), q.To.Unix())
	if err != nil {
		return nil, fmt.Errorf("list the file systems: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var m string
		if err := rows.Scan(&m); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) fetch(ctx context.Context, q Query, metrics []string, t tier, mounts []string) (Result, error) {
	res := Result{Resolution: t.name, From: q.From, To: q.To}
	for _, m := range metricColumns {
		if !slices.Contains(metrics, m.name) {
			continue
		}
		cols := m.column + ", " + m.column
		if t.aggregated {
			cols = m.column + "_avg, " + m.column + "_max"
		}
		pts, _, err := s.points(ctx, "SELECT ts, "+cols+" FROM "+t.table+" WHERE ts >= ? AND ts <= ? ORDER BY ts", q, "")
		if err != nil {
			return Result{}, err
		}
		res.Series = append(res.Series, Series{Metric: m.name, Points: pts})
	}
	for _, mount := range mounts {
		cols := "used, used"
		if t.aggregated {
			cols = "used_avg, used_max"
		}
		pts, total, err := s.points(ctx, "SELECT ts, "+cols+", total FROM "+t.table+"_fs WHERE ts >= ? AND ts <= ? AND mount = ? ORDER BY ts", q, mount)
		if err != nil {
			return Result{}, err
		}
		res.Series = append(res.Series, Series{Metric: MetricFilesystem, Mount: mount, TotalBytes: total, Points: pts})
	}
	return res, nil
}

// points runs query for one series; a row whose value is NULL is a gap and is left out. A query with a mount
// also returns the largest total it saw.
func (s *Store) points(ctx context.Context, query string, q Query, mount string) ([]Point, int64, error) {
	args := []any{q.From.Unix(), q.To.Unix()}
	if mount != "" {
		args = append(args, mount)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("read the usage history: %w", err)
	}
	defer rows.Close()
	pts, total := []Point{}, int64(0)
	for rows.Next() {
		var ts, t int64
		var mean, peak sql.NullFloat64
		dest := []any{&ts, &mean, &peak}
		if mount != "" {
			dest = append(dest, &t)
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, 0, err
		}
		total = max(total, t)
		if mean.Valid && peak.Valid {
			pts = append(pts, Point{Time: ts, Mean: mean.Float64, Max: peak.Float64})
		}
	}
	return pts, total, rows.Err()
}
