package engine

import (
	"context"
	"time"

	"github.com/castrowithcee/qatlas-host/internal/gitrepo"
	"github.com/castrowithcee/qatlas-host/internal/stacks"
)

// tagTimeout bounds setting and pushing the tag of a finished run.
const tagTimeout = 2 * time.Minute

// backupContext attaches the tag of the commit the stack runs at to the backup step's context, so the backup
// metadata names it.
func (r *run) backupContext(ctx context.Context) context.Context {
	cur, _, err := r.e.Stack(r.st.Name)
	if err != nil || cur.Deployed != r.deployed {
		return ctx
	}
	return stacks.WithDeployedTag(ctx, cur.Deployed, cur.DeployedTag)
}

// defaultKeepTags is the number of run tags a stack without backup rules keeps.
const defaultKeepTags = keepRuns

// staleRunIDs returns the IDs of the run tags to delete from ids, which are run IDs oldest first: the tags
// beyond the retention of the backup rules (keep, max age in days), or beyond defaultKeepTags without rules. A
// tag is kept while it satisfies every limit that is set, and the newest one is always kept. The age is that of
// the timestamp in the run ID.
func staleRunIDs(ids []string, b *stacks.Backup, now time.Time) []string {
	keep, maxAge := defaultKeepTags, 0
	if b != nil {
		keep, maxAge = b.Keep, b.MaxAgeDays
	}
	var stale []string
	for i := len(ids) - 2; i >= 0; i-- {
		rank := len(ids) - 1 - i
		drop := keep > 0 && rank >= keep
		if maxAge > 0 && len(ids[i]) >= 16 {
			if t, err := time.Parse("20060102T150405Z", ids[i][:16]); err == nil && now.Sub(t) > time.Duration(maxAge)*24*time.Hour {
				drop = true
			}
		}
		if drop {
			stale = append(stale, ids[i])
		}
	}
	return stale
}

// pruneTags deletes the old tags of this host and stack after the tag of a run was pushed. Its error never
// touches the run's result.
func (r *run) pruneTags() error {
	e := r.e
	ctx, cancel := context.WithTimeout(e.base, tagTimeout)
	defer cancel()
	var rules *stacks.Backup
	if r.st.Rules != nil {
		rules = r.st.Rules.Backup
	}
	_, err := r.b.Copy.PruneTags(ctx, e.cfg.Host, r.rec.Stack, func(ids []string) []string {
		return staleRunIDs(ids, rules, e.cfg.Clock.Now())
	})
	return err
}

// tagRun sets and pushes the fixed tag of a finished run on the commit the stack runs at and returns its name,
// or empty and the error. It never touches the run's result.
func (r *run) tagRun(commit string) (string, error) {
	e := r.e
	if commit == "" || r.b.Copy == nil {
		return "", nil
	}
	name := gitrepo.RunTag(e.cfg.Host, r.rec.Stack, r.rec.ID)
	ctx, cancel := context.WithTimeout(e.base, tagTimeout)
	defer cancel()
	if err := r.b.Copy.TagCommit(ctx, name, commit); err != nil {
		return "", err
	}
	return name, nil
}
