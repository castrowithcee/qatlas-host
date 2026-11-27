package cli

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-kit/release/releasetest"
)

// useTestTransition registers one transition that writes a marker below the root and counts its runs.
func useTestTransition(t *testing.T, fail error) (marker string, applied *int) {
	t.Helper()
	applied = new(int)
	old := transitions
	t.Cleanup(func() { transitions = old })
	transitions = []transition{{
		name: "invented-marker",
		done: func(_ context.Context, root string) (bool, error) {
			_, err := os.Lstat(filepath.Join(root, "invented-marker"))
			if errors.Is(err, os.ErrNotExist) {
				return false, nil
			}
			return err == nil, err
		},
		apply: func(_ context.Context, root string) error {
			if fail != nil {
				return fail
			}
			*applied++
			return os.WriteFile(filepath.Join(root, "invented-marker"), []byte("applied\n"), 0o644)
		},
	}}
	return "invented-marker", applied
}

func TestSelfUpdateRunsTheTransitionsOfTheNewProgramBeforeTheRestart(t *testing.T) {
	signer := releasetest.NewSigner(t)
	h := useFakeHost(t, newFakeReleases(t, signer, false, "v0.1.0", "v0.2.0"), signer, "v0.1.0", true)
	code, stdout, stderr := run(t, "update", "--yes", "--confirm")
	if code != 0 {
		t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if len(h.migrated) != 1 || h.migrated[0].exe != h.exe || h.migrated[0].restarts != 0 {
		t.Fatalf("transitions ran %+v, want once by the installed program before any restart", h.migrated)
	}
	if got := h.restarted(); !slices.Equal(got, []string{"qhost", "qhost-deploy"}) {
		t.Fatalf("restarted = %q", got)
	}
}

func TestSelfUpdateTransitionRunsOnceAndARepeatChangesNothing(t *testing.T) {
	signer := releasetest.NewSigner(t)
	h := useFakeHost(t, newFakeReleases(t, signer, false, "v0.1.0", "v0.2.0"), signer, "v0.1.0")
	marker, applied := useTestTransition(t, nil)
	// The new program runs in process, against the root of the fake host.
	selfUpdate.migrateRun = func(ctx context.Context, _ string, stderr io.Writer) error {
		_, err := runTransitions(ctx, h.dir, transitions, stderr)
		return err
	}
	code, _, stderr := run(t, "update", "--yes", "--confirm")
	if code != 0 || *applied != 1 || !strings.Contains(stderr, "transition invented-marker: applied\n") {
		t.Fatalf("code %d, applied %d, stderr %q", code, *applied, stderr)
	}
	if _, err := os.Stat(filepath.Join(h.dir, marker)); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := run(t, "update", "migrate", "--output", "json")
	if code != 0 || *applied != 1 || !strings.Contains(stderr, "transition invented-marker: already done\n") {
		t.Fatalf("repeat: code %d, applied %d, stderr %q", code, *applied, stderr)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil || got["transitions"] != float64(1) || got["applied"] != "" {
		t.Fatalf("repeat: stdout %q, %v", stdout, err)
	}
}

func TestSelfUpdateWarnsWhenTheTransitionsFail(t *testing.T) {
	signer := releasetest.NewSigner(t)
	h := useFakeHost(t, newFakeReleases(t, signer, false, "v0.1.0", "v0.2.0"), signer, "v0.1.0")
	selfUpdate.migrateRun = func(context.Context, string, io.Writer) error { return errors.New("exit status 1") }
	code, _, stderr := run(t, "update", "--yes", "--confirm")
	if code != 0 || !strings.Contains(stderr, "transitions of the new release did not all run") ||
		!strings.Contains(stderr, "Run 'sudo qhost update migrate' to retry.") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if h.program(t) != "new program v0.2.0" || !slices.Equal(h.restarted(), []string{"qhost"}) {
		t.Fatalf("program %q, restarted %q", h.program(t), h.restarted())
	}
}

func TestMigrateNamesTheCommandToRetryAfterAFailure(t *testing.T) {
	signer := releasetest.NewSigner(t)
	useFakeHost(t, newFakeReleases(t, signer, false, "v0.1.0"), signer, "v0.1.0")
	useTestTransition(t, errors.New("invented failure"))
	code, _, stderr := run(t, "update", "migrate")
	if code == 0 || !strings.Contains(stderr, "transition invented-marker: invented failure") ||
		!strings.Contains(stderr, "sudo qhost update migrate") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
}

func TestMigrateWithoutTransitionsChangesNothingAndNeedsRoot(t *testing.T) {
	signer := releasetest.NewSigner(t)
	h := useFakeHost(t, newFakeReleases(t, signer, false, "v0.1.0"), signer, "v0.1.0")
	if len(transitions) != 0 {
		t.Fatalf("this release registers transitions %v; the test expects none", transitions)
	}
	code, stdout, stderr := run(t, "update", "migrate", "--output", "json")
	if code != 0 || stderr != "" || strings.ReplaceAll(stdout, " ", "") != `{"transitions":0,"applied":""}`+"\n" {
		t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	useTestTransition(t, nil)
	selfUpdate.euid = func() int { return 1000 }
	if code, _, stderr := run(t, "update", "migrate"); code == 0 || !strings.Contains(stderr, "root") {
		t.Fatalf("not root: code %d, stderr %q", code, stderr)
	}
	if entries, _ := os.ReadDir(h.dir); slices.ContainsFunc(entries, func(e os.DirEntry) bool { return e.Name() == "invented-marker" }) {
		t.Fatal("a refused run applied the transition")
	}
}

func TestSelfUpdateRefusesAnInstalledVersionOlderThanTheReleaseUpdatesFrom(t *testing.T) {
	signer := releasetest.NewSigner(t)
	releases := newFakeReleasesWith(t, signer, false, map[string]string{"v0.3.0": `{"updatesFrom": "v0.2.5", "reinstall": ""}`},
		"v0.2.0", "v0.3.0")
	h := useFakeHost(t, releases, signer, "v0.2.0")
	code, _, stderr := run(t, "update", "--yes", "--confirm")
	if code != 2 || !strings.Contains(stderr, "updates directly only from v0.2.5") ||
		!strings.Contains(stderr, "Update to v0.2.5 first with 'sudo qhost update v0.2.5 --confirm'") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if h.program(t) != "old program" || len(h.calls) != 0 || len(h.migrated) != 0 {
		t.Fatal("a refused update changed the installation")
	}
}

func TestSelfUpdateRefusesAReleaseThatNeedsAFreshSetup(t *testing.T) {
	signer := releasetest.NewSigner(t)
	step := "Back up the stacks, then install v0.3.0 with the installer."
	releases := newFakeReleasesWith(t, signer, false,
		map[string]string{"v0.3.0": `{"updatesFrom": "v0.1.0", "reinstall": "` + step + `"}`}, "v0.2.0", "v0.3.0")
	h := useFakeHost(t, releases, signer, "v0.2.0")
	code, _, stderr := run(t, "update", "v0.3.0", "--yes", "--confirm")
	if code != 2 || !strings.Contains(stderr, "needs a fresh setup") || !strings.Contains(stderr, step) {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if h.program(t) != "old program" || len(h.calls) != 0 || len(h.migrated) != 0 {
		t.Fatal("a refused update changed the installation")
	}
}

func TestSelfUpdateRefusesInvalidRequirements(t *testing.T) {
	signer := releasetest.NewSigner(t)
	releases := newFakeReleasesWith(t, signer, false, map[string]string{"v0.3.0": `{"updatesFrom": "soon"}`}, "v0.2.0", "v0.3.0")
	h := useFakeHost(t, releases, signer, "v0.2.0")
	code, _, stderr := run(t, "update", "--yes", "--confirm")
	if code == 0 || !strings.Contains(stderr, "installation is unchanged") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if h.program(t) != "old program" || len(h.calls) != 0 {
		t.Fatal("a refused update changed the installation")
	}
}
