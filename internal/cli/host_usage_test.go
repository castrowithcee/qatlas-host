package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/host/usage"
)

func exampleUsage() control.HostUsageResult {
	cpu, up := 12.345, int64(90)
	return usage.Usage{
		CPUPercent: &cpu, Cores: 4, Load: &usage.Load{One: 0.5, Five: 1, Fifteen: 1.5}, UptimeSeconds: &up,
		Memory:      &usage.Memory{TotalBytes: 100, UsedBytes: 60, AvailableBytes: 40},
		Filesystems: []usage.Filesystem{{Mount: "/", Device: "/dev/demo1", Type: "ext4", TotalBytes: 1000, UsedBytes: 600, FreeBytes: 300}},
	}
}

func TestHostUsagePrintsJSONAndText(t *testing.T) {
	useHostFixture(t, 1000)
	startFakeService(t, fakeHandlers{})
	svc := startFakeHostService(t, fakeHandlers{control.HostUsageCommand: func(json.RawMessage) any { return exampleUsage() }})
	code, out, errOut := run(t, "host", "usage", "--output", "json")
	got := objectOf(t, out)
	if code != 0 || got["cpu_percent"] != 12.3 || got["cores"] != 4.0 || got["load_15"] != 1.5 || got["memory_used_bytes"] != 60.0 ||
		got["filesystem.1.mount"] != "/" || got["filesystem.1.free_bytes"] != 300.0 || got["uptime_seconds"] != 90.0 {
		t.Fatalf("code %d, out %q, stderr %q", code, out, errOut)
	}
	if _, has := got["swap_total_bytes"]; has {
		t.Errorf("swap is not provided but shown: %v", got)
	}
	code, out, errOut = run(t, "host", "usage", "--output", "table")
	if code != 0 || !strings.Contains(out, "/dev/demo1") || svc.count(control.HostUsageCommand) != 2 {
		t.Fatalf("code %d, out %q, stderr %q", code, out, errOut)
	}
}

func TestHostUsageNeedsAnAdministrator(t *testing.T) {
	useHostFixture(t, 1001)
	if code, _, errOut := run(t, "host", "usage"); code != 1 || !strings.Contains(errOut, "reserved for administrators") {
		t.Fatalf("code %d, stderr %q", code, errOut)
	}
}
