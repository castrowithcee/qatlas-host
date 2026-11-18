package service_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

func TestServiceLogAnswersTheServiceOwnLog(t *testing.T) {
	h := start(t)
	var res control.ServiceLogResult
	if err := h.call(control.ServiceLogCommand, control.ServiceLogArgs{Level: control.LogLevelInfo}, &res); err != nil {
		t.Fatal(err)
	}
	var started bool
	for _, e := range res.Entries {
		if e.Message == "service started" && e.Level == control.LogLevelInfo && e.Source == control.ServiceLogSourceHost {
			started = true
		}
	}
	if !started || res.Cursor == "" {
		t.Fatalf("answer = %+v", res)
	}
	var next control.ServiceLogResult
	if err := h.call(control.ServiceLogCommand, control.ServiceLogArgs{Cursor: res.Cursor}, &next); err != nil || next.Cursor != res.Cursor && len(next.Entries) == 0 {
		t.Fatalf("follow = %+v, %v", next, err)
	}
	if fi, err := os.Stat(filepath.Join(h.state, "service-log")); err != nil || fi.Mode().Perm() != 0o700 {
		t.Fatalf("store dir = %v, %v", fi, err)
	}
}
