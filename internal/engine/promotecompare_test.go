package engine

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/stacks"
)

var (
	regDigest   = "sha256:" + strings.Repeat("c", 64)
	otherDigest = "sha256:" + strings.Repeat("d", 64)
)

// LoadedImage answers from loaded, by reference; an unknown reference is an error.
func (f *fingerprintFake) LoadedImage(_ context.Context, ref string, _ *Log) (ImageFingerprint, error) {
	if fp, ok := f.loaded[ref]; ok {
		return fp, nil
	}
	return ImageFingerprint{}, errors.New("no such image")
}

// compareRig is a promote rig whose target compares fingerprints.
func compareRig(t *testing.T, on bool) *rig {
	t.Helper()
	r := promoteRig(t)
	body := "version: 1\npromote:\n  compare_images: " + map[bool]string{true: "true", false: "false"}[on] + "\n"
	writeSettings(t, r.wc, "web", body)
	return r
}

// printed is a source whose app image has the given fingerprint.
func printed(fp ImageFingerprint) PromoteSource {
	src := source("STAGE_APP=1.2.0\n")
	fp.Image = appRepo + ":1.2.0"
	src.State.Images = []ImageFingerprint{fp}
	return src
}

func digestIs(d string) ManifestDigest {
	return func(context.Context, stacks.ImageVar, string) (string, error) { return d, nil }
}

func TestPromoteWithoutCompareAsksNothing(t *testing.T) {
	r := compareRig(t, false)
	asked := false
	digest := func(context.Context, stacks.ImageVar, string) (string, error) { asked = true; return otherDigest, nil }
	res, err := r.eng.Promote(context.Background(), "web", source("STAGE_APP=1.2.0\n"), commit(9), registryCheck, digest)
	if err != nil || res.Result != ResultSucceeded || res.Compare || asked || slices.Contains(r.dep.stepsOf("web"), "verify") {
		t.Fatalf("promote: %v %+v asked %v", err, res, asked)
	}
}

func TestPromoteCompareSameDigestRuns(t *testing.T) {
	r := compareRig(t, true)
	r.dep.loaded = map[string]ImageFingerprint{appRepo + ":1.2.0": {RepoDigest: regDigest, ImageID: fpID}}
	src := printed(ImageFingerprint{RepoDigest: regDigest, ImageID: fpDigest})
	prev, err := r.eng.PreviewPromote(context.Background(), "web", src, registryCheck, digestIs(regDigest))
	if err != nil || !prev.Compare {
		t.Fatalf("preview: %v %+v", err, prev)
	}
	res, err := r.eng.Promote(context.Background(), "web", src, commit(9), registryCheck, digestIs(regDigest))
	if err != nil || res.Result != ResultSucceeded || !res.Compare {
		t.Fatalf("promote: %v %+v", err, res)
	}
}

func TestPromoteCompareRefusesAnOverwrittenTagBeforeAnyStep(t *testing.T) {
	r := compareRig(t, true)
	src := printed(ImageFingerprint{RepoDigest: regDigest})
	_, err := r.eng.Promote(context.Background(), "web", src, commit(9), registryCheck, digestIs(otherDigest))
	if !errors.Is(err, ErrImageChanged) {
		t.Fatalf("err = %v", err)
	}
	for _, want := range []string{appRepo, "1.2.0", regDigest, otherDigest} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("err %q lacks %q", err, want)
		}
	}
	if len(r.dep.stepsOf("web")) != 0 || r.copy.called("commit") != 0 {
		t.Fatalf("something ran: %v", r.dep.stepsOf("web"))
	}
}

func TestPromoteCompareDifferingLoadedImageRollsBack(t *testing.T) {
	r := compareRig(t, true)
	r.dep.loaded = map[string]ImageFingerprint{appRepo + ":1.2.0": {RepoDigest: otherDigest, ImageID: fpID}}
	src := printed(ImageFingerprint{RepoDigest: regDigest})
	res, err := r.eng.Promote(context.Background(), "web", src, commit(9), registryCheck, digestIs(regDigest))
	if err != nil || res.Result != ResultRolledBack || !strings.Contains(res.Error, "verify") {
		t.Fatalf("promote: %v %+v", err, res)
	}
	// the new version is pulled but never started: the rollback stops the stack before its first up
	if steps := r.dep.stepsOf("web"); len(steps) < 5 || !slices.Equal(steps[:4], []string{"stop", "backup", "pull", "stop"}) || slices.Contains(steps[:4], "up") {
		t.Fatalf("steps = %v", steps)
	}
	if !strings.Contains(r.env(), "APP_VERSION=1.0.0") {
		t.Fatalf("env %q", r.env())
	}
}

func TestPromoteCompareLocalImageUsesTheImageID(t *testing.T) {
	r := compareRig(t, true)
	src := printed(ImageFingerprint{ImageID: fpID})
	r.dep.loaded = map[string]ImageFingerprint{appRepo + ":1.2.0": {ImageID: fpID}}
	asked := false
	digest := func(context.Context, stacks.ImageVar, string) (string, error) { asked = true; return "", nil }
	res, err := r.eng.Promote(context.Background(), "web", src, commit(9), registryCheck, digest)
	if err != nil || res.Result != ResultSucceeded || asked {
		t.Fatalf("same ID: %v %+v asked %v", err, res, asked)
	}

	r = compareRig(t, true)
	r.dep.loaded = map[string]ImageFingerprint{appRepo + ":1.2.0": {ImageID: fpDigest}}
	res, err = r.eng.Promote(context.Background(), "web", src, commit(9), registryCheck, digest)
	if err != nil || res.Result != ResultRolledBack {
		t.Fatalf("other ID: %v %+v", err, res)
	}
}

func TestPromoteCompareNeedsTheFingerprintOfTheSource(t *testing.T) {
	for name, src := range map[string]PromoteSource{
		"none recorded": source("STAGE_APP=1.2.0\n"),
		"empty":         printed(ImageFingerprint{}),
		"malformed":     printed(ImageFingerprint{RepoDigest: "sha256:xyz", ImageID: "nope"}),
		"another service": func() PromoteSource {
			s := printed(ImageFingerprint{RepoDigest: regDigest})
			s.State.Images[0].Image = dbRepo + ":5.0.0"
			return s
		}(),
	} {
		t.Run(name, func(t *testing.T) {
			r := compareRig(t, true)
			_, err := r.eng.PreviewPromote(context.Background(), "web", src, registryCheck, digestIs(regDigest))
			if !errors.Is(err, ErrNoFingerprint) || !strings.Contains(err.Error(), "deploy the source stack again") {
				t.Fatalf("err = %v", err)
			}
		})
	}
}
