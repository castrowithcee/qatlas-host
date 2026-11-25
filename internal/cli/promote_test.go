package cli

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

const promoteCommit = "0123456789abcdef0123456789abcdef01234567"

// promoteHosts has the source in the rootless engine of alice and the target in the system engine; steps are
// the versions the check offers and result the answer of the run.
func promoteHosts(t *testing.T, steps []control.PromoteStepInfo, result control.PromoteRunResult, refuseOtherCommit ...bool) (system, alice *fakeService, runArgs func() control.PromoteArgs) {
	t.Helper()
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice")
	var mu sync.Mutex
	var last control.PromoteArgs
	src := fakeHandlers{control.PromoteSourceCommand: func(json.RawMessage) any {
		return control.PromoteSource{Stack: "stage", Commit: promoteCommit, LastResult: "succeeded",
			Versions: []control.PromoteVersion{{Variable: "APP_TAG", Image: "registry.example.test/demo/app", Version: "1.3.0"}}}
	}}
	target := fakeHandlers{
		control.PromoteCheckCommand: func(raw json.RawMessage) any {
			var a control.PromoteArgs
			_ = json.Unmarshal(raw, &a)
			return control.PromoteCheckResult{Stack: a.Stack, Source: a.Source.Stack, SourceCommit: a.Source.Commit, Steps: steps,
				Unchanged: []string{"DB_TAG"}, Kept: []string{"CACHE_TAG"},
				Observe: &control.ObserveInfo{Commit: promoteCommit, Until: time.Date(2999, 1, 1, 0, 0, 0, 0, time.UTC)}}
		},
		control.PromoteRunCommand: func(raw json.RawMessage) any {
			mu.Lock()
			defer mu.Unlock()
			_ = json.Unmarshal(raw, &last)
			if len(refuseOtherCommit) > 0 && last.Commit != last.Source.Commit {
				return &control.Error{Code: control.CodeNotUpdatable, Message: "the source stack deployed another commit since the preview; preview again"}
			}
			return result
		},
	}
	system = startFakeService(t, withStatus(target, nil, "prod"))
	alice = serveFake(t, eps["alice"], withStatus(src, nil, "stage"))
	return system, alice, func() control.PromoteArgs {
		mu.Lock()
		defer mu.Unlock()
		return last
	}
}

func TestPromoteShowsThePreviewWithoutConfirm(t *testing.T) {
	steps := []control.PromoteStepInfo{{Variable: "APP_VERSION", From: "1.2.0", To: "1.3.0"}}
	system, alice, _ := promoteHosts(t, steps, control.PromoteRunResult{})
	code, out, stderr := run(t, "stack", "promote", "stage", "prod", "--output", "json")
	if code == 0 || system.count(control.PromoteRunCommand) != 0 {
		t.Fatalf("code %d, run calls %d", code, system.count(control.PromoteRunCommand))
	}
	if alice.count(control.PromoteSourceCommand) != 1 || system.count(control.PromoteSourceCommand) != 0 || system.count(control.PromoteCheckCommand) != 1 {
		t.Fatal("the source was not read at its own engine and the check not asked at the target's")
	}
	var got []string
	for _, r := range rowsOf(t, out) {
		got = append(got, fmt.Sprint(r["variable"], " ", r["from"], " ", r["to"], " ", r["result"]))
	}
	if want := []string{"APP_VERSION 1.2.0 1.3.0 set", "DB_TAG <nil> <nil> unchanged", "CACHE_TAG <nil> <nil> kept"}; !slices.Equal(got, want) {
		t.Fatalf("rows = %q, want %q", got, want)
	}
	if !strings.Contains(stderr, "Source stage at commit 0123456789ab") || !strings.Contains(stderr, "under observation until") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestPromoteConfirmRunsBoundToTheShownCommit(t *testing.T) {
	steps := []control.PromoteStepInfo{{Variable: "APP_VERSION", From: "1.2.0", To: "1.3.0"}}
	system, _, runArgs := promoteHosts(t, steps, control.PromoteRunResult{
		PromoteCheckResult: control.PromoteCheckResult{Stack: "prod", Steps: steps}, Run: "20260102T030405Z-0123abcd", Commit: promoteCommit, Result: "succeeded"})
	code, out, stderr := run(t, "stack", "promote", "stage", "prod", "--confirm", "--output", "json")
	if code != 0 || system.count(control.PromoteRunCommand) != 1 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	got := runArgs()
	if got.Stack != "prod" || got.Commit != promoteCommit || got.Source.Stack != "stage" || got.Source.Commit != promoteCommit || len(got.Source.Versions) != 1 {
		t.Fatalf("run arguments = %+v", got)
	}
	rows := rowsOf(t, out)
	if len(rows) != 1 || rows[0]["result"] != "succeeded" || rows[0]["run"] != "20260102T030405Z-0123abcd" || rows[0]["to"] != "1.3.0" {
		t.Fatalf("rows = %s", out)
	}
}

func TestPromoteSourceCommitIsPassedToTheRun(t *testing.T) {
	steps := []control.PromoteStepInfo{{Variable: "APP_VERSION", From: "1.2.0", To: "1.3.0"}}
	_, _, runArgs := promoteHosts(t, steps, control.PromoteRunResult{
		PromoteCheckResult: control.PromoteCheckResult{Stack: "prod", Steps: steps}, Run: "20260102T030405Z-0123abcd", Result: "succeeded"})
	other := strings.Repeat("b", 40)
	if code, _, stderr := run(t, "stack", "promote", "stage", "prod", "--confirm", "--source-commit", other, "--output", "json"); code != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if got := runArgs(); got.Commit != other {
		t.Fatalf("run commit = %q, want %q", got.Commit, other)
	}
}

func TestPromotePreviewNamesTheCommitAndTheConfirmCommand(t *testing.T) {
	steps := []control.PromoteStepInfo{{Variable: "APP_VERSION", From: "1.2.0", To: "1.3.0"}}
	promoteHosts(t, steps, control.PromoteRunResult{})
	_, out, stderr := run(t, "stack", "promote", "stage", "prod", "--output", "json")
	if want := "sudo qhost stack promote stage prod --confirm --source-commit " + promoteCommit; !strings.Contains(stderr, want) {
		t.Fatalf("stderr = %q, want %q", stderr, want)
	}
	for _, r := range rowsOf(t, out) {
		if r["source_commit"] != promoteCommit {
			t.Fatalf("row = %v", r)
		}
	}
}

func TestPromoteSourceCommitMustBeACommit(t *testing.T) {
	system, _, _ := promoteHosts(t, nil, control.PromoteRunResult{})
	code, _, stderr := run(t, "stack", "promote", "stage", "prod", "--confirm", "--source-commit", "abc123")
	if code != 2 || !strings.Contains(stderr, "--source-commit") || system.count(control.PromoteRunCommand) != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
}

func TestPromoteReportsTheRefusalOfAChangedSource(t *testing.T) {
	steps := []control.PromoteStepInfo{{Variable: "APP_VERSION", From: "1.2.0", To: "1.3.0"}}
	promoteHosts(t, steps, control.PromoteRunResult{}, true)
	code, _, stderr := run(t, "stack", "promote", "stage", "prod", "--confirm", "--source-commit", strings.Repeat("b", 40))
	if code == 0 || !strings.Contains(stderr, "preview again") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
}

func TestPromoteFailedRunExitsWithOne(t *testing.T) {
	steps := []control.PromoteStepInfo{{Variable: "APP_VERSION", From: "1.2.0", To: "1.3.0"}}
	promoteHosts(t, steps, control.PromoteRunResult{PromoteCheckResult: control.PromoteCheckResult{Stack: "prod", Steps: steps},
		Run: "20260102T030405Z-0123abcd", Result: "rolled-back", Error: "the check failed"})
	if code, _, stderr := run(t, "stack", "promote", "stage", "prod", "--confirm"); code != 1 || !strings.Contains(stderr, "did not succeed") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
}

func TestPromoteWithNothingToDoSucceeds(t *testing.T) {
	system, _, _ := promoteHosts(t, nil, control.PromoteRunResult{})
	code, out, stderr := run(t, "stack", "promote", "stage", "prod")
	if code != 0 || system.count(control.PromoteRunCommand) != 0 || !strings.Contains(out, "current") || !strings.Contains(stderr, "Nothing to promote") {
		t.Fatalf("code %d, out %q, stderr %q", code, out, stderr)
	}
}

func TestPromoteTakesTwoDifferentStacks(t *testing.T) {
	startUpdate(t, okDeployer{})
	if code, _, stderr := run(t, "stack", "promote", "demo-stack"); code == 0 || !strings.Contains(stderr, "exactly two arguments") {
		t.Fatalf("one stack: %d %q", code, stderr)
	}
	if code, _, stderr := run(t, "stack", "promote", "demo-stack", "demo-stack"); code == 0 || !strings.Contains(stderr, "same stack") {
		t.Fatalf("same stack: %d %q", code, stderr)
	}
}

func TestPromoteForwardsTheRefusalsOfTheService(t *testing.T) {
	startUpdate(t, okDeployer{})
	if code, _, stderr := run(t, "stack", "promote", "demo-stack", "plain-stack", "--confirm"); code == 0 || !strings.Contains(stderr, "never updated") {
		t.Fatalf("target without settings: %d %q", code, stderr)
	}
	if code, _, _ := run(t, "stack", "update", "demo-stack"); code != 0 {
		t.Fatal("update failed")
	}
	if code, _, stderr := run(t, "stack", "promote", "plain-stack", "demo-stack", "--confirm"); code == 0 || !strings.Contains(stderr, "tag list") {
		t.Fatalf("lower version: %d %q", code, stderr)
	}
}
