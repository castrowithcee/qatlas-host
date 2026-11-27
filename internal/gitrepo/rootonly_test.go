package gitrepo

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// asRootService makes the package act as the deploy service of the system engine, with a umask that would leave
// group and others write bits on everything git creates; the current user stands in for root.
func asRootService(t *testing.T) {
	t.Helper()
	withSharedInit(t, false)
	enforceGitSafe = func() bool { return true }
	gitOwnerUID = func() uint32 { return uint32(os.Getuid()) }
	old := syscall.Umask(0)
	t.Cleanup(func() {
		syscall.Umask(old)
		enforceGitSafe = func() bool { return false }
		gitOwnerUID = func() uint32 { return 0 }
	})
}

func requireRootOnly(t *testing.T, r *Repo) {
	t.Helper()
	err := filepath.WalkDir(filepath.Join(r.dir, ".git"), func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err == nil && (info.Mode().Perm()&0o022 != 0 || info.Mode()&fs.ModeSetgid != 0) {
			t.Errorf("%s is %v", p, info.Mode())
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.CheckGit(); err != nil {
		t.Fatalf("the check refuses the new repository: %v", err)
	}
}

func TestInitAsTheRootServiceLeavesNothingWritableForGroupOrOthers(t *testing.T) {
	asRootService(t)
	dir := initFolder(t, map[string]string{"compose.yaml": "services: {}\n"})
	if _, err := Init(context.Background(), Config{Local: true}, dir, []string{"compose.yaml"}, nil); err != nil {
		t.Fatal(err)
	}
	r, err := Open(Config{Local: true}, dir)
	if err != nil {
		t.Fatal(err)
	}
	requireRootOnly(t, r)
}

func TestCloneAsTheRootServiceLeavesNothingWritableForGroupOrOthers(t *testing.T) {
	asRootService(t)
	r, _ := cloneFixture(t, newRemote(t))
	requireRootOnly(t, r)
}
