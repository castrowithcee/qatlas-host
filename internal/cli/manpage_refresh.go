package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/castrowithcee/qatlas-kit/cobracli"

	"github.com/castrowithcee/qatlas-host/internal/nofollow"
)

// refreshManpages brings the installed manpages below root to those of this program and returns the names it
// wrote. A self-update of an old release replaces only qhost.1, and the service of the host runs without root,
// so the program renews the pages from the steps that run as root. A page that is the same apart from the date in
// its header is left alone, so a repeat writes nothing. A missing manpage folder stays missing, a link in place of
// a page or of the folder is not followed and not replaced, and a page the program no longer has stays.
func refreshManpages(program *cobracli.Program, root string) ([]string, error) {
	dir, err := nofollow.Open(filepath.Join(root, filepath.Dir(filepath.Dir(manPath))), filepath.Base(filepath.Dir(manPath)))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = dir.Close() }()
	tmp, err := os.MkdirTemp("", "qhost-manpages-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	if err := writeManpages(program, tmp); err != nil {
		return nil, err
	}
	pages, err := os.ReadDir(tmp)
	if err != nil {
		return nil, err
	}
	var written []string
	var errs []error
	for _, page := range pages {
		want, err := os.ReadFile(filepath.Join(tmp, page.Name()))
		if err == nil {
			var wrote bool
			if wrote, err = installManpage(dir, page.Name(), want); wrote {
				written = append(written, page.Name())
			}
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("manpage %s: %w", page.Name(), err))
		}
	}
	sort.Strings(written)
	return written, errors.Join(errs...)
}

// installManpage writes page as name into dir unless the installed one has the same text.
func installManpage(dir *nofollow.Dir, name string, page []byte) (bool, error) {
	info, err := dir.Lstat(name)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return false, err
	case info.Mode&(fs.ModeDir|fs.ModeSymlink|fs.ModeIrregular) != 0:
		return false, errors.New("is not a regular file; left unchanged")
	default:
		f, err := dir.OpenFile(name, os.O_RDONLY, 0)
		if err != nil {
			return false, err
		}
		have, err := io.ReadAll(f)
		_ = f.Close()
		if err != nil {
			return false, err
		}
		if withoutHeader(have) == withoutHeader(page) {
			return false, nil
		}
	}
	f, err := dir.CreateTemp(".qhost-manpage-*", 0o644)
	if err != nil {
		return false, err
	}
	tmp := filepath.Base(f.Name())
	_, werr := f.Write(page)
	if cerr := f.Chmod(0o644); werr == nil {
		werr = cerr
	}
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = dir.Rename(tmp, name)
	}
	if werr != nil {
		_ = dir.Remove(tmp)
		return false, werr
	}
	return true, nil
}

// withoutHeader drops the .TH line of a page, which carries the month the page was generated in and so differs
// between two builds of the same text.
func withoutHeader(page []byte) string {
	var b bytes.Buffer
	for _, line := range bytes.SplitAfter(page, []byte("\n")) {
		if !bytes.HasPrefix(line, []byte(".TH ")) {
			b.Write(line)
		}
	}
	return b.String()
}

// reportManpages tells on w how many pages were written and warns when some could not be: manpages are no
// transition, so a failure never stops the step that called.
func reportManpages(program *cobracli.Program, root string, w io.Writer) {
	written, err := refreshManpages(program, root)
	if len(written) > 0 {
		_, _ = fmt.Fprintf(w, "manpages: updated %d\n", len(written))
	}
	if err != nil {
		_, _ = fmt.Fprintf(w, "warning: the manpages are not all up to date: %v\n", err)
	}
}
