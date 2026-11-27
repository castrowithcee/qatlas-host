package cli

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// shareGit makes the .git of the tree the way a group-shared repository leaves it: group write everywhere and
// core.sharedRepository=group.
func shareGit(t *testing.T, tree permTree) {
	t.Helper()
	gitIn(t, tree.dir, "config", "core.sharedRepository", "group")
	err := filepath.WalkDir(filepath.Join(tree.dir, ".git"), func(path string, d fs.DirEntry, err error) error {
		if err == nil && d.Type()&fs.ModeSymlink == 0 {
			err = os.Chmod(path, modeOf(t, path).Perm()|0o020)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// gitWritableByOthers lists the entries of .git that group or others can write.
func gitWritableByOthers(t *testing.T, tree permTree) []string {
	t.Helper()
	var bad []string
	_ = filepath.WalkDir(filepath.Join(tree.dir, ".git"), func(path string, d fs.DirEntry, err error) error {
		if err == nil && modeOf(t, path).Perm()&0o022 != 0 {
			bad = append(bad, path)
		}
		return nil
	})
	return bad
}

// requireSecured checks that .git is root-only (as the test user), without core.sharedRepository, and the work tree untouched.
func requireSecured(t *testing.T, tree permTree, before map[string]fs.FileMode) {
	t.Helper()
	if bad := gitWritableByOthers(t, tree); len(bad) != 0 {
		t.Fatalf("still writable for group or others: %v", bad)
	}
	if out, err := gitOut(tree.dir, "config", "--local", "--get", "core.sharedRepository"); err == nil {
		t.Fatalf("core.sharedRepository is still set: %q", out)
	}
	if !equalModes(before, tree.snapshot(t)) {
		t.Fatal("the work tree changed")
	}
}

func secureHandlers(bound ...string) fakeHandlers {
	return fakeHandlers{control.RepoFetchCommand: func(json.RawMessage) any { return control.RepoFetchResult{} },
		control.StatusCommand: statusWith(bound)}
}

func TestSourceAddTakesAGroupSharedGitForRootOnlyOfTheSystemEngine(t *testing.T) {
	d := useFolderHost(t)
	tree := newPermTree(t)
	shareGit(t, tree)
	svc := startFakeService(t, addHandlers(localInspect(true)))
	useStdin(t, strings.NewReader(""))
	before := tree.snapshot(t)

	code, _, stderr := run(t, "source", "add", tree.dir)
	if code != 2 || !strings.Contains(stderr, "confirmation-required") || !strings.Contains(stderr, "core.sharedRepository \"group\" is removed") ||
		!strings.Contains(stderr, "only root can change it") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if len(d.lchowns) != 0 || gitConfigIn(t, tree.dir, "core.sharedRepository") != "group" || len(gitWritableByOthers(t, tree)) == 0 {
		t.Fatal("the preview changed .git")
	}

	code, _, stderr = run(t, "source", "add", tree.dir, "--yes")
	if code != 0 || svc.count(control.SourceAddCommand) != 1 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	requireSecured(t, tree, before)
	for _, call := range d.lchowns {
		if !strings.HasPrefix(call.path, filepath.Join(tree.dir, ".git")) || call.uid != gitOwner.uid || call.gid != gitOwner.gid {
			t.Fatalf("unexpected owner change %+v", call)
		}
	}
}

func TestSourceSecureMakesGitRootOnlyAndFetches(t *testing.T) {
	d := useFolderHost(t)
	tree := newPermTree(t)
	shareGit(t, tree)
	svc := startFakeService(t, secureHandlers(tree.dir))
	useStdin(t, strings.NewReader(""))
	before := tree.snapshot(t)

	code, stdout, stderr := run(t, "source", "secure", tree.dir)
	if code != 2 || !strings.Contains(stderr, "confirmation-required") || stdout != "" || len(d.lchowns) != 0 || svc.count(control.RepoFetchCommand) != 0 ||
		len(gitWritableByOthers(t, tree)) == 0 {
		t.Fatalf("without confirmation: code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	for _, want := range []string{"Make .git of", "owner and group root", "no write for group and others", "core.sharedRepository \"group\" is removed", "\".git/config\""} {
		if !strings.Contains(stderr, want) {
			t.Errorf("preview lacks %q:\n%s", want, stderr)
		}
	}

	code, stdout, stderr = run(t, "source", "secure", tree.dir, "--yes", "--output", "json")
	if code != 0 || !strings.Contains(stdout, `"path":"`+tree.dir+`"`) || svc.count(control.RepoFetchCommand) != 1 {
		t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	requireSecured(t, tree, before)
	// A second run finds nothing to do.
	code, _, stderr = run(t, "source", "secure", tree.dir, "--yes")
	if code != 0 || !strings.Contains(stderr, "root-only already") {
		t.Fatalf("second run: code %d, stderr %q", code, stderr)
	}
}

func TestSourceSecureOfAnUnboundFolderDoesNotFetch(t *testing.T) {
	d := useFolderHost(t)
	tree := newPermTree(t)
	shareGit(t, tree)
	svc := startFakeService(t, secureHandlers())
	useStdin(t, strings.NewReader(""))
	before := tree.snapshot(t)

	code, _, stderr := run(t, "source", "secure", tree.dir, "--yes")
	if code != 0 || svc.count(control.RepoFetchCommand) != 0 || len(d.lchowns) == 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	requireSecured(t, tree, before)
}

func TestSourceSecureRefusesWhatItCannotRepairAndChangesNothing(t *testing.T) {
	d := useFolderHost(t)
	tree := newPermTree(t)
	svc := startFakeService(t, secureHandlers())
	useStdin(t, strings.NewReader(""))

	linked := filepath.Join(filepath.Dir(tree.dir), "linked")
	if err := os.Mkdir(linked, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(tree.outside, filepath.Join(tree.dir, ".git", "hooks", "pre-commit")); err != nil {
		t.Fatal(err)
	}
	shareGit(t, tree)
	for _, c := range []struct {
		name  string
		setup func()
	}{
		{"a link inside .git", func() {}},
		{".git as a link", func() {
			if err := os.Rename(filepath.Join(tree.dir, ".git"), filepath.Join(linked, "real")); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(linked, "real"), filepath.Join(tree.dir, ".git")); err != nil {
				t.Fatal(err)
			}
		}},
		{".git as a file", func() {
			_ = os.Remove(filepath.Join(tree.dir, ".git"))
			put(t, filepath.Join(tree.dir, ".git"), 0o666)
		}},
	} {
		name := c.name
		c.setup()
		code, stdout, stderr := run(t, "source", "secure", tree.dir, "--yes")
		if code == 0 || stdout != "" || len(d.lchowns) != 0 || svc.count(control.RepoFetchCommand) != 0 || (!strings.Contains(stderr, "nothing was changed") &&
			!strings.Contains(stderr, "not a directory")) {
			t.Fatalf("%s: code %d, stdout %q, stderr %q", name, code, stdout, stderr)
		}
	}
	if modeOf(t, tree.outside).Perm() != 0o600 {
		t.Fatal("the target of a link changed")
	}
}

func TestSourceSecureRefusesARootlessEngineAndNeedsRoot(t *testing.T) {
	d := useFolderHost(t)
	d.useRootless(t)
	tree := newPermTree(t)
	shareGit(t, tree)
	svc := startFakeService(t, secureHandlers())

	code, _, stderr := run(t, "source", "secure", tree.dir, "--engine", "rootless:alice", "--yes")
	if code == 0 || !strings.Contains(stderr, "only for stack folders of the system engine") || len(d.lchowns) != 0 || svc.count(control.RepoFetchCommand) != 0 {
		t.Fatalf("rootless: code %d, stderr %q", code, stderr)
	}
	folders.euid = func() int { return 1000 }
	code, _, stderr = run(t, "source", "secure", tree.dir, "--yes")
	if code == 0 || !strings.Contains(stderr, "needs root") || len(d.lchowns) != 0 {
		t.Fatalf("not root: code %d, stderr %q", code, stderr)
	}
}

func TestSourceAddOfARootlessEngineKeepsGroupSharedGit(t *testing.T) {
	d := useFolderHost(t)
	d.useRootless(t)
	tree := newPermTree(t)
	startFakeService(t, addHandlers(localInspect(true)))
	useStdin(t, strings.NewReader(""))
	code, _, stderr := run(t, "source", "add", tree.dir, "--yes")
	if code != 0 || strings.Contains(stderr, "only root can change it") || gitConfigIn(t, tree.dir, "core.sharedRepository") != "group" {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if modeOf(t, filepath.Join(tree.dir, ".git", "HEAD")).Perm()&0o020 == 0 {
		t.Fatal(".git lost its group write")
	}
}

func gitOut(dir string, args ...string) (string, error) {
	out, err := runGit(context.Background(), dir, args...)
	return string(out), err
}

func TestSourceAddSecuresAnUnsafeGitBeforeTheInspection(t *testing.T) {
	d := useFolderHost(t)
	tree := newPermTree(t)
	shareGit(t, tree)
	svc := startFakeService(t, addHandlers(localInspect(true)))
	useStdin(t, strings.NewReader(""))
	before := tree.snapshot(t)

	code, _, stderr := run(t, "source", "add", tree.dir)
	if code != 2 || !strings.Contains(stderr, "confirmation-required") || !strings.Contains(stderr, "is not root-only") ||
		!strings.Contains(stderr, "core.sharedRepository \"group\" is removed") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if svc.count(control.SourceInspectCommand) != 0 || len(d.lchowns) != 0 || gitConfigIn(t, tree.dir, "core.sharedRepository") != "group" {
		t.Fatal("without confirmation something happened")
	}

	code, _, stderr = run(t, "source", "add", tree.dir, "--yes")
	if code != 0 || svc.count(control.SourceAddCommand) != 1 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	requireSecured(t, tree, before)
}

func TestSourceAddWithExpectedEffectRefusesAnUnsafeGit(t *testing.T) {
	d := useFolderHost(t)
	tree := newPermTree(t)
	shareGit(t, tree)
	svc := startFakeService(t, addHandlers(localInspect(true)))
	code, _, stderr := run(t, "source", "add", tree.dir, "--yes", "--expect-effect", "sha256:x")
	if code == 0 || !strings.Contains(stderr, "sudo qhost source secure "+tree.dir) || len(d.lchowns) != 0 || svc.count(control.SourceInspectCommand) != 0 ||
		gitConfigIn(t, tree.dir, "core.sharedRepository") != "group" {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
}

func TestSourceAddRefusesAGitItCannotSecureAndChangesNothing(t *testing.T) {
	d := useFolderHost(t)
	tree := newPermTree(t)
	if err := os.Symlink(tree.outside, filepath.Join(tree.dir, ".git", "hooks", "pre-commit")); err != nil {
		t.Fatal(err)
	}
	shareGit(t, tree)
	svc := startFakeService(t, addHandlers(localInspect(true)))
	code, _, stderr := run(t, "source", "add", tree.dir, "--yes")
	if code == 0 || !strings.Contains(stderr, "nothing was changed") || len(d.lchowns) != 0 || svc.count(control.SourceInspectCommand) != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
}

func TestSourceInspectOfAnUnsafeGitNamesSecure(t *testing.T) {
	tree := newPermTree(t)
	useFolderHost(t)
	shareGit(t, tree)
	rejected := control.SourceInspectResult{Case: control.SourceCaseReject, Reason: "git cannot read the repository in the folder", Stacks: []control.SourceStack{}}
	startFakeService(t, addHandlers(rejected))
	code, stdout, stderr := run(t, "source", "inspect", tree.dir, "--output", "json")
	if code != 0 || !strings.Contains(stdout, "sudo qhost source secure "+tree.dir) {
		t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}
