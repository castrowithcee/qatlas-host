package gitrepo

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

// MaxDiffBytes bounds the text Diff returns; a longer diff is cut and reported as truncated.
const MaxDiffBytes = 256 << 10

// ErrNotChanged means a path is not one of the local changes of the working copy as the status reports them.
// Only an exact path of such a change is ever touched: no folder, no pattern, no path of an ignored or
// unchanged file.
var ErrNotChanged = errors.New("the path is not a local change of the working copy")

// ErrUnmerged means a change is an unresolved merge, which is never discarded here.
var ErrUnmerged = errors.New("the path has an unresolved merge")

// IsNew reports whether the status code Code marks a file that is not in the last commit: untracked or added.
func IsNew(code string) bool { return code == "??" || strings.HasPrefix(code, "A") }

// IsDeleted reports whether the status code marks a versioned file that is gone from the working copy or the
// index and is not a new one.
func IsDeleted(code string) bool { return !IsNew(code) && strings.Contains(code, "D") }

func isUnmerged(code string) bool {
	return strings.Contains(code, "U") || code == "AA" || code == "DD"
}

// changeOf returns the entry of the status for path.
func (r *Repo) changeOf(ctx context.Context, path string) (Change, error) {
	if err := checkRepoPath(path); err != nil {
		return Change{}, err
	}
	changes, err := r.Status(ctx)
	if err != nil {
		return Change{}, err
	}
	for _, c := range changes {
		if c.Path == path {
			return c, nil
		}
	}
	return Change{}, ErrNotChanged
}

// Diff returns the unified diff of one local change against the last commit, for an untracked file against
// nothing. path must be exactly the path of an entry of Status (ErrNotChanged otherwise). The text is cut at
// MaxDiffBytes, on a character boundary, and truncated says so. No external diff program or text conversion
// runs.
func (r *Repo) Diff(ctx context.Context, path string) (text string, truncated bool, err error) {
	c, err := r.changeOf(ctx, path)
	if err != nil {
		return "", false, err
	}
	args := []string{"--literal-pathspecs", "diff", "--no-ext-diff", "--no-textconv", "--no-color", "--no-renames",
		"--ignore-submodules=none"}
	if c.Code == "??" {
		// git diff --no-index exits 1 when the files differ, which an untracked file always does.
		args = append(args, "--no-index", "--", "/dev/null", path)
	} else {
		args = append(args, "HEAD", "--", path)
	}
	out, err := r.runIO(ctx, true, MaxDiffBytes+1, nil, ioOptions{truncate: true}, args...)
	var failure *runFailure
	if errors.As(err, &failure) && c.Code == "??" && failure.code == 1 {
		out, err = failure.stdout, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("diff failed: %w", err)
	}
	out, truncated = cutDiff(out)
	return out, truncated, nil
}

// cutDiff cuts a diff longer than MaxDiffBytes there, on a character boundary.
func cutDiff(out string) (string, bool) {
	if len(out) <= MaxDiffBytes {
		return out, false
	}
	out = out[:MaxDiffBytes]
	for !utf8.ValidString(out) && len(out) > 0 {
		out = out[:len(out)-1]
	}
	return out, true
}

// Discard takes the local changes at paths back: a modified or deleted versioned file is restored from the last
// commit, index and file alike, and a new file, untracked or added, is removed. Every path must be exactly the
// path of an entry of Status; nothing is changed unless all of them are (ErrNotChanged), and a folder, a
// pattern or an ignored file is never one of them. A path whose parent is a symbolic link is refused by git
// itself, and a link at the path is replaced or removed, never followed.
func (r *Repo) Discard(ctx context.Context, paths []string) error {
	changes, err := r.Status(ctx)
	if err != nil {
		return err
	}
	var restore, drop, clean []string
	for _, p := range slices.Compact(slices.Sorted(slices.Values(paths))) {
		if err := checkRepoPath(p); err != nil {
			return err
		}
		i := slices.IndexFunc(changes, func(c Change) bool { return c.Path == p })
		if i < 0 {
			return ErrNotChanged
		}
		switch code := changes[i].Code; {
		case isUnmerged(code):
			return ErrUnmerged
		case code == "??":
			clean = append(clean, p)
		case strings.HasPrefix(code, "A"):
			drop = append(drop, p)
		default:
			restore = append(restore, p)
		}
	}
	run := func(args []string, paths []string) error {
		if len(paths) == 0 {
			return nil
		}
		full := append([]string{"--literal-pathspecs"}, args...)
		full = append(full, "--")
		if _, err := r.run(ctx, true, maxStdout, append(full, paths...)...); err != nil {
			return fmt.Errorf("%s failed: %w", args[0], err)
		}
		return nil
	}
	if err := run([]string{"restore", "--source=HEAD", "--staged", "--worktree"}, restore); err != nil {
		return err
	}
	if err := run([]string{"rm", "-f", "-q"}, drop); err != nil {
		return err
	}
	return run([]string{"clean", "-f", "-q"}, clean)
}
