package gitrepo

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// runIDRe is the shape of a run ID, the last component of a run tag: a UTC timestamp and a random suffix. It
// is checked in full because tag names come from the repository and are not trusted.
var runIDRe = regexp.MustCompile(`^\d{8}T\d{6}Z-[0-9a-f]{8}$`)

// PruneTags deletes old run tags of one host and stack. It collects the run IDs of the tags
// qhost/<host>/<stack>/<run id> that exist locally or, outside the local mode, in the remote, passes them oldest
// first to pick and deletes the tags of the IDs pick returns. Names of any other shape, other hosts, other
// stacks and IDs pick was not given are never touched. In the remote mode a tag is deleted in the remote first,
// by a push that is neither forced nor runs hooks, and then locally; a refusal is ErrTagRejected, that tag stays
// and nothing after it is tried. It returns the IDs deleted so far.
func (r *Repo) PruneTags(ctx context.Context, hostname, stack string, pick func(ids []string) []string) ([]string, error) {
	prefix := RunTag(hostname, stack, "")
	local, err := r.runTagIDs(ctx, prefix, false)
	if err != nil {
		return nil, err
	}
	var remote []string
	if !r.cfg.Local {
		if remote, err = r.runTagIDs(ctx, prefix, true); err != nil {
			return nil, err
		}
	}
	all := slices.Concat(local, remote)
	slices.Sort(all)
	all = slices.Compact(all)
	var deleted []string
	for _, id := range pick(slices.Clone(all)) {
		if !slices.Contains(all, id) {
			continue
		}
		ref := "refs/tags/" + prefix + id
		if slices.Contains(remote, id) {
			_, err = r.run(ctx, true, maxStdout, "push", "--porcelain", "--no-verify", "--no-recurse-submodules",
				"--", "origin", ":"+ref)
			var failure *runFailure
			if errors.As(err, &failure) {
				if reason := rejectionReason(failure.stdout); reason != "" {
					return deleted, fmt.Errorf("%w: %s", ErrTagRejected, reason)
				}
			}
			if err != nil {
				return deleted, fmt.Errorf("delete tag in the remote failed: %w", err)
			}
		}
		if slices.Contains(local, id) {
			if _, err = r.run(ctx, true, maxErrorText, "update-ref", "-d", ref); err != nil {
				return deleted, fmt.Errorf("delete tag failed: %w", err)
			}
		}
		deleted = append(deleted, id)
	}
	return deleted, nil
}

// runTagIDs returns the run IDs of the tags that start with prefix, from the remote or the local repository.
func (r *Repo) runTagIDs(ctx context.Context, prefix string, remote bool) ([]string, error) {
	var out string
	var err error
	if remote {
		out, err = r.run(ctx, true, maxStdout, "ls-remote", "--tags", "--", "origin", "refs/tags/"+TagPrefix+"*")
	} else {
		out, err = r.run(ctx, true, maxStdout, "for-each-ref", "--format=%(refname)", "refs/tags/"+TagPrefix)
	}
	if err != nil {
		return nil, fmt.Errorf("list tags failed: %w", err)
	}
	var ids []string
	for _, line := range strings.Split(out, "\n") {
		if remote {
			_, line, _ = strings.Cut(line, "\t")
		}
		if id, ok := strings.CutPrefix(line, "refs/tags/"+prefix); ok && runIDRe.MatchString(id) {
			ids = append(ids, id)
		}
	}
	return ids, nil
}
