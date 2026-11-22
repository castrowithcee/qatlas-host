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
