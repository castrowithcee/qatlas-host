package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"syscall"

	"github.com/castrowithcee/qatlas-kit/docker"
	"github.com/castrowithcee/qatlas-kit/redact"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/stacks"
)

// handleMove answers source.move: the preview of moving a project folder into the source at Into. It reads the
// folder as source.inspect does, reads the source, and the containers of the folder in the engine of the
// service; it writes nothing. What only the client can tell (file system, other engines) is added there.
func (s *inspectService) handleMove(ctx context.Context, raw json.RawMessage) (any, error) {
	var a control.SourceMoveArgs
	if err := decodeArgs(raw, &a); err != nil {
		return nil, err
	}
	if err := checkDirShape(a.Into); err != nil {
		return nil, control.NewError(control.CodeInvalid, err.Error())
	}
	src := s.repo.set.byPath(a.Into)
	if src == nil {
		return nil, control.NewError(control.CodeNoSuchSource, "no source is bound to this folder")
	}
	res := control.SourceMoveResult{
		Path: maskControl(a.Path), Into: maskControl(a.Into), SourceKind: src.conf.Kind,
		Blocks: []control.SourceMoveIssue{}, Warnings: []control.SourceMoveIssue{},
	}
	m := &mover{s: s, res: &res, args: a, src: src}
	if err := m.run(ctx); err != nil {
		return nil, err
	}
	if err := fitAnswer(res, "the folder holds too many files to list in one answer; move data out of it or into a bind-mounted data folder"); err != nil {
		return nil, err
	}
	return res, nil
}

type mover struct {
	s    *inspectService
	res  *control.SourceMoveResult
	args control.SourceMoveArgs
	src  *source
}

func (m *mover) block(code, reason, fix string, items ...string) {
	m.res.Blocks = append(m.res.Blocks, control.SourceMoveIssue{Code: code, Reason: reason, Fix: fix, Items: items})
}

func (m *mover) warn(code, reason, fix string, items ...string) {
	m.res.Warnings = append(m.res.Warnings, control.SourceMoveIssue{Code: code, Reason: reason, Fix: fix, Items: items})
}

func (m *mover) run(ctx context.Context) error {
	a := m.args
	v, err := m.s.decide(ctx, control.SourceInspectArgs{Path: a.Path})
	if err != nil {
		return err
	}
	if v.plan != nil {
		defer v.plan.close()
	}
	validPath := v.kase != control.SourceCaseReject || checkDirShape(a.Path) == nil
	if validPath {
		m.res.Target = maskControl(filepath.Join(a.Into, filepath.Base(a.Path)))
	}
	m.checkSource()
	if validPath {
		m.checkTarget()
	}
	switch v.kase {
	case control.SourceCaseReject:
		m.block(control.MoveFolderRejected, "the project folder cannot be moved: "+v.reason, v.fix)
		return nil
	case control.SourceCaseClone:
		m.block(control.MoveNotAProject, "the folder is missing or empty, so there is no project to move",
			"Give the folder of a Compose project.")
		return nil
	case control.SourceCaseAdopt, control.SourceCaseLocal:
		m.block(control.MoveIsRepository, "the folder has a Git repository of its own, which would end up inside the repository of the source",
			fmt.Sprintf("Take the folder over as a source of its own with 'qhost source add %s'.", maskControl(a.Path)))
		return nil
	}
	m.res.PlannedFiles = v.plan.planned
	m.res.Excluded = v.plan.excluded
	m.res.Ignore = v.plan.ignoreResult()
	m.checkContainers(ctx)
	if v.loadErr != nil {
		m.block(control.MoveStackUnreadable, "the project cannot be read: "+printable(v.loadErr.Error()),
			"Repair the folder, or look at it with 'qhost source inspect'.")
		return nil
	}
	m.checkProject(v.list)
	return nil
}

// checkSource blocks a source that is one root stack or cannot be read.
func (m *mover) checkSource() {
	list, err := stacks.Load(m.args.Into)
	if err != nil {
		m.block(control.MoveSourceUnreadable, "the stacks of the source cannot be read: "+printable(err.Error()),
			"Repair the source folder, or choose another source.")
		return
	}
	if slices.ContainsFunc(list, func(st stacks.Stack) bool { return st.Folder == stacks.RootFolder }) {
		m.block(control.MoveRootStack,
			fmt.Sprintf("the source %s is one root stack: its folder holds a compose file, so the whole folder is that stack and a second stack cannot join it", m.res.Into),
			"Choose a source whose folder holds no compose file at its root, or take the project over as a source of its own with 'qhost source add'.")
	}
}

// checkTarget blocks a target folder that exists, overlaps another source or cannot be a stack folder.
func (m *mover) checkTarget() {
	base := filepath.Base(m.args.Path)
	target := filepath.Join(m.args.Into, base)
	if strings.HasPrefix(base, ".") {
		m.block(control.MoveTargetName, fmt.Sprintf("the folder name %s starts with a dot, so the source does not read it as a stack folder", maskControl(base)),
			"Rename the project folder to a name without a leading dot.")
	}
	if _, err := os.Lstat(target); err == nil {
		m.block(control.MoveTargetExists, fmt.Sprintf("the target folder %s exists already in the source", m.res.Target),
			"Rename or remove the folder in the source, or rename the project folder, then run the preview again.")
	} else if !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, syscall.ENOTDIR) {
		m.block(control.MoveTargetExists, fmt.Sprintf("the service cannot tell whether the target folder %s exists", m.res.Target),
			"Give the service search permission on the source folder, then run the preview again.")
	}
	for _, other := range m.s.repo.set.sources() {
		if other != m.src && overlaps(target, other.conf.Path) {
			m.block(control.MoveTargetOverlaps, fmt.Sprintf("the target folder %s overlaps the stack folder %s, which is another source", m.res.Target, printable(other.conf.Path)),
				"Choose a source whose folders do not overlap another stack folder.")
		}
	}
}

// checkContainers lists the containers that run from the project folder in the engine of the service, whatever
// the name of their project.
func (m *mover) checkContainers(ctx context.Context) {
	if m.s.docker == nil {
		m.res.ContainersError = "the container engine is not available"
		return
	}
	all, err := m.s.docker.List(ctx)
	if err != nil {
		m.res.ContainersError = "the container engine could not be asked"
		var ce *control.Error
		if errors.As((&containerService{log: m.s.log}).dockerAnswer(err), &ce) {
			m.res.ContainersError = ce.Message
		}
		return
	}
	m.res.ContainersChecked = true
	projects, _ := docker.Group(all)
	for _, p := range projects {
		if p.WorkingDir == "" || filepath.Clean(p.WorkingDir) != m.args.Path {
			continue
		}
		for _, c := range p.Containers {
			m.res.Running = append(m.res.Running, control.SourceMoveRunning{Container: maskControl(c.Name), Project: maskControl(p.Name)})
		}
	}
}

// checkProject examines the one stack of the project folder.
func (m *mover) checkProject(list []stacks.Stack) {
	if len(list) != 1 || list[0].Folder != stacks.RootFolder {
		m.block(control.MoveNotAProject, "the folder is not one Compose project: it holds no compose file at its root, or its stacks are in subfolders",
			"Move a folder whose compose file lies directly in it; a folder of several stacks is taken over as a source of its own.")
		return
	}
	st := list[0]
	red := &redact.Redactor{}
	registerStackSecrets(red, st)
	for _, val := range st.Env {
		red.Add(val)
	}
	text := func(t string) string { return printable(red.Apply(t)) }
	if st.Err != nil {
		m.block(control.MoveStackUnreadable, "the project cannot be read: "+text(st.Err.Error()),
			"Repair the project files, or look at them with 'qhost source inspect'.")
		return
	}

	base := filepath.Base(m.args.Path)
	// The running containers say which name the project has now; without any, the folder does.
	m.res.Project = st.Name
	if len(m.res.Running) > 0 && !slices.ContainsFunc(m.res.Running, func(r control.SourceMoveRunning) bool { return r.Project == st.Name }) {
		m.res.Project = m.res.Running[0].Project
	}
	newName, err := stacks.ProjectNameIn(st, base)
	if err != nil {
		m.block(control.MoveStackUnreadable, "the project name in the target cannot be resolved: "+text(err.Error()),
			"Set COMPOSE_PROJECT_NAME in .env or name in the compose file.")
	} else {
		m.res.NewProject = text(newName)
		if newName != m.res.Project {
			m.block(control.MoveProjectName,
				fmt.Sprintf("the Compose project name would change from %s to %s: containers and named volumes belong to the old name, so a deploy would create new, empty volumes; qhost renames nothing",
					maskControl(m.res.Project), text(newName)),
				fmt.Sprintf("Pin the old name yourself, with 'name: %s' in the compose file or COMPOSE_PROJECT_NAME in .env, then run the preview again.", maskControl(m.res.Project)))
		}
		m.checkNameTaken(newName)
	}

	if abs := stacks.AbsoluteBinds(st); len(abs) > 0 {
		m.block(control.MoveAbsoluteBind, "a bind mount names a path inside the old folder as an absolute path, which would keep pointing at the old place after the move",
			"Change the mount to a relative path such as ./data in the compose file yourself; qhost never rewrites compose files.", mapAll(abs, maskControl)...)
	}

	var keys []string
	for k, v := range st.Env {
		if stacks.SecretLikeKey(k) && strings.TrimSpace(v) != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	if len(keys) > 0 {
		keys = mapAll(keys, maskControl)
		if m.src.conf.Kind == control.SourceKindRemote {
			m.block(control.MoveSecretsInEnv, "the source has a remote, and .env would be committed and pushed with values of secret-like keys",
				"Move these keys from .env to secrets.env by hand, which is never versioned, then run the preview again.", keys...)
		} else {
			m.warn(control.MoveSecretsInEnv, "the source is local: .env is committed with values of secret-like keys, and a remote can be given to the source only after they are moved",
				"Move these keys from .env to secrets.env by hand if the source is to get a remote.", keys...)
		}
	}

	var fixed []string
	for _, f := range st.Findings {
		if f.Kind == stacks.FindingFixedTag {
			fixed = append(fixed, text(f.Service+": "+f.Image))
		}
	}
	if len(fixed) > 0 {
		m.warn(control.MoveFixedTags, "images with a fixed tag cannot be updated by qhost, so the stack counts as deploy only ('qhost stack check' shows the finding)",
			"Write the version as a variable in the compose file and set it in .env if updates are wanted.", fixed...)
	}
}

// checkNameTaken blocks a stack name that a stack of any source of this service has already.
func (m *mover) checkNameTaken(name string) {
	for _, other := range m.s.repo.set.sources() {
		list, err := stacks.Load(other.conf.Path)
		if err != nil {
			continue
		}
		for _, st := range list {
			if st.Name == name {
				m.block(control.MoveNameTaken, fmt.Sprintf("a stack named %s exists already in the source %s of this engine", printable(name), printable(other.conf.Path)),
					"Rename or remove that stack first.")
				return
			}
		}
	}
}
