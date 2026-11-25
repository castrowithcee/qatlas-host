package service

import (
	"context"
	"sync"
	"time"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/engine"
	"github.com/castrowithcee/qatlas-host/internal/registry"
	"github.com/castrowithcee/qatlas-host/internal/stacks"
	"github.com/castrowithcee/qatlas-host/internal/updates"
	"github.com/castrowithcee/qatlas-host/internal/window"
)

const (
	// pollTick is the longest wait between two readings of the stack list, so a stack or setting that is new is
	// picked up within it. It is the shortest interval a stack can set.
	pollTick = 30 * time.Second
	// maxBackoffWait bounds the doubled wait of the stacks of a registry that limits requests or cannot be
	// reached; a longer interval of the stack stays as it is.
	maxBackoffWait = 30 * time.Minute
	// maxBackoffLevel is the most doublings: the shortest interval doubled that often passes maxBackoffWait.
	maxBackoffLevel = 6
)

// pollService is the regular registry check of the stacks outside the maintenance window. It checks the stacks
// with the update trigger interval once their interval has passed, and any stack with the trigger interval or
// webhook on request through CheckStack. A stack in automatic mode is updated at once through the path of the
// window, without its bound; a stack in manual mode is reported once per newly available version.
type pollService struct {
	stackUpdater
	clock window.Clock
	// passed, when set, is told the clock time each pass of the loop began at, once the loop waits for the next.
	// Tests only.
	passed func(time.Time)
	// sem holds a token while a pass runs, so at most one runs at a time.
	sem chan struct{}

	mu      sync.Mutex
	checked map[string]time.Time         // stack -> clock time of its last check
	level   map[string]int               // registry host -> doublings of the wait of its stacks
	known   map[string]map[string]string // stack in manual mode -> variable -> available version last found, "" if current
	pending map[string]chan struct{}     // stack -> closed once the check of it that is asked for or running ends
}

func newPollService(win *windowService, clock window.Clock) *pollService {
	if clock == nil {
		clock = systemClock{}
	}
	u := win.stackUpdater
	u.label = "update check"
	return &pollService{
		stackUpdater: u, clock: clock, sem: make(chan struct{}, 1),
		checked: map[string]time.Time{}, level: map[string]int{}, known: map[string]map[string]string{},
		pending: map[string]chan struct{}{},
	}
}

type systemClock struct{}

func (systemClock) Now() time.Time                         { return time.Now() }
func (systemClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

// run checks the due stacks until ctx ends: once at the start, then whenever the next stack is due, and at the
// latest after pollTick.
func (p *pollService) run(ctx context.Context) {
	for ctx.Err() == nil {
		start := p.clock.Now()
		next, ok := p.scheduled(ctx, start)
		if !ok {
			return
		}
		var wake <-chan time.Time
		if d := next.Sub(p.clock.Now()); d > 0 {
			wake = p.clock.After(d)
		}
		if p.passed != nil {
			p.passed(start)
		}
		if wake == nil {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-wake:
		}
	}
}

// scheduled runs one pass of the loop at now and returns when the next is due; ok is false once ctx has ended.
func (p *pollService) scheduled(ctx context.Context, now time.Time) (next time.Time, ok bool) {
	select {
	case p.sem <- struct{}{}:
	case <-ctx.Done():
		return time.Time{}, false
	}
	defer func() { <-p.sem }()
	next = now.Add(pollTick)
	list, err := p.upd.loadStacks("")
	if err != nil {
		p.log.Debug(p.label+": the stacks cannot be read; nothing is checked", "error", err)
		return next, true
	}
	p.forget(list)
	var due []stacks.Stack
	for _, st := range list {
		if trigger, interval := st.UpdateTrigger(); trigger == stacks.TriggerInterval && !now.Before(p.dueAt(st, interval)) {
			due = append(due, st)
		}
	}
	p.check(ctx, now, due)
	for _, st := range list {
		if trigger, interval := st.UpdateTrigger(); trigger == stacks.TriggerInterval {
			if at := p.dueAt(st, interval); at.Before(next) {
				next = at
			}
		}
	}
	return next, true
}

// CheckStack checks one stack at once, as a due pass of the regular check does, and returns when that check
// has ended or ctx has ended. It acts only on a stack with the update trigger interval or webhook; for any
// other stack, or a stack it does not know, it writes a log line and does nothing. A call while a check of the
// same stack is asked for or running joins that check instead of starting another. The check, and an update it
// starts, end with ctx.
func (p *pollService) CheckStack(ctx context.Context, name string) {
	p.mu.Lock()
	ch, joined := p.pending[name]
	if !joined {
		ch = make(chan struct{})
		p.pending[name] = ch
	}
	p.mu.Unlock()
	if joined {
		select {
		case <-ch:
		case <-ctx.Done():
		}
		return
	}
	select {
	case p.sem <- struct{}{}:
	case <-ch:
		return // a pass of the loop checked the stack meanwhile
	case <-ctx.Done():
		p.done(name, ch)
		return
	}
	defer func() { <-p.sem }()
	select {
	case <-ch:
		return
	default:
	}
	defer p.done(name, ch)
	list, err := p.upd.loadStacks("")
	if err != nil {
		p.log.Info(p.label+": stack not checked", "stack", name, "reason", "the stacks cannot be read")
		return
	}
	for _, st := range list {
		if st.Name != name {
			continue
		}
		switch trigger, _ := st.UpdateTrigger(); {
		case st.Err != nil:
			p.log.Info(p.label+": stack not checked", "stack", name, "reason", "the stack cannot be read")
		case trigger != stacks.TriggerInterval && trigger != stacks.TriggerWebhook:
			p.log.Info(p.label+": stack not checked", "stack", name, "reason", "the update trigger is "+trigger)
		default:
			p.check(ctx, p.clock.Now(), []stacks.Stack{st})
		}
		return
	}
	p.log.Info(p.label+": stack not checked", "stack", name, "reason", "the stack is unknown")
}

// done ends the asked-for check of the stack: whoever waits for it returns.
func (p *pollService) done(name string, ch chan struct{}) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pending[name] == ch {
		delete(p.pending, name)
	}
	select {
	case <-ch:
	default:
		close(ch)
	}
}

// forget drops what the check remembers of stacks that are gone or wait for the window again.
func (p *pollService) forget(list []stacks.Stack) {
	keep := map[string]bool{}
	for _, st := range list {
		if trigger, _ := st.UpdateTrigger(); trigger != stacks.TriggerWindow {
			keep[st.Name] = true
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for name := range p.checked {
		if !keep[name] {
			delete(p.checked, name)
		}
	}
	for name := range p.known {
		if !keep[name] {
			delete(p.known, name)
		}
	}
}

// dueAt is when the stack is due next: its interval after its last check, doubled for each step of the wait of
// its registries; a stack never checked is due at once.
func (p *pollService) dueAt(st stacks.Stack, interval time.Duration) time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	last, ok := p.checked[st.Name]
	if !ok {
		return time.Time{}
	}
	lvl := 0
	for _, img := range st.Images {
		if repo, err := registry.ParseRepo(img.Image); err == nil {
			lvl = max(lvl, p.level[repo.Host])
		}
	}
	return last.Add(max(interval, min(interval<<lvl, maxBackoffWait)))
}

// check asks the registries for the stacks in one run, stores the results as the window does, and then reports
// each stack in manual mode and updates each stack in automatic mode, in the order of the list. The caller holds
// the token of a pass.
func (p *pollService) check(ctx context.Context, now time.Time, list []stacks.Stack) {
	if len(list) == 0 {
		return
	}
	p.mu.Lock()
	for _, st := range list {
		ch, ok := p.pending[st.Name]
		if !ok {
			ch = make(chan struct{})
			p.pending[st.Name] = ch
		}
		defer p.done(st.Name, ch)
	}
	p.mu.Unlock()
	cctx, cancel := context.WithTimeout(ctx, control.UpdatesRefreshTimeout)
	fresh, err := p.upd.detector.Check(cctx, list)
	cancel()
	p.mu.Lock()
	for _, st := range list {
		p.checked[st.Name] = now
	}
	p.mu.Unlock()
	if err != nil {
		p.log.Warn(p.label+": the registry check did not finish; nothing is updated", "error", err)
		return
	}
	p.backoff(fresh)
	p.remember(list)
	var updated []string
	for _, st := range list {
		check, ok := fresh[st.Name]
		if !ok {
			p.log.Debug(p.label+": stack skipped", "stack", st.Name, "reason", "the stack cannot be read or is deploy-only")
			continue
		}
		if err := p.upd.eng.RecordUpdates(st.Name, check); err != nil {
			p.log.Warn(p.label+": the registry check cannot be stored", "stack", st.Name, "error", err)
		}
		switch st.UpdateMode() {
		case stacks.ModeManual:
			p.report(ctx, st, engine.UpdateCheck{Checked: check.Checked, Items: p.newlyAvailable(st.Name, check)})
		case stacks.ModeAutomatic:
			if ctx.Err() != nil {
				return
			}
			if p.update(ctx, st.Name, check, func() bool { return true }) {
				updated = append(updated, st.Name)
			}
		}
	}
	p.upd.recheck(ctx, updated)
}

// remember takes, for a stack in manual mode seen for the first time, the versions of the stored check as
// found, so a restart reports no version again. It runs before the new results are stored.
func (p *pollService) remember(list []stacks.Stack) {
	var unseen []string
	p.mu.Lock()
	for _, st := range list {
		if _, ok := p.known[st.Name]; !ok && st.UpdateMode() == stacks.ModeManual {
			unseen = append(unseen, st.Name)
		}
	}
	p.mu.Unlock()
	if len(unseen) == 0 {
		return
	}
	snap, err := p.upd.eng.Snapshot()
	if err != nil {
		snap = engine.Snapshot{}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, name := range unseen {
		versions := map[string]string{}
		if stored := snap.State.Stacks[name].Updates; stored != nil {
			note(versions, *stored)
		}
		p.known[name] = versions
	}
}

// newlyAvailable returns the available items of a stack in manual mode whose version differs from the one last
// found, and remembers the new versions. A variable whose check failed keeps the version found before.
func (p *pollService) newlyAvailable(name string, check engine.UpdateCheck) []engine.UpdateItem {
	p.mu.Lock()
	defer p.mu.Unlock()
	versions := p.known[name]
	if versions == nil {
		versions = map[string]string{}
		p.known[name] = versions
	}
	var items []engine.UpdateItem
	for _, it := range check.Items {
		if last, ok := versions[it.Variable]; it.Status == engine.UpdateAvailable && (!ok || last != it.Selected) {
			items = append(items, it)
		}
	}
	note(versions, check)
	return items
}

func note(versions map[string]string, check engine.UpdateCheck) {
	for _, it := range check.Items {
		switch it.Status {
		case engine.UpdateAvailable:
			versions[it.Variable] = it.Selected
		case engine.UpdateCurrent:
			versions[it.Variable] = ""
		}
	}
}

// backoff doubles the wait of the stacks of a registry host whose check found it limiting requests or out of
// reach, up to maxBackoffWait, and restores their own interval once a check of it succeeds. Each change writes
// one log line.
func (p *pollService) backoff(fresh map[string]engine.UpdateCheck) {
	throttled := map[string]bool{}
	for _, check := range fresh {
		for _, it := range check.Items {
			repo, err := registry.ParseRepo(it.Image)
			if err != nil {
				continue
			}
			switch {
			case updates.Throttled(it):
				throttled[repo.Host] = true
			case it.Status == engine.UpdateAvailable || it.Status == engine.UpdateCurrent:
				if _, seen := throttled[repo.Host]; !seen {
					throttled[repo.Host] = false
				}
			}
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for host, failed := range throttled {
		old, lvl := p.level[host], 0
		if failed {
			lvl = min(old+1, maxBackoffLevel)
		}
		switch {
		case lvl == old:
			continue
		case lvl == 0:
			delete(p.level, host)
			p.log.Info(p.label+": the registry answers again; its stacks are checked at their own interval", "registry", host)
		default:
			p.level[host] = lvl
			p.log.Warn(p.label+": the registry limits requests or cannot be reached; its stacks wait longer",
				"registry", host, "factor", 1<<lvl)
		}
	}
}
