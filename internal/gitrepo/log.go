package gitrepo

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// MaxLogPaths bounds the changed paths Log reports per commit.
const MaxLogPaths = 100

// ErrCommitNotFound means a commit is not on the first-parent history of HEAD, or does not exist or is ambiguous.
var ErrCommitNotFound = errors.New("the commit is not in the history of the working copy")

// ErrBadCommitID means a commit argument is not a lower-case hexadecimal id of 7 to 64 digits.
var ErrBadCommitID = errors.New("the commit must be a hexadecimal commit id of at least 7 digits")

// LogKind is how a commit changed a path against its first parent.
type LogKind string

const (
	LogNew      LogKind = "new"
	LogModified LogKind = "modified"
	LogDeleted  LogKind = "deleted"
)

// LogPath is one changed path of a commit.
type LogPath struct {
	Kind LogKind
	Path string
}

// Commit is one entry of Log. Author, Subject and Paths are foreign, unmasked data.
type Commit struct {
	ID, Author, Subject string
	Date                time.Time
	Paths               []LogPath
	PathsTruncated      bool // more than MaxLogPaths paths changed, or the output limit cut the list
}

var shortIDRe = regexp.MustCompile(`^[0-9a-f]{7,64}$`)

// Log returns at most limit commits of the first-parent history of HEAD, newest first, each with its changed
// paths against the first parent (all paths new for the root commit; a merge is shown against its first
// parent). A rename is a deleted and a new path. truncated says the output limit of git ended the list
// early. A repository without a commit has an empty history.
func (r *Repo) Log(ctx context.Context, limit int) (commits []Commit, truncated bool, err error) {
	if limit < 1 {
		return nil, false, errors.New("limit must be at least 1")
	}
	if _, err := r.Head(ctx); err != nil {
		return nil, false, nil
	}
	out, err := r.runIO(ctx, true, maxStdout+1, nil, ioOptions{truncate: true},
		"--literal-pathspecs", "log", "--first-parent", "--no-show-signature", "--no-ext-diff", "--no-textconv",
		"--no-color", "--no-renames", "--ignore-submodules=none", "--root", "-m", "--diff-merges=first-parent",
		"--raw", "-z", fmt.Sprintf("--max-count=%d", limit), "--format=%H%x00%an%x00%aI%x00%s", "HEAD", "--")
	if err != nil {
		return nil, false, fmt.Errorf("log failed: %w", err)
	}
	truncated = len(out) > maxStdout
	commits = parseLog(strings.Split(out, "\x00"), truncated)
	return commits, truncated, nil
}

// parseLog reads the NUL-separated tokens of Log: id, author, date, subject, then per path a raw entry and the
// path. Git puts a newline before the first raw entry, which is no part of any field. With cut, the last token
// may be partial and is dropped, together with a commit whose header is incomplete.
func parseLog(tokens []string, cut bool) []Commit {
	if cut {
		tokens = tokens[:len(tokens)-1]
	}
	var commits []Commit
	for i := 0; i < len(tokens); {
		id := strings.TrimPrefix(tokens[i], "\n")
		if !hexRe.MatchString(id) || i+3 >= len(tokens) {
			break
		}
		date, _ := time.Parse(time.RFC3339, tokens[i+2])
		c := Commit{ID: id, Author: tokens[i+1], Date: date, Subject: tokens[i+3]}
		i += 4
		for i+1 < len(tokens) && strings.HasPrefix(strings.TrimPrefix(tokens[i], "\n"), ":") {
			meta := strings.Fields(tokens[i])
			if len(meta) == 5 && len(c.Paths) < MaxLogPaths {
				c.Paths = append(c.Paths, LogPath{Kind: logKind(meta[4]), Path: tokens[i+1]})
			} else if len(meta) == 5 {
				c.PathsTruncated = true
			}
			i += 2
		}
		commits = append(commits, c)
	}
	if cut && len(commits) > 0 {
		commits[len(commits)-1].PathsTruncated = true
	}
	return commits
}

func logKind(status string) LogKind {
	switch status[0] {
	case 'A':
		return LogNew
	case 'D':
		return LogDeleted
	}
	return LogModified
}

// Show returns the unified diff of one commit against its first parent, for the root commit against nothing.
// commit is a lower-case hexadecimal id of 7 to 64 digits (ErrBadCommitID otherwise) that must be on the
// first-parent history of HEAD (ErrCommitNotFound otherwise); a short id must be unambiguous. The text is cut
// at MaxDiffBytes, on a character boundary, and truncated says so. No external diff program or text
// conversion runs.
func (r *Repo) Show(ctx context.Context, commit string) (text string, truncated bool, err error) {
	if !shortIDRe.MatchString(commit) {
		return "", false, ErrBadCommitID
	}
	id, err := r.revParse(ctx, commit+"^{commit}")
	if err != nil {
		return "", false, ErrCommitNotFound
	}
	if err := r.onFirstParentChain(ctx, id); err != nil {
		return "", false, err
	}
	out, err := r.runIO(ctx, true, MaxDiffBytes+1, nil, ioOptions{truncate: true},
		"--literal-pathspecs", "diff-tree", "-p", "--root", "--no-commit-id", "-m", "--diff-merges=first-parent",
		"--no-ext-diff", "--no-textconv", "--no-color", "--no-renames", "--ignore-submodules=none", id)
	if err != nil {
		return "", false, fmt.Errorf("show failed: %w", err)
	}
	text, truncated = cutDiff(out)
	return text, truncated, nil
}

// onFirstParentChain reports ErrCommitNotFound unless id is on the first-parent history of HEAD. The ids of
// that history are listed whole: the list is bounded by maxStdout, far beyond a stack folder's history.
func (r *Repo) onFirstParentChain(ctx context.Context, id string) error {
	out, err := r.run(ctx, true, maxStdout, "rev-list", "--first-parent", "HEAD", "--")
	if err != nil {
		return fmt.Errorf("history failed: %w", err)
	}
	for _, line := range strings.Split(out, "\n") {
		if line == id {
			return nil
		}
	}
	return ErrCommitNotFound
}
