package gitrepo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/castrowithcee/qatlas-host/internal/stacks"
)

const maxPathsMessageRunes = 1000

var (
	// ErrHeadMoved means HEAD is not the commit the caller expected.
	ErrHeadMoved = errors.New("the branch moved since the changes were read")
	// ErrExcluded means a path is one the host keeps out of every commit (see ChangeOwner).
	ErrExcluded = errors.New("the path is never committed")
	// ErrNotRegular means a changed path is not a regular file or a deletion.
	ErrNotRegular = errors.New("the path is not a regular file")
)

// ChangeOwner maps a status entry to the stack folder it lies in (stacks.RootFolder for a root stack) and
// reports whether it is the host's own and so never committed: <stack>/secrets.env, anything below
// <stack>/data/, and an untracked file below a bind mount of the stack in list (Stack.BindMounts). A changed
// tracked file below a bind mount is not the host's own. inStack is false for a path outside every stack; a
// root stack holds every path but those below stacks.SettingsTop.
func ChangeOwner(c Change, list []stacks.Stack) (folder string, inStack, own bool) {
	mounts := map[string][]string{}
	root := false
	for _, st := range list {
		mounts[st.Folder] = st.BindMounts
		root = root || st.Folder == stacks.RootFolder
	}
	folder, rel, inStack := strings.Cut(c.Path, "/")
	if root {
		folder, rel, inStack = stacks.RootFolder, c.Path, folder != stacks.SettingsTop
	}
	if inStack && (rel == stacks.SecretsFile || strings.HasPrefix(rel, "data/")) {
		return folder, inStack, true
	}
	if inStack && c.Code == "??" && slices.ContainsFunc(mounts[folder], func(m string) bool {
		return rel == m || strings.HasPrefix(rel, m+"/")
	}) {
		return folder, inStack, true
	}
	return folder, inStack, false
}

// CommitPaths commits exactly paths, as the working copy has them now, on top of HEAD, and moves the
// checked-out branch to the commit: a new or changed regular file is taken with its content and executable bit,
// a file that is gone is deleted, and no other path changes. The commit is built in a temporary index, so the
// real index and the working copy keep their content; afterwards the index entries of paths follow the new
// HEAD, and staged changes elsewhere stay staged.
//
// expectHead is the full commit id the caller read the changes at (ErrHeadMoved otherwise). Every path must be
// exactly the path of an entry of Status (ErrNotChanged), not an unresolved merge (ErrUnmerged) and, judged
// against list, not the host's own (ErrExcluded); nothing happens unless all of them pass. message is
// sanitized and bounded here, and the author is the one of every qhost commit. Paths that leave the tree of
// HEAD as it is give ErrUnchanged.
//
// With a remote, the commit is scanned like PushBranch does (*SecretsError) and then pushed as a plain
// fast-forward, never forced: ErrPushRejected when the remote branch moved, *RemoteRejectedError when the
// remote refuses the update. The branch moves only after the push succeeded. In local mode the branch moves in
// place of the push. It returns the new commit id.
func (r *Repo) CommitPaths(ctx context.Context, expectHead string, paths []string, list []stacks.Stack, message string) (string, error) {
	if !hexRe.MatchString(expectHead) {
		return "", errors.New("commit must be a full lowercase commit id")
	}
	if message = sanitizeLine(message, maxPathsMessageRunes); message == "" {
		return "", errors.New("the commit message is empty")
	}
	branch, err := r.branch(ctx)
	if err != nil {
		return "", err
	}
	head, err := r.Head(ctx)
	if err != nil {
		return "", err
	}
	if head != expectHead {
		return "", ErrHeadMoved
	}
	changes, err := r.Status(ctx)
	if err != nil {
		return "", err
	}
	paths = slices.Compact(slices.Sorted(slices.Values(paths)))
	if len(paths) == 0 {
		return "", errors.New("there is no path to commit")
	}
	for _, p := range paths {
		if err := checkRepoPath(p); err != nil || strings.Contains("/"+p+"/", "/.git/") {
			return "", errors.New("the path must be a clean path of a file in the working copy")
		}
		i := slices.IndexFunc(changes, func(c Change) bool { return c.Path == p })
		switch {
		case i < 0:
			return "", ErrNotChanged
		case isUnmerged(changes[i].Code):
			return "", ErrUnmerged
		}
		if _, _, own := ChangeOwner(changes[i], list); own {
			return "", ErrExcluded
		}
	}

	commit, err := r.buildPathsCommit(ctx, head, paths, message)
	if err != nil {
		return "", err
	}
	if r.cfg.Local {
		err = r.moveBranch(ctx, branch, commit, head, paths)
	} else if err = r.scanUnpublished(ctx, branch, commit); err == nil {
		if err = r.Push(ctx, commit); err == nil {
			err = r.moveBranch(ctx, branch, commit, head, paths)
		}
	}
	if err != nil {
		return "", err
	}
	return commit, nil
}

// buildPathsCommit makes the commit on top of head with the working-copy state of paths, in a temporary index.
func (r *Repo) buildPathsCommit(ctx context.Context, head string, paths []string, message string) (string, error) {
	dir, err := os.MkdirTemp("", "qhost-commit-*")
	if err != nil {
		return "", fmt.Errorf("create temporary directory: %w", err)
	}
	defer os.RemoveAll(dir)
	git := r.gitIn(ctx, append([]string{"GIT_INDEX_FILE=" + filepath.Join(dir, "index")}, identityEnv()...))
	if _, err := git("read-tree", head); err != nil {
		return "", err
	}
	for _, p := range paths {
		info, err := os.Lstat(filepath.Join(r.dir, filepath.FromSlash(p)))
		switch {
		case errors.Is(err, os.ErrNotExist):
			_, err = git("update-index", "--force-remove", "--", p)
		case err != nil:
		case !info.Mode().IsRegular():
			return "", fmt.Errorf("%w: %s", ErrNotRegular, p)
		default:
			mode := "100644"
			if info.Mode()&0o111 != 0 {
				mode = "100755"
			}
			// --no-filters: the bytes are stored as the file has them, whatever the attributes say.
			var id string
			if id, err = git("hash-object", "-w", "--no-filters", "--", p); err == nil {
				_, err = git("update-index", "--add", "--cacheinfo", mode+","+id+","+p)
			}
		}
		if err != nil {
			return "", err
		}
	}
	tree, err := git("write-tree")
	if err != nil {
		return "", err
	}
	if headTree, err := r.revParse(ctx, head+"^{tree}"); err != nil {
		return "", err
	} else if tree == headTree {
		return "", ErrUnchanged
	}
	commit, err := git("commit-tree", tree, "-p", head, "-m", message)
	if err != nil {
		return "", err
	}
	if !hexRe.MatchString(commit) {
		return "", errors.New("commit-tree returned no commit id")
	}
	return commit, nil
}

// moveBranch moves the branch from old to commit, which has old as its parent, and lets the index entries of
// paths follow; the working copy already has their content.
func (r *Repo) moveBranch(ctx context.Context, branch, commit, old string, paths []string) error {
	if _, err := r.run(ctx, true, maxErrorText, "update-ref", "-m", "qhost: commit paths", "refs/heads/"+branch, commit, old); err != nil {
		if now, herr := r.Head(ctx); herr == nil && now != old {
			return ErrHeadMoved
		}
		return fmt.Errorf("move the branch failed: %w", err)
	}
	args := append([]string{"--literal-pathspecs", "reset", "-q", "--"}, paths...)
	if _, err := r.run(ctx, true, maxStdout, args...); err != nil {
		return fmt.Errorf("the commit is on the branch, but the index did not follow: %w", err)
	}
	return nil
}
