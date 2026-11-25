package service_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/engine"
	"github.com/castrowithcee/qatlas-host/internal/service/servicetest"
)

func updatePushes(r *pollRig) []string {
	var out []string
	for _, m := range r.git.Pushed() {
		if strings.HasPrefix(m, "Update ") {
			out = append(out, m)
		}
	}
	return out
}

func TestAutomaticModeSkipsAFailedVersionUntilAHigherOneAppears(t *testing.T) {
	r, dep := pollHarness(t, map[string]string{servicetest.Settings("demo-stack"): intervalRules("automatic", 30)})
	dep.failNextChecks(1)
	r.step(t)
	first := "Update demo-stack DEMO_APP_VERSION from 1.2.3 to 1.3.0"
	if got := updatePushes(r); !slices.Equal(got, []string{first}) {
		t.Fatalf("pushed after the failure = %q", got)
	}
	r.step(t)
	r.step(t)
	if got := updatePushes(r); !slices.Equal(got, []string{first}) {
		t.Fatalf("the failed version was planned again: %q", got)
	}
	r.reg.addTag("1.4.0")
	r.step(t)
	want := []string{first, "Update demo-stack DEMO_APP_VERSION from 1.3.0 to 1.4.0"}
	if got := updatePushes(r); !slices.Equal(got, want) {
		t.Fatalf("pushed after a higher version = %q", got)
	}
}

func TestManualModeDoesNotReportAFailedVersionAgain(t *testing.T) {
	r, dep := pollHarness(t, map[string]string{servicetest.Settings("demo-stack"): intervalRules("manual", 30)})
	dep.failNextChecks(1)
	res, err := r.update(control.UpdateRunArgs{Stacks: []string{"demo-stack"}, Version: "1.3.0"})
	if err != nil || res.Stacks[0].Steps[0].Result != engine.ResultRolledBack {
		t.Fatalf("failing update: %+v, %v", res, err)
	}
	r.step(t)
	r.step(t)
	if n := r.notices.all(); len(n) != 0 {
		t.Fatalf("notices for a failed version: %+v", n)
	}
	r.reg.addTag("1.4.0")
	r.step(t)
	if n := r.notices.all(); len(n) != 1 || n[0].Items[0].Selected != "1.4.0" {
		t.Fatalf("notices after a higher version: %+v", n)
	}
}
