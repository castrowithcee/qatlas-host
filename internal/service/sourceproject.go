package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/castrowithcee/qatlas-kit/redact"

	"github.com/castrowithcee/qatlas-host/internal/compose"
	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/stacks"
)

// projectRunner takes a Compose project that is no stack down, volumes kept, and up; *compose.Deployer
// implements it.
type projectRunner interface {
	ProjectDown(ctx context.Context, st stacks.Stack, log compose.Logger) error
	ProjectUp(ctx context.Context, st stacks.Stack, log compose.Logger) error
}

// handleProject answers source.project: docker compose down or up -d for the project in a folder outside every
// source of this service, in its engine. The folder must hold exactly the project the caller names.
func (s *inspectService) handleProject(ctx context.Context, raw json.RawMessage) (any, error) {
	var a control.SourceProjectArgs
	if err := decodeArgs(raw, &a); err != nil {
		return nil, err
	}
	if s.projects == nil {
		return nil, control.NewError(control.CodeNoDeployer, "deployments are not available in this version")
	}
	run := map[string]func(context.Context, stacks.Stack, compose.Logger) error{
		control.SourceProjectDown: s.projects.ProjectDown, control.SourceProjectUp: s.projects.ProjectUp,
	}[a.Action]
	if run == nil {
		return nil, control.NewError(control.CodeInvalid, "the action must be down or up")
	}
	if err := checkDirShape(a.Path); err != nil {
		return nil, control.NewError(control.CodeInvalid, err.Error())
	}
	for _, src := range s.repo.set.sources() {
		if overlaps(a.Path, src.conf.Path) {
			return nil, control.NewError(control.CodeInvalid, "the folder overlaps a source, whose stacks only runs take down or up")
		}
	}
	if info, err := os.Lstat(a.Path); err != nil || !info.IsDir() {
		return nil, control.NewError(control.CodeInvalid, "the folder is missing or is no folder")
	}
	list, err := stacks.Load(a.Path)
	if err != nil || len(list) != 1 || list[0].Folder != stacks.RootFolder || list[0].Err != nil {
		return nil, control.NewError(control.CodeInvalid, "the folder is not one Compose project that loads, with its compose file at its root")
	}
	st := list[0]
	if st.Name != a.Project {
		return nil, control.NewError(control.CodeInvalid, fmt.Sprintf("the folder holds the project %s, not %s", printable(st.Name), printable(a.Project)))
	}
	red := &redact.Redactor{}
	registerStackSecrets(red, st)
	out := &projectLog{}
	if err := run(ctx, st, out); err != nil {
		return nil, control.NewError(control.CodeStackAction, fmt.Sprintf("docker compose %s of %s failed: %s",
			a.Action, printable(st.Name), printable(red.Apply(out.reason(err)))))
	}
	return control.SourceProjectResult{Path: a.Path, Project: st.Name, Action: a.Action}, nil
}

// projectLog keeps the output of the last Compose call, whose last line says why a call failed.
type projectLog struct{ output string }

func (*projectLog) Step(string) {}

func (l *projectLog) Printf(format string, args ...any) {
	if text, ok := strings.CutPrefix(fmt.Sprintf(format, args...), "output:\n"); ok {
		l.output = text
	}
}

func (l *projectLog) reason(err error) string {
	lines := strings.Split(strings.TrimSpace(l.output), "\n")
	if last := strings.TrimSpace(lines[len(lines)-1]); last != "" {
		return err.Error() + ": " + last
	}
	return err.Error()
}
