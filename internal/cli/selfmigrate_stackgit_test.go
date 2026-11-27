package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/deploysetup"
	"github.com/castrowithcee/qatlas-host/internal/gitrepo"
)

// useStackGitHost sets up below a temporary root a host whose deploy service runs as root already, with the group
// file groups and the local sources dirs bound to the system engine. The user running the test stands in for root.
func useStackGitHost(t *testing.T, groups string, dirs ...string) (root string, d *hostDouble) {
	t.Helper()
	// /var/tmp, like the other folder tests: /tmp may be a repository of its own on the dev host.
	root, err := os.MkdirTemp("/var/tmp", "qhost-stackgit-")
	if err != nil {
		t.Skip("no /var/tmp:", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	writeTestFile(t, filepath.Join(root, "etc/passwd"), "root:x:0:0::/root:/bin/bash\n")
	writeTestFile(t, filepath.Join(root, "etc/group"), groups)
	writeTestFile(t, filepath.Join(root, "var/lib/qhost-deploy/sources.json"), sourcesJSON(dirs...))
	d = useFolderHost(t)
	old := deploySetup
	deploySetup = deploySetupHost{euid: func() int { return 0 }, host: deploysetup.Host{Root: root}}
	t.Cleanup(func() { deploySetup = old })
	return root, d
}

func sourcesJSON(dirs ...string) string {
	var list []string
	for _, dir := range dirs {
		list = append(list, fmt.Sprintf(`{"id":%q,"path":%q,"kind":"local","url":"","key":""}`,
			strings.ReplaceAll(strings.TrimPrefix(dir, "/"), "/", "-"), dir))
	}
	return `{"version":2,"sources":[` + strings.Join(list, ",") + "]}\n"
}

// oldStackFolder makes a repository at path the way the releases before the root service left a stack folder:
// core.sharedRepository=group, group write on every entry of .git and setgid on its folders; the group stays the one
// of the user running the test, which the group file of the host names.
func oldStackFolder(t *testing.T, path string) {
	t.Helper()
	// Nothing writable for others, whatever the umask of the user running the test.
	defer syscall.Umask(syscall.Umask(0o022))
	put(t, filepath.Join(path, "demo", "compose.yaml"), 0o644)
	gitIn(t, path, "init", "-q")
	gitIn(t, path, "add", "demo/compose.yaml")
	gitIn(t, path, "-c", "user.name=Invented", "-c", "user.email=invented@example.invalid", "commit", "-q", "-m", "add demo")
	gitIn(t, path, "config", "core.sharedRepository", "group")
	err := filepath.WalkDir(filepath.Join(path, ".git"), func(p string, e fs.DirEntry, err error) error {
		if err != nil || e.Type()&fs.ModeSymlink != 0 {
			return err
		}
		mode := modeOf(t, p).Perm() | 0o020
		if e.IsDir() {
			mode |= fs.ModeSetgid
		}
		return os.Chmod(p, mode)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// gitModes records the mode of every entry of .git of path and its core.sharedRepository.
func gitModes(t *testing.T, path string) map[string]fs.FileMode {
	t.Helper()
	modes := map[string]fs.FileMode{}
	_ = filepath.WalkDir(filepath.Join(path, ".git"), func(p string, _ fs.DirEntry, err error) error {
		if err == nil {
			info, _ := os.Lstat(p)
			modes[p] = info.Mode()
		}
		return nil
	})
	if out, err := gitOut(path, "config", "--local", "--get", "core.sharedRepository"); err == nil {
		modes["core.sharedRepository="+strings.TrimSpace(out)] = 0
	}
	return modes
}

func requireUnchanged(t *testing.T, path string, before map[string]fs.FileMode) {
	t.Helper()
	after := gitModes(t, path)
	if len(after) != len(before) {
		t.Fatalf(".git of %s changed: %d entries, before %d", path, len(after), len(before))
	}
	for p, m := range before {
		if after[p] != m {
			t.Fatalf("%s changed: %v, before %v", p, after[p], m)
		}
	}
}

// requireRootOnly checks that .git of path is root-only for the user standing in for root, without setgid and
// core.sharedRepository, and that the service would run git in it.
func requireRootOnly(t *testing.T, path string) {
	t.Helper()
	_ = filepath.WalkDir(filepath.Join(path, ".git"), func(p string, _ fs.DirEntry, err error) error {
		if err == nil && (modeOf(t, p).Perm()&0o022 != 0 || modeOf(t, p)&fs.ModeSetgid != 0) {
			t.Fatalf("%s is still %v", p, modeOf(t, p))
		}
		return nil
	})
	if out, err := gitOut(path, "config", "--local", "--get", "core.sharedRepository"); err == nil {
		t.Fatalf("core.sharedRepository is still set: %q", out)
	}
	if err := gitrepo.CheckGitDir(path, uint32(os.Getuid())); err != nil {
		t.Fatal(err)
	}
}

func TestDeployServicePrepareSecuresTheGitThatEarlierReleasesSharedAndARepeatChangesNothing(t *testing.T) {
	gid := os.Getgid()
	for name, groups := range map[string]string{
		"the id the removed group left": fmt.Sprintf("qhost:x:%d:\n", gid+1),
		"the group qhost-deploy":        fmt.Sprintf("qhost-deploy:x:%d:\n", gid),
		"the group root":                fmt.Sprintf("root:x:%d:\n", gid),
	} {
		t.Run(name, func(t *testing.T) {
			root, d := useStackGitHost(t, groups, "/srv/old")
			path := filepath.Join(root, "srv/old")
			oldStackFolder(t, path)
			code, _, stderr := run(t, "deploy-service", "prepare")
			if code != 0 || !strings.Contains(stderr, "transition stack-git-root-only: applied\n") ||
				!strings.Contains(stderr, `stack-git-root-only: made .git of "/srv/old" root-only`) {
				t.Fatalf("code %d, stderr %q", code, stderr)
			}
			requireRootOnly(t, path)
			n := len(d.lchowns)
			code, _, stderr = run(t, "deploy-service", "prepare")
			if code != 0 || !strings.Contains(stderr, "transition stack-git-root-only: already done\n") ||
				strings.Contains(stderr, "made .git") || len(d.lchowns) != n {
				t.Fatalf("repeat: code %d, stderr %q", code, stderr)
			}
		})
	}
}

// useUnsecurableFolders sets up bound stack folders that carry core.sharedRepository=group but cannot be secured,
// one that can (/srv/good), and folders that are not looked at. It returns the root, the target of the links and the
// modes of .git of every folder that must stay as it is.
func useUnsecurableFolders(t *testing.T) (root, target string, before map[string]map[string]fs.FileMode) {
	t.Helper()
	root, _ = useStackGitHost(t, fmt.Sprintf("qhost:x:%d:\n", os.Getgid()+1), "/srv/inner-link", "/srv/git-link",
		"/srv/hard-link", "/srv/others", "/srv/plain", "/srv/missing", "/srv/folder-link", "/srv/good")
	at := func(dir string) string { return filepath.Join(root, dir) }
	target = at("outside/target")
	put(t, target, 0o600)
	kept := []string{"/srv/inner-link", "/srv/hard-link", "/srv/others", "/srv/plain", "/elsewhere/repo", "/srv/unbound"}
	for _, dir := range append(kept, "/srv/good") {
		oldStackFolder(t, at(dir))
	}
	// A .git made group-writable without the marker, as by hand after source secure.
	gitIn(t, at("srv/plain"), "config", "--unset", "core.sharedRepository")
	put(t, at("srv/git-link/compose.yaml"), 0o644)
	for _, err := range []error{
		os.Symlink(target, at("srv/inner-link/.git/hooks/pre-commit")),
		os.Link(target, at("srv/hard-link/.git/description.linked")),
		os.Chmod(at("srv/others/.git/description"), 0o666),
		// .git as a link to a repository with the marker, and a bound folder that is a link to one.
		os.Symlink(at("elsewhere/repo/.git"), at("srv/git-link/.git")),
		os.Symlink(at("elsewhere/repo"), at("srv/folder-link")),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	before = map[string]map[string]fs.FileMode{}
	for _, dir := range kept {
		before[dir] = gitModes(t, at(dir))
	}
	return root, target, before
}

// requireLeftReported checks that the start step reported each folder it could not secure with the reason and the
// next step, and none of the folders it must not look at.
func requireLeftReported(t *testing.T, stderr string) {
	t.Helper()
	for dir, why := range map[string]string{"/srv/inner-link": "is a symbolic link", "/srv/git-link": "not a directory",
		"/srv/hard-link": "has further hard links", "/srv/others": "can be written by others"} {
		line := lineWith(stderr, fmt.Sprintf("stack-git-root-only: left .git of %q as it is: ", dir))
		if !strings.Contains(line, why) || !strings.HasSuffix(line, "; run 'sudo qhost source secure "+dir+"'") {
			t.Errorf("%s is not reported with %q and the next step: %q", dir, why, stderr)
		}
	}
	for _, dir := range []string{"/srv/plain", "/srv/missing", "/srv/folder-link", "/srv/unbound"} {
		if strings.Contains(stderr, fmt.Sprintf("%q", dir)) {
			t.Errorf("%s is reported: %q", dir, stderr)
		}
	}
}

func TestDeployServicePrepareLeavesAGitItCannotSecureAndStartsAnyway(t *testing.T) {
	root, target, before := useUnsecurableFolders(t)
	for i, state := range []string{"applied", "already done"} {
		code, _, stderr := run(t, "deploy-service", "prepare")
		if code != 0 || !strings.Contains(stderr, "transition stack-git-root-only: "+state+"\n") {
			t.Fatalf("run %d: code %d, want the transition %s: %q", i, code, state, stderr)
		}
		requireLeftReported(t, stderr)
	}
	requireRootOnly(t, filepath.Join(root, "srv/good"))
	for dir, modes := range before {
		requireUnchanged(t, filepath.Join(root, dir), modes)
	}
	if modeOf(t, target).Perm() != 0o600 {
		t.Fatal("the target of a link changed")
	}
}

func lineWith(text, prefix string) string {
	for _, l := range strings.Split(text, "\n") {
		if strings.HasPrefix(l, prefix) {
			return l
		}
	}
	return ""
}

func TestDeployServicePrepareLeavesAGitWithAnotherExistingGroup(t *testing.T) {
	root, d := useStackGitHost(t, fmt.Sprintf("staff:x:%d:\n", os.Getgid()), "/srv/old")
	path := filepath.Join(root, "srv/old")
	oldStackFolder(t, path)
	before := gitModes(t, path)
	code, _, stderr := run(t, "deploy-service", "prepare")
	if code != 0 || !strings.Contains(stderr, `left .git of "/srv/old" as it is: entries have the group staff, which is neither root nor qhost-deploy`) ||
		!strings.Contains(stderr, "transition stack-git-root-only: already done\n") || len(d.lchowns) != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	requireUnchanged(t, path, before)
}

func TestDeployServicePrepareSecuresTheNextFolderAfterAFailureAndTheNextStartTheRest(t *testing.T) {
	root, _ := useStackGitHost(t, fmt.Sprintf("qhost:x:%d:\n", os.Getgid()+1), "/srv/a", "/srv/b")
	for _, dir := range []string{"srv/a", "srv/b"} {
		oldStackFolder(t, filepath.Join(root, dir))
	}
	lchown := folders.lchown
	folders.lchown = func(path string, uid, gid int) error {
		if strings.HasPrefix(path, filepath.Join(root, "srv/a")+"/") {
			return errors.New("invented failure")
		}
		return lchown(path, uid, gid)
	}
	code, _, stderr := run(t, "deploy-service", "prepare")
	if code != 0 || !strings.Contains(stderr, `could not make .git of "/srv/a" root-only: cannot change the owner of`) ||
		!strings.Contains(stderr, "invented failure; run 'sudo qhost source secure /srv/a'") ||
		!strings.Contains(stderr, `made .git of "/srv/b" root-only`) {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	requireRootOnly(t, filepath.Join(root, "srv/b"))
	folders.lchown = lchown
	code, _, stderr = run(t, "deploy-service", "prepare")
	if code != 0 || !strings.Contains(stderr, `made .git of "/srv/a" root-only`) || strings.Contains(stderr, `"/srv/b"`) {
		t.Fatalf("next start: code %d, stderr %q", code, stderr)
	}
	requireRootOnly(t, filepath.Join(root, "srv/a"))
}

func TestDeployServicePrepareLeavesTheFoldersOfARootlessInstance(t *testing.T) {
	root, d := useStackGitHost(t, fmt.Sprintf("qhost:x:%d:\n", os.Getgid()+1))
	inst, err := deploysetup.NewInstance("alice")
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(root, inst.StateDir(), "sources.json"), sourcesJSON("/home/alice/stacks"))
	path := filepath.Join(root, "home/alice/stacks")
	oldStackFolder(t, path)
	before := gitModes(t, path)
	code, _, stderr := run(t, "deploy-service", "prepare")
	if code != 0 || !strings.Contains(stderr, "transition stack-git-root-only: already done\n") || strings.Contains(stderr, "alice") ||
		len(d.lchowns) != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	requireUnchanged(t, path, before)
}

func TestGitUnsafeInStatusFieldsNamesTheNextStepForTheSystemEngine(t *testing.T) {
	unsafe := control.EngineStatus{Sources: []control.SourceStatus{
		{ID: "srv-old", Path: "/srv/old", Kind: "local", GitUnsafe: ".git can be written by a group or by others"},
		{ID: "srv-ok", Path: "/srv/ok", Kind: "local"},
	}}
	got := map[string]any{}
	for _, f := range engineFields(control.EngineSystem, unsafe) {
		got[f.Name] = f.Value
	}
	if got["source.srv-old.git_unsafe_fix"] != "sudo qhost source secure /srv/old" {
		t.Errorf("git_unsafe_fix = %v", got["source.srv-old.git_unsafe_fix"])
	}
	if _, ok := got["source.srv-ok.git_unsafe_fix"]; ok {
		t.Error("a safe folder has a next step")
	}
	for _, f := range entryFields("instance.alice.", "rootless:alice", unsafe) {
		if strings.HasSuffix(f.Name, "git_unsafe_fix") {
			t.Errorf("an instance names source secure: %s", f.Name)
		}
	}
}
