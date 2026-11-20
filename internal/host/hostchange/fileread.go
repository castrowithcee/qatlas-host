package hostchange

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// Kinds of an entry that ReadPath reports.
const (
	KindFile   = "file"
	KindFolder = "folder"
	KindLink   = "link"
	KindOther  = "other"
)

// PathEntry is one entry of the file system as it is, a link not followed.
type PathEntry struct {
	Name    string
	Kind    string
	Size    int64
	UID     int
	GID     int
	Mode    uint32
	ModTime time.Time
	// Target is the text of a link, never resolved.
	Target string
}

// PathContent is what ReadPath finds at a path.
type PathContent struct {
	// Entry describes the path itself; its Name is the last element of the path.
	Entry PathEntry
	// Entries lists a folder, sorted by name.
	Entries []PathEntry
	// Content is the text of a regular file, empty when TooLarge or Binary.
	Content  []byte
	TooLarge bool
	Binary   bool
}

// ReadPath reads a folder as a list, a regular file as its content up to MaxFileContent bytes, and a link as
// itself. It resolves the path with the same link-free steps as the file actions, so a link anywhere on the
// way, or a link planted in a folder of a stack, never leads the read elsewhere.
func ReadPath(path string) (PathContent, error) {
	if err := checkReadPath(path); err != nil {
		return PathContent{}, err
	}
	if path == "/" {
		fd, err := openDirNoFollow("/")
		if err != nil {
			return PathContent{}, err
		}
		return readFolder(fd, path, "/")
	}
	dir, name, err := openParentDir(path)
	if err != nil {
		return PathContent{}, err
	}
	defer unix.Close(dir)
	var st unix.Stat_t
	if err := unix.Fstatat(dir, name, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		if errors.Is(err, unix.ENOENT) {
			return PathContent{}, Refusef("%s does not exist", path)
		}
		return PathContent{}, fmt.Errorf("read %s: %w", path, err)
	}
	entry, err := pathEntryOf(dir, name, &st)
	if err != nil {
		return PathContent{}, fmt.Errorf("read %s: %w", path, err)
	}
	switch entry.Kind {
	case KindFolder:
		fd, err := openDirAt(dir, name, path)
		if err != nil {
			return PathContent{}, err
		}
		return readFolder(fd, path, name)
	case KindFile:
		fd, _, err := openFileAt(dir, name, path, unix.O_RDONLY)
		if err != nil {
			return PathContent{}, err
		}
		defer unix.Close(fd)
		out := PathContent{Entry: entry}
		data, err := readFD(fd)
		switch {
		case errors.Is(err, errTooLarge):
			out.TooLarge = true
		case err != nil:
			return PathContent{}, fmt.Errorf("read %s: %w", path, err)
		case !isText(data):
			out.Binary = true
		default:
			out.Content = data
		}
		return out, nil
	}
	return PathContent{Entry: entry}, nil
}

func checkReadPath(path string) error {
	switch {
	case !strings.HasPrefix(path, "/") || strings.ContainsAny(path, "\x00\n\r"):
		return Argf("the path must be absolute and without control characters")
	case path != "/" && !isCleanPath(path):
		return Argf("the path must be given clean, without \"..\", \"//\" or a trailing \"/\"")
	}
	return nil
}

func isCleanPath(path string) bool {
	for _, p := range strings.Split(path[1:], "/") {
		if p == "" || p == "." || p == ".." {
			return false
		}
	}
	return true
}

func readFolder(fd int, path, name string) (PathContent, error) {
	// The File owns fd from here and closes it.
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return PathContent{}, fmt.Errorf("read %s: %w", path, err)
	}
	names, err := f.Readdirnames(-1)
	if err != nil {
		return PathContent{}, fmt.Errorf("read %s: %w", path, err)
	}
	sort.Strings(names)
	self, err := pathEntryOf(fd, "", &st)
	if err != nil {
		return PathContent{}, err
	}
	self.Name = name
	out := PathContent{Entry: self, Entries: []PathEntry{}}
	for _, n := range names {
		var est unix.Stat_t
		if err := unix.Fstatat(fd, n, &est, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			if errors.Is(err, unix.ENOENT) {
				continue
			}
			return PathContent{}, fmt.Errorf("read %s/%s: %w", strings.TrimSuffix(path, "/"), n, err)
		}
		e, err := pathEntryOf(fd, n, &est)
		if err != nil {
			return PathContent{}, fmt.Errorf("read %s/%s: %w", strings.TrimSuffix(path, "/"), n, err)
		}
		out.Entries = append(out.Entries, e)
	}
	return out, nil
}

// pathEntryOf describes name in dir from its stat; the text of a link is read, never followed.
func pathEntryOf(dir int, name string, st *unix.Stat_t) (PathEntry, error) {
	e := PathEntry{Name: name, Size: st.Size, UID: int(st.Uid), GID: int(st.Gid), Mode: st.Mode & 0o7777,
		ModTime: time.Unix(st.Mtim.Sec, st.Mtim.Nsec).UTC()}
	switch st.Mode & unix.S_IFMT {
	case unix.S_IFREG:
		e.Kind = KindFile
	case unix.S_IFDIR:
		e.Kind = KindFolder
	case unix.S_IFLNK:
		e.Kind = KindLink
		buf := make([]byte, unix.PathMax)
		n, err := unix.Readlinkat(dir, name, buf)
		if err != nil {
			return PathEntry{}, err
		}
		e.Target = string(buf[:n])
	default:
		e.Kind = KindOther
	}
	return e, nil
}
