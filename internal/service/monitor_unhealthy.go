package service

import (
	"time"

	"github.com/castrowithcee/qatlas-host/internal/engine"
	"github.com/castrowithcee/qatlas-kit/docker"
)

// unhealthy reports whether c counts as a failing unhealthy container and carries its timer forward. The timers
// are lost with the process, so a stored unhealthy failure of the same container stays open without a new wait.
func (m *containerMonitor) unhealthy(now time.Time, c docker.Container, open *engine.Failure,
	old map[string]time.Time) bool {
	if c.State != "running" || c.Health != "unhealthy" {
		return false
	}
	since, ok := old[c.ID]
	if !ok {
		since = now
	}
	m.unhealthySince[c.ID] = since
	if open != nil && open.Kind == engine.FailureUnhealthy && open.Container == maskControl(c.Name) {
		return true
	}
	return now.Sub(since) > m.UnhealthyAfter
}
