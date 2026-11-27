package nofollow

import (
	"errors"
	"io/fs"
	"math/rand/v2"
	"os"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// Info is what Lstat reports about a child.
type Info struct {
	Mode fs.FileMode // permission bits plus fs.ModeDir, fs.ModeSymlink or fs.ModeIrregular
	GID  uint32
}

// Lstat describes the child name itself, also when it is a link.
func (d *Dir) Lstat(name string) (Info, error) {
	if err := d.check("stat", name); err != nil {
		return Info{}, err
	}
	var st unix.Stat_t
	if err := unix.Fstatat(d.fd, name, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return Info{}, &Error{"stat", d.join(name), err}
	}
	mode := fs.FileMode(st.Mode & 0o777)
	switch st.Mode & unix.S_IFMT {
	case unix.S_IFREG:
	case unix.S_IFDIR:
		mode |= fs.ModeDir
	case unix.S_IFLNK:
		mode |= fs.ModeSymlink
	default:
		mode |= fs.ModeIrregular
	}
	return Info{Mode: mode, GID: st.Gid}, nil
}

// OpenFile opens the child file name with os flags. O_NOFOLLOW is always set, so a link as the file is refused
// with ErrLink.
func (d *Dir) OpenFile(name string, flag int, perm uint32) (*os.File, error) {
	if err := d.check("open", name); err != nil {
		return nil, err
	}
	fd, err := unix.Openat(d.fd, name, flag|unix.O_NOFOLLOW|unix.O_CLOEXEC, perm)
	if err != nil {
		return nil, openErr(d.fd, name, d.join(name), err)
	}
	return os.NewFile(uintptr(fd), d.join(name)), nil
}

// Create creates the child file name for writing and fails when the name exists, a link included.
func (d *Dir) Create(name string, perm uint32) (*os.File, error) {
	return d.OpenFile(name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL, perm)
}

// CreateTemp creates a new file for reading and writing; the first "*" of pattern is replaced by a random
// string. The rights are perm minus the umask, so a caller that needs exact bits calls Chmod on the file.
func (d *Dir) CreateTemp(pattern string, perm uint32) (*os.File, error) {
	prefix, suffix, _ := strings.Cut(pattern, "*")
	for range 100 {
		name := prefix + strconv.FormatUint(rand.Uint64(), 36) + suffix
		f, err := d.OpenFile(name, unix.O_RDWR|unix.O_CREAT|unix.O_EXCL, perm)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		return f, err
	}
	return nil, &Error{"create", d.join(pattern), fs.ErrExist}
}

// Rename moves the child from over the child to atomically. A link at to is refused with ErrLink and left
// alone, a stale name of a crash is replaced.
func (d *Dir) Rename(from, to string) error {
	if err := d.check("rename", from); err != nil {
		return err
	}
	info, err := d.Lstat(to)
	switch {
	case err == nil && info.Mode&fs.ModeSymlink != 0:
		return &Error{"rename", d.join(to), ErrLink}
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return err
	}
	if err := unix.Renameat(d.fd, from, d.fd, to); err != nil {
		return &Error{"rename", d.join(to), err}
	}
	return nil
}

// Remove deletes the child file or link name itself; a folder is refused.
func (d *Dir) Remove(name string) error {
	if err := d.check("remove", name); err != nil {
		return err
	}
	if err := unix.Unlinkat(d.fd, name, 0); err != nil {
		return &Error{"remove", d.join(name), err}
	}
	return nil
}

// Names lists the names of the children.
func (d *Dir) Names() ([]string, error) {
	fd, err := unix.Openat(d.fd, ".", dirFlags, 0)
	if err != nil {
		return nil, &Error{"read", d.path, err}
	}
	f := os.NewFile(uintptr(fd), d.path)
	defer f.Close()
	names, err := f.Readdirnames(-1)
	if err != nil {
		return nil, &Error{"read", d.path, err}
	}
	return names, nil
}

// RemoveAll deletes the child name with everything below it. A link is deleted itself and never entered.
// A missing name is no error.
func (d *Dir) RemoveAll(name string) error {
	err := d.Remove(name)
	if err == nil || errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if !errors.Is(err, unix.EISDIR) {
		return err
	}
	sub, err := d.Sub(name)
	if err != nil {
		return err
	}
	names, err := sub.Names()
	for _, n := range names {
		if err == nil {
			err = sub.RemoveAll(n)
		}
	}
	_ = sub.Close()
	if err != nil {
		return err
	}
	if err := unix.Unlinkat(d.fd, name, unix.AT_REMOVEDIR); err != nil {
		return &Error{"remove", d.join(name), err}
	}
	return nil
}
