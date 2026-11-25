package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/engine"
	"github.com/castrowithcee/qatlas-host/internal/service"
	"github.com/castrowithcee/qatlas-host/internal/service/servicetest"
)

// fakeRegistry serves the tag list of demo/app from tags a test can change, or answers it with a status, and
// counts the requests for it.
type fakeRegistry struct {
	URL    string
	mu     sync.Mutex
	tags   []string
	status int
	lists  int
}

func newFakeRegistry(t *testing.T, tags ...string) *fakeRegistry {
	t.Helper()
	r := &fakeRegistry{tags: tags}
	srv := httptest.NewServer(http.HandlerFunc(r.serve))
	t.Cleanup(srv.Close)
	r.URL = srv.URL
	return r
}

func (r *fakeRegistry) serve(w http.ResponseWriter, req *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if req.URL.Path != "/v2/demo/app/tags/list" {
		http.NotFound(w, req)
		return
	}
	r.lists++
	if r.status != 0 {
		w.WriteHeader(r.status)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"name": "demo/app", "tags": r.tags})
}

func (r *fakeRegistry) setStatus(status int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status = status
}

func (r *fakeRegistry) addTag(tag string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tags = append(r.tags, tag)
}

func (r *fakeRegistry) listCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lists
}

// passLog records the clock times the passes of the regular check began at.
type passLog struct {
	mu sync.Mutex
	at []time.Time
}

func (l *passLog) add(at time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.at = append(l.at, at)
}

// await waits until the pass that began at the time has ended and the check waits for the next.
func (l *passLog) await(t *testing.T, at time.Time) {
	t.Helper()
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(time.Millisecond) {
		l.mu.Lock()
		found := slices.ContainsFunc(l.at, at.Equal)
		l.mu.Unlock()
		if found {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("no pass of the regular check at %s", at)
		}
	}
}

type pollRig struct {
	*harness
	clock    *svcClock
	notices  *noticeLog
	reg      *fakeRegistry
	passes   *passLog
	checkNow func(context.Context, string)
}

// pollHarness binds and deploys the stacks with the given extra files on a clock at noon, far from the window,
// and returns once the first pass of the regular check, before the binding, has ended.
func pollHarness(t *testing.T, files map[string]string) (*pollRig, *sequentialDeployer) {
	t.Helper()
	dep := &sequentialDeployer{}
	r := &pollRig{clock: &svcClock{now: noon}, notices: &noticeLog{}, passes: &passLog{},
		reg: newFakeRegistry(t, "0.9.1", "0.9.2", "1.2.3", "1.3.0", "2.0.0", "2.1.0-rc.1")}
	// The poll loop and the failure monitor share the clock; long intervals keep them out of the way.
	quiet := func(c *service.Config) { c.MonitorInterval, c.PollInterval = 24*time.Hour, 24*time.Hour }
	watch := func(c *service.Config) { r.checkNow = service.SeeStackChecks(c, r.passes.add) }
	r.harness = boundWith(t, dep, files, withRegistry(r.reg.URL), withClock(r.clock, r.notices), quiet, watch)
	r.passes.await(t, noon)
	dep.reset(nil)
	return r, dep
}

// step lets the clock move on by the shortest interval and returns, once the pass then due has ended, the
// time it began at.
func (r *pollRig) step(t *testing.T) time.Time {
	t.Helper()
	r.clock.Advance(30 * time.Second)
	at := r.clock.Now()
	r.passes.await(t, at)
	return at
}

func intervalRules(mode string, seconds int) string {
	return fmt.Sprintf("version: 1\nupdate:\n  mode: %s\n  trigger: interval\n  interval: %d\n", mode, seconds)
}

func TestRegularCheckFollowsTheIntervalOfEachStack(t *testing.T) {
	r, _ := pollHarness(t, map[string]string{
		servicetest.Settings("demo-stack"):  intervalRules("manual", 30),
		servicetest.Settings("plain-stack"): intervalRules("manual", 60),
	})
	checked := func() (time.Time, time.Time) {
		return storedUpdates(t, r.harness, "demo-stack").Checked, storedUpdates(t, r.harness, "plain-stack").Checked
	}
	base := r.reg.listCount()

	// both are due at once; the image they share is asked for once
	r.step(t)
	demo1, plain1 := checked()
	if demo1.IsZero() || plain1.IsZero() || r.reg.listCount() != base+1 {
		t.Fatalf("checked %s, %s, tag lists %d", demo1, plain1, r.reg.listCount()-base)
	}
	r.step(t)
	demo2, plain2 := checked()
	if demo2.Equal(demo1) || !plain2.Equal(plain1) || r.reg.listCount() != base+2 {
		t.Fatalf("after 30s: demo %s, plain %s, tag lists %d", demo2, plain2, r.reg.listCount()-base)
	}
	r.step(t)
	demo3, plain3 := checked()
	if demo3.Equal(demo2) || plain3.Equal(plain2) || r.reg.listCount() != base+3 {
		t.Fatalf("after 60s: demo %s, plain %s, tag lists %d", demo3, plain3, r.reg.listCount()-base)
	}
}

func TestRegularCheckUpdatesAnAutomaticStackAtOnce(t *testing.T) {
	r, dep := pollHarness(t, map[string]string{servicetest.Settings("demo-stack"): intervalRules("automatic", 30)})
	before := runCount(t, r.harness)

	r.step(t)
	if got := r.git.Pushed(); !slices.Equal(got, []string{"Update demo-stack DEMO_APP_VERSION from 1.2.3 to 1.3.0"}) {
		t.Fatalf("pushed = %q", got)
	}
	if pulls, _ := dep.pulled(); !slices.Equal(pulls, []string{"demo-stack"}) {
		t.Fatalf("pulls = %v", pulls)
	}
	var runs control.RunsResult
	if err := r.call(control.RunsCommand, control.RunsArgs{}, &runs); err != nil {
		t.Fatal(err)
	}
	if len(runs.Runs) != before+1 || runs.Runs[0].Trigger != engine.TriggerUpdate || runs.Runs[0].Result != engine.ResultSucceeded {
		t.Fatalf("runs = %+v", runs.Runs)
	}
	if s := storedUpdates(t, r.harness, "demo-stack"); s.Items[0].Current != "1.3.0" || s.Items[0].Status != "current" {
		t.Fatalf("demo-stack updates = %+v", s)
	}
	var win control.WindowResult
	if err := r.call(control.WindowShowCommand, nil, &win); err != nil || win.Open || !win.LastRun.IsZero() {
		t.Fatalf("window = %+v, %v", win, err)
	}
	// plain-stack waits for the window
	if s := storedUpdates(t, r.harness, "plain-stack"); !s.Checked.IsZero() {
		t.Fatalf("plain-stack updates = %+v", s)
	}
}

func TestRegularCheckKeepsToTheMarksAndTriesAgain(t *testing.T) {
	r, dep := pollHarness(t, map[string]string{servicetest.Settings("demo-stack"): intervalRules("automatic", 30)})
	if err := r.call(control.PinCommand, control.HoldArgs{Stack: "demo-stack"}, nil); err != nil {
		t.Fatal(err)
	}
	before := runCount(t, r.harness)

	r.step(t)
	if pulls, _ := dep.pulled(); len(pulls) != 0 || len(r.git.Pushed()) != 0 || runCount(t, r.harness) != before {
		t.Fatalf("pinned stack updated: pulls %v, pushed %q", pulls, r.git.Pushed())
	}
	if s := storedUpdates(t, r.harness, "demo-stack"); s.Items[0].Status != "update" || s.Items[0].Selected != "1.3.0" {
		t.Fatalf("demo-stack updates = %+v", s)
	}
	if err := r.call(control.UnpinCommand, control.HoldArgs{Stack: "demo-stack"}, nil); err != nil {
		t.Fatal(err)
	}
	r.step(t)
	if got := r.git.Pushed(); !slices.Equal(got, []string{"Update demo-stack DEMO_APP_VERSION from 1.2.3 to 1.3.0"}) {
		t.Fatalf("pushed after unpin = %q", got)
	}
}

func TestRegularCheckReportsAManualStackOncePerVersion(t *testing.T) {
	r, dep := pollHarness(t, map[string]string{servicetest.Settings("demo-stack"): intervalRules("manual", 30)})
	selected := func() []string {
		var out []string
		for _, n := range r.notices.all() {
			if n.Stack != "demo-stack" || len(n.Items) != 1 {
				t.Fatalf("notice = %+v", n)
			}
			out = append(out, n.Items[0].Selected)
		}
		return out
	}

	r.step(t)
	r.step(t)
	if got := selected(); !slices.Equal(got, []string{"1.3.0"}) {
		t.Fatalf("notices = %q", got)
	}
	r.reg.addTag("1.4.0")
	r.step(t)
	r.step(t)
	if got := selected(); !slices.Equal(got, []string{"1.3.0", "1.4.0"}) {
		t.Fatalf("notices after a new version = %q", got)
	}
	// a failed check in between forgets nothing: the same version is not reported again
	r.reg.setStatus(http.StatusTooManyRequests)
	r.step(t)
	r.reg.setStatus(0)
	lists := r.reg.listCount()
	for r.reg.listCount() == lists {
		r.step(t)
	}
	if got := selected(); !slices.Equal(got, []string{"1.3.0", "1.4.0"}) {
		t.Fatalf("notices after a failed check = %q", got)
	}
	if pulls, _ := dep.pulled(); len(pulls) != 0 || len(r.git.Pushed()) != 0 {
		t.Fatalf("manual stack updated: pulls %v, pushed %q", pulls, r.git.Pushed())
	}
}

func TestRegularCheckReportsTheVersionOfThePromoteSourceOnce(t *testing.T) {
	rules := intervalRules("manual", 30) + "  promote_source: demo-stack\n"
	r, dep := pollHarness(t, map[string]string{servicetest.Settings("plain-stack"): rules})
	promoted := func() (out []string) {
		for _, n := range r.notices.all() {
			if n.Stack != "plain-stack" {
				t.Fatalf("notice = %+v", n)
			}
			for _, it := range n.Promote {
				out = append(out, it.Variable+" "+it.Current+" "+it.Selected)
			}
		}
		return out
	}

	r.step(t)
	r.step(t)
	// a new registry version makes a notice of its own, which does not repeat the version of the source
	r.reg.addTag("1.4.0")
	r.step(t)
	r.step(t)
	if got := promoted(); !slices.Equal(got, []string{"DEMO_APP_VERSION 0.9.1 1.2.3"}) {
		t.Fatalf("source versions reported = %q", got)
	}
	if pulls, _ := dep.pulled(); len(pulls) != 0 || len(r.git.Pushed()) != 0 {
		t.Fatalf("manual stack updated: pulls %v, pushed %q", pulls, r.git.Pushed())
	}
}

func TestRegularCheckReportsNothingForAMissingPromoteSource(t *testing.T) {
	rules := intervalRules("manual", 30) + "  promote_source: nowhere\n"
	r, _ := pollHarness(t, map[string]string{servicetest.Settings("plain-stack"): rules})
	r.step(t)
	r.step(t)
	for _, n := range r.notices.all() {
		if len(n.Promote) != 0 {
			t.Fatalf("notice = %+v", n)
		}
	}
}

func TestRegularCheckWaitsLongerForABusyRegistry(t *testing.T) {
	r, _ := pollHarness(t, map[string]string{servicetest.Settings("demo-stack"): intervalRules("manual", 30)})
	var at []time.Duration
	last := r.reg.listCount()
	until := func(end time.Duration) {
		for r.clock.Now().Sub(noon) < end {
			now := r.step(t)
			if n := r.reg.listCount(); n != last {
				last = n
				at = append(at, now.Sub(noon)/time.Second)
			}
		}
	}

	r.reg.setStatus(http.StatusTooManyRequests)
	until(5490 * time.Second)
	// the wait doubles from 30 s up to 30 minutes and stays there
	if want := []time.Duration{30, 90, 210, 450, 930, 1890, 3690, 5490}; !slices.Equal(at, want) {
		t.Fatalf("checks at %v s, want %v", at, want)
	}
	r.reg.setStatus(0)
	at = nil
	until(7350 * time.Second)
	// the first check after the wait succeeds, then the stack's own interval holds again
	if want := []time.Duration{7290, 7320, 7350}; !slices.Equal(at, want) {
		t.Fatalf("checks at %v s after recovery, want %v", at, want)
	}
	logs := r.logs.String()
	if longer, again := strings.Count(logs, "its stacks wait longer"), strings.Count(logs, "the registry answers again"); longer != 6 || again != 1 {
		t.Fatalf("log lines: %d longer, %d again", longer, again)
	}
}

func TestCheckStackActsOnIntervalAndWebhookStacksOnly(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r, _ := pollHarness(t, map[string]string{
		servicetest.Settings("demo-stack"):  "version: 1\nupdate:\n  mode: manual\n  trigger: webhook\n",
		servicetest.Settings("plain-stack"): intervalRules("manual", 86400),
	})
	base := r.reg.listCount()
	for i, name := range []string{"demo-stack", "plain-stack"} {
		r.checkNow(ctx, name)
		if s := storedUpdates(t, r.harness, name); s.Checked.IsZero() || s.Items[0].Status != "update" || r.reg.listCount() != base+i+1 {
			t.Fatalf("%s: updates %+v, tag lists %d", name, s, r.reg.listCount()-base)
		}
	}
	if got := r.notices.all(); len(got) != 2 || got[0].Stack != "demo-stack" || got[1].Stack != "plain-stack" {
		t.Fatalf("notices = %+v", got)
	}

	w, _ := pollHarness(t, map[string]string{servicetest.Settings("plain-stack"): manualRules})
	base = w.reg.listCount()
	for _, name := range []string{"demo-stack", "plain-stack", "no-such-stack"} {
		w.checkNow(ctx, name)
	}
	if w.reg.listCount() != base || len(w.notices.all()) != 0 || len(w.git.Pushed()) != 0 {
		t.Fatalf("tag lists %d, notices %+v, pushed %q", w.reg.listCount()-base, w.notices.all(), w.git.Pushed())
	}
	for _, name := range []string{"demo-stack", "plain-stack"} {
		if s := storedUpdates(t, w.harness, name); !s.Checked.IsZero() {
			t.Fatalf("%s was checked: %+v", name, s)
		}
	}
	logs := w.logs.String()
	if strings.Count(logs, `reason="the update trigger is window"`) != 2 || !strings.Contains(logs, `stack=no-such-stack reason="the stack is unknown"`) {
		t.Fatalf("logs = %s", logs)
	}
}
