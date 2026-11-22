package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"

	"github.com/castrowithcee/qatlas-kit/docker"
	"github.com/castrowithcee/qatlas-kit/redact"

	"github.com/castrowithcee/qatlas-host/internal/compose"
	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/credentials"
	"github.com/castrowithcee/qatlas-host/internal/gitrepo"
	"github.com/castrowithcee/qatlas-host/internal/stackcheck"
	"github.com/castrowithcee/qatlas-host/internal/stacks"
)

const (
	// maxInspectPath is PATH_MAX; a longer path cannot exist.
	maxInspectPath = 4096
	// maxInspectEntries bounds the directory entries read while looking for unreadable folders.
	maxInspectEntries = 1 << 20
	// accessWriteSearch is W_OK|X_OK: what creating entries in a folder takes.
	accessWriteSearch = 0x3
	accessRead        = 0x4
)

// inspectService answers source.inspect. It reads the folder, the git repository in it and the container
// engine, and changes nothing: the only thing it writes is a temporary directory outside the folder.
type inspectService struct {
	repo   *repoService
	docker Docker
	log    *slog.Logger
}

func registerInspectCommands(r *control.Registry, repo *repoService, d Docker, log *slog.Logger) error {
	s := &inspectService{repo: repo, docker: d, log: log}
	if err := r.Register(control.SourceInspectCommand, changing(s.handle)); err != nil {
		return err
	}
	return registerSourceCommands(r, s)
}

// verdict is the outcome of looking at a folder: a case, or a rejection with its reason and fix.
type verdict struct {
	kase, url, reason, fix string
	needsURL               bool
	// view and remote are what the stack check asks git; set for the cases that bring stacks.
	view   stackcheck.Git
	remote bool
	// plan is set for a folder without a repository.
	plan *filePlan
	// include is the choice of unclear files the plan was made with.
	include []string
	// list and loadErr are the stacks of the folder when the verdict had to read them already.
	list    []stacks.Stack
	loadErr error
	loaded  bool
	// repo is the folder's .git, set for an existing repository.
	repo bool
}

func rejected(reason, fix string) verdict {
	return verdict{kase: control.SourceCaseReject, reason: reason, fix: fix}
}

func (s *inspectService) handle(ctx context.Context, raw json.RawMessage) (any, error) {
	var a control.SourceInspectArgs
	if err := decodeArgs(raw, &a); err != nil {
		return nil, err
	}
	res := control.SourceInspectResult{Path: maskControl(a.Path), Stacks: []control.SourceStack{}}
	v, access, err := s.classify(ctx, a)
	if err != nil {
		return nil, err
	}
	if v.plan != nil {
		defer v.plan.close()
	}
	res.Case, res.URL, res.Reason, res.Fix, res.NeedsURL, res.Access = v.kase, v.url, v.reason, v.fix, v.needsURL, access
	if v.plan != nil {
		res.PlannedFiles = v.plan.planned
		res.Excluded = v.plan.excluded
		res.Included = v.plan.chosen
		res.Ignore = v.plan.ignoreResult()
	}
	if v.view != nil {
		if !v.loaded {
			v.list, v.loadErr = stacks.Load(a.Path)
		}
		if err := s.addStacks(ctx, &res, v, a.Path); err != nil {
			return nil, err
		}
	}
	if err := fitAnswer(res, "the folder holds too many files to list in one answer; move data out of it or into a bind-mounted data folder"); err != nil {
		return nil, err
	}
	return res, nil
}

// classify decides the case of the folder and gathers its permissions. A rejection has no permissions.
func (s *inspectService) classify(ctx context.Context, a control.SourceInspectArgs) (verdict, *control.SourceAccess, error) {
	v, err := s.decide(ctx, a)
	if err != nil || v.kase == control.SourceCaseReject {
		return v, nil, err
	}
	if len(a.Include) > 0 && v.kase != control.SourceCaseFiles {
		if v.plan != nil {
			v.plan.close()
		}
		return verdict{}, nil, control.NewError(control.CodeInvalid,
			"a choice of unclear files applies only to a folder of files without a repository")
	}
	target := a.Path
	if v.kase == control.SourceCaseClone {
		// A missing folder is created below its nearest existing parent.
		for {
			if _, err := os.Lstat(target); err == nil {
				break
			}
			target = filepath.Dir(target)
		}
	}
	acc, err := accessOf(target)
	if err != nil {
		return verdict{}, nil, err
	}
	if v.repo {
		w := syscall.Access(filepath.Join(a.Path, ".git"), accessWriteSearch) == nil
		acc.GitWritable = &w
	}
	return v, acc, nil
}

func (s *inspectService) decide(ctx context.Context, a control.SourceInspectArgs) (verdict, error) {
	if a.URL != "" {
		if err := gitrepo.ValidateURL(a.URL); err != nil {
			return rejected("the repository address is not a supported SSH address",
				"Use user@host:path or ssh://[user@]host[:port]/path."), nil
		}
	}
	if err := checkDirShape(a.Path); err != nil || len(a.Path) > maxInspectPath {
		return rejected("the path must be absolute, normalized and made of letters, digits and ._/@+-",
			"Give the folder as an absolute path without '..', '.', '//' or a trailing slash."), nil
	}
	if bad, isRejected := lstatChain(a.Path); isRejected {
		return bad, nil
	}
	if s.repo.checkDirOutsideState(a.Path) != nil {
		return rejected("the folder contains or lies inside the service's own state directory",
			"Choose a folder elsewhere."), nil
	}
	for _, src := range s.repo.set.sources() {
		if overlaps(a.Path, src.conf.Path) {
			return rejected(fmt.Sprintf("the folder is, contains or lies inside the stack folder %s, which is already a source", printable(src.conf.Path)),
				"Choose a folder that does not overlap a stack folder that is in use."), nil
		}
	}
	for dir := filepath.Dir(a.Path); ; dir = filepath.Dir(dir) {
		_, err := os.Lstat(filepath.Join(dir, ".git"))
		switch {
		case err == nil:
			return rejected(fmt.Sprintf("the folder is part of another repository, whose root is %s", printable(dir)),
				"Choose a folder outside every repository, or use the root of that repository."), nil
		case !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, syscall.ENOTDIR):
			return rejected(fmt.Sprintf("the service cannot look for a repository in %s", printable(dir)),
				"Give the service search permission on every folder of the path, or choose another folder."), nil
		}
		if dir == "/" {
			break
		}
	}

	entries, err := os.ReadDir(a.Path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return verdict{kase: control.SourceCaseClone, url: a.URL, needsURL: a.URL == ""}, nil
	case err != nil:
		return rejected("the service cannot read the folder",
			"Give the service read permission on the folder, or choose another folder."), nil
	case len(entries) == 0:
		return verdict{kase: control.SourceCaseClone, url: a.URL, needsURL: a.URL == ""}, nil
	}

	gi, err := os.Lstat(filepath.Join(a.Path, ".git"))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return s.decideFiles(ctx, a)
	case err != nil:
		return rejected("the service cannot inspect the .git entry of the folder",
			"Give the service read permission on the folder, or choose another folder."), nil
	case !gi.IsDir():
		return rejected(".git in the folder is not a folder (a linked worktree, a submodule or a link)",
			"Use the main working copy of the repository, or a folder with a repository of its own."), nil
	}
	return s.decideRepo(ctx, a)
}

// lstatChain walks the path from the root without following links. It returns a rejection (and true) when a
// component is a link, not a folder or cannot be inspected; a missing component ends the walk without one.
func lstatChain(p string) (verdict, bool) {
	cur := "/"
	for _, part := range strings.Split(p[1:], "/") {
		cur = filepath.Join(cur, part)
		info, err := os.Lstat(cur)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return verdict{}, false
		case err != nil:
			return rejected(fmt.Sprintf("the service cannot inspect %s", printable(cur)),
				"Give the service search permission on every folder of the path, or choose another folder."), true
		case info.Mode()&fs.ModeSymlink != 0:
			where := "the folder"
			if cur != p {
				where = "a folder above it"
			}
			return rejected(fmt.Sprintf("%s is a symbolic link (%s); a link is never followed", where, printable(cur)),
				"Give the real path of the folder."), true
		case !info.IsDir() && cur == p:
			return rejected("the path is not a folder", "Choose a folder, or a path that does not exist yet."), true
		case !info.IsDir():
			return rejected(fmt.Sprintf("%s is not a folder", printable(cur)), "Choose a path below existing folders."), true
		}
	}
	return verdict{}, false
}

func (s *inspectService) decideRepo(ctx context.Context, a control.SourceInspectArgs) (verdict, error) {
	unreadable := func(err error) verdict {
		return rejected("git cannot read the repository in the folder: "+printable(err.Error()),
			"Repair the repository, or choose another folder.")
	}
	// The key is never used: nothing here reaches the remote.
	wc, err := s.repo.set.git.Open(s.repo.set.gitConfig(sourceConfig{Key: credentials.DefaultKey}), a.Path)
	if err != nil {
		return unreadable(err), nil
	}
	names, err := wc.Remotes(ctx)
	if err != nil {
		return unreadable(err), nil
	}
	switch {
	case slices.Contains(names, "origin"):
		url, err := wc.RemoteURL(ctx)
		if err != nil {
			return unreadable(err), nil
		}
		if err := gitrepo.ValidateURL(url); err != nil {
			return rejected("the origin remote of the repository is not a supported SSH address",
				"Set origin to user@host:path or ssh://[user@]host[:port]/path, or remove the remote to run the folder locally."), nil
		}
		if a.URL != "" && a.URL != url {
			return rejected("the given address differs from the origin remote of the repository",
				"Give the address the repository already has as origin, or leave the address out."), nil
		}
		return verdict{kase: control.SourceCaseAdopt, url: url, view: wc, remote: true, repo: true}, nil
	case len(names) > 0:
		return rejected("the repository has remotes but none is called origin",
			"Rename the remote to origin, or remove the remotes to run the folder locally."), nil
	case a.URL != "":
		return rejected("the folder is a repository without a remote, and source add cannot add one",
			"Leave the address out to run the folder locally, then give it a remote with 'qhost source remote'."), nil
	}
	return verdict{kase: control.SourceCaseLocal, view: wc, repo: true}, nil
}

func (s *inspectService) decideFiles(ctx context.Context, a control.SourceInspectArgs) (verdict, error) {
	// The plan needs the stacks for what they refer to; a folder whose stacks cannot be read has none.
	list, loadErr := stacks.Load(a.Path)
	plan, err := planFiles(ctx, a.Path, list, loadErr != nil, nil, a.Include)
	var bad *gitrepo.IgnoreError
	if errors.As(err, &bad) {
		return rejected(printable(bad.Error()), ignoreFix), nil
	}
	if err != nil {
		return verdict{}, err
	}
	return verdict{kase: control.SourceCaseFiles, url: a.URL, view: plan, plan: plan, include: a.Include, list: list, loadErr: loadErr, loaded: true}, nil
}

// filePlan is what a first commit of a folder without a repository would hold, and the git view of it: the
// planned files count as tracked, what is left out or ignored counts as ignored.
type filePlan struct {
	planned  []string
	excluded []control.SourceExcluded
	// remote are the planned files that are planned only because the remote versions them.
	remote []string
	// chosen are the unclear files that were chosen into the plan, with the reason they would be left out for.
	chosen []control.SourceExcluded
	// rawPlanned and rawExcluded are planned and the paths of excluded as they are on disk, in the same order;
	// the lists above show them with unprintable characters replaced.
	rawPlanned, rawExcluded []string
	// ignore is what the repository writes to the .gitignore of the folder for rawExcluded.
	ignore  gitrepo.IgnorePlan
	preview *gitrepo.Preview
}

func (p *filePlan) close() { _ = p.preview.Close() }

// ignoreFix is the advice for a .gitignore that the first commit cannot extend.
const ignoreFix = "Make .gitignore a regular file of at most 1 MiB that the service can read, or remove it; qhost adds the paths it leaves out to it."

// ignoreResult is the change to the .gitignore of the folder, nil when there is none.
func (p *filePlan) ignoreResult() *control.SourceIgnore {
	if !p.ignore.Write {
		return nil
	}
	out := &control.SourceIgnore{Action: control.SourceIgnoreCreate, Lines: []string{}}
	if p.ignore.Exists {
		out.Action = control.SourceIgnoreExtend
	}
	for _, l := range p.ignore.Added {
		out.Lines = append(out.Lines, maskControl(l))
	}
	return out
}

// committedFiles is the number of files of the first commit: the planned files and a .gitignore that is written
// and not among them.
func (p *filePlan) committedFiles() int {
	n := len(p.planned)
	if p.ignore.Write && !slices.Contains(p.rawPlanned, gitrepo.IgnoreName) {
		n++
	}
	return n
}

func (p *filePlan) TrackedFiles(_ context.Context, dir string) ([]string, error) {
	if dir == stacks.RootFolder {
		return p.planned, nil
	}
	var out []string
	for _, f := range p.planned {
		if strings.HasPrefix(f, dir+"/") {
			out = append(out, f)
		}
	}
	return out, nil
}

func (p *filePlan) IsIgnored(ctx context.Context, rel string) (bool, error) {
	for _, e := range p.excluded {
		if underPath(e.Path, rel) {
			return true, nil
		}
	}
	return p.preview.IsIgnored(ctx, rel)
}

// underPath reports whether rel is root or lies below it; both are clean paths relative to the folder.
func underPath(root, rel string) bool { return rel == root || strings.HasPrefix(rel, root+"/") }

// referenced is what the stacks of a folder need, as paths from the folder's root.
type referenced struct {
	files, dirs      map[string]bool
	writable, secret map[string]string
	// rootBroken names the root stack when it cannot be read: it is the whole folder.
	rootBroken string
}

// referencesOf gathers the references of the stacks that can be read. A stack that cannot be read brings none.
func referencesOf(list []stacks.Stack) referenced {
	r := referenced{files: map[string]bool{}, dirs: map[string]bool{}, writable: map[string]string{}, secret: map[string]string{}}
	for _, st := range list {
		if st.Err != nil {
			if st.Folder == stacks.RootFolder {
				r.rootBroken = printable(st.Name)
			}
			continue
		}
		for _, f := range st.Refs.Files {
			r.files[f] = true
		}
		for _, d := range st.Refs.Dirs {
			r.dirs[d] = true
		}
		for _, f := range st.Refs.Writable {
			r.writable[f] = printable(st.Name)
		}
		for _, f := range st.Refs.Secrets {
			r.secret[f] = printable(st.Name)
		}
	}
	return r
}

// needed tells whether a stack refers to the file, directly or through a folder.
func (r referenced) needed(f string) bool {
	if r.files[f] {
		return true
	}
	for d := range r.dirs {
		if underPath(d, f) || d == stacks.RootFolder {
			return true
		}
	}
	return false
}

// alwaysPlanned are the files of the folder root that the first commit takes whatever the stacks refer to: the
// ignore and attribute files and the settings of the stacks.
func alwaysPlanned(f string) bool {
	return f == ".gitignore" || f == ".gitattributes" ||
		path.Dir(f) == stacks.SettingsDir && strings.HasSuffix(f, ".yaml")
}

// collapsible are the reasons a whole folder is left out as one entry when every file below it has the same one.
var collapsible = []string{control.SourceExcludedNotReferenced, control.SourceExcludedUnreadableStack}

// planFiles decides which files of a folder without a repository the first commit takes: the compose files, env
// files and everything else the readable stacks refer to, and the always planned files. Everything else is left
// out with the reason. stacksUnread says the stacks could not be read at all; the folder then brings no file.
//
// remote holds the files the commit of a remote versions when the folder joins it. Such a file is planned where
// it would only be left out as not referenced, so joining removes nothing the remote versions; a file that is
// left out for another reason, such as a writable mount, a secrets file, data of a bind mount or a stack that
// cannot be read, stays out, and so does a file the service cannot read.
//
// include holds the unclear files the operator chose: a file a stack mounts writable or names as a secrets file
// is planned instead of left out. A path that is not unclear by this classification, or that the plan cannot
// take for another reason, is refused, so a choice never goes unnoticed.
//
// The plan holds the .gitignore that keeps what is left out out of git; a .gitignore of the folder that cannot be
// extended gives a *gitrepo.IgnoreError.
func planFiles(ctx context.Context, root string, list []stacks.Stack, stacksUnread bool, remote map[string]bool, include []string) (*filePlan, error) {
	chosenSet := make(map[string]bool, len(include))
	for _, f := range include {
		chosenSet[f] = true
	}
	unclear := map[string]control.SourceExcluded{}
	ref := referencesOf(list)
	var excluded []control.SourceExcluded
	roots := bindRoots(root, list)
	for _, st := range list {
		if st.Err != nil && st.Folder != stacks.RootFolder && existsNoFollow(root, st.Folder) {
			roots = append(roots, control.SourceExcluded{Path: st.Folder, Reason: control.SourceExcludedUnreadableStack, Stack: printable(st.Name)})
		}
	}
	slices.SortFunc(roots, func(a, b control.SourceExcluded) int { return strings.Compare(a.Path, b.Path) })
	for _, ex := range roots {
		if !slices.ContainsFunc(excluded, func(o control.SourceExcluded) bool { return underPath(o.Path, ex.Path) }) {
			excluded = append(excluded, ex)
		}
	}
	hints := make([]string, len(excluded))
	for i, e := range excluded {
		hints[i] = e.Path
	}
	pv, err := gitrepo.OpenPreview(ctx, gitrepo.Config{Local: true}, root, hints)
	if err != nil {
		return nil, gitFailure(err)
	}
	plan := &filePlan{preview: pv, planned: []string{}}
	ok := false
	defer func() {
		if !ok {
			plan.close()
		}
	}()
	listed, err := pv.Untracked(ctx)
	if errors.Is(err, gitrepo.ErrOutputLimit) {
		return nil, control.NewError(control.CodeTooLarge, "the folder holds too many files to list; move data out of it or into a bind-mounted data folder")
	}
	if err != nil {
		return nil, gitFailure(err)
	}
	bind := slices.Clone(excluded)
	var loose []control.SourceExcluded
	for _, f := range listed {
		if slices.ContainsFunc(bind, func(o control.SourceExcluded) bool { return underPath(o.Path, strings.TrimSuffix(f, "/")) }) {
			continue
		}
		if nested, isDir := strings.CutSuffix(f, "/"); isDir {
			excluded = append(excluded, control.SourceExcluded{Path: nested, Reason: control.SourceExcludedNestedRepo})
			continue
		}
		byRemote := false
		switch {
		case ref.needed(f) || !stacksUnread && alwaysPlanned(f):
		case ref.writable[f] != "" || ref.secret[f] != "":
			e := control.SourceExcluded{Path: f, Reason: control.SourceExcludedWritableFile, Stack: ref.writable[f]}
			if ref.writable[f] == "" {
				e = control.SourceExcluded{Path: f, Reason: control.SourceExcludedSecretsFile, Stack: ref.secret[f]}
			}
			if !chosenSet[f] {
				excluded = append(excluded, e)
				continue
			}
			unclear[f] = e
		case ref.rootBroken != "" && !alwaysPlanned(f):
			loose = append(loose, control.SourceExcluded{Path: f, Reason: control.SourceExcludedUnreadableStack, Stack: ref.rootBroken})
			continue
		case remote[f] && !stacksUnread:
			byRemote = true
		default:
			loose = append(loose, control.SourceExcluded{Path: f, Reason: control.SourceExcludedNotReferenced})
			continue
		}
		full := filepath.Join(root, f)
		info, err := os.Lstat(full)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			continue
		case err != nil:
			excluded = append(excluded, control.SourceExcluded{Path: f, Reason: control.SourceExcludedUnreadable})
		case !info.Mode().IsRegular():
			excluded = append(excluded, control.SourceExcluded{Path: f, Reason: control.SourceExcludedNotRegular})
		case info.Size() > stacks.MaxFileSize:
			excluded = append(excluded, control.SourceExcluded{Path: f, Reason: control.SourceExcludedTooLarge})
		case syscall.Access(full, accessRead) != nil:
			excluded = append(excluded, control.SourceExcluded{Path: f, Reason: control.SourceExcludedUnreadable})
		default:
			plan.planned = append(plan.planned, maskControl(f))
			plan.rawPlanned = append(plan.rawPlanned, f)
			if e, ok := unclear[f]; ok {
				e.Path = maskControl(f)
				plan.chosen = append(plan.chosen, e)
			}
			if byRemote {
				plan.remote = append(plan.remote, maskControl(f))
			}
		}
	}
	if err := checkChosen(include, unclear, plan.rawPlanned); err != nil {
		return nil, err
	}
	excluded = append(excluded, collapseLoose(loose, plan.rawPlanned, excluded)...)

	// Git passes over a folder it cannot open and over a pipe, socket or device without a word, so they are
	// looked for here.
	odd, err := oddEntries(ctx, root, excluded)
	if err != nil {
		return nil, err
	}
	for _, o := range odd {
		ignored, err := pv.IsIgnored(ctx, o.Path)
		if err != nil {
			return nil, gitFailure(err)
		}
		if !ignored {
			excluded = append(excluded, o)
		}
	}
	slices.SortFunc(excluded, func(a, b control.SourceExcluded) int { return strings.Compare(a.Path, b.Path) })
	for i := range excluded {
		plan.rawExcluded = append(plan.rawExcluded, excluded[i].Path)
		excluded[i].Path = maskControl(excluded[i].Path)
	}
	plan.excluded = excluded
	// A name with a line break cannot be a pattern; source add refuses it as unprintable, so the answer still
	// shows the folder.
	patterned := slices.DeleteFunc(slices.Clone(plan.rawExcluded), func(p string) bool { return strings.ContainsAny(p, "\n\r\x00") })
	if plan.ignore, err = gitrepo.PlanIgnore(root, patterned); err != nil {
		return nil, err
	}
	ok = true
	return plan, nil
}

// checkChosen refuses a chosen path that is not an unclear file of the folder, or that is unclear but cannot be
// planned (unreadable, too large, not a plain file), so the plan never silently differs from the choice.
func checkChosen(include []string, unclear map[string]control.SourceExcluded, planned []string) error {
	for _, f := range include {
		if _, ok := unclear[f]; !ok {
			return control.NewError(control.CodeInvalid, fmt.Sprintf(
				"%s is not an unclear file of the folder; only a file left out as %s or %s can be chosen; nothing was changed",
				printable(f), control.SourceExcludedWritableFile, control.SourceExcludedSecretsFile))
		}
		if !slices.Contains(planned, f) {
			return control.NewError(control.CodeInvalid, fmt.Sprintf(
				"the chosen file %s cannot be taken: it is not a readable plain file of acceptable size; nothing was changed", printable(f)))
		}
	}
	return nil
}

// collapseLoose turns files that are left out for a collapsible reason into entries: a folder in which every
// file has that same reason and none is planned or left out otherwise is one entry, so a log folder does not
// list thousands of files. planned and fixed are what else lies in the folder.
func collapseLoose(loose []control.SourceExcluded, planned []string, fixed []control.SourceExcluded) []control.SourceExcluded {
	// reasons maps a folder to the reasons of everything below it.
	reasons := map[string]map[string]bool{}
	note := func(p, reason string) {
		for dir := path.Dir(p); dir != "." && dir != "/"; dir = path.Dir(dir) {
			if reasons[dir] == nil {
				reasons[dir] = map[string]bool{}
			}
			reasons[dir][reason] = true
		}
	}
	for _, p := range planned {
		note(p, "planned")
	}
	for _, e := range fixed {
		note(e.Path, e.Reason+"/"+e.Stack)
	}
	for _, e := range loose {
		note(e.Path, e.Reason+"/"+e.Stack)
	}
	var out []control.SourceExcluded
	seen := map[string]bool{}
	for _, e := range loose {
		entry := e
		if slices.Contains(collapsible, e.Reason) {
			parts := strings.Split(e.Path, "/")
			for i := 1; i < len(parts); i++ {
				dir := strings.Join(parts[:i], "/")
				if r := reasons[dir]; len(r) == 1 && r[e.Reason+"/"+e.Stack] {
					entry.Path = dir
					break
				}
			}
		}
		if !seen[entry.Path] {
			seen[entry.Path] = true
			out = append(out, entry)
		}
	}
	return out
}

// bindRoots lists the sources of the stacks' bind mounts that exist in the folder and are data, as paths below
// it. A mount the stack's references name, such as a read-only one, is no data.
func bindRoots(root string, list []stacks.Stack) []control.SourceExcluded {
	var out []control.SourceExcluded
	for _, st := range list {
		for _, bm := range st.BindMounts {
			rel := st.RepoPath(bm)
			if st.Err == nil && (slices.Contains(st.Refs.Files, rel) || slices.Contains(st.Refs.Dirs, rel) || slices.Contains(st.Refs.Writable, rel)) {
				continue
			}
			if existsNoFollow(root, rel) && !slices.ContainsFunc(out, func(o control.SourceExcluded) bool { return o.Path == rel }) {
				out = append(out, control.SourceExcluded{Path: rel, Reason: control.SourceExcludedBindMount, Stack: printable(st.Name)})
			}
		}
	}
	return out
}

// existsNoFollow tells whether rel exists below root, without following a link on the way.
func existsNoFollow(root, rel string) bool {
	cur := root
	parts := strings.Split(rel, "/")
	for i, part := range parts {
		cur = filepath.Join(cur, part)
		info, err := os.Lstat(cur)
		if err != nil {
			return false
		}
		if i < len(parts)-1 && !info.IsDir() {
			return false
		}
	}
	return true
}

var errTooManyEntries = control.NewError(control.CodeTooLarge, "the folder holds too many entries to inspect")

// oddEntries returns what git does not list and the first commit cannot take, as paths below root: folders the
// service cannot list and entries that are neither files, folders nor links. It does not look into folders that
// are excluded already or hold a repository of their own, and never follows a link.
func oddEntries(ctx context.Context, root string, skip []control.SourceExcluded) ([]control.SourceExcluded, error) {
	var out []control.SourceExcluded
	count := 0
	var walk func(rel string) error
	walk = func(rel string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		entries, err := os.ReadDir(filepath.Join(root, rel))
		if err != nil {
			if rel != "" {
				out = append(out, control.SourceExcluded{Path: rel, Reason: control.SourceExcludedUnreadable})
			}
			return nil
		}
		if count += len(entries); count > maxInspectEntries {
			return errTooManyEntries
		}
		for _, e := range entries {
			child := path.Join(rel, e.Name())
			if slices.ContainsFunc(skip, func(o control.SourceExcluded) bool { return underPath(o.Path, child) }) {
				continue
			}
			switch typ := e.Type(); {
			case typ.IsDir():
				if _, err := os.Lstat(filepath.Join(root, child, ".git")); err == nil {
					continue
				}
				if err := walk(child); err != nil {
					return err
				}
			case !typ.IsRegular() && typ&fs.ModeSymlink == 0:
				out = append(out, control.SourceExcluded{Path: child, Reason: control.SourceExcludedNotRegular})
			}
		}
		return nil
	}
	if err := walk(""); err != nil {
		return nil, err
	}
	return out, nil
}

// accessOf reads the owner and mode of a folder, and what the service may do there.
func accessOf(target string) (*control.SourceAccess, error) {
	info, err := os.Lstat(target)
	if err != nil {
		return nil, control.NewError(control.CodeInvalid, "the permissions of the folder cannot be read")
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil, control.NewError(control.CodeInvalid, "the permissions of the folder cannot be read")
	}
	acc := &control.SourceAccess{
		ModePath: target, OwnerID: st.Uid, GroupID: st.Gid, Mode: fmt.Sprintf("%04o", st.Mode&0o7777),
		ServiceCanWrite: syscall.Access(target, accessWriteSearch) == nil,
	}
	acc.SetsPermissions = !acc.ServiceCanWrite
	if u, err := user.LookupId(strconv.FormatUint(uint64(st.Uid), 10)); err == nil {
		acc.Owner = printable(u.Username)
	}
	if g, err := user.LookupGroupId(strconv.FormatUint(uint64(st.Gid), 10)); err == nil {
		acc.Group = printable(g.Name)
	}
	return acc, nil
}

// addStacks reads the stacks of the folder, checks each against the git view and looks for containers of
// other Compose projects in their folders.
func (s *inspectService) addStacks(ctx context.Context, res *control.SourceInspectResult, v verdict, root string) error {
	list := v.list
	if v.loadErr != nil {
		res.StacksError = "the stacks of the folder cannot be read: " + printable(v.loadErr.Error())
		return nil
	}
	reports, err := stackcheck.Check(ctx, list, v.view, v.remote)
	if err != nil {
		return gitFailure(err)
	}
	// Values of the env files must not reach the answer, however a rule's text came to quote them.
	red := &redact.Redactor{}
	for _, st := range list {
		registerStackSecrets(red, st)
		for _, val := range st.Env {
			red.Add(val)
		}
	}
	text := func(t string) string { return printable(red.Apply(t)) }
	for i, rep := range reports {
		out := control.SourceStack{
			Name: printable(rep.Stack), Folder: printable(list[i].Folder),
			Functions: make([]control.StackFunction, 0, len(rep.Functions)), Findings: make([]control.CheckFinding, 0, len(rep.Findings)),
		}
		for _, bm := range bindRoots(root, list[i:i+1]) {
			out.BindMounts = append(out.BindMounts, maskControl(bm.Path))
		}
		if list[i].Err == nil {
			out.Unresolved = mapAll(list[i].Refs.Unresolved, text)
		}
		if in, err := compose.InputsOf(list[i]); err == nil {
			out.Project = text(in.Project)
			out.ComposeFiles, out.EnvFiles = mapAll(in.ComposeFiles, text), mapAll(in.EnvFiles, text)
		}
		for _, f := range rep.Functions {
			out.Functions = append(out.Functions, control.StackFunction{
				Function: f.Name, Works: f.Works, Mode: f.Mode, Reason: text(f.Reason), Action: text(f.Action),
			})
		}
		for _, f := range rep.Findings {
			out.Findings = append(out.Findings, control.CheckFinding{
				Stack: printable(f.Stack), Rule: f.Rule, Severity: f.Severity, Cause: text(f.Cause), Fix: text(f.Fix),
			})
		}
		res.Stacks = append(res.Stacks, out)
	}
	if len(list) == 0 {
		return nil
	}
	conflicts, err := s.conflicts(ctx, list)
	if err != nil {
		res.ContainersError = err.Error()
		return nil
	}
	res.ContainersChecked = true
	for i := range res.Stacks {
		res.Stacks[i].Conflicts = conflicts[i]
	}
	return nil
}

// conflicts finds, per stack of list, the containers that run in the stack's folder under another Compose
// project name than the stack's.
func (s *inspectService) conflicts(ctx context.Context, list []stacks.Stack) (map[int][]control.SourceConflict, error) {
	if s.docker == nil {
		return nil, errors.New("the container engine is not available")
	}
	all, err := s.docker.List(ctx)
	if err != nil {
		var ce *control.Error
		if errors.As((&containerService{log: s.log}).dockerAnswer(err), &ce) {
			return nil, errors.New(ce.Message + "; containers were not checked")
		}
		return nil, errors.New("the container engine could not be asked; containers were not checked")
	}
	projects, _ := docker.Group(all)
	out := map[int][]control.SourceConflict{}
	for _, p := range projects {
		if p.WorkingDir == "" {
			continue
		}
		for i, st := range list {
			if st.Dir == "" || p.Name == st.Name || filepath.Clean(p.WorkingDir) != filepath.Clean(st.Dir) {
				continue
			}
			for _, c := range p.Containers {
				out[i] = append(out[i], control.SourceConflict{Container: maskControl(c.Name), Project: maskControl(p.Name), Engine: control.EngineSystem})
			}
		}
	}
	return out, nil
}

// mapAll applies f to every element; an empty list stays nil.
func mapAll(v []string, f func(string) string) []string {
	var out []string
	for _, e := range v {
		out = append(out, f(e))
	}
	return out
}
