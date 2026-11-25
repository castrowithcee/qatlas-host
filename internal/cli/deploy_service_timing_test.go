package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/castrowithcee/qatlas-host/internal/service"
)

func TestRunTimingFlagsSetTheMonitorWaits(t *testing.T) {
	var cfg service.Config
	if err := (&runTiming{}).apply(&cfg); err != nil || cfg.RepairDelays != [3]time.Duration{} || cfg.UnhealthyAfter != 0 || cfg.MonitorInterval != 0 {
		t.Fatalf("no flags: %+v, err %v", cfg, err)
	}
	tm := runTiming{repairDelays: "15s, 45s,1m30s", unhealthyAfter: "15s", monitorInterval: "5s"}
	if err := tm.apply(&cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.RepairDelays != [3]time.Duration{15 * time.Second, 45 * time.Second, 90 * time.Second} ||
		cfg.UnhealthyAfter != 15*time.Second || cfg.MonitorInterval != 5*time.Second {
		t.Fatalf("cfg = %+v", cfg)
	}
}

func TestRunTimingFlagsRefuseBadValuesBeforeStarting(t *testing.T) {
	for _, c := range []struct{ flag, value, want string }{
		{"--repair-delays", "15s,45s", "has 2 values, want 3"},
		{"--repair-delays", "15s,45s,90s,2m", "has 4 values, want 3"},
		{"--repair-delays", "15s,15s,90s", "does not rise strictly"},
		{"--repair-delays", "15s,0s,90s", "is not positive"},
		{"--repair-delays", "15s,x,90s", "is not a duration"},
		{"--unhealthy-after", "-1s", `"-1s" is not positive`},
		{"--unhealthy-after", "0", `"0" is not positive`},
		{"--monitor-interval", "soon", `"soon" is not a duration`},
	} {
		code, _, stderr := run(t, "deploy-service", "run", c.flag, c.value)
		if code == 0 || !strings.Contains(stderr, c.flag+": ") || !strings.Contains(stderr, c.want) {
			t.Errorf("%s %s: code %d, stderr %q", c.flag, c.value, code, stderr)
		}
	}
}

func TestRunTimingFlagsAreHidden(t *testing.T) {
	_, stdout, stderr := run(t, "deploy-service", "run", "--help")
	for _, name := range []string{"repair-delays", "unhealthy-after", "monitor-interval"} {
		if strings.Contains(stdout+stderr, name) {
			t.Errorf("--help names %s", name)
		}
	}
}
