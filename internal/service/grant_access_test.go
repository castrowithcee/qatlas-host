package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/castrowithcee/qatlas-kit/docker"
	"github.com/castrowithcee/qatlas-kit/localipc"

	"github.com/castrowithcee/qatlas-host/internal/client"
	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/formatnum/formatnumtest"
	"github.com/castrowithcee/qatlas-host/internal/registry/registrytest"
	"github.com/castrowithcee/qatlas-host/internal/service"
	"github.com/castrowithcee/qatlas-host/internal/service/servicetest"
)

// grantUseHarness is a bound service with invented users, a fake engine and a registry; demo-stack has update
// rules, plain-stack has manual ones. ep is its channel, for a connection that stays open.
func grantUseHarness(t *testing.T, d service.Docker) (h *harness, p *peers, ep localipc.ServiceEndpoint) {
	t.Helper()
	p = &peers{}
	var seen atomic.Pointer[control.Registry]
	srv := registrytest.New(t, registrytest.Config{Repos: map[string][]string{"demo/app": {"0.9.1", "0.9.2", "1.2.3", "1.3.0"}}})
	h = boundWith(t, &fakeDeployer{}, map[string]string{servicetest.Settings("plain-stack"): manualRules},
		withPeers(t, p, &seen), withDocker(d), withRegistry(srv.URL), func(c *service.Config) { ep = c.Endpoint })
	return h, p, ep
}

// grant adds a grant of example-admin as root and returns its ID; the caller's user is restored after.
func grant(t *testing.T, h *harness, p *peers, action, stack string) string {
	t.Helper()
	was := p.uid.Load()
	p.as(0)
	defer p.as(was)
	var res control.GrantAddResult
	if err := h.call(control.GrantAddCommand, control.GrantAddArgs{User: "example-admin", Action: action, Stack: stack, Confirm: true}, &res); err != nil {
		t.Fatal(err)
	}
	return res.Grant.ID
}

func hasCode(err error, code string) bool {
	var ce *control.Error
	return errors.As(err, &ce) && ce.Code == code
}

type request struct {
	command string
	args    any
}

func TestAGrantRunsItsNarrowFormForItsStackOnly(t *testing.T) {
	h, p, _ := grantUseHarness(t, &fakeDocker{})
	for _, action := range []string{control.GrantActionStackRestart, control.GrantActionUpdateCheck, control.GrantActionUpdateRun} {
		grant(t, h, p, action, "demo-stack")
	}
	p.as(exampleAdmin)
	for _, r := range []request{
		// Another stack.
		{control.StackRestartCommand, control.StackActionArgs{Stack: "plain-stack"}},
		{control.UpdatesListCommand, control.UpdatesArgs{Stack: "plain-stack", Refresh: true}},
		{control.UpdateRunCommand, control.UpdateRunArgs{Stacks: []string{"plain-stack"}}},
		// More than the narrow form.
		{control.StackRestartCommand, map[string]any{"stack": "demo-stack", "extra": 1}},
		{control.StackRestartCommand, map[string]any{"stack": "demo-stack", "STACK": "plain-stack"}},
		{control.StackRestartCommand, json.RawMessage(`{"stack":"demo-stack","stack":"demo-stack"}`)},
		{control.StackRestartCommand, map[string]any{"Stack": "demo-stack"}},
		{control.UpdatesListCommand, control.UpdatesArgs{Stack: "demo-stack", Refresh: true, Changelog: true}},
		{control.UpdatesListCommand, control.UpdatesArgs{Refresh: true}},
		{control.UpdatesListCommand, control.UpdatesArgs{Changelog: true, Stack: "demo-stack"}},
		{control.UpdatesListCommand, map[string]any{"stack": "demo-stack", "refresh": true, "extra": 1}},
		{control.UpdateRunCommand, control.UpdateRunArgs{Stacks: []string{"demo-stack", "plain-stack"}}},
		{control.UpdateRunCommand, control.UpdateRunArgs{Stacks: []string{"demo-stack", "demo-stack"}}},
		{control.UpdateRunCommand, control.UpdateRunArgs{Stacks: []string{"demo-stack"}, Version: "1.3.0"}},
		{control.UpdateRunCommand, control.UpdateRunArgs{Stacks: []string{"demo-stack"}, Variable: "DEMO_APP_VERSION"}},
		{control.UpdateRunCommand, map[string]any{"stacks": []string{"demo-stack"}, "extra": 1}},
		{control.UpdateRunCommand, "not an object"},
		// Another command.
		{control.StackStartCommand, control.StackActionArgs{Stack: "demo-stack"}},
		{control.StackStopCommand, control.StackActionArgs{Stack: "demo-stack"}},
		{control.DeployCommand, control.DeployArgs{Stack: "demo-stack"}},
		{control.ChangelogShowCommand, control.ChangelogArgs{Stack: "demo-stack"}},
		{control.BackupCreateCommand, control.BackupArgs{Stack: "demo-stack"}},
		{control.ContainerRestartCommand, control.ContainerActionArgs{Name: "demo-stack-app-1"}},
		{control.GrantAddCommand, control.GrantAddArgs{User: "example-admin", Action: control.GrantActionStackRestart, Stack: "plain-stack", Confirm: true}},
	} {
		if err := h.call(r.command, r.args, nil); !hasCode(err, control.CodeNotRoot) {
			t.Errorf("%s %+v = %v, want %s", r.command, r.args, err, control.CodeNotRoot)
		}
	}
	if pushed := h.git.Pushed(); len(pushed) != 0 {
		t.Fatalf("a refused request pushed %q", pushed)
	}
	for _, r := range []request{
		{control.StackRestartCommand, control.StackActionArgs{Stack: "demo-stack"}},
		{control.UpdatesListCommand, control.UpdatesArgs{Stack: "demo-stack", Refresh: true}},
		{control.UpdateRunCommand, control.UpdateRunArgs{Stacks: []string{"demo-stack"}}},
	} {
		if err := h.call(r.command, r.args, nil); err != nil {
			t.Errorf("%s %+v = %v", r.command, r.args, err)
		}
	}
	if pushed := h.git.Pushed(); len(pushed) != 1 {
		t.Fatalf("the granted update pushed %q", pushed)
	}
}

// driftingDocker answers the first lookup of one name with a container of its own and every later one with
// the container in later, as if the name moved to another container in between.
type driftingDocker struct {
	*fakeDocker
	name  string
	later docker.Details
	mu    sync.Mutex
	seen  int
}

func (d *driftingDocker) Inspect(ctx context.Context, ref string) (docker.Details, error) {
	d.mu.Lock()
	moved := ref == d.name && d.seen > 0
	if ref == d.name {
		d.seen++
	}
	d.mu.Unlock()
	if moved {
		return d.later, nil
	}
	return d.fakeDocker.Inspect(ctx, ref)
}

func TestAContainerGrantCoversContainersOfItsStackOnly(t *testing.T) {
	fd := &fakeDocker{details: map[string]docker.Details{}}
	dd := &driftingDocker{fakeDocker: fd, name: "drifting-app-1"}
	h, p, _ := grantUseHarness(t, dd)
	add := func(id, name string, labels map[string]string) docker.Details {
		d := docker.Details{Container: running(id, name, labels)}
		fd.details[name], fd.details[id] = d, d
		return d
	}
	own := func(stack string) map[string]string {
		return map[string]string{lblProject: stack, lblDir: filepath.Join(h.dir, stack)}
	}
	add("c1", "demo-stack-app-1", own("demo-stack"))
	add("c2", "plain-stack-app-1", own("plain-stack"))
	add("c3", "copied-app-1", map[string]string{lblProject: "demo-stack", lblDir: "/srv/example-other/demo-stack"})
	add("c4", "lonely", nil)
	add("c5", "drifting-app-1", own("demo-stack"))
	dd.later = docker.Details{Container: running("c6", "drifting-app-1", map[string]string{lblProject: "example-foreign"})}
	grant(t, h, p, control.GrantActionContainerRestart, "demo-stack")

	p.as(exampleAdmin)
	for _, r := range []request{
		{control.ContainerRestartCommand, control.ContainerActionArgs{Name: "plain-stack-app-1"}},
		{control.ContainerRestartCommand, control.ContainerActionArgs{Name: "copied-app-1"}},
		{control.ContainerRestartCommand, control.ContainerActionArgs{Name: "lonely"}},
		{control.ContainerRestartCommand, control.ContainerActionArgs{Name: "no-such-container"}},
		{control.ContainerRestartCommand, control.ContainerActionArgs{Name: "drifting-app-1"}},
		{control.ContainerRestartCommand, control.ContainerActionArgs{Name: "demo-stack-app-1", GraceSeconds: 5}},
		{control.ContainerRestartCommand, map[string]any{"name": "demo-stack-app-1", "extra": 1}},
		{control.ContainerStopCommand, control.ContainerActionArgs{Name: "demo-stack-app-1"}},
		{control.ContainerStartCommand, control.ContainerActionArgs{Name: "demo-stack-app-1"}},
		{control.StackRestartCommand, control.StackActionArgs{Stack: "demo-stack"}},
	} {
		if err := h.call(r.command, r.args, nil); !hasCode(err, control.CodeNotRoot) {
			t.Errorf("%s %+v = %v, want %s", r.command, r.args, err, control.CodeNotRoot)
		}
	}
	if got := fd.calls(); len(got) != 0 {
		t.Fatalf("a refused request reached the engine: %v", got)
	}
	var res control.ContainerActionResult
	if err := h.call(control.ContainerRestartCommand, control.ContainerActionArgs{Name: "demo-stack-app-1"}, &res); err != nil {
		t.Fatal(err)
	}
	if got := fd.calls(); res.Kind != control.ContainerKindOwn || !slices.Equal(got, []string{"restart/10s c1"}) {
		t.Fatalf("restart = %+v, calls %v", res, got)
	}
}

func TestAGrantThatNoLongerHoldsCoversNothing(t *testing.T) {
	h, p, ep := grantUseHarness(t, &fakeDocker{})
	row := func(id, user string, uid int, stack, expires string) string {
		if expires != "" {
			expires = `,"expires":"` + expires + `"`
		}
		return fmt.Sprintf(grantRow, id, user, uid, control.GrantActionStackRestart, stack, expires)
	}
	formatnumtest.Seed(t, grantsFile(h), `{"schema":1,"grants":[`+
		row("g-00000001", "example-admin", exampleAdmin, "demo-stack", "2020-01-01")+","+
		row("g-00000002", "example-old-name", exampleAdmin, "plain-stack", "")+","+
		row("g-00000003", "example-reader", exampleReader, "demo-stack", "")+`]}`)
	p.as(exampleAdmin)
	for _, stack := range []string{"demo-stack", "plain-stack"} {
		wantCode(t, h.call(control.StackRestartCommand, control.StackActionArgs{Stack: stack}, nil), control.CodeNotRoot)
	}
	p.as(exampleReader)
	wantCode(t, h.call(control.StackRestartCommand, control.StackActionArgs{Stack: "demo-stack"}, nil), control.CodeNoAccess)

	// A grant renewed by root counts at once, and its removal counts from the next request of a connection that
	// stays open.
	id := grant(t, h, p, control.GrantActionStackRestart, "demo-stack")
	p.as(exampleAdmin)
	cl, err := client.Dial(context.Background(), ep)
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	restart := func() error {
		return cl.Call(context.Background(), control.StackRestartCommand, control.StackActionArgs{Stack: "demo-stack"}, nil)
	}
	if err := restart(); err != nil {
		t.Fatal(err)
	}
	p.as(0)
	if err := h.call(control.GrantRemoveCommand, control.GrantRemoveArgs{ID: id}, nil); err != nil {
		t.Fatal(err)
	}
	wantCode(t, restart(), control.CodeNotRoot)

	// A grants file that cannot be read lets nothing run.
	grant(t, h, p, control.GrantActionStackRestart, "demo-stack")
	formatnumtest.Seed(t, grantsFile(h), `{"schema":1,"grants":[`)
	wantCode(t, restart(), control.CodeNotRoot)
}

func TestTheLogNamesTheGrantARequestRanUnder(t *testing.T) {
	h, p, _ := grantUseHarness(t, &fakeDocker{})
	id := grant(t, h, p, control.GrantActionStackRestart, "demo-stack")
	p.as(exampleAdmin)
	if err := h.call(control.StackRestartCommand, control.StackActionArgs{Stack: "demo-stack"}, nil); err != nil {
		t.Fatal(err)
	}
	wantCode(t, h.call(control.StackRestartCommand, control.StackActionArgs{Stack: "plain-stack"}, nil), control.CodeNotRoot)
	p.as(0)
	if err := h.call(control.StackRestartCommand, control.StackActionArgs{Stack: "plain-stack"}, nil); err != nil {
		t.Fatal(err)
	}
	var res control.DeployLogListResult
	if err := h.call(control.HostLogListCommand, control.HostLogListArgs{Action: control.StackRestartCommand}, &res); err != nil {
		t.Fatal(err)
	}
	type row struct {
		uid    uint32
		stack  string
		result string
		grant  string
	}
	var got []row
	for _, e := range res.Entries {
		got = append(got, row{*e.ClientUID, e.Stack, e.Result, e.Grant})
	}
	want := []row{ // newest first
		{0, "plain-stack", "success", ""},
		{exampleAdmin, "plain-stack", "failure", ""},
		{exampleAdmin, "demo-stack", "success", id},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("entries = %+v, want %+v", got, want)
	}
}
