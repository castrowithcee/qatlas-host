package engine

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/stacks"
)

var (
	fpID     = "sha256:" + strings.Repeat("a", 64)
	fpDigest = "sha256:" + strings.Repeat("b", 64)
)

// fingerprintFake is the answer of the fake deployer's ImageFingerprints.
type fingerprintFake struct {
	prints   []ImageFingerprint
	printErr error
	// loaded is what LoadedImage answers, by image reference.
	loaded map[string]ImageFingerprint
}

func (f *fingerprintFake) ImageFingerprints(context.Context, stacks.Stack, *Log) ([]ImageFingerprint, error) {
	return f.prints, f.printErr
}

func TestRunStoresFingerprints(t *testing.T) {
	r := rollbackRig(t, "web")
	r.dep.prints = []ImageFingerprint{
		{Service: "web", Image: "registry.example.test/demo/web:1", RepoDigest: fpDigest, ImageID: fpID},
		{Service: "db", Image: "demo-db:local", ImageID: fpID}, // local image: no repository digest
		{Service: "", Image: "dropped"},                        // invalid entry
		{Service: "bad", RepoDigest: "sha256:xyz", ImageID: fpID},
	}
	r.poll()
	st := r.stack("web")
	want := []ImageFingerprint{
		{Service: "bad", ImageID: fpID},
		{Service: "db", Image: "demo-db:local", ImageID: fpID},
		{Service: "web", Image: "registry.example.test/demo/web:1", RepoDigest: fpDigest, ImageID: fpID},
	}
	if !slices.Equal(st.Images, want) || !slices.Equal(st.LastRun.Images, want) {
		t.Fatalf("images = %+v / %+v", st.Images, st.LastRun.Images)
	}
	if err := validateState(State{Schema: stateSchema, Stacks: map[string]StackState{"web": st}, Runs: []RunRecord{*st.LastRun}}); err != nil {
		t.Fatal(err)
	}
}

func TestRollbackStoresFingerprints(t *testing.T) {
	r := rollbackRig(t, "web")
	r.dep.prints = []ImageFingerprint{{Service: "web", ImageID: fpID}}
	r.dep.fail("check", errBoom)
	r.poll()
	st := wantRolledBack(t, r, commit(1001))
	if len(st.Images) != 1 || len(st.LastRun.Images) != 1 || st.LastRun.Images[0].ImageID != fpID {
		t.Fatalf("images = %+v / %+v", st.Images, st.LastRun.Images)
	}
}

func TestMissingFingerprintsDoNotFailTheRun(t *testing.T) {
	r := rollbackRig(t, "web")
	r.dep.prints = []ImageFingerprint{{Service: "web", ImageID: fpID}}
	r.poll() // commit 2 deployed with fingerprints
	r.copy.set(func(f *fakeCopy) {
		f.remote = commit(3)
		f.folders[[2]string{commit(2), commit(3)}] = []string{"web"}
	})
	r.dep.printErr = errBoom
	r.poll()
	st := r.stack("web")
	if st.LastRun.Result != ResultSucceeded || st.Deployed != commit(3) || st.Images != nil || st.LastRun.Images != nil {
		t.Fatalf("web = %+v / %+v", st, st.LastRun)
	}
	if text := r.runLog(st); !strings.Contains(text, "not read") {
		t.Fatalf("log lacks the read failure:\n%s", text)
	}
}

func TestFailedRunKeepsNoFingerprints(t *testing.T) {
	r := rollbackRig(t, "web")
	r.dep.prints = []ImageFingerprint{{Service: "web", ImageID: fpID}}
	r.dep.fail("stop", errBoom)
	r.poll()
	if st := r.stack("web"); st.LastRun.Result != ResultFailed || st.LastRun.Images != nil {
		t.Fatalf("web = %+v / %+v", st, st.LastRun)
	}
}

func TestValidateFingerprints(t *testing.T) {
	for name, f := range map[string]ImageFingerprint{
		"no service":     {ImageID: fpID},
		"bad digest":     {Service: "web", RepoDigest: "sha256:" + strings.Repeat("A", 64)},
		"bad image id":   {Service: "web", ImageID: "sha256:" + strings.Repeat("a", 63)},
		"long reference": {Service: "web", Image: strings.Repeat("x", maxNameLen+1)},
	} {
		st := StackState{Images: []ImageFingerprint{f}}
		if validateState(State{Schema: stateSchema, Stacks: map[string]StackState{"web": st}}) == nil {
			t.Errorf("%s was accepted", name)
		}
		if validateState(State{Schema: stateSchema, Runs: []RunRecord{{ID: "x", Images: []ImageFingerprint{f}}}}) == nil {
			t.Errorf("%s was accepted in a run", name)
		}
	}
}
