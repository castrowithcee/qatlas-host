package usagehistory

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/castrowithcee/qatlas-host/internal/host/usage"
)

// fill writes a measurement every RawInterval from base for span, with the CPU share rising by 1 per value.
func fill(t *testing.T, s *Store, span time.Duration, mounts ...string) {
	t.Helper()
	var batch []Measurement
	for i := 0; time.Duration(i)*RawInterval <= span; i++ {
		cpu := float64(i)
		m := Measurement{Time: base.Add(time.Duration(i) * RawInterval), Usage: usage.Usage{CPUPercent: &cpu, Memory: &usage.Memory{UsedBytes: 1000}}}
		for _, mount := range mounts {
			m.Filesystems = append(m.Filesystems, usage.Filesystem{Mount: mount, UsedBytes: uint64(500 + i), TotalBytes: 5000})
		}
		batch = append(batch, m)
	}
	write(t, s, batch...)
}

func read(t *testing.T, s *Store, now time.Time, q Query) (Result, error) {
	t.Helper()
	return s.Read(context.Background(), now, q)
}

func wantQueryError(t *testing.T, err error, parts ...string) {
	t.Helper()
	var qe *QueryError
	if !errors.As(err, &qe) {
		t.Fatalf("err = %v, want a *QueryError", err)
	}
	for _, p := range parts {
		if !strings.Contains(qe.Message, p) {
			t.Errorf("message %q lacks %q", qe.Message, p)
		}
	}
}

func TestAutoPicksTheFinestTierThatFits(t *testing.T) {
	s, _ := openTemp(t, DefaultConfig())
	fill(t, s, 48*time.Hour+time.Hour, "/data")
	now := base.Add(50 * time.Hour)
	for _, c := range []struct {
		name       string
		from, to   time.Duration
		resolution string
	}{
		{"an hour fits raw", 49 * time.Hour, 50 * time.Hour, ResolutionRaw},
		{"a day is too long for raw", 26 * time.Hour, 50 * time.Hour, ResolutionMinute},
		{"older than the raw retention", 2 * time.Hour, 3 * time.Hour, ResolutionMinute},
		{"too long for minutes", 0, 49 * time.Hour, ResolutionHour},
	} {
		got, err := read(t, s, now, Query{From: base.Add(c.from), To: base.Add(c.to), Resolution: ResolutionAuto})
		if err != nil || got.Resolution != c.resolution {
			t.Errorf("%s: resolution %q, err %v, want %q", c.name, got.Resolution, err, c.resolution)
		}
	}
}

func TestExplicitResolutionOverTheLimitOrRetentionIsRefusedWithAHint(t *testing.T) {
	s, _ := openTemp(t, DefaultConfig())
	fill(t, s, 5*time.Hour)
	now := base.Add(5 * time.Hour)
	_, err := read(t, s, now, Query{From: base, To: now, Resolution: ResolutionRaw})
	wantQueryError(t, err, "1500", "--resolution minute")
	_, err = read(t, s, now.Add(30*time.Hour), Query{From: base, To: now.Add(30 * time.Hour), Resolution: ResolutionRaw})
	wantQueryError(t, err, "only keeps the last 24h0m0s", "--resolution")
	_, err = read(t, s, now.Add(400*24*time.Hour), Query{From: base, To: now.Add(400 * 24 * time.Hour), Resolution: ResolutionAuto})
	wantQueryError(t, err, "no resolution fits")
	_, err = read(t, s, now, Query{From: base, To: now, Resolution: "week"})
	wantQueryError(t, err, "auto, raw, minute or hour")
}

func TestPeriodsTooLongForEveryResolutionAreRefused(t *testing.T) {
	s, _ := openTemp(t, DefaultConfig())
	now := base.Add(100 * 24 * time.Hour)
	_, err := read(t, s, now, Query{From: base.Add(10 * 24 * time.Hour), To: now, Resolution: ResolutionHour})
	wantQueryError(t, err, "hour resolution would give", "narrow the period")
}

func TestGapsStayMissingAndRawValuesAreKept(t *testing.T) {
	s, _ := openTemp(t, DefaultConfig())
	write(t, s, sample(0, 10, 100), sample(10*time.Second, 20, 200), sample(60*time.Second, 40, 400))
	got, err := read(t, s, base.Add(time.Minute), Query{From: base, To: base.Add(time.Minute), Resolution: ResolutionRaw, Metrics: []string{"cpu_percent"}})
	if err != nil || len(got.Series) != 1 {
		t.Fatalf("%+v, %v", got, err)
	}
	pts := got.Series[0].Points
	if len(pts) != 3 || pts[1].Time != base.Add(10*time.Second).Unix() || pts[2].Time != base.Add(time.Minute).Unix() || pts[1].Mean != 20 || pts[1].Max != 20 {
		t.Fatalf("points %+v: the gap from 10 s to 60 s must stay empty", pts)
	}
}

func TestAggregatedTiersCarryMeanAndMax(t *testing.T) {
	s, _ := openTemp(t, DefaultConfig())
	write(t, s, sample(0, 10, 100), sample(10*time.Second, 30, 300), sample(2*time.Minute, 0, 0))
	now := base.Add(3 * time.Minute)
	got, err := read(t, s, now, Query{From: base, To: now, Resolution: ResolutionMinute, Metrics: []string{"cpu_percent", MetricFilesystem}})
	if err != nil || len(got.Series) != 2 {
		t.Fatalf("%+v, %v", got, err)
	}
	cpu, fsys := got.Series[0], got.Series[1]
	if cpu.Metric != "cpu_percent" || len(cpu.Points) != 1 || cpu.Points[0].Mean != 20 || cpu.Points[0].Max != 30 {
		t.Errorf("cpu = %+v", cpu)
	}
	if fsys.Mount != "/data" || fsys.TotalBytes != 5000 || len(fsys.Points) != 1 || fsys.Points[0].Mean != 200 || fsys.Points[0].Max != 300 {
		t.Errorf("fs = %+v", fsys)
	}
}

func TestUnknownMetricAndEmptyPeriodAreRefused(t *testing.T) {
	s, _ := openTemp(t, DefaultConfig())
	_, err := read(t, s, base, Query{From: base.Add(-time.Hour), To: base, Resolution: ResolutionAuto, Metrics: []string{"disk"}})
	wantQueryError(t, err, `unknown metric "disk"`, "cpu_percent")
	_, err = read(t, s, base, Query{From: base, To: base, Resolution: ResolutionAuto})
	wantQueryError(t, err, "empty")
	_, err = read(t, s, base, Query{From: base.Add(time.Minute), To: base.Add(time.Hour), Resolution: ResolutionAuto})
	wantQueryError(t, err, "empty") // the end is cut to now, which is before the start
}

func TestManyFileSystemsFallToACoarserTierOrAreRefusedByTheBudget(t *testing.T) {
	s, _ := openTemp(t, DefaultConfig())
	var mounts []string
	for i := range 30 {
		mounts = append(mounts, fmt.Sprintf("/mnt/demo%02d", i))
	}
	fill(t, s, 2*time.Hour, mounts...)
	now := base.Add(2*time.Hour + time.Minute)
	q := Query{From: base, To: now, Resolution: ResolutionRaw, Budget: 400 << 10}
	_, err := read(t, s, now, q)
	wantQueryError(t, err, "size of an answer", "--resolution minute")
	q.Resolution = ResolutionAuto
	got, err := read(t, s, now, q)
	if err != nil || got.Resolution != ResolutionMinute || len(got.Series) != 4+30 {
		t.Fatalf("resolution %q, %d series, err %v", got.Resolution, len(got.Series), err)
	}
}
