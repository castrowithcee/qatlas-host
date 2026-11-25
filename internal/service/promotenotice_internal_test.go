package service

import (
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/engine"
)

func TestPromoteSeenReportsEachSourceVersionOnce(t *testing.T) {
	var seen promoteSeen
	step := func(to string) []engine.VersionStep {
		return []engine.VersionStep{{Variable: "APP_TAG", From: "1.0.0", To: to}}
	}
	reported := func(stack string, steps []engine.VersionStep) int { return len(seen.fresh(stack, steps)) }

	if n := reported("prod", step("1.2.0")); n != 1 {
		t.Fatalf("first report: %d", n)
	}
	if n := reported("prod", step("1.2.0")); n != 0 {
		t.Fatalf("the same version again: %d", n)
	}
	if n := reported("other", step("1.2.0")); n != 1 {
		t.Fatalf("another stack: %d", n)
	}
	if n := reported("prod", step("1.3.0")); n != 1 {
		t.Fatalf("a new version of the source: %d", n)
	}
	// the target caught up: nothing to report, and the version is new again if it is offered later
	reported("prod", nil)
	if n := reported("prod", step("1.3.0")); n != 1 {
		t.Fatalf("after the target caught up: %d", n)
	}
}
