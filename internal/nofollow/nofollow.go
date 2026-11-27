// Package nofollow opens and changes files below a folder without following a link on any level. Stack
// folders hold content from commits, links included, and a service that runs with more rights than the
// content's author must never write through one.
//
// Every level is opened with openat and O_NOFOLLOW|O_DIRECTORY, so a link on the way is refused before any
// change, also when it is swapped in between two calls. The links above the root passed to Open are
// followed: only the root itself and everything below it are checked. openat2 is not used because the
// level-by-level walk gives the same guarantee on every kernel the package runs on.
package nofollow

import (
	"errors"
	"path/filepath"
	"strings"
	"unicode"

	"golang.org/x/sys/unix"
)

// ErrLink is the cause of an Error when a link is on the way or is the target itself.
var ErrLink = errors.New("is a link, and links are not followed")

// ErrBadPath is the cause of an Error for a path that is not absolute and clean, or a name that is not one
// plain path element.
var ErrBadPath = errors.New("is not a plain path")

// Error names the path an operation failed on. Paths come from foreign content, so Error masks the
// characters a terminal would act on; Path keeps the original.
type Error struct {
	Op, Path string
	Err      error
}

func (e *Error) Error() string {
	return e.Op + " " + strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) {
			return r
		}
		return '?'
	}, e.Path) + ": " + e.Err.Error()
}

func (e *Error) Unwrap() error { return e.Err }

// Dir is an open folder. Its methods act on direct children by name and never follow a link.
type Dir struct {
	fd   int
	path string
}

const dirFlags = unix.O_RDONLY | unix.O_DIRECTORY | unix.O_NOFOLLOW | unix.O_CLOEXEC

// Open opens root, an absolute clean path, and below it the slash-separated relative path rel (empty for
// root itself). The root and every level of rel must be a folder and not a link.
func Open(root, rel string) (*Dir, error) {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return nil, &Error{"open", root, ErrBadPath}
	}
	fd, err := unix.Open(root, dirFlags, 0)
	if err != nil {
		return nil, openErr(unix.AT_FDCWD, root, root, err)
	}
	cur := &Dir{fd, root}
	if rel == "" {
		return cur, nil
	}
	for _, el := range strings.Split(rel, "/") {
		next, err := cur.Sub(el)
		_ = cur.Close()
		if err != nil {
			return nil, err
		}
		cur = next
	}
	return cur, nil
}

// Sub opens the child folder name.
func (d *Dir) Sub(name string) (*Dir, error) {
	if err := d.check("open", name); err != nil {
		return nil, err
	}
	fd, err := unix.Openat(d.fd, name, dirFlags, 0)
	if err != nil {
		return nil, openErr(d.fd, name, d.join(name), err)
	}
	return &Dir{fd, d.join(name)}, nil
}

// MkdirSub creates the child folder name when it is missing and opens it. A link in its place is refused.
func (d *Dir) MkdirSub(name string, perm uint32) (*Dir, error) {
	if err := d.check("create", name); err != nil {
		return nil, err
	}
	if err := unix.Mkdirat(d.fd, name, perm); err != nil && !errors.Is(err, unix.EEXIST) {
		return nil, &Error{"create", d.join(name), err}
	}
	return d.Sub(name)
}

// Path returns the path the folder was opened by.
func (d *Dir) Path() string { return d.path }

// Sync flushes the folder entries to disk.
func (d *Dir) Sync() error {
	if err := unix.Fsync(d.fd); err != nil {
		return &Error{"sync", d.path, err}
	}
	return nil
}

// Close closes the folder.
func (d *Dir) Close() error {
	if err := unix.Close(d.fd); err != nil {
		return &Error{"close", d.path, err}
	}
	return nil
}

func (d *Dir) join(name string) string { return filepath.Join(d.path, name) }

// check refuses a name that is not one plain path element.
func (d *Dir) check(op, name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsRune(name, '/') || strings.ContainsRune(name, 0) {
		return &Error{op, d.path + "/" + name, ErrBadPath}
	}
	return nil
}

// openErr turns the failure of an open into ErrLink when a link caused it.
func openErr(dirfd int, name, full string, err error) error {
	if errors.Is(err, unix.ELOOP) || errors.Is(err, unix.ENOTDIR) || errors.Is(err, unix.EEXIST) {
		var st unix.Stat_t
		if unix.Fstatat(dirfd, name, &st, unix.AT_SYMLINK_NOFOLLOW) == nil && st.Mode&unix.S_IFMT == unix.S_IFLNK {
			return &Error{"open", full, ErrLink}
		}
	}
	return &Error{"open", full, err}
}
