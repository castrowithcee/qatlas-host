package service

import (
	"time"

	"github.com/castrowithcee/qatlas-host/internal/engine"
)

// defaultUnhealthyAfter is how long a container must be observed unhealthy without a break before it counts; it
// must be exceeded, not just reached.
const defaultUnhealthyAfter = time.Minute

// defaultRepairDelays are the waits before each repair attempt: the first from the start of the failure, each
// further one from the attempt before. The first restarts the failed containers, the others the stack. In the
// observation time of an update the rollback of the update takes the place of the second and third.
var defaultRepairDelays = [engine.MaxRepairAttempts]time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute}

// fillTimingDefaults replaces the zero values of the monitor's waits by the product values.
func (c *monitorConfig) fillTimingDefaults() {
	if c.RepairDelays == [engine.MaxRepairAttempts]time.Duration{} {
		c.RepairDelays = defaultRepairDelays
	}
	if c.UnhealthyAfter <= 0 {
		c.UnhealthyAfter = defaultUnhealthyAfter
	}
}
