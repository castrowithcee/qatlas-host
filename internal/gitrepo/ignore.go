package gitrepo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/castrowithcee/qatlas-host/internal/stacks"
)

// IgnoreName is the ignore file of the folder root that keeps what the repository leaves out out of git, also in
// every clone.
const IgnoreName = ".gitignore"

// The marker lines of the block of qhost in IgnoreName.
const (
	ignoreBegin = "# BEGIN qhost: left out when the folder was taken over"
	ignoreEnd   = "# END qhost"
)

// IgnoreError means the IgnoreName of the folder cannot be read or extended safely.
type IgnoreError struct{ Problem string }

func (e *IgnoreError) Error() string { return IgnoreName + " of the folder " + e.Problem }

// IgnorePlan is what a repository started in a folder writes to its IgnoreName. Write says the file is created
// (Exists false) or changed; without it the file stays as it is, or the folder keeps having none. Added are the
// lines that go into the block of qhost, one anchored path each; they are data of the folder.
type IgnorePlan struct {
	Write, Exists bool
	Added         []string
	old, content  []byte
	exec          bool
}

// PlanIgnore works out the IgnoreName of dir for excluded, the paths relative to dir that the repository leaves
// out. Each becomes a pattern that matches exactly that path, so a new file is never hidden. The patterns go
// into one block between marker lines: an existing file keeps its own lines and gets the block at its end, a
// block an earlier takeover left is extended and never repeated, and a folder without the file gets one. With
// nothing to add nothing is written, and no file is created. A file that is a link, not a regular file, cannot
// be read or is larger than stacks.MaxFileSize gives an *IgnoreError. Nothing is written here.
func PlanIgnore(dir string, excluded []string) (IgnorePlan, error) {
	var p IgnorePlan
	name := filepath.Join(dir, IgnoreName)
	info, err := os.Lstat(name)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return p, &IgnoreError{"cannot be inspected"}
	case info.Mode()&fs.ModeSymlink != 0:
		return p, &IgnoreError{"is a symbolic link"}
	case !info.Mode().IsRegular():
		return p, &IgnoreError{"is not a regular file"}
	default:
		f, err := os.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
		if err != nil {
			return p, &IgnoreError{"cannot be read"}
		}
		p.old, err = readIgnore(f)
		_ = f.Close()
		if err != nil {
			return p, err
		}
		p.Exists, p.exec = true, info.Mode()&0o111 != 0
	}
	start, end, block, err := ignoreBlock(p.old)
	if err != nil {
		return p, err
	}
	have := make(map[string]bool, len(block))
	for _, l := range block {
		have[l] = true
	}
	for _, e := range excluded {
		pat, ok := excludePattern(e)
		if !ok {
			return p, fmt.Errorf("%q cannot be written as an ignore pattern", e)
		}
		if !have[pat] {
			have[pat] = true
			p.Added = append(p.Added, pat)
		}
	}
	if len(p.Added) == 0 {
		return p, nil
	}
	text := ignoreBegin + "\n" + strings.Join(append(block, p.Added...), "\n") + "\n" + ignoreEnd + "\n"
	if start < 0 {
		start, end = len(p.old), len(p.old)
		if start > 0 && p.old[start-1] != '\n' {
			text = "\n" + text
		}
	}
	p.content = append(append(append([]byte{}, p.old[:start]...), text...), p.old[end:]...)
	if len(p.content) > stacks.MaxFileSize {
		return p, &IgnoreError{fmt.Sprintf("would grow larger than %d bytes", stacks.MaxFileSize)}
	}
	p.Write = true
	return p, nil
}

func readIgnore(f *os.File) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(f, stacks.MaxFileSize+1))
	switch {
	case err != nil:
		return nil, &IgnoreError{"cannot be read"}
	case len(b) > stacks.MaxFileSize:
		return nil, &IgnoreError{fmt.Sprintf("is larger than %d bytes", stacks.MaxFileSize)}
	}
	return b, nil
}

// ignoreBlock finds the block of qhost in content: the byte range from the start of its first marker line to the
// end of its last, and the lines between, or a start of -1 when there is none. A marker without its partner or a
// second block is refused, so the block is never doubled or cut.
func ignoreBlock(content []byte) (start, end int, lines []string, err error) {
	start, end = -1, -1
	damaged := &IgnoreError{fmt.Sprintf("holds a damaged block of qhost; remove the lines from %q to %q", ignoreBegin, ignoreEnd)}
	off := 0
	for off < len(content) {
		next := len(content)
		if i := bytes.IndexByte(content[off:], '\n'); i >= 0 {
			next = off + i + 1
		}
		line := strings.TrimRight(string(content[off:next]), "\r\n")
		switch {
		case line == ignoreBegin:
			if start >= 0 {
				return 0, 0, nil, damaged
			}
			start = off
		case line == ignoreEnd:
			if start < 0 || end >= 0 {
				return 0, 0, nil, damaged
			}
			end = next
		case start >= 0 && end < 0 && line != "":
			lines = append(lines, line)
		}
		off = next
	}
	if start >= 0 && end < 0 {
		return 0, 0, nil, damaged
	}
	return start, end, lines, nil
}

// stageIgnore puts the planned content of IgnoreName into the index that env selects, in place of what the
// folder holds, so the tree has the file before it is written.
func (r *Repo) stageIgnore(ctx context.Context, env []string, p IgnorePlan) error {
	if !p.Write {
		return nil
	}
	out, err := r.runIO(ctx, true, maxErrorText, env, ioOptions{stdin: bytes.NewReader(p.content)}, "hash-object", "-w", "--stdin", "--no-filters")
	blob := strings.TrimSpace(out)
	if err != nil {
		return fmt.Errorf("hash-object failed: %w", err)
	}
	if !hexRe.MatchString(blob) {
		return errors.New("hash-object returned no object id")
	}
	mode := "100644"
	if p.exec {
		mode = "100755"
	}
	if _, err := r.gitIn(ctx, env)("update-index", "--add", "--cacheinfo", mode+","+blob+","+IgnoreName); err != nil {
		return err
	}
	return nil
}

// putIgnore writes the planned IgnoreName, which the commit holds already, into the folder and lets the index
// know it, so the work tree is clean. The file must still be what PlanIgnore read; on any failure it is left
// as it was.
func (r *Repo) putIgnore(ctx context.Context, p IgnorePlan) error {
	if !p.Write {
		return nil
	}
	if err := p.write(r.dir); err != nil {
		return err
	}
	if _, err := r.run(ctx, true, maxErrorText, "update-index", "-q", "--refresh"); err != nil {
		_ = p.restore(r.dir)
		return fmt.Errorf("update-index failed: %w", err)
	}
	return nil
}

// write puts the planned content into the file in place, so its owner, group and mode stay.
func (p IgnorePlan) write(dir string) error {
	name := filepath.Join(dir, IgnoreName)
	if !p.Exists {
		f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0o664)
		if err != nil {
			return fmt.Errorf("create %s: %w", IgnoreName, err)
		}
		_, err = f.Write(p.content)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			_ = os.Remove(name)
			return fmt.Errorf("write %s: %w", IgnoreName, err)
		}
		return nil
	}
	f, err := os.OpenFile(name, os.O_RDWR|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return fmt.Errorf("open %s: %w", IgnoreName, err)
	}
	defer f.Close()
	if info, err := f.Stat(); err != nil || !info.Mode().IsRegular() {
		return &IgnoreError{"is not a regular file"}
	}
	now, err := readIgnore(f)
	if err != nil {
		return err
	}
	if !bytes.Equal(now, p.old) {
		return &IgnoreError{"changed since it was read"}
	}
	if err := writeAt(f, p.content); err != nil {
		_ = writeAt(f, p.old)
		return fmt.Errorf("write %s: %w", IgnoreName, err)
	}
	return nil
}

func writeAt(f *os.File, b []byte) error {
	if err := f.Truncate(0); err != nil {
		return err
	}
	_, err := f.WriteAt(b, 0)
	return err
}

// restore puts back what the folder held before write.
func (p IgnorePlan) restore(dir string) error {
	name := filepath.Join(dir, IgnoreName)
	if !p.Exists {
		return os.Remove(name)
	}
	f, err := os.OpenFile(name, os.O_WRONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	err = writeAt(f, p.old)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// checkNotExcluded refuses a file that lies at or below an excluded path: the ignore rules would hide it.
func checkNotExcluded(files, excluded []string) error {
	out := make(map[string]bool, len(excluded))
	for _, e := range excluded {
		out[e] = true
	}
	for _, f := range files {
		for p := f; p != "." && p != "/"; p = path.Dir(p) {
			if out[p] {
				return fmt.Errorf("%q is listed and left out at the same time", f)
			}
		}
	}
	return nil
}
