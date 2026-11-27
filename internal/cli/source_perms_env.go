package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"sort"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// planAccess works out what must change so that the service can write the folder of res and read the
// unversioned env files of its stacks: nil when nothing needs to. The env files are planned whether or not the
// service can write the folder. It changes nothing. The system engine needs none of it and gets no plan.
func planAccess(ctx context.Context, res control.SourceInspectResult) (*permPlan, error) {
	if targetOf(ctx).inst == nil {
		return nil, nil // the deploy service of the system engine is root and writes regardless of owner and mode
	}
	p, err := planFolderAccess(ctx, res)
	if err != nil || res.Access == nil || len(res.Access.UnreadableEnvFiles) == 0 || res.Case == control.SourceCaseClone {
		return p, err
	}
	t := targetOf(ctx)
	_, gid, err := folders.owner(t.account())
	if err != nil {
		return nil, folderFailure("cannot find the service account "+t.account(), err)
	}
	if p == nil {
		p = &permPlan{dir: res.Path, gid: gid}
	}
	if err := requireRealDir(p.dir); err != nil {
		return nil, err
	}
	mounts := cleanMounts(p.dir, bindMounts(res))
	for _, rel := range res.Access.UnreadableEnvFiles {
		// The service's answer is not trusted: the file is looked at again here, as root.
		full := filepath.Join(p.dir, rel)
		if !filepath.IsLocal(rel) || underAny(rel, mounts) || slices.Contains(p.files, full) {
			continue
		}
		if _, ok := plainBelow(p.dir, full, false); ok {
			p.envFiles = append(p.envFiles, full)
		}
	}
	sort.Strings(p.envFiles)
	if p.changed() == 0 {
		return nil, nil
	}
	return p, nil
}

// envSummary is the part of the preview that names the unversioned env files.
func (p *permPlan) envSummary() []string {
	if len(p.envFiles) == 0 {
		return nil
	}
	lines := []string{fmt.Sprintf("%d unversioned env files the service cannot read get the group of the service account (gid %d) "+
		"and group read; the owner and the other permissions stay:", len(p.envFiles), p.gid)}
	for _, f := range p.envFiles {
		rel, err := filepath.Rel(p.dir, f)
		if err != nil {
			rel = f
		}
		lines = append(lines, "  "+rel)
	}
	return lines
}
