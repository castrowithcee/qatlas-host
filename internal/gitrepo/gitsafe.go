package gitrepo

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unicode"
)

// ErrGitUnsafe means the folder's .git could be changed by someone other than root, so the service of the
// system engine, which runs as root, does not run git in it: git would run code from that .git as root.
var ErrGitUnsafe = errors.New("git is not run in this folder because its .git is not safe")

// UnsafeError is ErrGitUnsafe with the reason in plain words; paths in it are masked.
type UnsafeError struct{ Reason string }

func (e *UnsafeError) Error() string { return ErrGitUnsafe.Error() + ": " + e.Reason }

func (e *UnsafeError) Is(target error) bool { return target == ErrGitUnsafe }

// UnsafeReason returns the reason of an error that wraps ErrGitUnsafe.
func UnsafeReason(err error) (string, bool) {
	var u *UnsafeError
	if errors.As(err, &u) {
		return u.Reason, true
	}
	return "", false
}

// enforceGitSafe says the service is the deploy service of the system engine, which runs as root. Only that
// service checks .git; a rootless instance runs as the user who owns its folders. It is the process's own
// identity, never a property of the folder, so a root command on a folder of a rootless instance, which runs
// in that instance's service, is not affected.
var enforceGitSafe = func() bool { return os.Geteuid() == 0 }

// gitOwnerUID is the user who must own .git and what is checked in it.
var gitOwnerUID = func() uint32 { return 0 }

// CheckGit reports ErrGitUnsafe if the folder's .git is one that root must not run git in. A folder without
// .git passes: git has nothing to read there.
func (r *Repo) CheckGit() error { return r.checkGit() }

// gitChecks are the entries of .git that git reads and that decide which code it runs; "" is .git itself.
var gitChecks = []struct {
	rel string
	dir bool
}{{"", true}, {"config", false}, {"config.worktree", false}, {"info", true}, {"info/attributes", false}, {"hooks", true}, {"worktrees", true}}

// checkGit is called before every git call. Nothing is followed: a link in the place of an entry is unsafe.
func (r *Repo) checkGit() error {
	if !enforceGitSafe() {
		return nil
	}
	gitDir := filepath.Join(r.dir, ".git")
	if _, err := os.Lstat(gitDir); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	for _, e := range gitChecks {
		if err := checkEntry(gitDir, e.rel, e.dir); err != nil {
			return err
		}
	}
	linked, err := os.ReadDir(filepath.Join(gitDir, "worktrees"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return &UnsafeError{".git/worktrees cannot be read"}
	}
	for _, w := range linked {
		if err := checkEntry(gitDir, "worktrees/"+w.Name()+"/config.worktree", false); err != nil {
			return err
		}
	}
	return nil
}

// checkEntry checks one entry of .git: owned by root, no link, a folder if dir, not writable for group or others.
// An entry that does not exist is fine, except .git itself.
func checkEntry(gitDir, rel string, dir bool) error {
	fi, err := os.Lstat(filepath.Join(gitDir, rel))
	switch {
	case errors.Is(err, fs.ErrNotExist) && rel != "":
		return nil
	case err != nil:
		return &UnsafeError{".git cannot be read"}
	}
	name := ".git"
	if rel != "" {
		name += "/" + maskName(rel)
	}
	st, _ := fi.Sys().(*syscall.Stat_t)
	switch {
	case fi.Mode()&fs.ModeSymlink != 0:
		return &UnsafeError{name + " is a link"}
	case dir && !fi.IsDir():
		return &UnsafeError{name + " is not a folder"}
	case st == nil || st.Uid != gitOwnerUID():
		return &UnsafeError{name + " is not owned by root"}
	case fi.Mode().Perm()&0o022 != 0:
		return &UnsafeError{name + " can be written by a group or by others"}
	}
	return nil
}

// maskName replaces the characters of a foreign name that a terminal would act on.
func maskName(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) {
			return r
		}
		return '?'
	}, s)
}
