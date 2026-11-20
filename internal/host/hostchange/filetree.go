package hostchange

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/sys/unix"
)

// The tree steps rename, copy and remove a file, a link or a folder with everything below it. They resolve the
// path to the last entry with the same link-free steps as the file steps; the last entry itself may be a link,
// which is renamed, copied or removed as a link and never followed. Below a folder they open every entry
// relative to its folder and never leave the mount of the tree.
const (
	// stepFileRename renames Argv[1] to Argv[2], which must not exist. With a fileRenameState on standard input,
	// the undo, it renames only while Argv[1] is still the entry the change renamed.
	stepFileRename = "file-rename"
	// stepFileCopy copies Argv[1] to Argv[2], which must not exist, with the owner and group of the folder of
	// Argv[2].
	stepFileCopy = "file-copy"
	// stepFileRemove removes Argv[1] with everything below it; Argv[2] optionPermanent lifts the limit of an undo.
	stepFileRemove = "file-remove"
	// stepFileRemoveCopy removes the copy Argv[1] only while it matches the treeManifest on standard input.
	stepFileRemoveCopy = "file-remove-copy"
	// stepFileRestoreTree creates Argv[1] again from the removeState on standard input, only while it is free.
	stepFileRestoreTree = "file-restore-tree"
)

// MaxUndoBytes bounds the size of the regular files of a copy or a removal that can be undone, because the
// journal keeps all of a removed tree.
const MaxUndoBytes = 10 << 20

// MaxTreeEntries bounds every tree a tree action walks, also with optionPermanent: a root command holds the
// journal lock while it runs, and one confirmation should not wipe or copy a whole data disk by a wrong path.
const MaxTreeEntries = 100_000

// Kinds of an entry of a tree.
const (
	kindDir   = "dir"
	kindFile  = "file"
	kindLink  = "link"
	kindOther = "other"
)

// node is one entry of the file system as it is, a link not followed.
type node struct {
	kind     string
	uid, gid int
	// mode holds the permission and the special bits.
	mode  uint32
	size  int64
	dev   uint64
	mnt   uint64
	ino   uint64
	mtime unix.Timespec
}

// statxAt reads name in dir without following a link; an empty name reads dir itself. A missing entry is
// errNoEntry.
func statxAt(dir int, name string) (node, error) {
	flags := unix.AT_SYMLINK_NOFOLLOW | unix.AT_STATX_SYNC_AS_STAT
	if name == "" {
		flags |= unix.AT_EMPTY_PATH
	}
	var sx unix.Statx_t
	if err := unix.Statx(dir, name, flags, unix.STATX_BASIC_STATS|unix.STATX_MNT_ID, &sx); err != nil {
		if errors.Is(err, unix.ENOENT) {
			return node{}, errNoEntry
		}
		return node{}, err
	}
	n := node{
		uid: int(sx.Uid), gid: int(sx.Gid), mode: uint32(sx.Mode) & 0o7777, size: int64(sx.Size),
		dev: unix.Mkdev(sx.Dev_major, sx.Dev_minor), ino: sx.Ino,
		mtime: unix.Timespec{Sec: sx.Mtime.Sec, Nsec: int64(sx.Mtime.Nsec)},
	}
	// The mount id tells a bind mount of the same file system apart, where the kernel reports it.
	if sx.Mask&unix.STATX_MNT_ID != 0 {
		n.mnt = sx.Mnt_id
	}
	switch uint32(sx.Mode) & unix.S_IFMT {
	case unix.S_IFDIR:
		n.kind = kindDir
	case unix.S_IFREG:
		n.kind = kindFile
	case unix.S_IFLNK:
		n.kind = kindLink
	default:
		n.kind = kindOther
	}
	return n, nil
}

func sameMount(a, b node) bool { return a.dev == b.dev && a.mnt == b.mnt }

func readLink(dir int, name, path string) (string, error) {
	buf := make([]byte, unix.PathMax)
	n, err := unix.Readlinkat(dir, name, buf)
	if err != nil {
		return "", fmt.Errorf("read the link %s: %w", path, err)
	}
	if n >= len(buf) {
		return "", fmt.Errorf("the link %s is too long", path)
	}
	return string(buf[:n]), nil
}

// listNames lists the folder fd, sorted, without closing fd.
func listNames(fd int, path string) ([]string, error) {
	dup, err := unix.FcntlInt(uintptr(fd), unix.F_DUPFD_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(dup), path)
	defer f.Close()
	names, err := f.Readdirnames(-1)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	sort.Strings(names)
	return names, nil
}

func joinRel(rel, name string) string {
	if rel == "" {
		return name
	}
	return rel + "/" + name
}

func joinPath(base, rel string) string {
	if rel == "" {
		return base
	}
	return base + "/" + rel
}

func tempName() (string, error) {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return ".qhost-" + hex.EncodeToString(b[:]), nil
}

// formatSize names a size exactly and in MiB.
func formatSize(n int64) string {
	return fmt.Sprintf("%d bytes (%.1f MiB)", n, float64(n)/(1<<20))
}

// scanOpts says what scanTree reads and refuses.
type scanOpts struct {
	// limit is the most entries the tree may have; more answer ErrTooManyEntries.
	limit int
	// maxBytes bounds the content read; 0 reads no content.
	maxBytes int64
	// lockBases are the paths at which the tree is written, moved or removed; no entry may be locked there.
	lockBases []string
	// specials allows devices, FIFOs and sockets, which are refused otherwise.
	specials bool
	// content keeps the content of every regular file, hash its SHA-256 only.
	content, hash bool
}

// scanned is one entry of a scanned tree.
type scanned struct {
	node
	rel     string
	link    string
	content []byte
	sha256  string
}

// treeScan is a tree as scanTree found it: the entries in order, every folder before what lies in it.
type treeScan struct {
	parent  node
	entries []scanned
	bytes   int64
}

// scanTree reads the entry at path and everything below it without following a link and without leaving its
// mount. A missing path is a refusal that wraps errNoEntry.
func scanTree(path string, o scanOpts) (treeScan, error) {
	dir, name, err := openParentDir(path)
	if err != nil {
		return treeScan{}, err
	}
	defer unix.Close(dir)
	return scanAt(dir, name, path, o)
}

func scanAt(dir int, name, path string, o scanOpts) (treeScan, error) {
	parent, err := statxAt(dir, "")
	if err != nil {
		return treeScan{}, fmt.Errorf("read the folder of %s: %w", path, err)
	}
	s := &scanner{o: o, path: path, out: treeScan{parent: parent}}
	if err := s.walk(dir, name, ""); err != nil {
		return treeScan{}, err
	}
	return s.out, nil
}

type scanner struct {
	o    scanOpts
	path string
	out  treeScan
}

// missingError is the refusal of a missing path, which callers tell apart with errNoEntry.
type missingError struct{ *RefusedError }

func (e missingError) Unwrap() []error { return []error{e.RefusedError, errNoEntry} }

func (s *scanner) walk(dir int, name, rel string) error {
	path := joinPath(s.path, rel)
	n, err := statxAt(dir, name)
	if errors.Is(err, errNoEntry) {
		return missingError{&RefusedError{fmt.Sprintf("%s does not exist", path)}}
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if len(s.out.entries) > 0 && !sameMount(n, s.out.entries[0].node) {
		return Refusef("%s lies on another file system or mount; mounts are not changed together with the tree", path)
	}
	if len(s.out.entries) >= s.o.limit {
		return fmt.Errorf("%w (more than %d)", ErrTooManyEntries, s.o.limit)
	}
	for _, base := range s.o.lockBases {
		if reason := lockedReason(joinPath(base, rel)); reason != "" {
			return Refusef("%s is locked: %s", joinPath(base, rel), reason)
		}
	}
	e := scanned{node: n, rel: rel}
	switch n.kind {
	case kindOther:
		if !s.o.specials {
			return Refusef("%s is a device, FIFO or socket; only files, folders and links are copied or removed", path)
		}
	case kindLink:
		if e.link, err = readLink(dir, name, path); err != nil {
			return err
		}
	case kindFile:
		s.out.bytes += n.size
		if s.o.content || s.o.hash {
			if err := s.read(dir, name, path, &e); err != nil {
				return err
			}
		}
	}
	s.out.entries = append(s.out.entries, e)
	if n.kind != kindDir {
		return nil
	}
	fd, err := openDirAt(dir, name, path)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	names, err := listNames(fd, path)
	if err != nil {
		return err
	}
	for _, child := range names {
		if err := s.walk(fd, child, joinRel(rel, child)); err != nil {
			return err
		}
	}
	return nil
}

func (s *scanner) read(dir int, name, path string, e *scanned) error {
	fd, _, err := openFileAt(dir, name, path, unix.O_RDONLY)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	h := sha256.New()
	var data []byte
	buf := make([]byte, 128<<10)
	for off := int64(0); ; {
		n, err := unix.Pread(fd, buf, off)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		if n == 0 {
			break
		}
		h.Write(buf[:n])
		off += int64(n)
		if s.o.content {
			s.o.maxBytes -= int64(n)
			if s.o.maxBytes < 0 {
				return fmt.Errorf("the files below %s grew beyond %s while they were read", s.path, formatSize(MaxUndoBytes))
			}
			data = append(data, buf[:n]...)
		}
	}
	e.sha256 = hex.EncodeToString(h.Sum(nil))
	if s.o.content {
		e.content = data
		if e.content == nil {
			e.content = []byte{}
		}
	}
	return nil
}

// MeasureTree counts the entries at path, the entry itself and everything below it, and the size of its regular
// files, with the same link-free steps and the same refusals as file.copy and file.remove. A tree with more than
// MaxTreeEntries entries answers ErrTooManyEntries.
func MeasureTree(path string) (entries int, bytes int64, err error) {
	if err := checkCleanPath(path); err != nil {
		return 0, 0, err
	}
	s, err := scanTree(path, scanOpts{limit: MaxTreeEntries})
	if err != nil {
		return 0, 0, err
	}
	return len(s.entries), s.bytes, nil
}

// undoLimit refuses a tree that is too large to be undone, naming its size and the way out.
func undoLimit(path, verb string, entries int, bytes int64) error {
	if entries <= MaxFolderEntries && bytes <= MaxUndoBytes {
		return nil
	}
	return Refusef("%s holds %d entries with %s of files; an undo is possible up to %d entries and %s. "+
		"Add %s to %s it without an undo", path, entries, formatSize(bytes), MaxFolderEntries, formatSize(MaxUndoBytes),
		optionPermanent, verb)
}

// measureForChange scans the tree before a change: limited to MaxTreeEntries with permanent, otherwise to what an
// undo allows. verb and done name the change, such as "copy" and "copied".
func measureForChange(path, verb, done string, permanent bool, lockBases []string) (treeScan, error) {
	s, err := scanTree(path, scanOpts{limit: MaxTreeEntries, lockBases: lockBases})
	if errors.Is(err, ErrTooManyEntries) {
		return treeScan{}, Refusef("%s has more than %d entries; it is not %s in one change", path, MaxTreeEntries, done)
	}
	if err != nil {
		return treeScan{}, err
	}
	if !permanent {
		if err := undoLimit(path, verb, len(s.entries), s.bytes); err != nil {
			return treeScan{}, err
		}
	}
	return s, nil
}

var treeSteps = map[string]bool{
	stepFileRename: true, stepFileCopy: true, stepFileRemove: true, stepFileRemoveCopy: true, stepFileRestoreTree: true,
}

func runTreeStep(c Command, ops fileOps) error {
	wrong := fmt.Errorf("built-in step %s has the wrong arguments", c.Argv[0])
	switch c.Argv[0] {
	case stepFileRename:
		if len(c.Argv) != 3 {
			return wrong
		}
		src, dst := c.Argv[1], c.Argv[2]
		if err := checkMovePaths(src, dst); err != nil {
			return err
		}
		var expect *fileRenameState
		if len(c.Secret) > 0 {
			var st fileRenameState
			if err := decodeState(c.Secret, &st); err != nil {
				return err
			}
			if st.Target != src || st.Source != dst {
				return errors.New("the recorded state of the rename is not usable")
			}
			expect = &st
		}
		return renameEntry(src, dst, expect)
	case stepFileCopy:
		if len(c.Argv) != 3 {
			return wrong
		}
		src, dst := c.Argv[1], c.Argv[2]
		if err := checkCopyPaths(src, dst); err != nil {
			return err
		}
		return copyTree(src, dst, ops)
	case stepFileRemove:
		if len(c.Argv) != 2 && (len(c.Argv) != 3 || c.Argv[2] != optionPermanent) {
			return wrong
		}
		if err := checkRemovePath(c.Argv[1]); err != nil {
			return err
		}
		return removePath(c.Argv[1], len(c.Argv) == 3)
	case stepFileRemoveCopy:
		if len(c.Argv) != 2 {
			return wrong
		}
		var m treeManifest
		if err := decodeState(c.Secret, &m); err != nil {
			return err
		}
		if err := checkManifest(m); err != nil || m.Path != c.Argv[1] {
			return errors.New("the recorded state of the copy is not usable")
		}
		return removeCopy(m)
	case stepFileRestoreTree:
		if len(c.Argv) != 2 {
			return wrong
		}
		var st removeState
		if err := decodeState(c.Secret, &st); err != nil {
			return err
		}
		if err := checkRemoveState(st); err != nil || st.Path != c.Argv[1] {
			return errors.New("the recorded state of the removed tree is not usable")
		}
		return restoreTree(st, ops)
	}
	return fmt.Errorf("unknown built-in step %q", c.Argv[0])
}

// checkRenameTree refuses to move a folder that holds a locked entry, at its old or its new place.
func checkRenameTree(dir int, name, src, dst string) error {
	_, err := scanAt(dir, name, src, scanOpts{limit: MaxTreeEntries, lockBases: []string{src, dst}, specials: true})
	if errors.Is(err, ErrTooManyEntries) {
		return Refusef("%s has more than %d entries; it is not renamed in one change", src, MaxTreeEntries)
	}
	return err
}

// renameEntry renames src to dst without replacing anything; expect, on an undo, names the entry src must be.
func renameEntry(src, dst string, expect *fileRenameState) error {
	sdir, sname, err := openParentDir(src)
	if err != nil {
		return err
	}
	defer unix.Close(sdir)
	ddir, dname, err := openParentDir(dst)
	if err != nil {
		return err
	}
	defer unix.Close(ddir)
	n, err := statxAt(sdir, sname)
	if errors.Is(err, errNoEntry) {
		return fmt.Errorf("%s does not exist; nothing is renamed", src)
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", src, err)
	}
	if expect != nil && (n.dev != expect.Dev || n.ino != expect.Ino) {
		return fmt.Errorf("%s is no longer the entry the change renamed; it is left as it is", src)
	}
	if err := checkRenameTree(sdir, sname, src, dst); err != nil {
		return err
	}
	switch err := unix.Renameat2(sdir, sname, ddir, dname, unix.RENAME_NOREPLACE); {
	case errors.Is(err, unix.EEXIST):
		return fmt.Errorf("%s exists already; nothing is replaced", dst)
	case errors.Is(err, unix.EXDEV):
		return fmt.Errorf("%s and %s lie on different file systems or mounts; a rename does not copy", src, filepath.Dir(dst))
	case err != nil:
		return fmt.Errorf("rename %s to %s: %w", src, dst, err)
	}
	if err := unix.Fsync(ddir); err != nil {
		return err
	}
	return unix.Fsync(sdir)
}

// copier copies a tree into a new tree that only root can enter until it is complete.
type copier struct {
	ops      fileOps
	uid, gid int
	top      node
	count    int
	src, dst string
}

func copyTree(src, dst string, ops fileOps) error {
	sdir, sname, err := openParentDir(src)
	if err != nil {
		return err
	}
	defer unix.Close(sdir)
	ddir, dname, err := openParentDir(dst)
	if err != nil {
		return err
	}
	defer unix.Close(ddir)
	parent, err := statxAt(ddir, "")
	if err != nil {
		return err
	}
	if _, err := statxAt(ddir, dname); !errors.Is(err, errNoEntry) {
		if err == nil {
			return fmt.Errorf("%s exists already", dst)
		}
		return fmt.Errorf("read %s: %w", dst, err)
	}
	top, err := statxAt(sdir, sname)
	if errors.Is(err, errNoEntry) {
		return fmt.Errorf("%s does not exist", src)
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", src, err)
	}
	tmp, err := tempName()
	if err != nil {
		return err
	}
	c := &copier{ops: ops, uid: parent.uid, gid: parent.gid, top: top, src: src, dst: dst}
	err = c.copy(sdir, sname, ddir, tmp, "")
	if err == nil {
		// The copy appears at once and complete, and never replaces what appeared at dst meanwhile.
		if err = unix.Renameat2(ddir, tmp, ddir, dname, unix.RENAME_NOREPLACE); errors.Is(err, unix.EEXIST) {
			err = fmt.Errorf("%s exists already", dst)
		}
	}
	if err != nil {
		if cerr := removeOwn(ddir, tmp, joinPath(filepath.Dir(dst), tmp)); cerr != nil {
			err = fmt.Errorf("%w; the partial copy %s is left: %v", err, joinPath(filepath.Dir(dst), tmp), cerr)
		}
		return fmt.Errorf("copy %s to %s: %w", src, dst, err)
	}
	return unix.Fsync(ddir)
}

// copyMode is the mode of a copy: the permissions of the source without setuid, setgid and sticky bit, since
// the copy belongs to someone else than the source and must not run with the rights of its new owner.
func copyMode(source uint32) uint32 { return source & 0o777 }

func (c *copier) copy(sdir int, sname string, ddir int, dname, rel string) error {
	spath := joinPath(c.src, rel)
	n, err := statxAt(sdir, sname)
	if errors.Is(err, errNoEntry) {
		return fmt.Errorf("%s disappeared during the copy", spath)
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", spath, err)
	}
	if rel != "" && !sameMount(n, c.top) {
		return Refusef("%s lies on another file system or mount; mounts are not copied", spath)
	}
	if c.count++; c.count > MaxTreeEntries {
		return Refusef("%s has more than %d entries; it is not copied in one change", c.src, MaxTreeEntries)
	}
	if reason := lockedReason(joinPath(c.dst, rel)); reason != "" {
		return Refusef("%s is locked: %s", joinPath(c.dst, rel), reason)
	}
	switch n.kind {
	case kindFile:
		return c.copyFile(sdir, sname, spath, ddir, dname, n)
	case kindLink:
		text, err := readLink(sdir, sname, spath)
		if err != nil {
			return err
		}
		if err := unix.Symlinkat(text, ddir, dname); err != nil {
			return fmt.Errorf("create the link %s: %w", joinPath(c.dst, rel), err)
		}
		return c.ops.chownLink(ddir, dname, c.uid, c.gid)
	case kindDir:
		in, err := openDirAt(sdir, sname, spath)
		if err != nil {
			return err
		}
		defer unix.Close(in)
		names, err := listNames(in, spath)
		if err != nil {
			return err
		}
		if err := unix.Mkdirat(ddir, dname, 0o700); err != nil {
			return fmt.Errorf("create %s: %w", joinPath(c.dst, rel), err)
		}
		out, err := openDirAt(ddir, dname, joinPath(c.dst, rel))
		if err != nil {
			return err
		}
		defer unix.Close(out)
		for _, child := range names {
			if err := c.copy(in, child, out, child, joinRel(rel, child)); err != nil {
				return err
			}
		}
		// The folder opens up only now that everything below it is in place.
		if err := c.ops.chown(out, c.uid, c.gid); err != nil {
			return fmt.Errorf("set the owner of %s: %w", joinPath(c.dst, rel), err)
		}
		return unix.Fchmod(out, copyMode(n.mode))
	}
	return Refusef("%s is a device, FIFO or socket; only files, folders and links are copied", spath)
}

func (c *copier) copyFile(sdir int, sname, spath string, ddir int, dname string, n node) error {
	in, _, err := openFileAt(sdir, sname, spath, unix.O_RDONLY)
	if err != nil {
		return err
	}
	defer unix.Close(in)
	out, err := unix.Openat(ddir, dname, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
	if err != nil {
		return fmt.Errorf("create the copy of %s: %w", spath, err)
	}
	defer unix.Close(out)
	if err := copyData(in, out); err != nil {
		return fmt.Errorf("copy %s: %w", spath, err)
	}
	if err := c.ops.chown(out, c.uid, c.gid); err != nil {
		return fmt.Errorf("set the owner of the copy of %s: %w", spath, err)
	}
	return unix.Fchmod(out, copyMode(n.mode))
}

func copyData(in, out int) error {
	buf := make([]byte, 128<<10)
	for {
		n, err := unix.Read(in, buf)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
		if err := writeAll(out, buf[:n]); err != nil {
			return err
		}
	}
}

func writeAll(fd int, data []byte) error {
	for len(data) > 0 {
		n, err := unix.Write(fd, data)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return err
		}
		data = data[n:]
	}
	return nil
}

// removeOwn removes the hidden tree name that a step created in dir; a missing one is fine.
func removeOwn(dir int, name, path string) error {
	n, err := statxAt(dir, name)
	if errors.Is(err, errNoEntry) {
		return nil
	}
	if err != nil {
		return err
	}
	return removeAt(dir, name, path, n)
}

// removeAt removes name in dir and everything below it, refusing to leave the mount of top. It opens every
// folder relative to the one before and never follows a link.
func removeAt(dir int, name, path string, top node) error {
	n, err := statxAt(dir, name)
	if errors.Is(err, errNoEntry) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if !sameMount(n, top) {
		return fmt.Errorf("%s lies on another file system or mount; it is not removed", path)
	}
	if n.kind != kindDir {
		if err := unix.Unlinkat(dir, name, 0); err != nil && !errors.Is(err, unix.ENOENT) {
			return fmt.Errorf("remove %s: %w", path, err)
		}
		return nil
	}
	fd, err := openDirAt(dir, name, path)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	names, err := listNames(fd, path)
	if err != nil {
		return err
	}
	for _, child := range names {
		if err := removeAt(fd, child, path+"/"+child, top); err != nil {
			return err
		}
	}
	if err := unlinkSame(dir, name, fd, unix.AT_REMOVEDIR); err != nil {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	return nil
}

// removeAway first renames the entry name, which must still be top, to a hidden name in the same folder and
// then removes it there: the path is free at once, and a removal that stops halfway leaves the rest only under
// the hidden name, so that the previous state can be restored at the path.
func removeAway(dir int, name, path string, top node) error {
	now, err := statxAt(dir, name)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if now.dev != top.dev || now.ino != top.ino {
		return fmt.Errorf("%s was replaced meanwhile; it is left as it is", path)
	}
	tmp, err := tempName()
	if err != nil {
		return err
	}
	if err := unix.Renameat2(dir, name, dir, tmp, unix.RENAME_NOREPLACE); err != nil {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	hidden := joinPath(filepath.Dir(path), tmp)
	if err := removeAt(dir, tmp, hidden, top); err != nil {
		return fmt.Errorf("%w; the rest of %s lies in %s", err, path, hidden)
	}
	return unix.Fsync(dir)
}

// removePath removes the tree at path; without permanent only while an undo can keep it.
func removePath(path string, permanent bool) error {
	dir, name, err := openParentDir(path)
	if err != nil {
		return err
	}
	defer unix.Close(dir)
	o := scanOpts{limit: MaxTreeEntries, lockBases: []string{path}}
	if !permanent {
		o.limit = MaxFolderEntries
	}
	s, err := scanAt(dir, name, path, o)
	if errors.Is(err, ErrTooManyEntries) {
		return fmt.Errorf("%s has more than %d entries now; nothing was removed", path, o.limit)
	}
	if err != nil {
		return err
	}
	if !permanent && s.bytes > MaxUndoBytes {
		return fmt.Errorf("the files below %s grew to %s since the previous state was read; nothing was removed", path, formatSize(s.bytes))
	}
	top := s.entries[0].node
	if top.kind == kindDir && !sameMount(top, s.parent) {
		return fmt.Errorf("%s is a mount point; it is not removed", path)
	}
	return removeAway(dir, name, path, top)
}

// treeManifest names a tree that a change made: every entry with its inode, size, content hash and link text.
type treeManifest struct {
	Path    string          `json:"path"`
	Dev     uint64          `json:"dev"`
	Entries []manifestEntry `json:"entries"`
}

type manifestEntry struct {
	Rel    string `json:"rel"`
	Kind   string `json:"kind"`
	Ino    uint64 `json:"ino"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256,omitempty"`
	Link   string `json:"link,omitempty"`
}

// entriesOf describes a scanned tree as manifest entries; withIno keeps the inodes. A hash is set only where the
// scan read the content.
func entriesOf(s treeScan, withIno bool) []manifestEntry {
	out := make([]manifestEntry, len(s.entries))
	for i, e := range s.entries {
		out[i] = manifestEntry{Rel: e.rel, Kind: e.kind, Link: e.link}
		if withIno {
			out[i].Ino = e.ino
		}
		if e.kind == kindFile {
			out[i].Size, out[i].SHA256 = e.size, e.sha256
		}
	}
	return out
}

func manifestOf(path string, s treeScan) treeManifest {
	return treeManifest{Path: path, Dev: s.entries[0].dev, Entries: entriesOf(s, true)}
}

// matchTree compares the tree at name in dir with want: first without reading any content, so that a tree that
// grew large is not read, then with the content hashes. A missing tree answers errNoEntry.
func matchTree(dir int, name, path string, want []manifestEntry, withIno bool, o scanOpts) (treeScan, bool, error) {
	o.limit = len(want)
	compare := func(hash bool) (treeScan, bool, error) {
		o.hash = hash
		s, err := scanAt(dir, name, path, o)
		if errors.Is(err, ErrTooManyEntries) {
			return treeScan{}, false, nil
		}
		if err != nil {
			return treeScan{}, false, err
		}
		return s, equalEntries(entriesOf(s, withIno), want, hash), nil
	}
	if s, same, err := compare(false); err != nil || !same {
		return s, false, err
	}
	return compare(true)
}

func equalEntries(got, want []manifestEntry, hashes bool) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		g, w := got[i], want[i]
		if !hashes {
			g.SHA256, w.SHA256 = "", ""
		}
		if g != w {
			return false
		}
	}
	return true
}

func checkManifest(m treeManifest) error {
	bad := errors.New("the recorded state of the copy is not usable")
	if checkFilePath(m.Path) != nil || len(m.Entries) == 0 || len(m.Entries) > MaxTreeEntries {
		return bad
	}
	rels := make([]string, len(m.Entries))
	kinds := make([]string, len(m.Entries))
	for i, e := range m.Entries {
		rels[i], kinds[i] = e.Rel, e.Kind
		if e.Size < 0 || (e.Kind == kindFile && len(e.SHA256) != sha256.Size*2) {
			return bad
		}
	}
	if checkTreeRels(m.Path, rels, kinds) != nil {
		return bad
	}
	return nil
}

// checkTreeRels checks the relative paths of a recorded tree: the first is the tree itself, every other is a
// clean name below a folder listed before it, and none is locked.
func checkTreeRels(path string, rels, kinds []string) error {
	bad := errors.New("the recorded tree is not usable")
	if len(rels) == 0 || rels[0] != "" {
		return bad
	}
	dirs := map[string]bool{}
	seen := map[string]bool{}
	for i, rel := range rels {
		switch kinds[i] {
		case kindDir, kindFile, kindLink:
		default:
			return bad
		}
		if i > 0 {
			if seen[rel] || strings.ContainsRune(rel, 0) {
				return bad
			}
			for _, part := range strings.Split(rel, "/") {
				if part == "" || part == "." || part == ".." {
					return bad
				}
			}
			parent := ""
			if j := strings.LastIndexByte(rel, '/'); j >= 0 {
				parent = rel[:j]
			}
			if !dirs[parent] {
				return bad
			}
		}
		if lockedReason(joinPath(path, rel)) != "" {
			return bad
		}
		seen[rel] = true
		if kinds[i] == kindDir {
			dirs[rel] = true
		}
	}
	return nil
}

// removeCopy removes the copy only while it is exactly the tree the copy made; a missing copy is fine.
func removeCopy(m treeManifest) error {
	dir, name, err := openParentDir(m.Path)
	if err != nil {
		return err
	}
	defer unix.Close(dir)
	s, ok, err := matchTree(dir, name, m.Path, m.Entries, true, scanOpts{lockBases: []string{m.Path}, specials: true})
	if errors.Is(err, errNoEntry) {
		return nil
	}
	if err != nil {
		return err
	}
	if !ok || s.entries[0].dev != m.Dev {
		return fmt.Errorf("%s changed after the copy; it is left as it is", m.Path)
	}
	return removeAway(dir, name, m.Path, s.entries[0].node)
}

// treeEntry is one removed entry as the journal keeps it.
type treeEntry struct {
	Rel  string `json:"rel"`
	Kind string `json:"kind"`
	UID  int    `json:"uid"`
	GID  int    `json:"gid"`
	// Mode holds the permission and the special bits; it is not restored for a link.
	Mode uint32 `json:"mode"`
	// MTime is the modification time in nanoseconds since 1970.
	MTime   int64  `json:"mtime"`
	Content []byte `json:"content,omitempty"`
	Link    string `json:"link,omitempty"`
}

func checkRemoveState(st removeState) error {
	bad := errors.New("the recorded state of the removed tree is not usable")
	if checkFilePath(st.Path) != nil || st.Permanent || len(st.Entries) == 0 || len(st.Entries) > MaxFolderEntries {
		return bad
	}
	rels := make([]string, len(st.Entries))
	kinds := make([]string, len(st.Entries))
	var total int64
	for i, e := range st.Entries {
		rels[i], kinds[i] = e.Rel, e.Kind
		total += int64(len(e.Content))
		if e.UID < 0 || e.GID < 0 || e.Mode > 0o7777 || (e.Kind != kindFile && len(e.Content) > 0) ||
			(e.Kind == kindLink) != (e.Link != "") || strings.ContainsRune(e.Link, 0) {
			return bad
		}
	}
	if total > MaxUndoBytes || checkTreeRels(st.Path, rels, kinds) != nil {
		return bad
	}
	return nil
}

// recordedManifest is the content of a recorded tree, to compare it with a tree found at its path.
func recordedManifest(st removeState) []manifestEntry {
	out := make([]manifestEntry, len(st.Entries))
	for i, e := range st.Entries {
		out[i] = manifestEntry{Rel: e.Rel, Kind: e.Kind, Link: e.Link}
		if e.Kind == kindFile {
			out[i].Size, out[i].SHA256 = int64(len(e.Content)), sha256Hex(e.Content)
		}
	}
	return out
}

// restoreTree creates the removed tree again, complete or not at all, and only while its path is free. A tree
// at the path that already holds the recorded content is left as it is: that is a removal that did not start.
func restoreTree(st removeState, ops fileOps) error {
	dir, name, err := openParentDir(st.Path)
	if err != nil {
		return err
	}
	defer unix.Close(dir)
	_, same, err := matchTree(dir, name, st.Path, recordedManifest(st), false, scanOpts{specials: true})
	switch {
	case err == nil && same:
		return nil
	case err == nil:
		return fmt.Errorf("%s exists; it is not overwritten", st.Path)
	case !errors.Is(err, errNoEntry):
		return err
	}
	tmp, err := tempName()
	if err != nil {
		return err
	}
	b := builder{ops: ops, path: st.Path, entries: st.Entries, children: map[string][]int{}}
	for i, e := range st.Entries[1:] {
		parent := ""
		if j := strings.LastIndexByte(e.Rel, '/'); j >= 0 {
			parent = e.Rel[:j]
		}
		b.children[parent] = append(b.children[parent], i+1)
	}
	err = b.build(dir, tmp, 0)
	if err == nil {
		if err = unix.Renameat2(dir, tmp, dir, name, unix.RENAME_NOREPLACE); errors.Is(err, unix.EEXIST) {
			err = fmt.Errorf("%s exists; it is not overwritten", st.Path)
		}
	}
	if err != nil {
		hidden := joinPath(filepath.Dir(st.Path), tmp)
		if cerr := removeOwn(dir, tmp, hidden); cerr != nil {
			err = fmt.Errorf("%w; the partial tree %s is left: %v", err, hidden, cerr)
		}
		return fmt.Errorf("restore %s: %w", st.Path, err)
	}
	return unix.Fsync(dir)
}

// builder creates a recorded tree below a folder that only root can enter until it is complete.
type builder struct {
	ops      fileOps
	path     string
	entries  []treeEntry
	children map[string][]int
}

func (b builder) build(dir int, name string, i int) error {
	e := b.entries[i]
	path := joinPath(b.path, e.Rel)
	mtime := unix.NsecToTimespec(e.MTime)
	times := []unix.Timeval{unix.NsecToTimeval(e.MTime), unix.NsecToTimeval(e.MTime)}
	switch e.Kind {
	case kindFile:
		fd, err := unix.Openat(dir, name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
		if err != nil {
			return fmt.Errorf("create %s: %w", path, err)
		}
		defer unix.Close(fd)
		if err := writeAll(fd, e.Content); err != nil {
			return err
		}
		return b.finish(fd, path, e, times)
	case kindLink:
		if err := unix.Symlinkat(e.Link, dir, name); err != nil {
			return fmt.Errorf("create the link %s: %w", path, err)
		}
		if err := b.ops.chownLink(dir, name, e.UID, e.GID); err != nil {
			return fmt.Errorf("restore the owner of %s: %w", path, err)
		}
		return unix.UtimesNanoAt(dir, name, []unix.Timespec{mtime, mtime}, unix.AT_SYMLINK_NOFOLLOW)
	case kindDir:
		if err := unix.Mkdirat(dir, name, 0o700); err != nil {
			return fmt.Errorf("create %s: %w", path, err)
		}
		fd, err := openDirAt(dir, name, path)
		if err != nil {
			return err
		}
		defer unix.Close(fd)
		for _, c := range b.children[e.Rel] {
			if err := b.build(fd, filepath.Base(b.entries[c].Rel), c); err != nil {
				return err
			}
		}
		return b.finish(fd, path, e, times)
	}
	return fmt.Errorf("unknown kind of entry %q", e.Kind)
}

// finish sets owner, then mode, then time: a change of owner clears setuid and setgid, and the entries made
// below a folder change its time.
func (b builder) finish(fd int, path string, e treeEntry, times []unix.Timeval) error {
	if err := b.ops.chown(fd, e.UID, e.GID); err != nil {
		return fmt.Errorf("restore the owner of %s: %w", path, err)
	}
	if err := unix.Fchmod(fd, e.Mode); err != nil {
		return fmt.Errorf("restore the permissions of %s: %w", path, err)
	}
	return unix.Futimes(fd, times)
}
