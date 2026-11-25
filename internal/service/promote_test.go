package service_test

import (
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/engine"
)

func (h *harness) promoteSource(stack string) control.PromoteSource {
	h.t.Helper()
	var src control.PromoteSource
	if err := h.call(control.PromoteSourceCommand, control.PromoteSourceArgs{Stack: stack}, &src); err != nil {
		h.t.Fatal(err)
	}
	return src
}

func TestPromoteSourceReportsTheVersionsOfItsDeployedCommit(t *testing.T) {
	h := updateHarness(t, &fakeDeployer{})
	src := h.promoteSource("demo-stack")
	if src.Stack != "demo-stack" || src.Commit == "" || src.LastResult != engine.ResultSucceeded || src.Unhealthy ||
		len(src.Versions) != 1 || src.Versions[0] != (promoteVersion("DEMO_APP_VERSION", "registry.example.test/demo/app", "1.2.3")) {
		t.Fatalf("source = %+v", src)
	}
	wantCode(t, h.call(control.PromoteSourceCommand, control.PromoteSourceArgs{Stack: "nope"}, nil), control.CodeNoSuchStack)
}

func TestPromoteChecksThenRunsBoundToTheSourceCommit(t *testing.T) {
	h := updateHarness(t, &fakeDeployer{})
	src := h.promoteSource("demo-stack")
	args := control.PromoteArgs{Stack: "plain-stack", Source: src}
	var plan control.PromoteCheckResult
	if err := h.call(control.PromoteCheckCommand, args, &plan); err != nil {
		t.Fatal(err)
	}
	if plan.SourceCommit != src.Commit || len(plan.Steps) != 1 || plan.Steps[0] != (control.PromoteStepInfo{Variable: "DEMO_APP_VERSION", From: "0.9.1", To: "1.2.3"}) {
		t.Fatalf("plan = %+v", plan)
	}
	wantRefusedWithoutBinding(t, h, args)

	args.Commit = plan.SourceCommit
	var res control.PromoteRunResult
	if err := h.call(control.PromoteRunCommand, args, &res); err != nil {
		t.Fatal(err)
	}
	if res.Result != engine.ResultSucceeded || res.Run == "" || res.Commit == "" || len(res.Steps) != 1 {
		t.Fatalf("result = %+v", res)
	}
	if got := h.git.Pushed(); len(got) != 1 || got[0] != "Promote plain-stack from demo-stack: DEMO_APP_VERSION from 0.9.1 to 1.2.3" {
		t.Fatalf("pushed = %q", got)
	}

	wantNothingToDo(t, h, args)
}

// wantNothingToDo checks that a run of a target that already has the versions makes no run and pushes nothing
// more.
func wantNothingToDo(t *testing.T, h *harness, args control.PromoteArgs) {
	t.Helper()
	var res control.PromoteRunResult
	if err := h.call(control.PromoteRunCommand, args, &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Steps) != 0 || res.Run != "" || len(h.git.Pushed()) != 1 {
		t.Fatalf("second result = %+v, pushed = %q", res, h.git.Pushed())
	}
}

// wantRefusedWithoutBinding checks that the check pushed nothing and that a run needs the commit of the check
// and refuses when it is not the source's.
func wantRefusedWithoutBinding(t *testing.T, h *harness, args control.PromoteArgs) {
	t.Helper()
	wantCode(t, h.call(control.PromoteRunCommand, args, nil), control.CodeInvalid)
	args.Commit = strings.Repeat("a", 40)
	err := h.call(control.PromoteRunCommand, args, nil)
	wantCode(t, err, control.CodeNotUpdatable)
	if !strings.Contains(err.Error(), "preview again") || len(h.git.Pushed()) != 0 {
		t.Fatalf("err = %v, pushed = %q", err, h.git.Pushed())
	}
}

func TestPromoteForwardsTheRefusals(t *testing.T) {
	h := updateHarness(t, &fakeDeployer{})
	src := h.promoteSource("demo-stack")
	for _, tc := range []struct {
		name   string
		target string
		change func(*control.PromoteSource)
		code   string
		text   string
	}{
		{"last run failed", "plain-stack", func(s *control.PromoteSource) { s.LastResult = engine.ResultFailed }, control.CodeNotUpdatable, "last run"},
		{"failed commit", "plain-stack", func(s *control.PromoteSource) { s.FailedCommit = strings.Repeat("b", 40) }, control.CodeNotUpdatable, "failed"},
		{"unhealthy", "plain-stack", func(s *control.PromoteSource) { s.Unhealthy = true }, control.CodeNotUpdatable, "not healthy"},
		{"never deployed", "plain-stack", func(s *control.PromoteSource) { *s = control.PromoteSource{Stack: "demo-stack"} }, control.CodeNotUpdatable, "never deployed"},
		{"lower version", "demo-stack", func(s *control.PromoteSource) { s.Versions[0].Version = "0.9.1" }, control.CodeNotUpdatable, "rollback"},
		{"unknown version", "plain-stack", func(s *control.PromoteSource) { s.Versions[0].Version = "9.9.9" }, control.CodeBadVersion, ""},
		{"two versions of one image", "plain-stack", func(s *control.PromoteSource) {
			s.Versions = append(s.Versions, promoteVersion("OTHER", s.Versions[0].Image, "1.3.0"))
		}, control.CodeNotUpdatable, "different versions"},
		{"unknown target", "nope", func(*control.PromoteSource) {}, control.CodeNoSuchStack, ""},
	} {
		s := src
		s.Versions = append([]control.PromoteVersion(nil), src.Versions...)
		tc.change(&s)
		err := h.call(control.PromoteCheckCommand, control.PromoteArgs{Stack: tc.target, Source: s}, nil)
		wantCode(t, err, tc.code)
		if !strings.Contains(err.Error(), tc.text) {
			t.Errorf("%s: err = %v, want text %q", tc.name, err, tc.text)
		}
	}
	// A target without settings is refused as an update is.
	h2 := bound(t, &fakeDeployer{})
	wantCode(t, h2.call(control.PromoteCheckCommand, control.PromoteArgs{Stack: "plain-stack", Source: h2.promoteSource("demo-stack")}, nil), control.CodeNotUpdatable)
}

func TestPromoteChecksTheShapeOfAHandedOverSource(t *testing.T) {
	h := updateHarness(t, &fakeDeployer{})
	good := h.promoteSource("demo-stack")
	for name, change := range map[string]func(*control.PromoteSource){
		"stack name":   func(s *control.PromoteSource) { s.Stack = "../x" },
		"commit":       func(s *control.PromoteSource) { s.Commit = "main" },
		"result":       func(s *control.PromoteSource) { s.LastResult = "succeeded\n" },
		"variable":     func(s *control.PromoteSource) { s.Versions[0].Variable = "A=B" },
		"image":        func(s *control.PromoteSource) { s.Versions[0].Image = "a b" },
		"version":      func(s *control.PromoteSource) { s.Versions[0].Version = "1.0\nX=1" },
		"observe":      func(s *control.PromoteSource) { s.Observe = &control.ObserveInfo{Commit: "x"} },
		"long version": func(s *control.PromoteSource) { s.Versions[0].Version = strings.Repeat("1", 129) },
		"many versions": func(s *control.PromoteSource) {
			s.Versions = make([]control.PromoteVersion, control.MaxPromoteVersions+1)
		},
	} {
		s := good
		s.Versions = append([]control.PromoteVersion(nil), good.Versions...)
		change(&s)
		wantCode(t, h.call(control.PromoteCheckCommand, control.PromoteArgs{Stack: "plain-stack", Source: s}, nil), control.CodeInvalid)
		if t.Failed() {
			t.Fatalf("shape %q was not refused", name)
		}
	}
}

func promoteVersion(variable, image, version string) control.PromoteVersion {
	return control.PromoteVersion{Variable: variable, Image: image, Version: version}
}
