package gitrepo

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/castrowithcee/qatlas-host/internal/nofollow"
)

// ErrHasGit means the folder already has a .git entry.
var ErrHasGit = errors.New("the folder already has a .git entry")

const firstCommitMessage = "Import the stack files"

// Init turns the folder dir, which has no .git, into a repository without a remote: branch main,
// core.sharedRepository=group, and a first commit by qhost that holds exactly the regular files in files, paths
// relative to dir, and the IgnoreName that PlanIgnore works out for excluded, the paths relative to dir that the
// repository leaves out. That file is the only one of the folder that is created or changed, and only when
// something is left out; nothing is removed. Hooks, filters and the configuration of the caller do not apply,
// and the files are stored as they are on disk. It returns the id of the first commit. If a step fails after
// .git was created, Init removes that .git again and leaves IgnoreName as it was.
func Init(ctx context.Context, cfg Config, dir string, files, excluded []string) (string, error) {
	r, err := Open(cfg, dir)
	if err != nil {
		return "", err
	}
	ignore, err := r.checkFolder(files, excluded)
	if err != nil {
		return "", err
	}
	if err := r.claimForInit(); err != nil {
		return "", err
	}
	commit, err := r.initIn(ctx, files, ignore)
	if err != nil {
		_ = r.removeGit()
		return "", err
	}
	return commit, nil
}

// checkFolder validates the file list and the exclusions and plans the IgnoreName of the folder.
func (r *Repo) checkFolder(files, excluded []string) (IgnorePlan, error) {
	if len(files) == 0 {
		return IgnorePlan{}, errors.New("there is no file to commit")
	}
	if err := r.checkFiles(files); err != nil {
		return IgnorePlan{}, err
	}
	if err := checkNotExcluded(files, excluded); err != nil {
		return IgnorePlan{}, err
	}
	return PlanIgnore(r.dir, excluded)
}

// checkFiles requires every path to be a clean path of a regular file inside the folder and outside .git.
func (r *Repo) checkFiles(files []string) error {
	for _, f := range files {
		if err := checkRepoPath(f); err != nil || strings.Contains("/"+f+"/", "/.git/") {
			return errors.New("every file must be a clean path inside the folder and outside .git")
		}
		fi, err := os.Lstat(filepath.Join(r.dir, f))
		if err != nil || !fi.Mode().IsRegular() {
			return fmt.Errorf("%q is not a regular file", f)
		}
	}
	return nil
}

// claimGit checks that the folder has no .git. From then on the name is ours: it did not exist a moment ago.
func (r *Repo) claimGit() error {
	if _, err := os.Lstat(filepath.Join(r.dir, ".git")); err == nil {
		return ErrHasGit
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("look for .git: %w", err)
	}
	return nil
}

// claimForInit is claimGit plus the check that the folder itself is no link, before git writes anything.
func (r *Repo) claimForInit() error {
	if err := r.claimGit(); err != nil {
		return err
	}
	d, err := nofollow.Open(r.dir, "")
	if err != nil {
		return err
	}
	return d.Close()
}

// removeGit deletes the .git a failed start left; a link in its place is deleted itself and never entered.
func (r *Repo) removeGit() error {
	d, err := nofollow.Open(r.dir, "")
	if err != nil {
		return err
	}
	defer d.Close()
	return d.RemoveAll(".git")
}

func (r *Repo) initIn(ctx context.Context, files []string, ignore IgnorePlan) (string, error) {
	git, tree, err := r.stage(ctx, files, ignore, "main")
	if err != nil {
		return "", err
	}
	commit, err := git("commit-tree", tree, "-m", firstCommitMessage)
	if err != nil {
		return "", err
	}
	if !hexRe.MatchString(commit) {
		return "", errors.New("commit-tree returned no commit id")
	}
	if _, err := git("update-ref", "refs/heads/main", commit); err != nil {
		return "", err
	}
	if err := r.putIgnore(ctx, ignore); err != nil {
		return "", err
	}
	return commit, nil
}

// gitFunc runs git in the repository with literal pathspecs and returns its trimmed output.
type gitFunc func(args ...string) (string, error)

func (r *Repo) gitIn(ctx context.Context, env []string) gitFunc {
	return func(args ...string) (string, error) {
		out, err := r.runEnv(ctx, true, maxStdout, env, append([]string{"--literal-pathspecs"}, args...)...)
		if err != nil {
			return "", fmt.Errorf("%s failed: %w", args[0], err)
		}
		return strings.TrimSpace(out), nil
	}
}

// stage creates the repository in the folder on the given branch and stages exactly files and the planned
// IgnoreName, which is not written yet. It returns the tree of the index and a runner for the next steps.
func (r *Repo) stage(ctx context.Context, files []string, ignore IgnorePlan, branch string) (gitFunc, string, error) {
	// --template= keeps the host's template (hooks, sample files) out of the new repository.
	if _, err := r.run(ctx, false, maxErrorText, "init", "--quiet", "--template=", "--initial-branch="+branch,
		"--shared=group", "--", r.dir); err != nil {
		return nil, "", fmt.Errorf("init failed: %w", err)
	}
	if _, err := r.run(ctx, true, maxErrorText, "config", "--local", "core.sharedRepository", "group"); err != nil {
		return nil, "", fmt.Errorf("config failed: %w", err)
	}
	info, err := putAttributes(r.dir)
	if err != nil {
		return nil, "", err
	}
	defer info.Close()
	env := []string{
		"GIT_AUTHOR_NAME=" + commitName, "GIT_AUTHOR_EMAIL=" + commitEmail,
		"GIT_COMMITTER_NAME=" + commitName, "GIT_COMMITTER_EMAIL=" + commitEmail,
	}
	tree, err := r.stageFiles(ctx, env, files, ignore)
	if err != nil {
		return nil, "", err
	}
	if err := info.Remove("attributes"); err != nil {
		return nil, "", fmt.Errorf("remove the attribute rules: %w", err)
	}
	git := r.gitIn(ctx, env)
	return git, tree, nil
}

// putAttributes writes .git/info/attributes in the folder dir and returns the open info folder. No level from
// dir down is a link.
func putAttributes(dir string) (*nofollow.Dir, error) {
	fail := func(err error) (*nofollow.Dir, error) { return nil, fmt.Errorf("write the attribute rules: %w", err) }
	git, err := nofollow.Open(dir, ".git")
	if err != nil {
		return fail(err)
	}
	defer git.Close()
	// An attribute rule of the folder could convert line endings or run a filter; this one wins and goes away
	// with the first commit, so the blobs hold the bytes of the files.
	info, err := git.MkdirSub("info", 0o755)
	if err != nil {
		return fail(err)
	}
	f, err := info.Create("attributes", 0o644)
	if err == nil {
		_, err = f.WriteString(noFilterAttributes)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}
	if err != nil {
		_ = info.Close()
		return fail(err)
	}
	return info, nil
}

const noFilterAttributes = "* -text -filter -ident -working-tree-encoding\n"

// stageFiles adds exactly files and the planned IgnoreName to the index that env selects and returns the tree
// of that index. The paths are read from a list file, so no path is ever a command-line word.
func (r *Repo) stageFiles(ctx context.Context, env, files []string, ignore IgnorePlan) (string, error) {
	list, err := os.CreateTemp("", "qhost-init-*")
	if err != nil {
		return "", fmt.Errorf("create temporary file: %w", err)
	}
	defer os.Remove(list.Name())
	_, err = list.WriteString(strings.Join(files, "\x00") + "\x00")
	if cerr := list.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", fmt.Errorf("write the file list: %w", err)
	}
	git := r.gitIn(ctx, env)
	if _, err := git("add", "--pathspec-from-file="+list.Name(), "--pathspec-file-nul"); err != nil {
		return "", err
	}
	if err := r.stageIgnore(ctx, env, ignore); err != nil {
		return "", err
	}
	tree, err := git("write-tree")
	if err != nil {
		return "", err
	}
	if !hexRe.MatchString(tree) {
		return "", errors.New("write-tree returned no tree id")
	}
	return tree, nil
}
