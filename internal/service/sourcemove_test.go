package service_test

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-kit/docker"
	"github.com/castrowithcee/qatlas-kit/output"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

const (
	moveCompose = "services:\n  app:\n    image: registry.example.test/demo/web:${WEB_VERSION}\n    volumes:\n      - ./data:/var/lib/demo\n"
)

func projectFiles() map[string]string {
	return map[string]string{
		"compose.yaml": moveCompose, ".env": "WEB_VERSION=1.2.3\n", "data/db.bin": "x", "notes.txt": "x",
	}
}

// moveSource is a service with a local source whose only stack is "other".
func moveSource(t *testing.T, fd *fakeDocker, extra map[string]string) (h *harness, into string) {
	t.Helper()
	h = realGitService(t, withDocker(fd))
	files := map[string]string{"other/compose.yaml": moveCompose, "other/.env": "WEB_VERSION=1.2.3\n"}
	for k, v := range extra {
		files[k] = v
	}
	into = localRepo(t, files)
	if _, err := h.sourceAdd(into); err != nil {
		t.Fatal(err)
	}
	return h, into
}

// project makes a project folder outside every source and returns its path.
func project(t *testing.T, name string, files map[string]string) string {
	t.Helper()
	dir := filepath.Join(tempBase(t), name)
	writeTree(t, dir, files)
	return dir
}

func (h *harness) move(path, into string) control.SourceMoveResult {
	h.t.Helper()
	var res control.SourceMoveResult
	if err := h.call(control.SourceMoveCommand, control.SourceMoveArgs{Path: path, Into: into}, &res); err != nil {
		h.t.Fatalf("move %s: %v", path, err)
	}
	return res
}

func issue(list []control.SourceMoveIssue, code string) (control.SourceMoveIssue, bool) {
	for _, i := range list {
		if i.Code == code {
			return i, true
		}
	}
	return control.SourceMoveIssue{}, false
}

func wantBlock(t *testing.T, res control.SourceMoveResult, code string) control.SourceMoveIssue {
	t.Helper()
	i, ok := issue(res.Blocks, code)
	if !ok || i.Reason == "" || i.Fix == "" {
		t.Fatalf("no block %s with reason and fix in %+v", code, res.Blocks)
	}
	return i
}

func wantNoBlock(t *testing.T, res control.SourceMoveResult, code string) {
	t.Helper()
	if _, ok := issue(res.Blocks, code); ok {
		t.Fatalf("unexpected block %s in %+v", code, res.Blocks)
	}
}

func TestMovePreviewListsTargetFilesAndExclusionsAndChangesNothing(t *testing.T) {
	h, into := moveSource(t, &fakeDocker{}, nil)
	p := project(t, "myapp", projectFiles())
	beforeP, beforeQ := tree(t, p), tree(t, into)

	res := h.move(p, into)
	if len(res.Blocks) != 0 || res.Target != filepath.Join(into, "myapp") || res.SourceKind != control.SourceKindLocal {
		t.Fatalf("result = %+v", res)
	}
	if res.Project != "myapp" || res.NewProject != "myapp" {
		t.Fatalf("project %q -> %q", res.Project, res.NewProject)
	}
	// The files are those of a first commit of the folder, the exclusions go to its .gitignore.
	if want := []string{".env", "compose.yaml"}; !slices.Equal(res.PlannedFiles, want) {
		t.Fatalf("planned = %q, want %q", res.PlannedFiles, want)
	}
	if !slices.Equal(res.BindMounts, []string{"data"}) {
		t.Fatalf("bind mounts = %q", res.BindMounts)
	}
	why := excludedReasons(control.SourceInspectResult{Excluded: res.Excluded})
	if why["data"] != control.SourceExcludedBindMount || why["notes.txt"] != control.SourceExcludedNotReferenced {
		t.Fatalf("excluded = %+v", res.Excluded)
	}
	if res.Ignore == nil || res.Ignore.Action != control.SourceIgnoreCreate || !slices.Contains(res.Ignore.Lines, "/data") {
		t.Fatalf("ignore = %+v", res.Ignore)
	}
	if !maps.Equal(beforeP, tree(t, p)) || !maps.Equal(beforeQ, tree(t, into)) {
		t.Fatal("the preview changed a folder")
	}
	if _, err := os.Lstat(filepath.Join(p, ".gitignore")); err == nil {
		t.Fatal("the preview wrote the .gitignore")
	}
	if _, err := os.Lstat(res.Target); err == nil {
		t.Fatal("the preview made the target")
	}
}

func TestMoveBlocksAnUnknownSource(t *testing.T) {
	h, into := moveSource(t, &fakeDocker{}, nil)
	var res control.SourceMoveResult
	err := h.call(control.SourceMoveCommand, control.SourceMoveArgs{Path: project(t, "myapp", projectFiles()), Into: into + "-none"}, &res)
	wantCode(t, err, control.CodeNoSuchSource)
}

func TestMoveBlocksARootStackSource(t *testing.T) {
	h, into := moveSource(t, &fakeDocker{}, map[string]string{"compose.yaml": moveCompose, ".env": "WEB_VERSION=1\n"})
	res := h.move(project(t, "myapp", projectFiles()), into)
	b := wantBlock(t, res, control.MoveRootStack)
	if !strings.Contains(b.Reason, "root stack") {
		t.Fatalf("block = %+v", b)
	}
}

func TestMoveBlocksATargetThatExistsAndAHiddenName(t *testing.T) {
	h, into := moveSource(t, &fakeDocker{}, nil)
	res := h.move(project(t, "other", projectFiles()), into)
	wantBlock(t, res, control.MoveTargetExists)
	res = h.move(project(t, ".hidden", projectFiles()), into)
	wantBlock(t, res, control.MoveTargetName)
	wantNoBlock(t, res, control.MoveTargetExists)
	// A revert of the move commit takes only plain stack folder names.
	files := projectFiles()
	files[".env"] += "COMPOSE_PROJECT_NAME=myapp\n"
	wantBlock(t, h.move(project(t, "my+app", files), into), control.MoveTargetName)
}

func TestMoveBlocksAProjectNameThatChanges(t *testing.T) {
	fd := &fakeDocker{}
	h, into := moveSource(t, fd, nil)
	p := project(t, "myapp", projectFiles())
	fd.list = []docker.Container{running("c1", "legacy-app-1", map[string]string{lblProject: "legacy", lblDir: p})}
	res := h.move(p, into)
	b := wantBlock(t, res, control.MoveProjectName)
	if res.Project != "legacy" || res.NewProject != "myapp" || !strings.Contains(b.Fix, "COMPOSE_PROJECT_NAME") {
		t.Fatalf("result = %+v", res)
	}
	if len(res.Running) != 1 || res.Running[0] != (control.SourceMoveRunning{Container: "legacy-app-1", Project: "legacy"}) {
		t.Fatalf("running = %+v", res.Running)
	}

	// The same name in the compose file keeps the project.
	files := projectFiles()
	files["compose.yaml"] = "name: legacy\n" + moveCompose
	q := project(t, "myapp", files)
	fd.list = []docker.Container{running("c1", "legacy-app-1", map[string]string{lblProject: "legacy", lblDir: q})}
	res = h.move(q, into)
	wantNoBlock(t, res, control.MoveProjectName)
	if res.Project != "legacy" || res.NewProject != "legacy" {
		t.Fatalf("pinned: %q -> %q", res.Project, res.NewProject)
	}
	// Without running containers the name of the folder stays the name of the folder.
	fd.list = nil
	wantNoBlock(t, h.move(p, into), control.MoveProjectName)
}

func TestMoveBlocksAStackNameThatIsTaken(t *testing.T) {
	h, into := moveSource(t, &fakeDocker{}, nil)
	files := projectFiles()
	files["compose.yaml"] = "name: other\n" + moveCompose
	res := h.move(project(t, "myapp", files), into)
	b := wantBlock(t, res, control.MoveNameTaken)
	if !strings.Contains(b.Reason, "other") {
		t.Fatalf("block = %+v", b)
	}
	wantNoBlock(t, res, control.MoveProjectName)
}

func TestMoveBlocksAnAbsoluteBindMountIntoTheOldFolder(t *testing.T) {
	h, into := moveSource(t, &fakeDocker{}, nil)
	p := filepath.Join(tempBase(t), "myapp")
	files := projectFiles()
	files["compose.yaml"] = "services:\n  app:\n    image: registry.example.test/demo/web:${WEB_VERSION}\n    volumes:\n" +
		"      - " + p + "/data:/var/lib/demo\n      - /srv/shared/cache:/cache\n      - ./data:/other\n"
	writeTree(t, p, files)
	res := h.move(p, into)
	b := wantBlock(t, res, control.MoveAbsoluteBind)
	if !slices.Equal(b.Items, []string{p + "/data"}) {
		t.Fatalf("items = %q", b.Items)
	}
	// Relative mounts alone are fine.
	wantNoBlock(t, h.move(project(t, "plain", projectFiles()), into), control.MoveAbsoluteBind)
}

func TestMoveSecretsInEnvBlockWithARemoteAndWarnWithout(t *testing.T) {
	files := projectFiles()
	files[".env"] = "WEB_VERSION=1.2.3\nAPI_PASSWORD=" + fakeSecretValue + "\nEMPTY_TOKEN=\nPLAIN=1\n"
	p := project(t, "myapp", files)

	h, into := moveSource(t, &fakeDocker{}, nil)
	res := h.move(p, into)
	wantNoBlock(t, res, control.MoveSecretsInEnv)
	w, ok := issue(res.Warnings, control.MoveSecretsInEnv)
	if !ok || !slices.Equal(w.Items, []string{"API_PASSWORD"}) || w.Fix == "" {
		t.Fatalf("warnings = %+v", res.Warnings)
	}

	remote := startWith(t, nil, withDocker(&fakeDocker{}))
	remote.withKey()
	if _, err := remote.set(remote.srv.Key().Fingerprint); err != nil {
		t.Fatal(err)
	}
	res = remote.move(p, remote.dir)
	b := wantBlock(t, res, control.MoveSecretsInEnv)
	if !slices.Equal(b.Items, []string{"API_PASSWORD"}) || !strings.Contains(b.Fix, "secrets.env") {
		t.Fatalf("block = %+v", b)
	}
	if _, ok := issue(res.Warnings, control.MoveSecretsInEnv); ok {
		t.Fatal("a block is also a warning")
	}
	for _, r := range []control.SourceMoveResult{res, h.move(p, into)} {
		raw, _ := json.Marshal(r)
		if strings.Contains(string(raw), fakeSecretValue) {
			t.Fatalf("a secret value reached the answer: %s", raw)
		}
	}
}

func TestMoveWarnsOfFixedTags(t *testing.T) {
	h, into := moveSource(t, &fakeDocker{}, nil)
	files := projectFiles()
	files["compose.yaml"] = "services:\n  app:\n    image: registry.example.test/demo/web:1.2.3\n"
	res := h.move(project(t, "myapp", files), into)
	w, ok := issue(res.Warnings, control.MoveFixedTags)
	if !ok || len(res.Blocks) != 0 || len(w.Items) != 1 || !strings.Contains(w.Items[0], "app: registry.example.test/demo/web") {
		t.Fatalf("result = %+v", res)
	}
	if _, ok := issue(h.move(project(t, "plain", projectFiles()), into).Warnings, control.MoveFixedTags); ok {
		t.Fatal("a tag variable counts as fixed")
	}
}

func TestMoveBlocksFoldersThatAreNoProject(t *testing.T) {
	h, into := moveSource(t, &fakeDocker{}, nil)

	empty := filepath.Join(tempBase(t), "empty")
	if err := os.Mkdir(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	wantBlock(t, h.move(empty, into), control.MoveNotAProject)

	stacksOnly := project(t, "several", map[string]string{"a/compose.yaml": moveCompose, "a/.env": "WEB_VERSION=1\n"})
	res := h.move(stacksOnly, into)
	wantBlock(t, res, control.MoveNotAProject)

	repo := project(t, "cloned", projectFiles())
	gitIn(t, repo, "init", "-q", "-b", "main")
	wantBlock(t, h.move(repo, into), control.MoveIsRepository)

	// The folder lies in the source or contains it.
	wantBlock(t, h.move(filepath.Join(into, "other"), into), control.MoveFolderRejected)
	wantBlock(t, h.move(filepath.Dir(into), into), control.MoveFolderRejected)
	wantBlock(t, h.move("relative/dir", into), control.MoveFolderRejected)

	broken := project(t, "broken", map[string]string{"compose.yaml": "services: [\n"})
	wantBlock(t, h.move(broken, into), control.MoveStackUnreadable)
}

func TestMoveNamesRunningContainersAndSaysWhenTheEngineCannotBeAsked(t *testing.T) {
	fd := &fakeDocker{}
	h, into := moveSource(t, fd, nil)
	p := project(t, "myapp", projectFiles())
	fd.list = []docker.Container{
		running("c1", "myapp-app-1", map[string]string{lblProject: "myapp", lblDir: p}),
		running("c2", "other-1", map[string]string{lblProject: "myapp", lblDir: "/srv/else"}),
	}
	res := h.move(p, into)
	if !res.ContainersChecked || len(res.Running) != 1 || res.Running[0].Container != "myapp-app-1" || len(res.Blocks) != 0 {
		t.Fatalf("result = %+v", res)
	}
	fd.listErr = &docker.Error{Code: output.CodeUnreachable, Message: "x"}
	res = h.move(p, into)
	if res.ContainersChecked || !strings.Contains(res.ContainersError, "not reachable") || len(res.Running) != 0 || res.Project != "myapp" {
		t.Fatalf("unreachable engine = %+v", res)
	}
}

func TestMoveMasksUnprintableNames(t *testing.T) {
	h, into := moveSource(t, &fakeDocker{}, nil)
	files := projectFiles()
	files["odd\x01name.txt"] = "x"
	res := h.move(project(t, "myapp", files), into)
	found := false
	for _, e := range res.Excluded {
		found = found || e.Path == "odd�name.txt"
		if strings.ContainsRune(e.Path, 1) {
			t.Fatalf("unmasked path %q", e.Path)
		}
	}
	if !found {
		t.Fatalf("excluded = %+v", res.Excluded)
	}
}
