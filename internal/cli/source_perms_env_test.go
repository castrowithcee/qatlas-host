package cli

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

const envGroup = 4242

// useEnvGroup makes the service account's group a fixed one and records group changes without making them, so
// the tests depend on no group of the user that runs them.
func useEnvGroup(t *testing.T) *hostDouble {
	t.Helper()
	d := useFolderHost(t)
	folders.owner = func(string) (int, int, error) { return os.Getuid(), envGroup, nil }
	folders.lchown = func(path string, uid, gid int) error {
		d.lchowns = append(d.lchowns, lchownCall{path, uid, gid})
		return nil
	}
	return d
}

// envInspect is a local repository whose service can write everything and cannot read the listed files.
func envInspect(files ...string) control.SourceInspectResult {
	res := localInspect(false)
	res.Access.UnreadableEnvFiles = files
	return res
}

func TestPlanAccessTakesOnlyTheUnreadableEnvFilesItCanTrust(t *testing.T) {
	useEnvGroup(t)
	tree := newPermTree(t)
	put(t, filepath.Join(tree.dir, "demo", "app.env"), 0o600)
	put(t, filepath.Join(tree.dir, "demo", "data", "mounted.env"), 0o600)
	put(t, filepath.Join(tree.dir, "demo", "other.txt"), 0o600)
	if err := os.Link(filepath.Join(tree.dir, "demo", "other.txt"), filepath.Join(tree.dir, "demo", "hard.env")); err != nil {
		t.Fatal(err)
	}
	res := envInspect("demo/secrets.env", "demo/app.env", "demo/link", "demo/hard.env", "demo/data/mounted.env",
		"../outside/target.txt", tree.dir+"/demo/secrets.env", "demo/missing.env")
	res.Path = tree.dir
	before := tree.snapshot(t)

	plan, err := planAccess(context.Background(), res)
	if err != nil {
		t.Fatal(err)
	}
	if !equalModes(before, tree.snapshot(t)) {
		t.Fatal("planning changed the tree")
	}
	var got []string
	for _, f := range plan.envFiles {
		got = append(got, strings.TrimPrefix(f, tree.dir+"/"))
	}
	if want := []string{"demo/app.env", "demo/secrets.env"}; !slices.Equal(got, want) || len(plan.dirs) != 0 || len(plan.files) != 0 {
		t.Fatalf("env files %q, dirs %q, files %q", got, plan.dirs, plan.files)
	}
	text := strings.Join(plan.summary(), "\n")
	for _, want := range []string{"2 unversioned env files", "gid 4242", "group read", "demo/secrets.env", "demo/app.env"} {
		if !strings.Contains(text, want) {
			t.Errorf("summary lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "paths get the group") || plan.changed() != 2 {
		t.Errorf("changed %d, summary:\n%s", plan.changed(), text)
	}
}

func TestPlanAccessWithoutUnreadableEnvFilesIsNil(t *testing.T) {
	useEnvGroup(t)
	tree := newPermTree(t)
	res := envInspect("demo/link")
	res.Path = tree.dir
	if plan, err := planAccess(context.Background(), res); err != nil || plan != nil {
		t.Fatalf("plan %+v, err %v", plan, err)
	}
}

func TestApplyEnvFilesSetsOnlyTheGroupAndGroupRead(t *testing.T) {
	d := useEnvGroup(t)
	tree := newPermTree(t)
	res := envInspect("demo/secrets.env")
	res.Path = tree.dir
	plan, err := planAccess(context.Background(), res)
	if err != nil || plan == nil {
		t.Fatalf("plan %+v, err %v", plan, err)
	}
	before := tree.snapshot(t)
	if err := d.applyPermissionsForTest(t, plan); err != nil {
		t.Fatal(err)
	}
	secrets := filepath.Join(tree.dir, "demo", "secrets.env")
	if len(d.lchowns) != 1 || d.lchowns[0] != (lchownCall{secrets, -1, envGroup}) {
		t.Fatalf("group changes = %+v", d.lchowns)
	}
	after := tree.snapshot(t)
	if after[secrets] != before[secrets]|0o040 {
		t.Fatalf("mode %v, was %v", after[secrets], before[secrets])
	}
	delete(before, secrets)
	delete(after, secrets)
	if !equalModes(before, after) {
		t.Fatal("something else changed")
	}
}

func TestApplyEnvFilesSkipsAFileThatBecameALink(t *testing.T) {
	d := useEnvGroup(t)
	tree := newPermTree(t)
	res := envInspect("demo/secrets.env")
	res.Path = tree.dir
	plan, err := planAccess(context.Background(), res)
	if err != nil || plan == nil {
		t.Fatalf("plan %+v, err %v", plan, err)
	}
	secrets := filepath.Join(tree.dir, "demo", "secrets.env")
	if err := os.Remove(secrets); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(tree.outside, secrets); err != nil {
		t.Fatal(err)
	}
	outsideBefore := modeOf(t, tree.outside)
	if err := d.applyPermissionsForTest(t, plan); err != nil {
		t.Fatal(err)
	}
	if len(d.lchowns) != 0 || modeOf(t, tree.outside) != outsideBefore || modeOf(t, tree.outside)&fs.ModePerm != 0o600 {
		t.Fatalf("group changes %+v, target mode %v", d.lchowns, modeOf(t, tree.outside))
	}
}

func TestSourceAddPlansEnvFilesWhenTheServiceCanWriteTheFolder(t *testing.T) {
	d := useEnvGroup(t)
	tree := newPermTree(t)
	svc := startFakeService(t, addHandlers(envInspect("demo/secrets.env")))
	useStdin(t, strings.NewReader(""))

	code, _, stderr := run(t, "source", "add", tree.dir)
	if code != 2 || !strings.Contains(stderr, "1 unversioned env files") || !strings.Contains(stderr, "demo/secrets.env") ||
		strings.Contains(stderr, "paths get the group") || len(d.lchowns) != 0 || svc.count(control.SourceAddCommand) != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	code, _, stderr = run(t, "source", "add", tree.dir, "--yes")
	secrets := filepath.Join(tree.dir, "demo", "secrets.env")
	if code != 0 || len(d.lchowns) != 1 || d.lchowns[0] != (lchownCall{secrets, -1, envGroup}) || modeOf(t, secrets).Perm() != 0o640 {
		t.Fatalf("code %d, stderr %q, group changes %+v, mode %v", code, stderr, d.lchowns, modeOf(t, secrets))
	}
	if modeOf(t, tree.dir).Perm() != 0o750 {
		t.Fatal("the folder changed although the service can write it")
	}
}

func TestSourceEffectCoversTheEnvFiles(t *testing.T) {
	useEnvGroup(t)
	tree := newPermTree(t)
	put(t, filepath.Join(tree.dir, "demo", "app.env"), 0o600)
	effect := func(files ...string) string {
		res := envInspect(files...)
		res.Path = tree.dir
		perms, err := planAccess(context.Background(), res)
		if err != nil {
			t.Fatal(err)
		}
		e, err := newSourceEffect(res, perms)
		if err != nil {
			t.Fatal(err)
		}
		return e.fingerprint
	}
	none, one, two := effect(), effect("demo/secrets.env"), effect("demo/secrets.env", "demo/app.env")
	if none == one || one == two || none == two || one != effect("demo/secrets.env") {
		t.Fatalf("fingerprints none %s, one %s, two %s", none, one, two)
	}
}
