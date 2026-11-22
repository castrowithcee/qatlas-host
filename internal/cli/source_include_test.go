package cli

import (
	"encoding/json"
	"io"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// choosingHandlers is a service that plans acme.json into the first commit only when the arguments choose it, and
// records the arguments of source.add.
func choosingHandlers(added *control.SourceAddArgs, mu *sync.Mutex) fakeHandlers {
	handlers := addHandlers(listedFilesInspect())
	handlers[control.SourceInspectCommand] = func(raw json.RawMessage) any {
		var a control.SourceInspectArgs
		_ = json.Unmarshal(raw, &a)
		res := listedFilesInspect()
		if slices.Contains(a.Include, "acme.json") {
			res.PlannedFiles = []string{".env", "acme.json", "compose.yaml", "conf/app.yaml"}
			res.Excluded = slices.DeleteFunc(res.Excluded, func(e control.SourceExcluded) bool { return e.Path == "acme.json" })
			res.Included = []control.SourceExcluded{{Path: "acme.json", Reason: control.SourceExcludedWritableFile, Stack: "demo"}}
		}
		return inspectWith(res)(raw)
	}
	handlers[control.SourceAddCommand] = func(raw json.RawMessage) any {
		mu.Lock()
		defer mu.Unlock()
		_ = json.Unmarshal(raw, added)
		return control.SourceAddResult{ID: "src-1", Path: added.Path, Kind: control.SourceKindLocal, Head: "abc1234", Files: len(added.Files)}
	}
	return handlers
}

func TestSourceAddIncludeSendsTheChoiceWithTheConfirmedList(t *testing.T) {
	useFolderHost(t)
	tree := newFilesTree(t)
	var added control.SourceAddArgs
	var mu sync.Mutex
	svc := startFakeService(t, choosingHandlers(&added, &mu))
	useStdin(t, strings.NewReader(""))

	code, _, stderr := run(t, "source", "add", tree.dir, "--include", "acme.json", "--yes", "--output", "json")
	if code != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if svc.count(control.SourceAddCommand) != 1 || !slices.Equal(added.Include, []string{"acme.json"}) ||
		!slices.Contains(added.Files, "acme.json") || len(added.Files) != 4 {
		t.Fatalf("source.add args %+v", added)
	}
}

func TestSourceAddWithoutTerminalAndWithoutIncludeLeavesTheUnclearFileOut(t *testing.T) {
	useFolderHost(t)
	tree := newFilesTree(t)
	var added control.SourceAddArgs
	var mu sync.Mutex
	startFakeService(t, choosingHandlers(&added, &mu))
	useStdin(t, strings.NewReader("y\n"))

	code, _, stderr := run(t, "source", "add", tree.dir, "--yes", "--output", "json")
	if code != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if len(added.Include) != 0 || slices.Contains(added.Files, "acme.json") {
		t.Fatalf("source.add args %+v", added)
	}
	if strings.Contains(stderr, "Take it into the first commit") {
		t.Fatalf("a question was asked without a terminal:\n%s", stderr)
	}
}

func TestSourceAddPreviewShowsUnclearFilesApartAndMarksChosenOnes(t *testing.T) {
	useFolderHost(t)
	tree := newFilesTree(t)
	var added control.SourceAddArgs
	var mu sync.Mutex
	startFakeService(t, choosingHandlers(&added, &mu))
	useStdin(t, strings.NewReader(""))

	_, _, stderr := run(t, "source", "add", tree.dir)
	if !strings.Contains(stderr, "Unclear (1), may hold data or a secret, left out unless chosen with --include <path>:\n    acme.json: writable-file (stack demo)") {
		t.Fatalf("unclear files are not shown apart:\n%s", stderr)
	}
	useStdin(t, strings.NewReader(""))
	_, _, stderr = run(t, "source", "add", tree.dir, "--include", "acme.json")
	if !strings.Contains(stderr, "acme.json (chosen, writable-file)") || strings.Contains(stderr, "Unclear (") {
		t.Fatalf("a chosen file is not marked:\n%s", stderr)
	}
}

func TestSourceAddIncludeOfAFileThatIsNotUnclearIsRefusedByTheService(t *testing.T) {
	useFolderHost(t)
	tree := newFilesTree(t)
	handlers := addHandlers(listedFilesInspect())
	handlers[control.SourceInspectCommand] = func(json.RawMessage) any {
		return &control.Error{Code: control.CodeInvalid, Message: "notes.txt is not an unclear file of the folder"}
	}
	svc := startFakeService(t, handlers)
	useStdin(t, strings.NewReader(""))
	code, stdout, stderr := run(t, "source", "add", tree.dir, "--include", "notes.txt", "--yes")
	if code == 0 || stdout != "" || !strings.Contains(stderr, "not an unclear file") || svc.count(control.SourceAddCommand) != 0 || tree.hasGit() {
		t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestSourceAddIncludeNeedsAFolderOfFiles(t *testing.T) {
	useFolderHost(t)
	tree := newPermTree(t)
	svc := startFakeService(t, addHandlers(localInspect(true)))
	useStdin(t, strings.NewReader(""))
	code, _, stderr := run(t, "source", "add", tree.dir, "--include", "acme.json", "--yes")
	if code == 0 || !strings.Contains(stderr, "--include applies to a folder of files") || svc.count(control.SourceAddCommand) != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
}

func TestAskUnclearAsksPerFileAndTakesOnlyYes(t *testing.T) {
	res := control.SourceInspectResult{Excluded: []control.SourceExcluded{
		{Path: "a.json", Reason: control.SourceExcludedWritableFile, Stack: "demo"},
		{Path: "notes.txt", Reason: control.SourceExcludedNotReferenced},
		{Path: "secret.txt", Reason: control.SourceExcludedSecretsFile, Stack: "demo"},
		{Path: "b.json", Reason: control.SourceExcludedWritableFile, Stack: "demo"},
		{Path: "c.json", Reason: control.SourceExcludedWritableFile, Stack: "demo"},
	}}
	useStdin(t, strings.NewReader("y\n\nno\nYES\n"))
	cmd := &cobra.Command{}
	cmd.SetErr(io.Discard)
	got, err := askUnclear(cmd, res)
	if err != nil || !slices.Equal(got, []string{"a.json", "c.json"}) {
		t.Fatalf("chosen %v, %v", got, err)
	}
	// An input that ends early answers no for the rest.
	useStdin(t, strings.NewReader("y\n"))
	got, _ = askUnclear(cmd, res)
	if !slices.Equal(got, []string{"a.json"}) {
		t.Fatalf("chosen %v", got)
	}
}

func TestEffectCoversTheChoiceOfUnclearFiles(t *testing.T) {
	useFolderHost(t)
	plain := listedFilesInspect()
	plain.Path = t.TempDir()
	chosen := plain
	chosen.PlannedFiles = []string{".env", "acme.json", "compose.yaml", "conf/app.yaml"}
	chosen.Excluded = chosen.Excluded[1:]
	chosen.Included = []control.SourceExcluded{{Path: "acme.json", Reason: control.SourceExcludedWritableFile, Stack: "demo"}}
	other := chosen
	other.Included = nil // the same lists, but the choice is not named

	e0, err := newSourceEffect(plain, nil)
	if err != nil {
		t.Fatal(err)
	}
	e1, err := newSourceEffect(chosen, nil)
	if err != nil {
		t.Fatal(err)
	}
	e2, err := newSourceEffect(other, nil)
	if err != nil {
		t.Fatal(err)
	}
	if e0.fingerprint == e1.fingerprint || e1.fingerprint == e2.fingerprint {
		t.Fatalf("the effect does not cover the choice: %s %s %s", e0.fingerprint, e1.fingerprint, e2.fingerprint)
	}
	// An effect without a choice keeps the form it had.
	again, _ := newSourceEffect(plain, nil)
	if again.fingerprint != e0.fingerprint {
		t.Fatal("the effect is not stable")
	}

	a := &sourceAdd{res: plain, question: "q"}
	b := &sourceAdd{res: chosen, question: "q"}
	fa, _ := effectOf(a, "text")
	fb, _ := effectOf(b, "text")
	if fa == fb {
		t.Fatal("the signature of the approval does not cover the choice")
	}
}

func TestSourceAddExpectEffectMustMatchTheChoice(t *testing.T) {
	useFolderHost(t)
	tree := newFilesTree(t)
	var added control.SourceAddArgs
	var mu sync.Mutex
	svc := startFakeService(t, choosingHandlers(&added, &mu))
	_, plain := inspectJSON(t, tree.dir)
	_, chosen := inspectJSON(t, tree.dir, "--include", "acme.json")
	if plain["effect"] == chosen["effect"] || plain["effect"] == nil {
		t.Fatalf("effects: %v and %v", plain["effect"], chosen["effect"])
	}

	// An effect shown for another choice never passes, in either direction.
	for name, args := range map[string][]string{
		"choice added":   {"--include", "acme.json", "--expect-effect", plain["effect"].(string)},
		"choice dropped": {"--expect-effect", chosen["effect"].(string)},
	} {
		useStdin(t, strings.NewReader(""))
		code, _, stderr := run(t, append([]string{"source", "add", tree.dir, "--yes"}, args...)...)
		if code == 0 || !strings.Contains(stderr, "state-changed") {
			t.Errorf("%s: code %d, stderr %q", name, code, stderr)
		}
	}
	if svc.count(control.SourceAddCommand) != 0 {
		t.Fatal("a wrong effect reached the service")
	}
	useStdin(t, strings.NewReader(""))
	code, _, stderr := run(t, "source", "add", tree.dir, "--yes", "--include", "acme.json", "--expect-effect", chosen["effect"].(string))
	if code != 0 || svc.count(control.SourceAddCommand) != 1 || !slices.Equal(added.Include, []string{"acme.json"}) {
		t.Fatalf("code %d, stderr %q, args %+v", code, stderr, added)
	}
}

// chooseEnv is a source-add proposal for a folder of files whose service plans acme.json only when it is chosen.
func chooseEnv(t *testing.T, dir string, added *control.SourceAddArgs, mu *sync.Mutex) *approveEnv {
	t.Helper()
	extra := choosingHandlers(added, mu)
	env := &approveEnv{dir: dir, extra: extra}
	env.inspect = func(int) control.SourceInspectResult { return listedFilesInspect() }
	startApproveEnv(t, env)
	return env
}

func TestProposalApproveAsksForEachUnclearFileAndSignsTheChoice(t *testing.T) {
	for name, tc := range map[string]struct {
		stdin   string
		include []string
	}{
		"chosen":   {"y\ny\n", []string{"acme.json"}},
		"declined": {"n\ny\n", nil},
	} {
		t.Run(name, func(t *testing.T) {
			useFolderHost(t)
			tree := newFilesTree(t)
			var added control.SourceAddArgs
			var mu sync.Mutex
			env := chooseEnv(t, tree.dir, &added, &mu)
			useTerminalStdin(t, tc.stdin)
			code, stdout, stderr := run(t, "proposal", "approve", approveID, "--output", "json")
			if code != 0 {
				t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
			}
			if !strings.Contains(stderr, "Take it into the first commit? [y/N]") || !strings.Contains(env.history(), "approve,result") || added.Path == "" {
				t.Fatalf("order %q, stderr:\n%s", env.history(), stderr)
			}
			if !slices.Equal(added.Include, tc.include) || slices.Contains(added.Files, "acme.json") != (tc.include != nil) {
				t.Fatalf("source.add args %+v", added)
			}
			// The effect shown is worked out again from the same choice right before it is applied; a changed
			// choice would stop the approval, so it ran to the end only because both effects matched.
			if tc.include != nil && !strings.Contains(stderr, "(chosen, writable-file)") {
				t.Errorf("the effect does not show the choice:\n%s", stderr)
			}
		})
	}
}

func TestProposalApproveIncludeChoosesInAdvance(t *testing.T) {
	useFolderHost(t)
	tree := newFilesTree(t)
	var added control.SourceAddArgs
	var mu sync.Mutex
	chooseEnv(t, tree.dir, &added, &mu)
	useTerminalStdin(t, "y\n")
	code, _, stderr := run(t, "proposal", "approve", approveID, "--include", "acme.json", "--output", "json")
	if code != 0 || !slices.Equal(added.Include, []string{"acme.json"}) || strings.Contains(stderr, "Take it into the first commit") {
		t.Fatalf("code %d, args %+v, stderr %q", code, added, stderr)
	}
}

func TestReviewedSourceAddIsBoundToTheChoiceOfUnclearFiles(t *testing.T) {
	useFolderHost(t)
	tree := newFilesTree(t)
	var added control.SourceAddArgs
	var mu sync.Mutex
	env := chooseEnv(t, tree.dir, &added, &mu)
	effects := regexp.MustCompile(`Effect fingerprint of the folder: (\S+)`)
	show := func(extra ...string) string {
		useReviewedRun(t, "")
		_, _, stderr := run(t, append([]string{"proposal", "approve", approveID, "--fingerprint", "fp-one"}, extra...)...)
		m := effects.FindStringSubmatch(stderr)
		if m == nil {
			t.Fatalf("no effect in:\n%s", stderr)
		}
		return m[1]
	}
	plain, chosen := show(), show("--include", "acme.json")
	if plain == chosen {
		t.Fatal("the effect does not cover the choice")
	}
	// The effect is the one 'source inspect' shows for the same choice.
	if _, got := inspectJSON(t, tree.dir, "--include", "acme.json"); got["effect"] != chosen {
		t.Fatalf("preview effect %q, source inspect %v", chosen, got["effect"])
	}
	for name, args := range map[string][]string{
		"choice added":   {"--include", "acme.json", "--expect-effect", plain},
		"choice dropped": {"--expect-effect", chosen},
	} {
		useReviewedRun(t, "")
		code, _, stderr := run(t, append([]string{"proposal", "approve", approveID, "--fingerprint", "fp-one", "--confirm"}, args...)...)
		if code == 0 || !strings.Contains(stderr, "state-changed") || strings.Contains(env.history(), "approve") {
			t.Errorf("%s: code %d, order %q, stderr %q", name, code, env.history(), stderr)
		}
	}
	useReviewedRun(t, "")
	code, _, stderr := run(t, "proposal", "approve", approveID, "--fingerprint", "fp-one", "--confirm",
		"--include", "acme.json", "--expect-effect", chosen)
	if code != 0 || !strings.Contains(env.history(), "approve,result") || !slices.Equal(added.Include, []string{"acme.json"}) {
		t.Fatalf("code %d, order %q, args %+v, stderr %q", code, env.history(), added, stderr)
	}
}
