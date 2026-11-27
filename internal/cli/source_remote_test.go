package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

const (
	remoteURL      = "ssh://git.example.test/demo/stacks.git"
	remoteCommit   = "0123456789abcdef0123456789abcdef01234567"
	remoteKeyPrint = "SHA256:demo-fingerprint"
)

// remoteFake answers the commands of the comparison and records what the client passes.
type remoteFake struct {
	mu       sync.Mutex
	compare  control.SourceCompareResult
	remoted  []control.SourceRemoteArgs
	added    []control.SourceAddArgs
	compared []control.SourceCompareArgs
	refusal  *control.Error
}

func (r *remoteFake) handlers(inspect control.SourceInspectResult) fakeHandlers {
	return fakeHandlers{
		control.SourceInspectCommand: inspectWith(inspect),
		control.RepoProbeCommand: func(json.RawMessage) any {
			return control.RepoProbeResult{Host: "git.example.test", Port: "22", KeyType: "ssh-ed25519", Fingerprint: remoteKeyPrint}
		},
		control.SourceCompareCommand: func(raw json.RawMessage) any {
			var a control.SourceCompareArgs
			_ = json.Unmarshal(raw, &a)
			r.mu.Lock()
			defer r.mu.Unlock()
			r.compared = append(r.compared, a)
			return r.compare
		},
		control.SourceRemoteCommand: func(raw json.RawMessage) any {
			var a control.SourceRemoteArgs
			_ = json.Unmarshal(raw, &a)
			r.mu.Lock()
			defer r.mu.Unlock()
			r.remoted = append(r.remoted, a)
			if r.refusal != nil {
				return r.refusal
			}
			return control.SourceRemoteResult{ID: "src-2", Path: a.Path, URL: a.URL, State: a.ExpectState, Branch: "main",
				Head: "feedface0000", Pushed: a.ExpectState != control.SourceRemoteEqual, HistoryRef: "refs/qhost/local-before-remote"}
		},
		control.SourceAddCommand: func(raw json.RawMessage) any {
			var a control.SourceAddArgs
			_ = json.Unmarshal(raw, &a)
			r.mu.Lock()
			defer r.mu.Unlock()
			r.added = append(r.added, a)
			if r.refusal != nil {
				return r.refusal
			}
			return control.SourceAddResult{ID: "src-3", Path: a.Path, Kind: control.SourceKindRemote, Head: "feedface0000",
				Files: len(a.Files), Pushed: a.ExpectState == control.SourceRemoteDifferent}
		},
	}
}

func (r *remoteFake) untouched() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.remoted) == 0 && len(r.added) == 0
}

// differentCompare is a remote that differs; for a folder of files it carries the plan of listedFilesInspect, as
// the service compares the plan.
func differentCompare(kase string) control.SourceCompareResult {
	res := control.SourceCompareResult{Case: kase, State: control.SourceRemoteDifferent, Branch: "main", Commit: remoteCommit,
		Paths: []string{".env", "web/compose.yaml"}, PathCount: 2}
	if kase == control.SourceCaseFiles {
		plan := listedFilesInspect()
		res.PlannedFiles, res.Excluded = plan.PlannedFiles, plan.Excluded
	}
	return res
}

func secretCompare(kase string) control.SourceCompareResult {
	res := differentCompare(kase)
	res.Secrets = &control.SourceSecrets{Total: 3, Findings: []control.SourceSecretFinding{
		{Commit: remoteCommit, Path: "secrets.env", Kind: control.SourceSecretFile},
		{Commit: remoteCommit, Path: "web/key.pem", Kind: control.SourceSecretPrivateKey}}}
	return res
}

func TestSourceRemoteWithoutConfirmationChangesNothing(t *testing.T) {
	useFolderHost(t)
	dir := t.TempDir()
	fake := &remoteFake{compare: differentCompare(control.SourceCaseLocal)}
	svc := startFakeService(t, fake.handlers(localInspect(false)))
	useStdin(t, strings.NewReader(""))

	code, stdout, stderr := run(t, "source", "remote", dir, remoteURL, "--fingerprint", remoteKeyPrint)
	if code != 2 || stdout != "" || !strings.Contains(stderr, "confirmation-required") {
		t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	for _, want := range []string{
		"State different (remote branch main at 0123456789ab)", "2 paths differ", "  .env", "  web/compose.yaml",
	} {
		if !strings.Contains(stderr, want) {
			t.Errorf("preview lacks %q:\n%s", want, stderr)
		}
	}
	if !fake.untouched() || svc.count(control.SourceCompareCommand) != 1 {
		t.Fatal("an unconfirmed source remote changed something")
	}
}

func TestSourceRemoteYesPassesTheShownStateAndCommit(t *testing.T) {
	useFolderHost(t)
	dir := t.TempDir()
	fake := &remoteFake{compare: differentCompare(control.SourceCaseLocal)}
	startFakeService(t, fake.handlers(localInspect(false)))
	useStdin(t, strings.NewReader(""))

	code, stdout, stderr := run(t, "source", "remote", dir, remoteURL, "--fingerprint", remoteKeyPrint, "--yes", "--output", "json")
	if code != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	var res map[string]any
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatalf("invalid JSON %q: %v", stdout, err)
	}
	if res["id"] != "src-2" || res["url"] != remoteURL || res["state"] != "different" || res["branch"] != "main" ||
		res["head"] != "feedface0000" || res["pushed"] != true || res["history_ref"] != "refs/qhost/local-before-remote" {
		t.Fatalf("result = %v", res)
	}
	want := control.SourceRemoteArgs{Path: dir, URL: remoteURL, Fingerprint: remoteKeyPrint, ExpectState: "different", ExpectCommit: remoteCommit}
	if len(fake.remoted) != 1 || fake.remoted[0] != want {
		t.Fatalf("source.remote args %+v, want %+v", fake.remoted, want)
	}
}

func TestSourceRemoteAsksADifferentQuestionPerState(t *testing.T) {
	for state, want := range map[string]string{
		control.SourceRemoteEmpty:     "Push the local history to " + remoteURL + " (branch main)?",
		control.SourceRemoteEqual:     "the local history stays under refs/qhost/local-before-remote?",
		control.SourceRemoteDifferent: "Put the state of the host as a commit on main and push it?",
	} {
		got, err := remoteQuestion(control.SourceCompareResult{State: state, Branch: "main"}, remoteURL)
		if err != nil || !strings.Contains(got, want) {
			t.Errorf("state %s: question %q, error %v, want %q", state, got, err, want)
		}
	}
	if _, err := remoteQuestion(control.SourceCompareResult{State: "other"}, remoteURL); err == nil {
		t.Error("an unknown state was accepted")
	}
}

func TestSourceRemoteYesDoesNotConfirmTheHostKey(t *testing.T) {
	useFolderHost(t)
	fake := &remoteFake{compare: differentCompare(control.SourceCaseLocal)}
	svc := startFakeService(t, fake.handlers(localInspect(false)))
	useStdin(t, strings.NewReader(""))

	code, _, stderr := run(t, "source", "remote", t.TempDir(), remoteURL, "--yes")
	if code != 2 || !strings.Contains(stderr, remoteKeyPrint) || !strings.Contains(stderr, "--fingerprint") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if svc.count(control.SourceCompareCommand) != 0 || !fake.untouched() {
		t.Fatal("an unconfirmed host key was used")
	}
}

func TestSourceRemoteSecretsStopBeforeAnyChangeAndShowNoValue(t *testing.T) {
	useFolderHost(t)
	fake := &remoteFake{compare: secretCompare(control.SourceCaseLocal)}
	startFakeService(t, fake.handlers(localInspect(false)))
	useStdin(t, strings.NewReader(""))

	code, stdout, stderr := run(t, "source", "remote", t.TempDir(), remoteURL, "--fingerprint", remoteKeyPrint, "--yes")
	if code == 0 || stdout != "" || !strings.Contains(stderr, "secrets-found") {
		t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	for _, want := range []string{"3 secret-like files", "0123456789ab secrets.env (secrets-file)",
		"0123456789ab web/key.pem (private-key)", "and 1 more", "no override", "yourself"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("output lacks %q:\n%s", want, stderr)
		}
	}
	if !fake.untouched() {
		t.Fatal("a secrets finding did not stop before the change")
	}
}

func TestSourceRemoteLimitsTheListOfPaths(t *testing.T) {
	useFolderHost(t)
	cmp := differentCompare(control.SourceCaseLocal)
	cmp.Paths, cmp.PathCount = nil, 120
	for i := 0; i < 60; i++ {
		cmp.Paths = append(cmp.Paths, fmt.Sprintf("stack/file%02d.yaml", i))
	}
	fake := &remoteFake{compare: cmp}
	startFakeService(t, fake.handlers(localInspect(false)))
	useStdin(t, strings.NewReader(""))
	_, _, stderr := run(t, "source", "remote", t.TempDir(), remoteURL, "--fingerprint", remoteKeyPrint)
	if !strings.Contains(stderr, "120 paths differ") || !strings.Contains(stderr, "... and 70 more") ||
		strings.Count(stderr, "stack/file") != 50 {
		t.Fatalf("the list is not limited to 50:\n%s", stderr)
	}
}

func TestSourceRemoteRefusesAFolderOfFilesAndNeedsRoot(t *testing.T) {
	useFolderHost(t)
	fake := &remoteFake{compare: differentCompare(control.SourceCaseFiles)}
	startFakeService(t, fake.handlers(localInspect(false)))
	useStdin(t, strings.NewReader(""))
	code, _, stderr := run(t, "source", "remote", t.TempDir(), remoteURL, "--fingerprint", remoteKeyPrint, "--yes")
	if code != 2 || !strings.Contains(stderr, "source add") || !fake.untouched() {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	for _, args := range [][]string{{"source", "remote"}, {"source", "remote", "/srv/x"}, {"source", "remote", "rel", remoteURL},
		{"source", "remote", "/srv/x", "https://example.test/x.git"}} {
		if code, _, _ := run(t, args...); code != 2 {
			t.Errorf("%v: code %d, want 2", args, code)
		}
	}
	folders.euid = func() int { return 1000 }
	if code, _, _ := run(t, "source", "remote", t.TempDir(), remoteURL, "--yes"); code == 0 {
		t.Error("a non-root run succeeded")
	}
}

func TestSourceRemoteServiceRefusalOfSecretsIsNamed(t *testing.T) {
	useFolderHost(t)
	fake := &remoteFake{compare: differentCompare(control.SourceCaseLocal),
		refusal: control.NewError(control.CodeSecrets, "the commits to publish hold 1 secret-like file(s) and are not pushed: 0123456789ab \"secrets.env\" (secrets-file)")}
	startFakeService(t, fake.handlers(localInspect(false)))
	useStdin(t, strings.NewReader(""))
	code, _, stderr := run(t, "source", "remote", t.TempDir(), remoteURL, "--fingerprint", remoteKeyPrint, "--yes")
	if code == 0 || !strings.Contains(stderr, "secrets-found") || !strings.Contains(stderr, "secrets.env") || !strings.Contains(stderr, "no override") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
}

func TestSourceRemoteReportsAFailedRemote(t *testing.T) {
	err := failureOf(control.NewError(control.CodeRemote, "the remote refused the push"))
	if !strings.Contains(err.Error(), "refused the push") {
		t.Fatalf("failure %v", err)
	}
}

func TestSourceAddFilesWithURLEmptyRemoteStopsWithAHint(t *testing.T) {
	d := useFolderHost(t)
	tree := newFilesTree(t)
	fake := &remoteFake{compare: control.SourceCompareResult{Case: control.SourceCaseFiles, State: control.SourceRemoteEmpty, Branch: "main"}}
	startFakeService(t, fake.handlers(listedFilesInspect()))
	useStdin(t, strings.NewReader(""))
	before := tree.snapshot(t)

	code, _, stderr := run(t, "source", "add", tree.dir, "--url", remoteURL, "--fingerprint", remoteKeyPrint, "--yes")
	if code == 0 || !strings.Contains(stderr, "has no commit") || !strings.Contains(stderr, "without --url") ||
		!strings.Contains(stderr, "qhost source remote "+tree.dir+" "+remoteURL) {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if !fake.untouched() || !equalModes(before, tree.snapshot(t)) || len(d.lchowns) != 0 || d.dropIn(t) != "" {
		t.Fatal("an empty remote changed something")
	}
}

func TestSourceAddFilesWithURLWithoutConfirmationChangesNothing(t *testing.T) {
	d := useFolderHost(t)
	d.useRootless(t)
	tree := newFilesTree(t)
	fake := &remoteFake{compare: differentCompare(control.SourceCaseFiles)}
	startFakeService(t, fake.handlers(listedFilesInspect()))
	useStdin(t, strings.NewReader(""))
	before := tree.snapshot(t)

	code, stdout, stderr := run(t, "source", "add", tree.dir, "--url", remoteURL, "--fingerprint", remoteKeyPrint)
	if code != 2 || stdout != "" || !strings.Contains(stderr, "confirmation-required") {
		t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	for _, want := range []string{"State different", "  .env", "on the history of " + remoteURL, "one commit as qhost on main",
		"takes 3 files", "big.bin: too-large", "paths get the group"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("preview lacks %q:\n%s", want, stderr)
		}
	}
	if !fake.untouched() || tree.hasGit() || !equalModes(before, tree.snapshot(t)) || len(d.lchowns) != 0 || d.dropIn(t) != "" || len(d.cmds) != 0 {
		t.Fatal("an unconfirmed source add changed something")
	}
}

func TestSourceAddFilesWithURLYesPassesTheShownState(t *testing.T) {
	d := useFolderHost(t)
	d.useRootless(t)
	tree := newFilesTree(t)
	fake := &remoteFake{compare: differentCompare(control.SourceCaseFiles)}
	startFakeService(t, fake.handlers(listedFilesInspect()))
	useStdin(t, strings.NewReader(""))

	code, stdout, stderr := run(t, "source", "add", tree.dir, "--url", remoteURL, "--fingerprint", remoteKeyPrint, "--yes", "--output", "json")
	if code != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	var res map[string]any
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatalf("invalid JSON %q: %v", stdout, err)
	}
	if res["kind"] != "remote" || res["state"] != "different" || res["pushed"] != true || res["url"] != remoteURL ||
		res["branch"] != "main" || res["files"] != float64(3) {
		t.Fatalf("result = %v", res)
	}
	if len(fake.added) != 1 {
		t.Fatalf("source.add calls: %+v", fake.added)
	}
	got := fake.added[0]
	if got.Path != tree.dir || got.URL != remoteURL || got.Fingerprint != remoteKeyPrint || got.ExpectState != "different" ||
		got.ExpectCommit != remoteCommit || strings.Join(got.Files, ",") != ".env,compose.yaml,conf/app.yaml" {
		t.Fatalf("source.add args %+v", got)
	}
	if !strings.Contains(d.dropIn(t), tree.dir) || len(d.lchowns) == 0 {
		t.Fatalf("drop-in %q, chowns %d", d.dropIn(t), len(d.lchowns))
	}
}

func TestSourceAddFilesWithURLTakesThePlanOfTheComparisonAndMarksWhatTheRemoteVersions(t *testing.T) {
	useFolderHost(t)
	tree := newFilesTree(t)
	cmp := differentCompare(control.SourceCaseFiles)
	cmp.State, cmp.Paths, cmp.PathCount = control.SourceRemoteEqual, nil, 0
	cmp.PlannedFiles = []string{".env", "README.md", "compose.yaml", "conf/app.yaml"}
	cmp.RemoteFiles = []string{"README.md"}
	cmp.Excluded = listedFilesInspect().Excluded[:3]
	fake := &remoteFake{compare: cmp}
	startFakeService(t, fake.handlers(listedFilesInspect()))
	useStdin(t, strings.NewReader(""))

	code, _, stderr := run(t, "source", "add", tree.dir, "--url", remoteURL, "--fingerprint", remoteKeyPrint)
	if code != 2 || !strings.Contains(stderr, "confirmation-required") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	for _, want := range []string{"takes 4 files, 1 of them only because the remote versions them", "  README.md (versioned in the remote)",
		"  compose.yaml\n", "Unclear (1)", "Left out (2)"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("preview lacks %q:\n%s", want, stderr)
		}
	}
	if strings.Contains(stderr, "notes.txt") || !fake.untouched() {
		t.Fatalf("the preview is not the plan of the comparison, or something changed:\n%s", stderr)
	}

	code, _, stderr = run(t, "source", "add", tree.dir, "--url", remoteURL, "--fingerprint", remoteKeyPrint, "--yes")
	if code != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if len(fake.added) != 1 || strings.Join(fake.added[0].Files, ",") != ".env,README.md,compose.yaml,conf/app.yaml" {
		t.Fatalf("source.add args %+v", fake.added)
	}
}

func TestSourceAddFilesWithURLChecksTheEffectOfTheInspection(t *testing.T) {
	useFolderHost(t)
	tree := newFilesTree(t)
	cmp := differentCompare(control.SourceCaseFiles)
	cmp.State, cmp.Paths, cmp.PathCount = control.SourceRemoteEqual, nil, 0
	cmp.PlannedFiles = []string{".env", "README.md", "compose.yaml", "conf/app.yaml"}
	cmp.RemoteFiles = []string{"README.md"}
	cmp.Excluded = listedFilesInspect().Excluded[:3]
	fake := &remoteFake{compare: cmp}
	startFakeService(t, fake.handlers(listedFilesInspect()))
	useStdin(t, strings.NewReader(""))

	// The effect source inspect shows still applies when the comparison plans files the remote versions.
	_, got := inspectJSON(t, tree.dir)
	effect, _ := got["effect"].(string)
	if effect == "" {
		t.Fatalf("no effect: %v", got)
	}
	code, _, stderr := run(t, "source", "add", tree.dir, "--url", remoteURL, "--fingerprint", remoteKeyPrint,
		"--expect-effect", effect, "--expect-state", cmp.State, "--expect-commit", cmp.Commit, "--yes")
	if code != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if len(fake.added) != 1 || strings.Join(fake.added[0].Files, ",") != ".env,README.md,compose.yaml,conf/app.yaml" {
		t.Fatalf("source.add args %+v", fake.added)
	}
}

func TestSourceAddFilesWithURLStopsWhenThePlanOfTheComparisonChanged(t *testing.T) {
	d := useFolderHost(t)
	tree := newFilesTree(t)
	var asked atomic.Int32
	fake := &remoteFake{}
	handlers := fake.handlers(listedFilesInspect())
	handlers[control.SourceCompareCommand] = func(json.RawMessage) any {
		res := differentCompare(control.SourceCaseFiles)
		if asked.Add(1) > 1 {
			res.PlannedFiles = append(res.PlannedFiles, "README.md")
			res.RemoteFiles = []string{"README.md"}
		}
		return res
	}
	startFakeService(t, handlers)
	useStdin(t, strings.NewReader(""))
	before := tree.snapshot(t)

	code, _, stderr := run(t, "source", "add", tree.dir, "--url", remoteURL, "--fingerprint", remoteKeyPrint, "--yes")
	if code == 0 || !strings.Contains(stderr, "new in files: README.md") || !strings.Contains(stderr, "nothing was changed") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if !fake.untouched() || !equalModes(before, tree.snapshot(t)) || len(d.lchowns) != 0 || d.dropIn(t) != "" {
		t.Fatal("a changed plan did not stop before the change")
	}
}

func TestSourceAddFilesWithURLAsksPerState(t *testing.T) {
	for state, want := range map[string]string{
		control.SourceRemoteEqual:     "Take over the folder with the history of the remote?",
		control.SourceRemoteDifferent: "Start the repository on the history of the remote, put the state of the host as a commit on main and push it?",
	} {
		remote := &filesRemote{address: remoteURL, cmp: control.SourceCompareResult{State: state, Branch: "main"}}
		if got := filesRemoteQuestion(remote); got != want {
			t.Errorf("state %s: question %q, want %q", state, got, want)
		}
	}
}

func TestSourceAddFilesWithURLSecretsStopBeforeAnyChange(t *testing.T) {
	d := useFolderHost(t)
	tree := newFilesTree(t)
	fake := &remoteFake{compare: secretCompare(control.SourceCaseFiles)}
	startFakeService(t, fake.handlers(listedFilesInspect()))
	useStdin(t, strings.NewReader(""))
	before := tree.snapshot(t)

	code, stdout, stderr := run(t, "source", "add", tree.dir, "--url", remoteURL, "--fingerprint", remoteKeyPrint, "--yes")
	if code == 0 || stdout != "" || !strings.Contains(stderr, "secrets-found") || !strings.Contains(stderr, "secrets.env (secrets-file)") {
		t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if !fake.untouched() || !equalModes(before, tree.snapshot(t)) || len(d.lchowns) != 0 || d.dropIn(t) != "" {
		t.Fatal("a secrets finding did not stop before the change")
	}
}

func TestSourceAddFilesWithURLYesDoesNotConfirmTheHostKey(t *testing.T) {
	useFolderHost(t)
	tree := newFilesTree(t)
	fake := &remoteFake{compare: differentCompare(control.SourceCaseFiles)}
	svc := startFakeService(t, fake.handlers(listedFilesInspect()))
	useStdin(t, strings.NewReader(""))
	code, _, stderr := run(t, "source", "add", tree.dir, "--url", remoteURL, "--yes")
	if code != 2 || !strings.Contains(stderr, remoteKeyPrint) || svc.count(control.SourceCompareCommand) != 0 || !fake.untouched() {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
}
