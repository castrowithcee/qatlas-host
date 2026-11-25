package engine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/castrowithcee/qatlas-host/internal/stacks"
	"github.com/castrowithcee/qatlas-host/internal/version"
)

const (
	appRepo = "registry.example.test/demo/app"
	dbRepo  = "registry.example.test/demo/db"
)

// promoteRig deploys web with the variables APP_VERSION (app) and DB_VERSION (db) at 1.0.0 and 5.0.0.
func promoteRig(t *testing.T) *rig {
	t.Helper()
	r := newRig(t, "web")
	body := "services:\n  app:\n    image: " + appRepo + ":${APP_VERSION}\n  db:\n    image: " + dbRepo + ":${DB_VERSION}\n"
	for name, content := range map[string]string{"compose.yaml": body, ".env": twoVarEnv} {
		if err := os.WriteFile(filepath.Join(r.wc, "web", name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	r.copy.files[commit(1)] = map[string][]byte{"web/.env": []byte(twoVarEnv)}
	r.deployAll()
	return r
}

// registryCheck judges a move as an update by hand does, against a fixed tag list.
func registryCheck(_ context.Context, img stacks.ImageVar, to string) error {
	_, err := version.Path(version.Input{Current: img.Value, Candidates: []string{"1.0.0", "1.1.0", "1.2.0", "2.0.0", "5.0.0", "5.1.0"}}, to, false)
	return err
}

// source is a healthy source whose images carry other variable names than the target's.
func source(env string) PromoteSource {
	return PromoteSource{
		Name:  "stage",
		State: StackState{Deployed: commit(9), LastRun: &RunRecord{Result: ResultSucceeded}},
		Images: []stacks.ImageVar{
			{Image: appRepo, Var: "STAGE_APP", Source: stacks.SourceEnv},
			{Image: "registry.example.test/demo/cache", Var: "CACHE_TAG", Source: stacks.SourceEnv},
		},
		Env: []byte(env),
	}
}

func TestPromoteMapsByRepositoryAndCommitsOnce(t *testing.T) {
	r := promoteRig(t)
	src := source("STAGE_APP=1.2.0\nCACHE_TAG=9.9.9\n")
	prev, err := r.eng.PreviewPromote(context.Background(), "web", src, registryCheck, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []VersionStep{{Variable: "APP_VERSION", From: "1.0.0", To: "1.2.0"}}
	if len(prev.Steps) != 1 || prev.Steps[0] != want[0] || len(prev.Kept) != 1 || prev.Kept[0] != "DB_VERSION" || prev.Commit != commit(9) {
		t.Fatalf("preview = %+v", prev)
	}
	res, err := r.eng.Promote(context.Background(), "web", src, prev.Commit, registryCheck, nil)
	if err != nil || res.Result != ResultSucceeded {
		t.Fatalf("promote: %v %+v", err, res)
	}
	runs, _ := r.eng.Runs("web")
	if runs[0].Trigger != TriggerPromote || runs[0].ID != res.RunID {
		t.Fatalf("run = %+v", runs[0])
	}
	if r.copy.called("commit") != 1 || !strings.Contains(r.env(), "APP_VERSION=1.2.0") || !strings.Contains(r.env(), "DB_VERSION=5.0.0") {
		t.Fatalf("commits %d env %q", r.copy.called("commit"), r.env())
	}
}

func TestPromoteSetsSeveralVariablesInOneCommit(t *testing.T) {
	r := promoteRig(t)
	src := source("STAGE_APP=1.1.0\nSTAGE_DB=5.1.0\n")
	src.Images = append(src.Images, stacks.ImageVar{Image: dbRepo, Var: "STAGE_DB", Source: stacks.SourceEnv})
	res, err := r.eng.Promote(context.Background(), "web", src, commit(9), registryCheck, nil)
	if err != nil || res.Result != ResultSucceeded || len(res.Steps) != 2 {
		t.Fatalf("promote: %v %+v", err, res)
	}
	if r.copy.called("commit") != 1 || !strings.Contains(r.env(), "APP_VERSION=1.1.0") || !strings.Contains(r.env(), "DB_VERSION=5.1.0") {
		t.Fatalf("commits %d env %q", r.copy.called("commit"), r.env())
	}
}

func TestPromoteAmbiguousRepositoryNamesTheImage(t *testing.T) {
	r := promoteRig(t)
	src := source("STAGE_APP=1.1.0\nOLD_APP=1.2.0\n")
	src.Images = append(src.Images, stacks.ImageVar{Image: appRepo, Var: "OLD_APP", Source: stacks.SourceEnv})
	_, err := r.eng.PreviewPromote(context.Background(), "web", src, registryCheck, nil)
	if !errors.Is(err, ErrAmbiguousImage) || !strings.Contains(err.Error(), appRepo) {
		t.Fatalf("err = %v", err)
	}
}

func TestPromoteSameVersionDoesNothingAndLowerIsRefused(t *testing.T) {
	r := promoteRig(t)
	res, err := r.eng.Promote(context.Background(), "web", source("STAGE_APP=1.0.0\n"), commit(9), registryCheck, nil)
	if err != nil || res.RunID != "" || len(res.Steps) != 0 || len(res.Unchanged) != 1 {
		t.Fatalf("same: %v %+v", err, res)
	}
	r.update(VersionStep{Variable: "APP_VERSION", From: "1.0.0", To: "1.2.0"})
	r.resetSteps()
	_, err = r.eng.Promote(context.Background(), "web", source("STAGE_APP=1.1.0\n"), commit(9), registryCheck, nil)
	if !errors.Is(err, ErrLowerVersion) || !strings.Contains(err.Error(), "qhost stack rollback") {
		t.Fatalf("lower: %v", err)
	}
	if len(r.dep.stepsOf("web")) != 0 {
		t.Fatal("something ran")
	}
}

func TestPromoteRefusesAFailedOrUnhealthySourceButNotAnObservedOne(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	for name, tc := range map[string]struct {
		change func(*StackState)
		want   error
	}{
		"last run failed":   {func(s *StackState) { s.LastRun.Result = ResultRolledBack }, ErrSourceRun},
		"failed commit":     {func(s *StackState) { s.FailedCommit = commit(7) }, ErrSourceFailed},
		"failure":           {func(s *StackState) { s.Failure = &Failure{Kind: "exited", Since: now} }, ErrSourceUnhealthy},
		"repair halted":     {func(s *StackState) { s.Repair = &Repair{Halted: &Hold{Since: now}} }, ErrSourceUnhealthy},
		"paused":            {func(s *StackState) { s.Paused = &Pause{Time: now} }, ErrSourceUnhealthy},
		"stopped":           {func(s *StackState) { s.Stopped = &Hold{Since: now} }, ErrSourceUnhealthy},
		"never deployed":    {func(s *StackState) { *s = StackState{} }, ErrSourceNotDeployed},
		"under observation": {func(s *StackState) { s.Observe = &Observation{Until: now.Add(time.Hour)} }, nil},
	} {
		t.Run(name, func(t *testing.T) {
			r := promoteRig(t)
			src := source("STAGE_APP=1.1.0\n")
			tc.change(&src.State)
			prev, err := r.eng.PreviewPromote(context.Background(), "web", src, registryCheck, nil)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if tc.want == nil && (prev.Observe == nil || len(prev.Steps) != 1) {
				t.Fatalf("preview = %+v", prev)
			}
		})
	}
}

func TestPromoteRefusesTheTargetAsAnUpdateByHandDoes(t *testing.T) {
	r := promoteRig(t)
	if _, err := r.eng.SetPin(context.Background(), "web", true); err != nil {
		t.Fatal(err)
	}
	_, err := r.eng.Promote(context.Background(), "web", source("STAGE_APP=1.1.0\n"), commit(9), registryCheck, nil)
	if !errors.Is(err, ErrPinned) || r.copy.called("commit") != 0 {
		t.Fatalf("err = %v", err)
	}
}

func TestPromoteRefusesAChangedSource(t *testing.T) {
	r := promoteRig(t)
	src := source("STAGE_APP=1.1.0\n")
	src.State.Deployed = commit(10)
	res, err := r.eng.Promote(context.Background(), "web", src, commit(9), registryCheck, nil)
	if !errors.Is(err, ErrSourceChanged) || res.RunID != "" || len(r.dep.stepsOf("web")) != 0 {
		t.Fatalf("err = %v %+v", err, res)
	}
}

// A source of another deploy instance has an engine of its own: it reads the source, the caller hands it over.
func TestPromoteSourceOfAnotherEngine(t *testing.T) {
	r := promoteRig(t)
	other := newRig(t, "stage")
	if err := os.WriteFile(filepath.Join(other.wc, "stage", ".env"), []byte("APP_VERSION=1.2.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	other.copy.files[commit(1)] = map[string][]byte{"stage/.env": []byte("APP_VERSION=1.2.0\n")}
	other.deployAll()
	src, err := other.eng.ReadPromoteSource(context.Background(), "stage")
	if err != nil {
		t.Fatal(err)
	}
	res, err := r.eng.Promote(context.Background(), "web", src, src.State.Deployed, registryCheck, nil)
	if err != nil || res.Result != ResultSucceeded || !strings.Contains(r.env(), "APP_VERSION=1.2.0") {
		t.Fatalf("promote: %v %+v env %q", err, res, r.env())
	}
	if _, err := r.eng.ReadPromoteSource(context.Background(), "stage"); !errors.Is(err, ErrUnknownStack) {
		t.Fatalf("err = %v", err)
	}
}
