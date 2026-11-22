package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/tui"
)

func proposalsAnswer(list ...control.ProposalInfo) func(json.RawMessage) any {
	return func(json.RawMessage) any { return control.ProposalListResult{Proposals: list} }
}

// The interface lists the proposals of every deploy service, each marked with the engine it was submitted to, newest
// first, so that approving one reaches the service that stores it.
func TestEngineBackendProposalListMergesAndMarksTheEngine(t *testing.T) {
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice")
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	startFakeService(t, fakeHandlers{control.ProposalListCommand: proposalsAnswer(
		control.ProposalInfo{ID: "s1", Kind: control.ProposalKindSourceAdd, Submitted: now.Add(-4 * time.Hour)})})
	serveFake(t, eps["alice"], fakeHandlers{control.ProposalListCommand: proposalsAnswer(
		control.ProposalInfo{ID: "a1", Kind: control.ProposalKindSourceAdd, Submitted: now.Add(-time.Hour)})})
	res, err := newEngineBackend().ProposalList(context.Background())
	if err != nil || len(res.Proposals) != 2 {
		t.Fatalf("proposals %+v, err %v", res, err)
	}
	if p := res.Proposals[0]; p.ID != "a1" || p.Engine != "rootless:alice" {
		t.Fatalf("first proposal %+v", p)
	}
	if p := res.Proposals[1]; p.ID != "s1" || p.Engine != control.EngineSystem {
		t.Fatalf("second proposal %+v", p)
	}
}

// A call about a proposal that names its engine reaches that deploy service only.
func TestEngineBackendProposalShowGoesToTheNamedEngine(t *testing.T) {
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice")
	system := startFakeService(t, fakeHandlers{control.ProposalShowCommand: func(json.RawMessage) any { return control.ProposalShowResult{} }})
	alice := serveFake(t, eps["alice"], fakeHandlers{control.ProposalShowCommand: func(json.RawMessage) any {
		return control.ProposalShowResult{ProposalInfo: control.ProposalInfo{ID: "a1"}}
	}})
	res, err := newEngineBackend().ProposalShow(tui.WithEngine(context.Background(), "rootless:alice"), "a1")
	if err != nil || res.ID != "a1" || alice.count(control.ProposalShowCommand) != 1 || system.count(control.ProposalShowCommand) != 0 {
		t.Fatalf("result %+v, err %v, alice %v, system %v", res, err, alice.calls, system.calls)
	}
}

// source inspect --engine asks the deploy service of that engine, as source add does, and refuses an engine
// without an instance.
func TestSourceInspectForAnEngineAsksItsInstance(t *testing.T) {
	d := useFolderHost(t)
	eps := engineHost(t, d, "alice")
	system := startFakeService(t, addHandlers(localInspect(false)))
	alice := serveFake(t, eps["alice"], addHandlers(localInspect(false)))
	dir := t.TempDir()
	code, _, stderr := run(t, "source", "inspect", dir, "--engine", "rootless:alice")
	if code != 0 || alice.count(control.SourceInspectCommand) != 1 || system.count(control.SourceInspectCommand) != 0 {
		t.Fatalf("code %d, stderr %q, alice %v, system %v", code, stderr, alice.calls, system.calls)
	}
	code, _, stderr = run(t, "source", "inspect", dir, "--engine", "rootless:carol")
	if code != 2 || !strings.Contains(stderr, "no deploy service is set up") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
}

// The steps of the interface on the host follow the engine its context names: the drop-in and the owner of a created
// folder are those of that engine's instance, and a name that is no engine is a refusal.
func TestTuiFoldersGrantFollowsTheEngineOfTheContext(t *testing.T) {
	d := useFolderHost(t)
	engineHost(t, d, "alice")
	dir := filepath.Join(t.TempDir(), "stacks")
	ctx := tui.WithEngine(t.Context(), "rootless:alice")
	if err := (tuiFolders{}).Grant(ctx, dir, true); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(d.root, d.instanceList[0].StackFoldersDropIn()))
	if err != nil || string(data) != "[Service]\nReadWritePaths=-"+dir+"\n" {
		t.Fatalf("drop-in %q, %v", data, err)
	}
	if d.dropIn(t) != "" || len(d.owners) != 1 || d.owners[0] != "alice" {
		t.Fatalf("system drop-in %q, owners %v", d.dropIn(t), d.owners)
	}
	if err := (tuiFolders{}).Grant(tui.WithEngine(t.Context(), "rootless:carol"), dir, true); err == nil {
		t.Fatal("an engine without an instance was granted")
	}
}
