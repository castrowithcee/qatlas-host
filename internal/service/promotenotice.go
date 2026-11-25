package service

import (
	"context"
	"sync"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/engine"
	"github.com/castrowithcee/qatlas-host/internal/stacks"
)

// promoteSeen remembers, per stack and variable, the version of the source last reported, so a version proven
// in the source is reported once however often the stack is checked.
type promoteSeen struct {
	mu   sync.Mutex
	seen map[string]map[string]string
}

// fresh returns the steps whose version was not reported yet and remembers the given ones. A variable that has
// no step any more is forgotten, so a later higher version of the source is reported again.
func (p *promoteSeen) fresh(stack string, steps []engine.VersionStep) []engine.UpdateItem {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.seen == nil {
		p.seen = map[string]map[string]string{}
	}
	last, now := p.seen[stack], map[string]string{}
	var items []engine.UpdateItem
	for _, s := range steps {
		now[s.Variable] = s.To
		if last[s.Variable] != s.To {
			items = append(items, engine.UpdateItem{Variable: s.Variable, Current: s.From, Selected: s.To, Status: engine.UpdateAvailable})
		}
	}
	p.seen[stack] = now
	return items
}

// promoteNotice returns the versions of the source of the stack's promote_source that run proven there, are
// higher than the stack's own and were not reported yet. It reads the source through this service's own
// engines only: a source in another deploy instance is out of reach of this service and yields nothing, as do
// a missing or unproven source and any refusal that a promotion of it would meet.
func (u stackUpdater) promoteNotice(ctx context.Context, st stacks.Stack) []engine.UpdateItem {
	if st.Rules == nil || st.Rules.Update.PromoteSource == "" {
		return u.promoted.fresh(st.Name, nil)
	}
	pctx, cancel := context.WithTimeout(ctx, control.UpdatesRefreshTimeout)
	defer cancel()
	src, err := u.upd.eng.ReadPromoteSource(pctx, st.Rules.Update.PromoteSource)
	if err != nil {
		return nil
	}
	check, err := u.upd.detector.PromoteCheck(st)
	if err != nil {
		return nil
	}
	prev, err := u.upd.eng.PreviewPromote(pctx, st.Name, src, check, u.upd.detector.PromoteDigest())
	if err != nil {
		// A transient failure keeps what was reported, so it is not reported again afterwards.
		return nil
	}
	return u.promoted.fresh(st.Name, prev.Steps)
}
