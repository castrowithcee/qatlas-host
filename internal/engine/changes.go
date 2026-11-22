package engine

import (
	"context"
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/castrowithcee/qatlas-host/internal/gitrepo"
	"github.com/castrowithcee/qatlas-host/internal/stacks"
)

// ErrNotChanged means a path is none of the local changes that count (see classify).
var ErrNotChanged = errors.New("the path is not a local change of the stack folder")

// The kinds of a Change.
const (
	ChangeModified = "modified"
	ChangeNew      = "new"
	ChangeDeleted  = "deleted"
)

// Change is one local change of the stack folder. Path is relative to the stack folder, the root of the working
// copy. Stack names the stack the change lies in, empty for a folder that is no stack, such as one whose compose
// file was deleted.
type Change struct {
	Path  string
	Kind  string
	Stack string
}

// counted is a local change that counts with the stack that owns its folder, empty when there is none.
type counted struct {
	gitrepo.Change
	stack string
}

// countedAll returns every local change that counts, for all folders, sorted by path: the union of what
// classify counts per stack folder, so the same set the finding local-changes is built from. The caller holds
// the working-copy lock.
func (e *Engine) countedAll(ctx context.Context, b Binding) ([]counted, []stacks.Stack, error) {
	status, err := b.Copy.Status(ctx)
	if err != nil {
		return nil, nil, err
	}
	list, err := stacks.Load(b.Dir)
	if err != nil {
		return nil, nil, fmt.Errorf("read stacks: %w", err)
	}
	byFolder, _ := countedChanges(status, list)
	var out []counted
	for folder, cs := range byFolder {
		owner := ""
		if i := slices.IndexFunc(list, func(s stacks.Stack) bool { return s.Folder == folder }); i >= 0 {
			owner = list[i].Name
		}
		for _, c := range cs {
			out = append(out, counted{c, owner})
		}
	}
	slices.SortFunc(out, func(a, b counted) int { return strings.Compare(a.Path, b.Path) })
	return out, list, nil
}

// cleanPath accepts only a clean relative path that stays below the folder.
func cleanPath(rel string) error {
	if rel == "" || path.IsAbs(rel) || path.Clean(rel) != rel || rel == ".." || strings.HasPrefix(rel, "../") ||
		strings.ContainsRune(rel, 0) {
		return ErrNotChanged
	}
	return nil
}

func kindOf(code string) string {
	switch {
	case gitrepo.IsNew(code):
		return ChangeNew
	case gitrepo.IsDeleted(code):
		return ChangeDeleted
	}
	return ChangeModified
}

// Changes lists the local changes of the working copy that keep a stack out of a run, sorted by path. The files
// the host owns itself (secrets.env, data/, untracked files below a bind mount) are not among them.
func (e *Engine) Changes(ctx context.Context) ([]Change, error) {
	b, err := e.cfg.Source()
	if err != nil {
		return nil, err
	}
	if err := e.lockCommand(ctx); err != nil {
		return nil, err
	}
	defer e.lock.Unlock()
	all, _, err := e.countedAll(ctx, b)
	if err != nil {
		return nil, err
	}
	out := make([]Change, 0, len(all))
	for _, c := range all {
		out = append(out, Change{Path: c.Path, Kind: kindOf(c.Code), Stack: c.stack})
	}
	return out, nil
}

// find returns the counted change at the path, ErrNotChanged when none is it.
func find(all []counted, rel string) (counted, error) {
	if err := cleanPath(rel); err != nil {
		return counted{}, err
	}
	i := slices.IndexFunc(all, func(c counted) bool { return c.Path == rel })
	if i < 0 {
		return counted{}, ErrNotChanged
	}
	return all[i], nil
}

// Diff returns the diff of one local change, cut where gitrepo.Diff cuts it. rel is a path below the stack
// folder and must be one Changes lists.
func (e *Engine) Diff(ctx context.Context, rel string) (text string, truncated bool, err error) {
	b, err := e.cfg.Source()
	if err != nil {
		return "", false, err
	}
	if err := e.lockCommand(ctx); err != nil {
		return "", false, err
	}
	defer e.lock.Unlock()
	all, _, err := e.countedAll(ctx, b)
	if err != nil {
		return "", false, err
	}
	c, err := find(all, rel)
	if err != nil {
		return "", false, err
	}
	return b.Copy.Diff(ctx, c.Path)
}

// Discard takes the local changes at the paths back (see gitrepo.Repo.Discard). Every path must be one Changes
// lists, and nothing is changed unless all are. It holds the run slot of every stack the paths lie in, so it
// never overlaps a run of one of them, and it wakes the poll so that the local-changes findings are judged again
// at once.
func (e *Engine) Discard(ctx context.Context, rels []string) error {
	b, err := e.cfg.Source()
	if err != nil {
		return err
	}
	if err := e.lockCommand(ctx); err != nil {
		return err
	}
	defer e.lock.Unlock()
	all, _, err := e.countedAll(ctx, b)
	if err != nil {
		return err
	}
	paths := make([]string, 0, len(rels))
	var owners []string
	for _, rel := range rels {
		c, err := find(all, rel)
		if err != nil {
			return err
		}
		paths = append(paths, c.Path)
		if c.stack != "" && !slices.Contains(owners, c.stack) {
			owners = append(owners, c.stack)
		}
	}
	slices.Sort(owners)
	for _, name := range owners {
		release, err := e.claim(name, "discard")
		if err != nil {
			return err
		}
		defer release()
	}
	if err := b.Copy.Discard(ctx, paths); err != nil {
		if errors.Is(err, gitrepo.ErrNotChanged) {
			return ErrNotChanged
		}
		return err
	}
	e.Wake()
	return nil
}
