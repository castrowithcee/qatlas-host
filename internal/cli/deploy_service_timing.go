package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-kit/cli"
	"github.com/castrowithcee/qatlas-kit/output"

	"github.com/castrowithcee/qatlas-host/internal/engine"
	"github.com/castrowithcee/qatlas-host/internal/service"
)

// runTiming holds the hidden flags of 'deploy-service run' that shorten the waits of the container monitor for
// tests. The zero value means the product values.
type runTiming struct {
	repairDelays, unhealthyAfter, monitorInterval string
}

func (t *runTiming) addFlags(cmd *cobra.Command) {
	for name, v := range map[string]*string{"repair-delays": &t.repairDelays, "unhealthy-after": &t.unhealthyAfter,
		"monitor-interval": &t.monitorInterval} {
		cmd.Flags().StringVar(v, name, "", "")
		_ = cmd.Flags().MarkHidden(name)
	}
}

// apply sets the monitor waits of cfg from the flags that were given; a bad value fails before the service starts.
func (t *runTiming) apply(cfg *service.Config) error {
	var err error
	if t.repairDelays != "" {
		if cfg.RepairDelays, err = parseRepairDelays(t.repairDelays); err != nil {
			return timingFailure("--repair-delays", err)
		}
	}
	if t.unhealthyAfter != "" {
		if cfg.UnhealthyAfter, err = parsePositiveDuration(t.unhealthyAfter); err != nil {
			return timingFailure("--unhealthy-after", err)
		}
	}
	if t.monitorInterval != "" {
		if cfg.MonitorInterval, err = parsePositiveDuration(t.monitorInterval); err != nil {
			return timingFailure("--monitor-interval", err)
		}
	}
	return nil
}

func timingFailure(flag string, err error) error {
	return &cli.Failure{Code: output.CodeInvalidRequest, Usage: true, Message: flag + ": " + err.Error(),
		NextStep: "Give Go durations such as 15s or 2m."}
}

func parsePositiveDuration(s string) (time.Duration, error) {
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("%q is not a duration", s)
	}
	if d <= 0 {
		return 0, fmt.Errorf("%q is not positive", s)
	}
	return d, nil
}

func parseRepairDelays(s string) ([engine.MaxRepairAttempts]time.Duration, error) {
	var out [engine.MaxRepairAttempts]time.Duration
	parts := strings.Split(s, ",")
	if len(parts) != len(out) {
		return out, fmt.Errorf("%q has %d values, want %d", s, len(parts), len(out))
	}
	for i, p := range parts {
		d, err := parsePositiveDuration(strings.TrimSpace(p))
		if err != nil {
			return out, err
		}
		if i > 0 && d <= out[i-1] {
			return out, fmt.Errorf("%q does not rise strictly: %s is not above %s", s, d, out[i-1])
		}
		out[i] = d
	}
	return out, nil
}
