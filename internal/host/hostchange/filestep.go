package hostchange

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// The file steps write where a container or another user may have planted a link, for example to
// /etc/shadow in a data folder of a stack. They open every folder on the way relative to the one before,
// starting at the root, and refuse a link or a non-folder at every level, the last entry included.
const (
	// stepFileWrite writes the content of its writeInput to the file Argv[1]: an existing regular file in
	// place, so that inode, owner, permissions, ACL, extended attributes and further hard links stay; a missing
	// one new, with the owner and group of its folder.
	stepFileWrite = "file-write"
	// stepFileRestore returns the file Argv[1] to the fileState on standard input.
	stepFileRestore = "file-restore"
	// stepFolderCreate creates the folder Argv[1] with the owner and group of its folder.
	stepFolderCreate = "folder-create"
	// stepFolderRemove removes the folder Argv[1] if it is empty.
	stepFolderRemove = "folder-remove"
)

var fileSteps = map[string]bool{
	stepFileWrite: true, stepFileRestore: true, stepFolderCreate: true, stepFolderRemove: true,
	stepFileRename: true, stepFileCopy: true, stepFileRemove: true, stepFileRemoveCopy: true, stepFileRestoreTree: true,
}

// fileOps is what a file step does that only root may do; tests replace it.
type fileOps struct {
	chown func(fd, uid, gid int) error
	// chownLink changes the owner of the link name in dir itself, which cannot be opened.
	chownLink func(dir int, name string, uid, gid int) error
}

var systemFileOps = fileOps{
	chown: unix.Fchown,
	chownLink: func(dir int, name string, uid, gid int) error {
		return unix.Fchownat(dir, name, uid, gid, unix.AT_SYMLINK_NOFOLLOW)
	},
}

var (
	errNoEntry  = errors.New("no such entry")
	errTooLarge = errors.New("too large")
)

func runFileStep(c Command, ops fileOps) error {
	if len(c.Argv) > 0 && treeSteps[c.Argv[0]] {
		return runTreeStep(c, ops)
	}
	if len(c.Argv) != 2 {
		return fmt.Errorf("built-in step %s has the wrong arguments", c.Argv[0])
	}
	path := c.Argv[1]
	if err := checkFilePath(path); err != nil {
		return err
	}
	switch c.Argv[0] {
	case stepFileWrite:
		var in writeInput
		if err := decodeState(c.Secret, &in); err != nil {
			return err
		}
		return writeFile(path, in.Content, ops)
	case stepFileRestore:
		var st fileState
		if err := decodeState(c.Secret, &st); err != nil {
			return err
		}
		if err := checkFileState(st); err != nil || st.Path != path {
			return errors.New("the recorded state of the file is not usable")
		}
		return restoreFile(st, ops)
	case stepFolderCreate:
		return createFolder(path, ops)
	case stepFolderRemove:
		return removeFolder(path)
	}
	return fmt.Errorf("unknown built-in step %q", c.Argv[0])
}

// openParentDir opens the folder of path without following a link and returns it with the last entry's name.
func openParentDir(path string) (int, string, error) {
	fd, err := openDirNoFollow(filepath.Dir(path))
	return fd, filepath.Base(path), err
}

// openDirNoFollow opens the folder path one entry at a time from the root.
func openDirNoFollow(path string) (int, error) {
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
	if path == "/" {
		return fd, nil
	}
	cur := ""
	for _, part := range strings.Split(strings.TrimPrefix(path, "/"), "/") {
		cur += "/" + part
		next, err := openDirAt(fd, part, cur)
		unix.Close(fd)
		if errors.Is(err, errNoEntry) {
			return -1, Refusef("the folder %s does not exist", cur)
		}
		if err != nil {
			return -1, err
		}
		fd = next
	}
	return fd, nil
}

func openDirAt(dir int, name, path string) (int, error) {
	var st unix.Stat_t
	if err := unix.Fstatat(dir, name, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		if errors.Is(err, unix.ENOENT) {
			return -1, errNoEntry
		}
		return -1, fmt.Errorf("read %s: %w", path, err)
	}
	switch st.Mode & unix.S_IFMT {
	case unix.S_IFDIR:
	case unix.S_IFLNK:
		return -1, Refusef("%s is a link; links are not followed", path)
	default:
		return -1, Refusef("%s is not a folder", path)
	}
	fd, err := unix.Openat(dir, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, fmt.Errorf("open %s (a link is refused): %w", path, err)
	}
	return fd, nil
}

// statFolder reads owner and permissions of the folder path without following a link.
func statFolder(path string) (fileMeta, error) {
	fd, err := openDirNoFollow(path)
	if err != nil {
		return fileMeta{}, err
	}
	defer unix.Close(fd)
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return fileMeta{}, err
	}
	return metaOf(&st), nil
}

// openFileAt opens the regular file name in dir with flags; a missing entry is errNoEntry.
func openFileAt(dir int, name, path string, flags int) (int, fileMeta, error) {
	var st unix.Stat_t
	if err := unix.Fstatat(dir, name, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		if errors.Is(err, unix.ENOENT) {
			return -1, fileMeta{}, errNoEntry
		}
		return -1, fileMeta{}, fmt.Errorf("read %s: %w", path, err)
	}
	switch st.Mode & unix.S_IFMT {
	case unix.S_IFREG:
	case unix.S_IFLNK:
		return -1, fileMeta{}, Refusef("%s is a link; links are not followed, write the real file instead", path)
	default:
		return -1, fileMeta{}, Refusef("%s is not a plain file", path)
	}
	// O_NONBLOCK keeps a FIFO swapped in after the check from blocking the open; the type is checked again.
	fd, err := unix.Openat(dir, name, flags|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, fileMeta{}, fmt.Errorf("open %s (a link is refused): %w", path, err)
	}
	var now unix.Stat_t
	if err := unix.Fstat(fd, &now); err != nil {
		unix.Close(fd)
		return -1, fileMeta{}, err
	}
	if now.Mode&unix.S_IFMT != unix.S_IFREG || now.Dev != st.Dev || now.Ino != st.Ino {
		unix.Close(fd)
		return -1, fileMeta{}, Refusef("%s changed while it was opened", path)
	}
	return fd, metaOf(&now), nil
}

// readFD reads the whole file from the start, at most MaxFileContent bytes.
func readFD(fd int) ([]byte, error) {
	var out []byte
	buf := make([]byte, 64<<10)
	for off := int64(0); ; {
		n, err := unix.Pread(fd, buf, off)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if n == 0 {
			return out, nil
		}
		out = append(out, buf[:n]...)
		off += int64(n)
		if len(out) > MaxFileContent {
			return nil, errTooLarge
		}
	}
}

// writeInPlace replaces the content of the open file without replacing the file.
func writeInPlace(fd int, data []byte) error {
	if len(data) > 0 {
		// Reserving the space first makes a write that stops halfway for lack of space unlikely; a file
		// system that cannot reserve simply writes.
		_ = unix.Fallocate(fd, unix.FALLOC_FL_KEEP_SIZE, 0, int64(len(data)))
	}
	for off := 0; off < len(data); {
		n, err := unix.Pwrite(fd, data[off:], int64(off))
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return fmt.Errorf("write: %w", err)
		}
		off += n
	}
	if err := unix.Ftruncate(fd, int64(len(data))); err != nil {
		return fmt.Errorf("set the length: %w", err)
	}
	return unix.Fsync(fd)
}

// newFileMode derives the mode of a new file from its folder: the read and write bits of the folder, never
// writable for everyone.
func newFileMode(folder uint32) uint32 { return folder & 0o666 &^ 0o002 }

// newFolderMode derives the mode of a new folder from its folder: its permissions and setgid, which keeps the
// group of a shared folder for what is created below it, never writable for everyone and without sticky bit.
func newFolderMode(folder uint32) uint32 { return folder & 0o2775 }

func writeFile(path string, content []byte, ops fileOps) error {
	if len(content) > MaxFileContent || !isText(content) {
		return errors.New("the content is not text of at most 1 MiB")
	}
	dir, name, err := openParentDir(path)
	if err != nil {
		return err
	}
	defer unix.Close(dir)
	fd, _, err := openFileAt(dir, name, path, unix.O_RDWR)
	if errors.Is(err, errNoEntry) {
		return createFileAt(dir, name, path, content, ops)
	}
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	if err := writeInPlace(fd, content); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// createFileAt creates the file name in dir with the owner and group of dir. It leaves nothing behind when a
// step fails.
func createFileAt(dir int, name, path string, content []byte, ops fileOps) error {
	var parent unix.Stat_t
	if err := unix.Fstat(dir, &parent); err != nil {
		return err
	}
	fd, err := unix.Openat(dir, name, unix.O_RDWR|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	err = writeInPlace(fd, content)
	if err == nil {
		err = ops.chown(fd, int(parent.Uid), int(parent.Gid))
	}
	if err == nil {
		err = unix.Fchmod(fd, newFileMode(parent.Mode&0o7777))
	}
	if err != nil {
		_ = unlinkSame(dir, name, fd, 0)
		unix.Close(fd)
		return fmt.Errorf("create %s: %w", path, err)
	}
	unix.Close(fd)
	return unix.Fsync(dir)
}

// unlinkSame removes name from dir only while it still names the open entry fd.
func unlinkSame(dir int, name string, fd int, flags int) error {
	var open, named unix.Stat_t
	if err := unix.Fstat(fd, &open); err != nil {
		return err
	}
	if err := unix.Fstatat(dir, name, &named, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return err
	}
	if open.Dev != named.Dev || open.Ino != named.Ino {
		return errors.New("the entry was replaced")
	}
	return unix.Unlinkat(dir, name, flags)
}

// sameAsAfter refuses a file that is no longer the one the change left.
func sameAsAfter(fd int, after *fileResult, path string) error {
	changed := fmt.Errorf("%s changed after the change; it is left as it is", path)
	if !after.Exists || after.Meta == nil {
		return changed
	}
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return err
	}
	if uint64(st.Dev) != after.Meta.Dev || st.Ino != after.Meta.Ino {
		return changed
	}
	data, err := readFD(fd)
	if errors.Is(err, errTooLarge) || (err == nil && sha256Hex(data) != after.SHA256) {
		return changed
	}
	return err
}

func restoreFile(st fileState, ops fileOps) error {
	dir, name, err := openParentDir(st.Path)
	if err != nil {
		return err
	}
	defer unix.Close(dir)
	if !st.Existed {
		return removeCreatedFile(dir, name, st)
	}
	fd, now, err := openFileAt(dir, name, st.Path, unix.O_RDWR)
	if errors.Is(err, errNoEntry) {
		return fmt.Errorf("%s no longer exists; it is not created again", st.Path)
	}
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	// Without After the restore follows the failed change right away, which left the file in an unknown state.
	if st.After != nil {
		if err := sameAsAfter(fd, st.After, st.Path); err != nil {
			return err
		}
	}
	if err := writeInPlace(fd, st.Content); err != nil {
		return fmt.Errorf("%s: %w", st.Path, err)
	}
	was := st.Meta
	chowned := false
	if now.UID != was.UID || now.GID != was.GID {
		if err := ops.chown(fd, was.UID, was.GID); err != nil {
			return fmt.Errorf("restore the owner of %s: %w", st.Path, err)
		}
		chowned = true
	}
	// A change of owner clears setuid and setgid, so the mode is set again after it.
	if chowned || now.Mode != was.Mode {
		if err := unix.Fchmod(fd, was.Mode); err != nil {
			return fmt.Errorf("restore the permissions of %s: %w", st.Path, err)
		}
	}
	return nil
}

func removeCreatedFile(dir int, name string, st fileState) error {
	if st.After == nil {
		return fmt.Errorf("the state of %s after the change is unknown; it is left in place", st.Path)
	}
	fd, _, err := openFileAt(dir, name, st.Path, unix.O_RDONLY)
	if errors.Is(err, errNoEntry) {
		return nil
	}
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	if !st.After.Exists {
		return fmt.Errorf("%s was not made by the change; it is left in place", st.Path)
	}
	if err := sameAsAfter(fd, st.After, st.Path); err != nil {
		return err
	}
	if err := unlinkSame(dir, name, fd, 0); err != nil {
		return fmt.Errorf("remove %s: %w", st.Path, err)
	}
	return unix.Fsync(dir)
}

// createFolder creates the folder with the owner and group of its folder and leaves nothing behind when a
// step fails.
func createFolder(path string, ops fileOps) error {
	dir, name, err := openParentDir(path)
	if err != nil {
		return err
	}
	defer unix.Close(dir)
	var parent unix.Stat_t
	if err := unix.Fstat(dir, &parent); err != nil {
		return err
	}
	if err := unix.Mkdirat(dir, name, 0o700); err != nil {
		if errors.Is(err, unix.EEXIST) {
			return fmt.Errorf("%s exists already", path)
		}
		return fmt.Errorf("create %s: %w", path, err)
	}
	fd, err := openDirAt(dir, name, path)
	if err == nil {
		err = ops.chown(fd, int(parent.Uid), int(parent.Gid))
		if err == nil {
			err = unix.Fchmod(fd, newFolderMode(parent.Mode&0o7777))
		}
		if err != nil {
			_ = unlinkSame(dir, name, fd, unix.AT_REMOVEDIR)
		}
		unix.Close(fd)
	}
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	return unix.Fsync(dir)
}

func removeFolder(path string) error {
	dir, name, err := openParentDir(path)
	if err != nil {
		return err
	}
	defer unix.Close(dir)
	var st unix.Stat_t
	if err := unix.Fstatat(dir, name, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		if errors.Is(err, unix.ENOENT) {
			return nil
		}
		return fmt.Errorf("read %s: %w", path, err)
	}
	if st.Mode&unix.S_IFMT != unix.S_IFDIR {
		return fmt.Errorf("%s is no longer a folder; it is left in place", path)
	}
	if err := unix.Unlinkat(dir, name, unix.AT_REMOVEDIR); err != nil {
		if errors.Is(err, unix.ENOTEMPTY) || errors.Is(err, unix.EEXIST) {
			return fmt.Errorf("%s is not empty; it is left in place", path)
		}
		return fmt.Errorf("remove %s: %w", path, err)
	}
	return unix.Fsync(dir)
}
