package service_test

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/compose"
	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/service"
	"github.com/castrowithcee/qatlas-host/internal/stacks"
)

// projectDeployer also takes projects down and up and records each call with the folder.
type projectDeployer struct {
	fakeDeployer
	pmu      sync.Mutex
	projects []string
	fail     bool
}

func (d *projectDeployer) note(action string, st stacks.Stack, log compose.Logger) error {
	d.pmu.Lock()
	defer d.pmu.Unlock()
	d.projects = append(d.projects, action+" "+st.Name+" "+st.Dir)
	if d.fail {
		log.Printf("output:\nContainer myapp-app-1 Creating\nError: port 8080 is already allocated")
		return errors.New("docker compose " + action + " failed: exit status 1")
	}
	return nil
}

func (d *projectDeployer) ProjectDown(_ context.Context, st stacks.Stack, log compose.Logger) error {
	return d.note("down", st, log)
}

func (d *projectDeployer) ProjectUp(_ context.Context, st stacks.Stack, log compose.Logger) error {
	return d.note("up", st, log)
}

func (h *harness) project(path, name, action string) error {
	return h.call(control.SourceProjectCommand, control.SourceProjectArgs{Path: path, Project: name, Action: action}, nil)
}

func TestProjectGoesDownAndUpInItsFolderOutsideTheSources(t *testing.T) {
	d := &projectDeployer{}
	h := startWith(t, d, func(c *service.Config) { c.Git = nil }, withDocker(&fakeDocker{}))
	into := localRepo(t, map[string]string{"other/compose.yaml": moveCompose, "other/.env": "WEB_VERSION=1.2.3\n"})
	if _, err := h.sourceAdd(into); err != nil {
		t.Fatal(err)
	}
	p := project(t, "myapp", projectFiles())

	for _, action := range []string{control.SourceProjectDown, control.SourceProjectUp} {
		if err := h.project(p, "myapp", action); err != nil {
			t.Fatalf("%s: %v", action, err)
		}
	}
	for _, bad := range []struct{ path, name, action string }{
		{p, "other-name", control.SourceProjectDown},
		{p, "myapp", "restart"},
		{filepath.Join(into, "other"), "other", control.SourceProjectDown},
		{filepath.Join(p, "data"), "data", control.SourceProjectUp},
	} {
		wantCode(t, h.project(bad.path, bad.name, bad.action), control.CodeInvalid)
	}
	want := []string{"down myapp " + p, "up myapp " + p}
	if d.pmu.Lock(); !slices.Equal(d.projects, want) {
		t.Fatalf("calls = %v", d.projects)
	}
	d.fail = true
	d.pmu.Unlock()
	err := h.project(p, "myapp", control.SourceProjectUp)
	wantCode(t, err, control.CodeStackAction)
	if !strings.Contains(err.Error(), "port 8080 is already allocated") {
		t.Fatalf("err = %v", err)
	}
}

func TestProjectNeedsADeployer(t *testing.T) {
	h := realGitService(t)
	wantCode(t, h.project(project(t, "myapp", projectFiles()), "myapp", control.SourceProjectDown), control.CodeNoDeployer)
}
