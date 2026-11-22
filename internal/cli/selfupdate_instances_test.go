package cli

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/deploysetup"
	"github.com/castrowithcee/qatlas-host/internal/window"
	"github.com/castrowithcee/qatlas-host/packaging"
	"github.com/castrowithcee/qatlas-kit/release/releasetest"
)

func instanceOf(t *testing.T, user string) deploysetup.Instance {
	t.Helper()
	inst, err := deploysetup.NewInstance(user)
	if err != nil {
		t.Fatal(err)
	}
	return inst
}

// setUpInstances marks the users as set up below root, as the setup would.
func setUpInstances(t *testing.T, root string, users ...string) {
	t.Helper()
	for _, u := range users {
		full := filepath.Join(root, instanceOf(t, u).TmpfilesConf())
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("d /run/invented\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// useRefresh lets the update's refresh step run the refresh of the instances in process, against the fake
// host, and records its systemctl calls and whether it ran at all.
func useRefresh(t *testing.T, h *fakeHost, failRestart string) (ran *[]string, called *bool) {
	t.Helper()
	ran, called = new([]string), new(bool)
	host := deploysetup.Host{Root: h.dir, Run: func(_ context.Context, _ string, args ...string) error {
		*ran = append(*ran, strings.Join(args, " "))
		if failRestart != "" && slices.Equal(args, []string{"restart", failRestart}) {
			return errors.New("exit status 1")
		}
		return nil
	}}
	selfUpdate.refresh = func(ctx context.Context, _ string, stderr io.Writer) error {
		*called = true
		res, err := host.RefreshInstances(ctx)
		for _, u := range res.Shadowed {
			_, _ = io.WriteString(stderr, "warning: a unit in /etc/systemd/system overrides the installed unit of "+u+"\n")
		}
		return err
	}
	return ran, called
}

func newUpdateHost(t *testing.T) *fakeHost {
	t.Helper()
	signer := releasetest.NewSigner(t)
	return useFakeHost(t, newFakeReleases(t, signer, false, "v0.1.0", "v0.2.0"), signer, "v0.1.0")
}

func TestSelfUpdateWithoutInstanceDoesNotRefresh(t *testing.T) {
	h := newUpdateHost(t)
	ran, called := useRefresh(t, h, "")
	if code, _, stderr := run(t, "update", "--yes", "--confirm"); code != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if *called || len(*ran) != 0 {
		t.Fatalf("called %v, ran %v", *called, *ran)
	}
	if _, err := os.Stat(filepath.Join(h.dir, deploysetup.TemplatePath())); err == nil {
		t.Error("the template was written without an instance")
	}
}

func TestSelfUpdateRestartsTwoInstancesAndWritesTheNewTemplate(t *testing.T) {
	h := newUpdateHost(t)
	setUpInstances(t, h.dir, "alice", "bob")
	template := filepath.Join(h.dir, deploysetup.TemplatePath())
	if err := os.MkdirAll(filepath.Dir(template), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(template, []byte("old template"), 0o644); err != nil {
		t.Fatal(err)
	}
	ran, _ := useRefresh(t, h, "")
	if code, _, stderr := run(t, "update", "--yes", "--confirm"); code != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	want := []string{"daemon-reload", "restart " + instanceOf(t, "alice").Unit(), "restart " + instanceOf(t, "bob").Unit()}
	if !slices.Equal(*ran, want) {
		t.Fatalf("ran %q, want %q", *ran, want)
	}
	if got, _ := os.ReadFile(template); string(got) != string(packaging.DeployInstanceUnit) {
		t.Errorf("template = %q", got)
	}
	if got := h.restarted(); !slices.Equal(got, []string{"qhost"}) {
		t.Errorf("service restarts = %q", got)
	}
}

func TestSelfUpdateKeepsTheSameTemplateAndStillRestarts(t *testing.T) {
	h := newUpdateHost(t)
	setUpInstances(t, h.dir, "alice")
	template := filepath.Join(h.dir, deploysetup.TemplatePath())
	if err := os.MkdirAll(filepath.Dir(template), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(template, packaging.DeployInstanceUnit, 0o444); err != nil {
		t.Fatal(err)
	}
	before, _ := os.Stat(template)
	ran, _ := useRefresh(t, h, "")
	if code, _, stderr := run(t, "update", "--yes", "--confirm"); code != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	after, _ := os.Stat(template)
	if !os.SameFile(before, after) || !slices.Contains(*ran, "restart "+instanceOf(t, "alice").Unit()) {
		t.Fatalf("template replaced %v, ran %q", !os.SameFile(before, after), *ran)
	}
}

func TestSelfUpdateWarnsAboutAnOverriddenInstanceUnit(t *testing.T) {
	h := newUpdateHost(t)
	setUpInstances(t, h.dir, "alice", "bob")
	shadow := filepath.Join(h.dir, "etc/systemd/system", instanceOf(t, "bob").Unit())
	if err := os.MkdirAll(filepath.Dir(shadow), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(shadow, []byte("[Service]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	useRefresh(t, h, "")
	code, _, stderr := run(t, "update", "--yes", "--confirm")
	bob, alice := instanceOf(t, "bob").Unit(), instanceOf(t, "alice").Unit()
	if code != 0 || !strings.Contains(stderr, "installed unit of "+bob+"\n") || strings.Contains(stderr, "installed unit of "+alice) {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
}

func TestSelfUpdateSurvivesAFailedInstanceRestart(t *testing.T) {
	h := newUpdateHost(t)
	setUpInstances(t, h.dir, "alice", "bob")
	ran, _ := useRefresh(t, h, instanceOf(t, "alice").Unit())
	code, stdout, stderr := run(t, "update", "--yes", "--confirm")
	if code != 0 || !strings.Contains(stdout, "true") || !strings.Contains(stderr, "warning: the deploy service instances were not all renewed") ||
		!strings.Contains(stderr, "refresh-instances") {
		t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if !slices.Contains(*ran, "restart "+instanceOf(t, "bob").Unit()) {
		t.Errorf("ran %q", *ran)
	}
}

func TestRefreshInstancesCommand(t *testing.T) {
	root, ran := useHostRoot(t, 0)
	setUpInstances(t, root, "alice")
	code, stdout, stderr := run(t, "deploy-service", "refresh-instances", "--output", "json")
	if code != 0 || !strings.Contains(stdout, `"template_written":true`) && !strings.Contains(stdout, `"template_written": true`) {
		t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if !slices.Contains(*ran, "systemctl restart "+instanceOf(t, "alice").Unit()) {
		t.Errorf("ran %q", *ran)
	}
	// The update runs the command without --output.
	if code, stdout, stderr := run(t, "deploy-service", "refresh-instances"); code != 0 || !strings.Contains(stdout, instanceOf(t, "alice").Unit()) {
		t.Fatalf("default format: code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if code, _, _ := run(t, "help"); code != 0 {
		t.Fatal("help failed")
	}
}

func TestRefreshInstancesCommandNeedsRootAndIsHidden(t *testing.T) {
	root, ran := useHostRoot(t, 1000)
	setUpInstances(t, root, "alice")
	if code, _, stderr := run(t, "deploy-service", "refresh-instances"); code == 0 || !strings.Contains(stderr, "needs root") || len(*ran) != 0 {
		t.Fatalf("code %d, stderr %q, ran %v", code, stderr, *ran)
	}
	_, stdout, _ := run(t, "deploy-service", "--help")
	if strings.Contains(stdout, "refresh-instances") {
		t.Errorf("the internal command is listed: %q", stdout)
	}
}

func TestRefreshInstancesAdoptsTheSystemWindowBeforeTheRestart(t *testing.T) {
	root, _ := useHostRoot(t, 0)
	hostFile, state := useWindowPaths(t, root)
	setUpInstances(t, root, "alice")
	seedSystemWindow(t, state)
	var adoptedAtRestart bool
	deploySetup.host.Run = func(_ context.Context, name string, args ...string) error {
		if name == "systemctl" && len(args) > 0 && args[0] == "restart" {
			_, adoptedAtRestart, _ = window.ReadHost(hostFile)
		}
		return nil
	}
	if code, _, stderr := run(t, "deploy-service", "refresh-instances"); code != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if !adoptedAtRestart {
		t.Error("the host window did not exist when the instance restarted")
	}
	if w, ok, err := window.ReadHost(hostFile); err != nil || !ok || w.Start != 240 {
		t.Fatalf("host window = %+v, %v, %v", w, ok, err)
	}
}

func TestRefreshInstancesWarnsAndGoesOnWhenAdoptionFails(t *testing.T) {
	root, ran := useHostRoot(t, 0)
	hostFile, state := useWindowPaths(t, root)
	setUpInstances(t, root, "alice")
	seedSystemWindow(t, state)
	writeTestFile(t, hostFile, "not a window")
	code, _, stderr := run(t, "deploy-service", "refresh-instances")
	if code != 0 || !strings.Contains(stderr, "warning: the maintenance window of the system deploy service was not taken over") ||
		!slices.Contains(*ran, "systemctl restart "+instanceOf(t, "alice").Unit()) {
		t.Fatalf("code %d, stderr %q, ran %q", code, stderr, *ran)
	}
}

func TestRefreshInstancesWithoutInstanceLeavesTheHostWindowAlone(t *testing.T) {
	root, _ := useHostRoot(t, 0)
	hostFile, state := useWindowPaths(t, root)
	seedSystemWindow(t, state)
	if code, _, stderr := run(t, "deploy-service", "refresh-instances"); code != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if _, ok, _ := window.ReadHost(hostFile); ok {
		t.Error("the host window was written without an instance")
	}
}
