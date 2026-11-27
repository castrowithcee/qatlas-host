package hostservice

import (
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/host/auditlog"
)

func TestUsageIsReadWithoutRootAndLogged(t *testing.T) {
	e := start(t, 4100)
	var res control.HostUsageResult
	if err := e.do(control.HostUsageCommand, nil, &res); err != nil {
		t.Fatal(err)
	}
	if res.Memory == nil || res.Memory.TotalBytes == 0 {
		t.Fatalf("memory = %+v", res.Memory)
	}
	got := e.entries(t)
	if len(got) != 1 || got[0].Entry.Action != auditlog.OperationUsageRead || got[0].Entry.Result != auditlog.ResultSuccess {
		t.Fatalf("log = %+v", got)
	}
}
