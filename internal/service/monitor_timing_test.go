package service

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/castrowithcee/qatlas-kit/docker"

	"github.com/castrowithcee/qatlas-host/internal/engine"
)

var testTiming = [engine.MaxRepairAttempts]time.Duration{20 * time.Second, 40 * time.Second, 90 * time.Second}

// timedRig is a monitor rig with shortened waits, read every 5 seconds.
func timedRig(t *testing.T) *monRig {
	t.Helper()
	r := newMonRig(t)
	cfg := r.mon.monitorConfig
	cfg.RepairDelays, cfg.UnhealthyAfter, cfg.Interval = testTiming, 15*time.Second, 5*time.Second
	r.mon = newContainerMonitor(cfg)
	r.eng.now = func() time.Time { return r.clock.now }
	r.dock.list = []docker.Container{ctr("1", "app"), ctr("2", "db")}
	r.poll(5 * time.Second)
	return r
}

// advance reads every 5 seconds for d.
func (r *monRig) advance(d time.Duration) {
	r.t.Helper()
	for end := r.clock.now.Add(d); r.clock.now.Before(end); {
		r.poll(5 * time.Second)
	}
}

func TestMonitorTimingDefaultsToTheProductValues(t *testing.T) {
	m := newContainerMonitor(monitorConfig{})
	want := [engine.MaxRepairAttempts]time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute}
	if m.RepairDelays != want || m.UnhealthyAfter != time.Minute || m.Interval != 30*time.Second {
		t.Fatalf("delays %v, unhealthy after %v, interval %v", m.RepairDelays, m.UnhealthyAfter, m.Interval)
	}
}

func TestMonitorTimingSetValuesDriveTheRepairAttempts(t *testing.T) {
	r := timedRig(t)
	r.setApp("exited")
	r.poll(5 * time.Second)
	r.wantEvents(1, 0)
	t0 := r.clock.now
	r.advance(15 * time.Second)
	if len(r.eng.repairs) != 0 {
		t.Fatalf("attempt before the first wait: %+v", r.eng.repairs)
	}
	r.advance(5 * time.Second)
	if len(r.eng.repairs) != 1 || !slices.Equal(r.dock.restarted, []string{"1"}) || !r.eng.repairs[0].Last.Equal(t0.Add(20*time.Second)) ||
		!r.eng.repairs[0].Next.Equal(t0.Add(60*time.Second)) {
		t.Fatalf("attempt 1: %+v, containers %v", r.eng.repairs, r.dock.restarted)
	}
	r.advance(35 * time.Second)
	if len(r.eng.repairs) != 1 {
		t.Fatalf("attempt 2 too early: %+v", r.eng.repairs)
	}
	r.advance(5 * time.Second)
	if len(r.eng.repairs) != 2 || r.eng.stackRestarts != 1 || !r.eng.repairs[1].Next.Equal(t0.Add(150*time.Second)) {
		t.Fatalf("attempt 2: %+v, stack restarts %d", r.eng.repairs, r.eng.stackRestarts)
	}
	r.advance(90 * time.Second)
	if len(r.eng.repairs) != 3 || r.eng.stackRestarts != 2 {
		t.Fatalf("attempt 3: %+v, stack restarts %d", r.eng.repairs, r.eng.stackRestarts)
	}
	r.advance(5 * time.Second)
	if r.halts() != 1 {
		t.Fatalf("halts = %d", r.halts())
	}
}

func TestMonitorTimingSetValuesDriveTheRollbackInTheObservationTime(t *testing.T) {
	r := timedRig(t)
	st := r.eng.states["web"]
	since := r.clock.now.Add(5 * time.Second).UTC()
	st.Observe = &engine.Observation{Run: "20260504T025500Z-00000001", From: strings.Repeat("b", 40), Commit: st.Deployed,
		Backup: "20260504T025400Z-00000002", Since: since, Until: since.Add(time.Hour)}
	r.eng.states["web"] = st
	r.setApp("exited")
	r.poll(5 * time.Second)
	r.wantEvents(1, 0)
	t0 := r.clock.now
	r.advance(20 * time.Second)
	if len(r.eng.repairs) != 1 || len(r.eng.rollbacks) != 0 {
		t.Fatalf("attempt 1: repairs %+v, rollbacks %v", r.eng.repairs, r.eng.rollbacks)
	}
	r.advance(35 * time.Second)
	if len(r.eng.rollbacks) != 0 {
		t.Fatalf("rollback too early: %v", r.eng.rollbacks)
	}
	r.advance(5 * time.Second)
	if len(r.eng.rollbacks) != 1 || r.eng.stackRestarts != 0 || !r.clock.now.Equal(t0.Add(60*time.Second)) {
		t.Fatalf("rollbacks %v, stack restarts %d at %v", r.eng.rollbacks, r.eng.stackRestarts, r.clock.now.Sub(t0))
	}
}

func TestMonitorTimingSetUnhealthyAfterCounts(t *testing.T) {
	r := timedRig(t)
	r.dock.list[0].Health = "unhealthy"
	r.poll(0)
	r.advance(15 * time.Second) // exactly 15 s
	r.wantEvents(0, 0)
	r.poll(5 * time.Second)
	r.wantEvents(1, 0)
}
