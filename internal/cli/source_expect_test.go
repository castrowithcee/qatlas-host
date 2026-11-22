package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// inspectJSON runs source inspect with the JSON output and returns the answer as a map.
func inspectJSON(t *testing.T, path string, extra ...string) (raw string, fields map[string]any) {
	t.Helper()
	args := append([]string{"source", "inspect", path, "--output", "json"}, extra...)
	code, stdout, stderr := run(t, args...)
	if code != 0 {
		t.Fatalf("source inspect: code %d, stderr %q", code, stderr)
	}
	if err := json.Unmarshal([]byte(stdout), &fields); err != nil {
		t.Fatalf("invalid JSON %q: %v", stdout, err)
	}
	return stdout, fields
}

func TestSourceInspectAsRootShowsMissingPermissionsAndEffect(t *testing.T) {
	useFolderHost(t)
	tree := newFilesTree(t)
	startFakeService(t, fakeHandlers{control.SourceInspectCommand: inspectWith(listedFilesInspect())})
	raw, got := inspectJSON(t, tree.dir)

	if got["missing"] != false {
		t.Errorf("missing = %v, want false", got["missing"])
	}
	perms, ok := got["permissions"].(map[string]any)
	if !ok || perms["changed"].(float64) < 1 {
		t.Fatalf("permissions = %v", got["permissions"])
	}
	summary, _ := perms["summary"].([]any)
	if len(summary) == 0 || !strings.Contains(summary[0].(string), "paths get the group of the service account") {
		t.Errorf("summary = %v", perms["summary"])
	}
	if effect, _ := got["effect"].(string); !strings.HasPrefix(effect, "sha256:") && len(effect) < 16 {
		t.Errorf("effect = %q", effect)
	}

	// The TUI decodes the answer as the service result; every field of the service survives.
	var decoded control.SourceInspectResult
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatal(err)
	}
	want := listedFilesInspect()
	want.Path = tree.dir
	if !reflect.DeepEqual(decoded, want) {
		t.Fatalf("decoded %+v, want %+v", decoded, want)
	}
}

func TestSourceInspectJSONDecodesAsTheServiceResult(t *testing.T) {
	useFolderHost(t)
	folders.euid = func() int { return 1000 }
	gw := false
	results := []control.SourceInspectResult{
		{Case: control.SourceCaseClone, URL: inspectURL, NeedsURL: true, Stacks: []control.SourceStack{},
			Access: &control.SourceAccess{ModePath: "/srv", Owner: "root", Group: "root", Mode: "0755", GitWritable: &gw}},
		{Case: control.SourceCaseReject, Reason: "the folder is not empty", Fix: "Choose another folder.", Stacks: []control.SourceStack{}},
		filesResult(control.SourceInspectArgs{Path: "/srv/files"}),
	}
	var mu sync.Mutex
	next := 0
	startFakeService(t, fakeHandlers{control.SourceInspectCommand: func(raw json.RawMessage) any {
		mu.Lock()
		defer mu.Unlock()
		r := results[next]
		var a control.SourceInspectArgs
		_ = json.Unmarshal(raw, &a)
		r.Path = a.Path
		return r
	}})
	for i := range results {
		mu.Lock()
		next = i
		mu.Unlock()
		want := results[i]
		want.Path = "/srv/files"
		code, stdout, stderr := run(t, "source", "inspect", "/srv/files", "--output", "json")
		var got control.SourceInspectResult
		if err := json.Unmarshal([]byte(stdout), &got); code != 0 || err != nil {
			t.Fatalf("case %d: code %d, err %v, stderr %q, stdout %q", i, code, err, stderr, stdout)
		}
		if got.Stacks == nil {
			got.Stacks = []control.SourceStack{}
		}
		if want.Stacks == nil {
			want.Stacks = []control.SourceStack{}
		}
		// The engines that cannot be asked are this client's answer, not the service's.
		want.EnginesNotChecked = got.EnginesNotChecked
		if !reflect.DeepEqual(got, want) {
			t.Errorf("case %d: decoded %+v, want %+v", i, got, want)
		}
	}
}

func TestSourceInspectAsRootReportsAMissingFolder(t *testing.T) {
	useFolderHost(t)
	dir := filepath.Join(t.TempDir(), "new-stacks")
	res := control.SourceInspectResult{Case: control.SourceCaseClone, URL: inspectURL, Stacks: []control.SourceStack{},
		Access: &control.SourceAccess{ModePath: filepath.Dir(dir), Mode: "0755", ServiceCanWrite: true}}
	startFakeService(t, fakeHandlers{control.SourceInspectCommand: inspectWith(res)})
	_, got := inspectJSON(t, dir)
	if got["missing"] != true || got["effect"] == nil {
		t.Fatalf("result = %v", got)
	}
	if _, ok := got["permissions"]; ok {
		t.Errorf("an empty plan is shown: %v", got["permissions"])
	}
}

func TestSourceInspectWithoutRootHasNoEffect(t *testing.T) {
	useFolderHost(t)
	folders.euid = func() int { return 1000 }
	tree := newFilesTree(t)
	startFakeService(t, fakeHandlers{control.SourceInspectCommand: inspectWith(listedFilesInspect())})
	_, got := inspectJSON(t, tree.dir)
	for _, k := range []string{"missing", "permissions", "effect"} {
		if _, ok := got[k]; ok {
			t.Errorf("a run without root has %s", k)
		}
	}
}

func TestSourceInspectEffectInTextAndTOON(t *testing.T) {
	useFolderHost(t)
	tree := newFilesTree(t)
	startFakeService(t, fakeHandlers{control.SourceInspectCommand: inspectWith(listedFilesInspect())})
	_, text, _ := run(t, "source", "inspect", tree.dir)
	for _, want := range []string{"MISSING", "PERMISSIONS_CHANGED", "PERMISSION", "paths get the group of the service account", "EFFECT"} {
		if !strings.Contains(text, want) {
			t.Errorf("text lacks %q:\n%s", want, text)
		}
	}
	_, toon, _ := run(t, "source", "inspect", tree.dir, "--output", "toon")
	for _, want := range []string{"missing", "permissions", "changed", "summary", "effect"} {
		if !strings.Contains(toon, want) {
			t.Errorf("toon lacks %q:\n%s", want, toon)
		}
	}
}

func TestSourceInspectRejectionHasNoEffect(t *testing.T) {
	useFolderHost(t)
	startFakeService(t, fakeHandlers{control.SourceInspectCommand: inspectWith(control.SourceInspectResult{
		Case: control.SourceCaseReject, Reason: "r", Fix: "f"})})
	_, got := inspectJSON(t, "/srv/demo")
	for _, k := range []string{"missing", "permissions", "effect"} {
		if _, ok := got[k]; ok {
			t.Errorf("a rejection has %s", k)
		}
	}
}

// mutableInspect answers source.inspect with the current value of res.
type mutableInspect struct {
	mu  sync.Mutex
	res control.SourceInspectResult
}

func (m *mutableInspect) handler(raw json.RawMessage) any {
	m.mu.Lock()
	defer m.mu.Unlock()
	var a control.SourceInspectArgs
	_ = json.Unmarshal(raw, &a)
	r := m.res
	r.Path = a.Path
	return r
}

func (m *mutableInspect) set(f func(*control.SourceInspectResult)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	f(&m.res)
}

func TestSourceAddExpectEffectOfInspectApplies(t *testing.T) {
	d := useFolderHost(t)
	tree := newFilesTree(t)
	insp := &mutableInspect{res: listedFilesInspect()}
	handlers := addHandlers(insp.res)
	handlers[control.SourceInspectCommand] = insp.handler
	svc := startFakeService(t, handlers)
	useStdin(t, strings.NewReader(""))

	_, got := inspectJSON(t, tree.dir)
	effect, _ := got["effect"].(string)
	if effect == "" {
		t.Fatalf("no effect: %v", got)
	}
	code, stdout, stderr := run(t, "source", "add", tree.dir, "--expect-effect", effect, "--yes", "--output", "json")
	if code != 0 {
		t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if svc.count(control.SourceAddCommand) != 1 || len(d.lchowns) == 0 || !strings.Contains(d.dropIn(t), tree.dir) {
		t.Fatalf("add calls %d, chowns %d, drop-in %q", svc.count(control.SourceAddCommand), len(d.lchowns), d.dropIn(t))
	}
}

func TestSourceAddExpectEffectMismatchChangesNothing(t *testing.T) {
	for name, change := range map[string]func(*control.SourceInspectResult){
		"another file": func(r *control.SourceInspectResult) { r.PlannedFiles = append(r.PlannedFiles, "new.yaml") },
		"left out":     func(r *control.SourceInspectResult) { r.Excluded = r.Excluded[:1] },
		"no permission": func(r *control.SourceInspectResult) {
			r.Access = &control.SourceAccess{Mode: "0750", ServiceCanWrite: true}
		},
	} {
		t.Run(name, func(t *testing.T) {
			d := useFolderHost(t)
			tree := newFilesTree(t)
			insp := &mutableInspect{res: listedFilesInspect()}
			handlers := addHandlers(insp.res)
			handlers[control.SourceInspectCommand] = insp.handler
			svc := startFakeService(t, handlers)
			useStdin(t, strings.NewReader(""))
			_, got := inspectJSON(t, tree.dir)
			effect, _ := got["effect"].(string)
			before := tree.snapshot(t)

			insp.set(change)
			code, stdout, stderr := run(t, "source", "add", tree.dir, "--expect-effect", effect, "--yes")
			if code == 0 || stdout != "" || !strings.Contains(stderr, "state-changed") ||
				!strings.Contains(stderr, "the folder changed since it was inspected; nothing was changed") ||
				!strings.Contains(stderr, "qhost source inspect "+tree.dir) {
				t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
			}
			if svc.count(control.SourceAddCommand) != 0 || len(d.lchowns) != 0 || d.dropIn(t) != "" || len(d.cmds) != 0 ||
				!equalModes(before, tree.snapshot(t)) {
				t.Fatal("a refused source add changed something")
			}
		})
	}
}

func TestSourceAddExpectEffectIgnoresStacksConflictsAndURL(t *testing.T) {
	useFolderHost(t)
	tree := newFilesTree(t)
	insp := &mutableInspect{res: listedFilesInspect()}
	handlers := addHandlers(insp.res)
	handlers[control.SourceInspectCommand] = insp.handler
	startFakeService(t, handlers)
	useStdin(t, strings.NewReader(""))
	_, got := inspectJSON(t, tree.dir)
	effect, _ := got["effect"].(string)

	insp.set(func(r *control.SourceInspectResult) {
		r.URL = inspectURL
		r.NeedsURL = true
		r.Stacks = append(r.Stacks, control.SourceStack{Name: "other", Conflicts: []control.SourceConflict{{Container: "c-1", Project: "p"}}})
		r.Stacks[0].BindMounts = []string{"data"}
	})
	code, _, stderr := run(t, "source", "add", tree.dir, "--expect-effect", effect, "--yes")
	if code != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
}

func TestSourceAddWithoutExpectEffectIsUnchanged(t *testing.T) {
	useFolderHost(t)
	tree := newFilesTree(t)
	svc := startFakeService(t, addHandlers(listedFilesInspect()))
	useStdin(t, strings.NewReader(""))
	if code, _, stderr := run(t, "source", "add", tree.dir, "--yes"); code != 0 || svc.count(control.SourceAddCommand) != 1 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
}

func TestSourceRemoteExpectStateAndCommit(t *testing.T) {
	tests := []struct {
		name          string
		compare       control.SourceCompareResult
		state, commit string
		changed       bool
	}{
		{"different matches", differentCompare(control.SourceCaseLocal), "different", remoteCommit, false},
		{"empty remote needs no commit", control.SourceCompareResult{Case: control.SourceCaseLocal, State: control.SourceRemoteEmpty, Branch: "main"},
			"empty", "", false},
		{"commit moved", differentCompare(control.SourceCaseLocal), "different", "fedcba9876543210fedcba9876543210fedcba98", true},
		{"state moved", differentCompare(control.SourceCaseLocal), "equal", remoteCommit, true},
		{"remote got its first commit", differentCompare(control.SourceCaseLocal), "empty", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			useFolderHost(t)
			fake := &remoteFake{compare: tc.compare}
			startFakeService(t, fake.handlers(localInspect(false)))
			useStdin(t, strings.NewReader(""))
			args := []string{"source", "remote", t.TempDir(), remoteURL, "--fingerprint", remoteKeyPrint, "--yes",
				"--expect-state", tc.state}
			if tc.commit != "" {
				args = append(args, "--expect-commit", tc.commit)
			}
			code, stdout, stderr := run(t, args...)
			if tc.changed {
				if code == 0 || stdout != "" || !strings.Contains(stderr, "state-changed") ||
					!strings.Contains(stderr, "the remote or the folder changed since the comparison was shown; nothing was changed") ||
					!strings.Contains(stderr, "qhost source compare") || !fake.untouched() {
					t.Fatalf("code %d, stdout %q, stderr %q, untouched %v", code, stdout, stderr, fake.untouched())
				}
				return
			}
			if code != 0 || len(fake.remoted) != 1 {
				t.Fatalf("code %d, stderr %q, calls %d", code, stderr, len(fake.remoted))
			}
		})
	}
}

func TestExpectStateAndCommitBelongTogether(t *testing.T) {
	for name, flags := range map[string][]string{
		"commit alone":              {"--expect-commit", remoteCommit},
		"state needs the commit":    {"--expect-state", "different"},
		"equal needs the commit":    {"--expect-state", "equal"},
		"empty takes no commit":     {"--expect-state", "empty", "--expect-commit", remoteCommit},
		"unknown state":             {"--expect-state", "other", "--expect-commit", remoteCommit},
		"unknown state without any": {"--expect-state", "other"},
	} {
		for _, cmd := range [][]string{{"source", "remote"}, {"source", "add"}} {
			t.Run(name+" "+cmd[1], func(t *testing.T) {
				useFolderHost(t)
				tree := newFilesTree(t)
				fake := &remoteFake{compare: differentCompare(control.SourceCaseLocal)}
				svc := startFakeService(t, fake.handlers(localInspect(false)))
				useStdin(t, strings.NewReader(""))
				args := append([]string{}, cmd...)
				args = append(args, tree.dir)
				if cmd[1] == "remote" {
					args = append(args, remoteURL)
				}
				args = append(args, "--fingerprint", remoteKeyPrint, "--yes")
				code, _, stderr := run(t, append(args, flags...)...)
				if code != 2 || !fake.untouched() || svc.count(control.SourceCompareCommand) != 0 {
					t.Fatalf("code %d, stderr %q", code, stderr)
				}
			})
		}
	}
}

func TestSourceAddExpectRemoteOnlyForFilesWithURL(t *testing.T) {
	useFolderHost(t)
	tree := newPermTree(t)
	fake := &remoteFake{compare: differentCompare(control.SourceCaseLocal)}
	svc := startFakeService(t, fake.handlers(localInspect(true)))
	useStdin(t, strings.NewReader(""))
	before := tree.snapshot(t)
	code, _, stderr := run(t, "source", "add", tree.dir, "--yes", "--expect-state", "different", "--expect-commit", remoteCommit)
	if code != 2 || !strings.Contains(stderr, "only to a folder of files with --url") || !fake.untouched() ||
		svc.count(control.SourceAddCommand) != 0 || !equalModes(before, tree.snapshot(t)) {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
}

func TestSourceAddFilesWithURLExpectStateAndCommit(t *testing.T) {
	for name, tc := range map[string]struct {
		state, commit string
		changed       bool
	}{
		"matches":      {"different", remoteCommit, false},
		"commit moved": {"different", "fedcba9876543210fedcba9876543210fedcba98", true},
		"state moved":  {"equal", remoteCommit, true},
		"emptied":      {"empty", "", true},
	} {
		t.Run(name, func(t *testing.T) {
			d := useFolderHost(t)
			tree := newFilesTree(t)
			fake := &remoteFake{compare: differentCompare(control.SourceCaseFiles)}
			startFakeService(t, fake.handlers(listedFilesInspect()))
			useStdin(t, strings.NewReader(""))
			before := tree.snapshot(t)
			args := []string{"source", "add", tree.dir, "--url", remoteURL, "--fingerprint", remoteKeyPrint, "--yes", "--expect-state", tc.state}
			if tc.commit != "" {
				args = append(args, "--expect-commit", tc.commit)
			}
			code, stdout, stderr := run(t, args...)
			if !tc.changed {
				if code != 0 || len(fake.added) != 1 || fake.added[0].ExpectState != "different" || fake.added[0].ExpectCommit != remoteCommit {
					t.Fatalf("code %d, stderr %q, added %+v", code, stderr, fake.added)
				}
				return
			}
			if code == 0 || stdout != "" || !strings.Contains(stderr, "state-changed") ||
				!strings.Contains(stderr, "the remote or the folder changed since the comparison was shown; nothing was changed") {
				t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
			}
			if !fake.untouched() || !equalModes(before, tree.snapshot(t)) || len(d.lchowns) != 0 || d.dropIn(t) != "" || len(d.cmds) != 0 {
				t.Fatal("a refused source add changed something")
			}
		})
	}
}

func TestRepoProbeShowsWhatTheServerPresents(t *testing.T) {
	useFolderHost(t)
	svc := startFakeService(t, fakeHandlers{control.RepoProbeCommand: func(raw json.RawMessage) any {
		var a control.RepoProbeArgs
		_ = json.Unmarshal(raw, &a)
		if a.URL != remoteURL {
			return control.NewError(control.CodeInvalid, "unexpected address")
		}
		return control.RepoProbeResult{Host: "git.example.test", Port: "2222", KeyType: "ssh-ed25519",
			PublicKey: "AAAAdemo", Fingerprint: remoteKeyPrint, Pinned: true}
	}})
	code, stdout, stderr := run(t, "repo", "probe", remoteURL, "--output", "json")
	if code != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	var got control.RepoProbeResult
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("invalid JSON %q: %v", stdout, err)
	}
	want := control.RepoProbeResult{Host: "git.example.test", Port: "2222", KeyType: "ssh-ed25519", PublicKey: "AAAAdemo",
		Fingerprint: remoteKeyPrint, Pinned: true}
	if got != want || svc.count(control.RepoProbeCommand) != 1 || len(svc.calls) != 1 {
		t.Fatalf("got %+v, calls %v", got, svc.calls)
	}
	_, text, _ := run(t, "repo", "probe", remoteURL)
	for _, w := range []string{"HOST", "KEY_TYPE", "PUBLIC_KEY", "FINGERPRINT", remoteKeyPrint, "PINNED"} {
		if !strings.Contains(text, w) {
			t.Errorf("text lacks %q:\n%s", w, text)
		}
	}
}

func TestRepoProbeNeedsRootAndAValidAddress(t *testing.T) {
	useFolderHost(t)
	fake := &remoteFake{}
	svc := startFakeService(t, fake.handlers(localInspect(false)))
	for _, args := range [][]string{{"repo", "probe"}, {"repo", "probe", "a", "b"}, {"repo", "probe", "https://example.test/x.git"}} {
		if code, _, _ := run(t, args...); code != 2 {
			t.Errorf("%v: code %d, want 2", args, code)
		}
	}
	folders.euid = func() int { return 1000 }
	if code, _, stderr := run(t, "repo", "probe", remoteURL); code == 0 || !strings.Contains(stderr, "sudo") {
		t.Errorf("a non-root run: code %d, stderr %q", code, stderr)
	}
	if len(svc.calls) != 0 {
		t.Fatalf("a refused call reached the service: %v", svc.calls)
	}
}

func TestSourceCompareShowsTheComparisonAndChangesNothing(t *testing.T) {
	useFolderHost(t)
	cmp := secretCompare(control.SourceCaseFiles)
	cmp.PlannedFiles, cmp.RemoteFiles = []string{"README.md", "compose.yaml"}, []string{"README.md"}
	cmp.Ignore = &control.SourceIgnore{Action: control.SourceIgnoreCreate, Lines: []string{"/notes.txt"}}
	fake := &remoteFake{compare: cmp}
	svc := startFakeService(t, fake.handlers(localInspect(false)))
	useStdin(t, strings.NewReader(""))
	dir := t.TempDir()

	code, stdout, stderr := run(t, "source", "compare", dir, remoteURL, "--fingerprint", remoteKeyPrint, "--output", "json")
	if code != 0 {
		t.Fatalf("a secrets finding is an answer: code %d, stderr %q", code, stderr)
	}
	var got control.SourceCompareResult
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("invalid JSON %q: %v", stdout, err)
	}
	if !reflect.DeepEqual(got, cmp) {
		t.Fatalf("got %+v, want %+v", got, cmp)
	}
	var fields map[string]any
	_ = json.Unmarshal([]byte(stdout), &fields)
	for _, k := range []string{"case", "state", "branch", "commit", "paths", "path_count", "planned_files", "excluded", "remote_files", "ignore", "secrets"} {
		if _, ok := fields[k]; !ok {
			t.Errorf("no field %s in %s", k, stdout)
		}
	}
	if secrets := fields["secrets"].(map[string]any); secrets["total"].(float64) != 3 || len(secrets["findings"].([]any)) != 2 {
		t.Errorf("secrets = %v", secrets)
	}
	if !fake.untouched() || svc.count(control.SourceInspectCommand) != 0 ||
		len(fake.compared) != 1 || !reflect.DeepEqual(fake.compared[0], control.SourceCompareArgs{Path: dir, URL: remoteURL, Fingerprint: remoteKeyPrint}) {
		t.Fatalf("compared %+v", fake.compared)
	}

	_, text, _ := run(t, "source", "compare", dir, remoteURL, "--fingerprint", remoteKeyPrint)
	for _, w := range []string{"STATE", "different", "BRANCH", "main", "PATH_COUNT", "SECRETS_TOTAL", "0123456789ab secrets.env (secrets-file)"} {
		if !strings.Contains(text, w) {
			t.Errorf("text lacks %q:\n%s", w, text)
		}
	}
}

func TestSourceCompareOmitsWhatIsEmpty(t *testing.T) {
	useFolderHost(t)
	fake := &remoteFake{compare: control.SourceCompareResult{Case: control.SourceCaseLocal, State: control.SourceRemoteEmpty, Branch: "main"}}
	startFakeService(t, fake.handlers(localInspect(false)))
	_, stdout, _ := run(t, "source", "compare", t.TempDir(), remoteURL, "--fingerprint", remoteKeyPrint, "--output", "json")
	var fields map[string]any
	if err := json.Unmarshal([]byte(stdout), &fields); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"commit", "paths", "planned_files", "secrets"} {
		if _, ok := fields[k]; ok {
			t.Errorf("empty %s is shown: %s", k, stdout)
		}
	}
	if fields["path_count"] != float64(0) {
		t.Errorf("path_count = %v", fields["path_count"])
	}
}

func TestSourceCompareNeedsRootATrustedKeyAndValidInput(t *testing.T) {
	useFolderHost(t)
	fake := &remoteFake{compare: differentCompare(control.SourceCaseLocal)}
	svc := startFakeService(t, fake.handlers(localInspect(false)))
	useStdin(t, strings.NewReader(""))
	dir := t.TempDir()
	for _, args := range [][]string{
		{"source", "compare"}, {"source", "compare", dir}, {"source", "compare", "rel", remoteURL},
		{"source", "compare", dir, "https://example.test/x.git"}, {"source", "compare", dir, remoteURL, "--fingerprint", "md5:abc"},
	} {
		if code, _, _ := run(t, args...); code != 2 {
			t.Errorf("%v: code %d, want 2", args, code)
		}
	}
	// Without a terminal and without --fingerprint the host key is not trusted, and --yes does not exist here.
	code, _, stderr := run(t, "source", "compare", dir, remoteURL)
	if code != 2 || !strings.Contains(stderr, remoteKeyPrint) || svc.count(control.SourceCompareCommand) != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	code, _, stderr = run(t, "source", "compare", dir, remoteURL, "--fingerprint", "SHA256:other")
	if code == 0 || svc.count(control.SourceCompareCommand) != 0 {
		t.Fatalf("a wrong fingerprint was accepted: code %d, stderr %q", code, stderr)
	}
	probes := svc.count(control.RepoProbeCommand)
	folders.euid = func() int { return 1000 }
	if code, _, _ := run(t, "source", "compare", dir, remoteURL, "--fingerprint", remoteKeyPrint); code == 0 || svc.count(control.RepoProbeCommand) != probes {
		t.Fatalf("a non-root run: code %d, calls %v", code, svc.calls)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatal(err)
	}
}
