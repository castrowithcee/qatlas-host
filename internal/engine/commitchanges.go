package engine

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/castrowithcee/qatlas-host/internal/gitrepo"
	"github.com/castrowithcee/qatlas-host/internal/stacks"
)

// confirmed matches the paths against the local changes that count and returns the paths with their kinds and
// the stacks they lie in. Every path must be exactly one change (ErrNotChanged), and every stack among them
// must load (ErrUnreadable); a folder that is no stack, such as one whose compose file was deleted, has no
// stack to check.
func (e *Engine) confirmed(all []counted, list []stacks.Stack, rels []string) (kinds map[string]string, owners []string, err error) {
	kinds = map[string]string{}
	for _, rel := range rels {
		c, err := find(all, rel)
		if err != nil {
			return nil, nil, err
		}
		kinds[c.Path] = kindOf(c.Code)
		if c.stack != "" && !slices.Contains(owners, c.stack) {
			owners = append(owners, c.stack)
		}
	}
	slices.Sort(owners)
	for _, name := range owners {
		if i := slices.IndexFunc(list, func(s stacks.Stack) bool { return s.Name == name }); list[i].Err != nil {
			return nil, nil, fmt.Errorf("%w: %s: %s", ErrUnreadable, name, e.text(list[i].Err.Error()))
		}
	}
	return kinds, owners, nil
}

// CommitChanges commits exactly the local changes at rels (see gitrepo.Repo.CommitPaths), with who named in the
// message, and wakes the poll so that the stacks of the commit run at once. expectHead is the commit the
// changes were read at: a head that is another one is refused with gitrepo.ErrHeadMoved. Every path must be a
// change Changes lists now (ErrNotChanged), and every stack among them must still load (ErrUnreadable). Like
// Discard it holds the run slot of each of these stacks and refuses with ErrRunActive. It returns the commit.
//
// A push the remote rejects because its branch moved is repeated once: the working copy catches up with the
// remote, which git refuses when a change of the paths lies in its way, and the paths must still be exactly the
// same changes of stacks that still load. The expected head of the caller then gives way to the new remote
// head, because what the caller confirmed are these changes and not the commits of other people; anything else
// gives gitrepo.ErrPushRejected.
func (e *Engine) CommitChanges(ctx context.Context, expectHead string, rels []string, who string) (string, error) {
	b, err := e.cfg.Source()
	if err != nil {
		return "", err
	}
	if err := e.lockCommand(ctx); err != nil {
		return "", err
	}
	defer e.lock.Unlock()
	e.refs.Lock()
	defer e.refs.Unlock()
	wc := b.Copy

	var first map[string]string
	for attempt := 1; ; attempt++ {
		all, list, err := e.countedAll(ctx, b)
		if err != nil {
			return "", err
		}
		kinds, owners, err := e.confirmed(all, list, rels)
		if err != nil {
			return "", err
		}
		if attempt == 1 {
			first = kinds
			for _, name := range owners {
				release, err := e.claim(name, "commit")
				if err != nil {
					return "", err
				}
				defer release()
			}
		} else if !maps.Equal(first, kinds) {
			return "", gitrepo.ErrPushRejected
		}
		paths := slices.Sorted(maps.Keys(kinds))
		message := fmt.Sprintf("Commit %s by %s", strings.Join(paths, ", "), who)
		c, err := wc.CommitPaths(ctx, expectHead, paths, list, message)
		switch {
		case err == nil:
			e.cfg.Logger.Info("stack changes committed", "commit", c, "files", len(paths))
			e.Wake()
			return c, nil
		case errors.Is(err, gitrepo.ErrPushRejected) && attempt == 1:
			if expectHead, err = e.catchUp(ctx, wc); err != nil {
				return "", err
			}
		default:
			return "", err
		}
	}
}

// catchUp fetches and fast-forwards the working copy, which has local changes, to the remote branch and returns
// its head. A remote that is not ahead of the working copy, or an update that would overwrite a change, gives
// gitrepo.ErrPushRejected.
func (e *Engine) catchUp(ctx context.Context, wc WorkingCopy) (string, error) {
	if err := wc.Fetch(ctx); err != nil {
		return "", fmt.Errorf("fetch: %w", err)
	}
	head, err := wc.Head(ctx)
	if err != nil {
		return "", err
	}
	remote, err := wc.RemoteHead(ctx)
	if err != nil {
		return "", err
	}
	if ok, err := wc.IsAncestor(ctx, head, remote); err != nil || !ok || head == remote {
		return "", gitrepo.ErrPushRejected
	}
	if err := wc.FastForward(ctx, remote); err != nil {
		return "", gitrepo.ErrPushRejected
	}
	return remote, nil
}
