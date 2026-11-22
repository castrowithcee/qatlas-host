package gitrepo

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// TagPrefix starts the name of every tag qhost sets.
const TagPrefix = "qhost/"

var (
	// ErrTagRejected means the remote refused the tag, for example by a tag protection rule or a hook.
	ErrTagRejected = errors.New("tag rejected by the remote")
	// ErrTagMoved means a tag of that name already points to another commit; tags are never moved.
	ErrTagMoved = errors.New("the tag exists at another commit")

	hostUnsafeRe = regexp.MustCompile(`[^a-z0-9_-]+`)
)

// TagHost turns a host name into one path component of a tag name: the short name, lower case, with every run
// of characters outside [a-z0-9_-] replaced by one dash and no dash at either end. A name nothing is left of
// becomes "host".
func TagHost(hostname string) string {
	short, _, _ := strings.Cut(strings.TrimSpace(hostname), ".")
	h := strings.Trim(hostUnsafeRe.ReplaceAllString(strings.ToLower(short), "-"), "-_")
	if h == "" {
		return "host"
	}
	return h
}

// RunTag returns the name qhost/<host>/<stack>/<run> of the tag a finished run sets, with host made safe by
// TagHost. Whether the whole name is a valid ref is decided by TagCommit.
func RunTag(hostname, stack, run string) string {
	return TagPrefix + TagHost(hostname) + "/" + stack + "/" + run
}

// TagCommit sets the tag name on commit and, in the remote mode, pushes it to origin with the deploy key, never
// forced and without hooks. A tag of that name that already points to commit is no error, so a repeated call
// only pushes again; one that points elsewhere is ErrTagMoved and nothing is sent. The remote refusing the push
// is ErrTagRejected; the local tag then stays. In local mode only the local tag is set.
func (r *Repo) TagCommit(ctx context.Context, name, commit string) error {
	if !hexRe.MatchString(commit) {
		return errors.New("commit must be a full lowercase commit id")
	}
	if !strings.HasPrefix(name, TagPrefix) {
		return errors.New("invalid tag name")
	}
	if _, err := r.run(ctx, true, maxErrorText, "check-ref-format", "refs/tags/"+name); err != nil {
		return errors.New("invalid tag name")
	}
	if _, err := r.revParse(ctx, commit+"^{commit}"); err != nil {
		return err
	}
	ref := "refs/tags/" + name
	have, err := r.revParse(ctx, ref+"^{commit}")
	switch {
	case err == nil && have != commit:
		return ErrTagMoved
	case err != nil:
		// An empty old value makes update-ref create the ref only if it does not exist yet.
		if _, uerr := r.run(ctx, true, maxErrorText, "update-ref", ref, commit, ""); uerr != nil {
			return fmt.Errorf("set tag failed: %w", uerr)
		}
	}
	if r.cfg.Local {
		return nil
	}
	_, err = r.run(ctx, true, maxStdout, "push", "--porcelain", "--no-verify", "--no-recurse-submodules",
		"--", "origin", ref+":"+ref)
	var failure *runFailure
	if errors.As(err, &failure) {
		if reason := rejectionReason(failure.stdout); reason != "" {
			return fmt.Errorf("%w: %s", ErrTagRejected, reason)
		}
	}
	if err != nil {
		return fmt.Errorf("push tag failed: %w", err)
	}
	return nil
}

// rejectionReason returns the text of the first refused ref in git's --porcelain output ("!\t<ref>\t[rejected]
// (<reason>)" or "[remote rejected]"), empty when no ref was refused.
func rejectionReason(porcelain string) string {
	for _, line := range strings.Split(porcelain, "\n") {
		f := strings.Split(line, "\t")
		if len(f) >= 3 && f[0] == "!" {
			return strings.TrimSpace(f[2])
		}
	}
	return ""
}
