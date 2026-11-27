package cli

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

func f64(v float64) *float64 { return &v }

func exampleHistory() control.HostUsageHistoryResult {
	return control.HostUsageHistoryResult{
		Resolution: "minute",
		From:       time.Date(2001, 2, 3, 4, 0, 0, 0, time.UTC),
		To:         time.Date(2001, 2, 3, 5, 0, 0, 0, time.UTC),
		Series: []control.HostUsageSeries{
			{Metric: "cpu_percent", Points: []control.HostUsagePoint{
				{Time: 981172800, Mean: f64(10), Max: f64(30)}, {Time: 981172920, Mean: f64(20), Max: f64(90)}}},
			{Metric: "swap_used_bytes", Points: []control.HostUsagePoint{}},
			{Metric: "filesystem_used_bytes", Mount: "/data", TotalBytes: 5000, Points: []control.HostUsagePoint{
				{Time: 981172800, Mean: f64(400), Max: f64(450)}}},
		},
	}
}

func startHistoryService(t *testing.T, args *control.HostUsageHistoryArgs) {
	t.Helper()
	useHostFixture(t, 1000)
	startFakeService(t, fakeHandlers{})
	startFakeHostService(t, fakeHandlers{control.HostUsageHistoryCommand: func(raw json.RawMessage) any {
		_ = json.Unmarshal(raw, args)
		return exampleHistory()
	}})
}

func TestHostUsageHistorySummarizesAsText(t *testing.T) {
	var args control.HostUsageHistoryArgs
	startHistoryService(t, &args)
	code, out, errOut := run(t, "host", "usage-history", "--output", "compact", "--resolution", "minute", "--metric", "cpu_percent",
		"--from", "2001-02-03T04:00:00Z")
	if code != 0 || args.Resolution != "minute" || args.From != "2001-02-03T04:00:00Z" || len(args.Metrics) != 1 {
		t.Fatalf("code %d, args %+v, stderr %q", code, args, errOut)
	}
	for _, want := range []string{"resolution=minute", "cpu_percent_min=10", "cpu_percent_mean=15", "cpu_percent_max=90",
		"swap_used_bytes_points=0", "filesystem.1.mount=/data", "filesystem.1.used_min=400", "filesystem.1.used_max=450"} {
		if !strings.Contains(out, want) {
			t.Errorf("text lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "981172920") {
		t.Errorf("text shows single points:\n%s", out)
	}
}

func TestHostUsageHistoryListsEveryPointAsJSON(t *testing.T) {
	var args control.HostUsageHistoryArgs
	startHistoryService(t, &args)
	code, out, errOut := run(t, "host", "usage-history", "--output", "json")
	var got struct {
		Resolution string                    `json:"resolution"`
		Series     []control.HostUsageSeries `json:"series"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil || code != 0 || got.Resolution != "minute" || len(got.Series) != 3 ||
		len(got.Series[0].Points) != 2 || *got.Series[0].Points[1].Max != 90 || got.Series[2].Mount != "/data" {
		t.Fatalf("code %d, out %q, stderr %q, err %v", code, out, errOut, err)
	}
}

func TestHostUsageHistoryShowsTheReasonWhenThereIsNoHistory(t *testing.T) {
	useHostFixture(t, 1000)
	startFakeService(t, fakeHandlers{})
	startFakeHostService(t, fakeHandlers{control.HostUsageHistoryCommand: func(json.RawMessage) any {
		return control.NewError(control.CodeNoHistory, "the usage history is turned off on this host. Ask an administrator to turn it on")
	}})
	code, out, errOut := run(t, "host", "usage-history")
	if code == 0 || out != "" || !strings.Contains(errOut, "turned off") || !strings.Contains(errOut, "Ask an administrator") {
		t.Fatalf("code %d, out %q, stderr %q", code, out, errOut)
	}
}

func TestHostUsageHistoryRejectsABadTime(t *testing.T) {
	useHostFixture(t, 1000)
	if code, _, errOut := run(t, "host", "usage-history", "--from", "soon"); code == 0 || !strings.Contains(errOut, "--from") {
		t.Fatalf("code %d, stderr %q", code, errOut)
	}
}

func TestHostUsageHistoryNeedsAnAdministrator(t *testing.T) {
	useHostFixture(t, 1001)
	if code, _, errOut := run(t, "host", "usage-history"); code != 1 || !strings.Contains(errOut, "reserved for administrators") {
		t.Fatalf("code %d, stderr %q", code, errOut)
	}
}
