package service_test

import (
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// unclearFiles is a folder of files whose stack mounts one file writable and names another as a secret file.
func unclearFiles() map[string]string {
	return map[string]string{
		"web/compose.yaml": `services:
  app:
    image: registry.example.test/demo/web:${WEB_VERSION}
    volumes:
      - ./acme.json:/acme.json
secrets:
  token:
    file: ./token.txt
`,
		"web/.env":             "WEB_VERSION=1.2.3\n",
		"web/acme.json":        "{}\n",
		"web/token.txt":        "example-token-value\n",
		"web/notes.txt":        "notes\n",
		"web/compose.yaml.bak": "old\n",
	}
}

func (h *harness) inspectWith(path string, include []string) (control.SourceInspectResult, error) {
	var res control.SourceInspectResult
	err := h.call(control.SourceInspectCommand, control.SourceInspectArgs{Path: path, Include: include}, &res)
	return res, err
}

func TestInspectPlansAChosenUnclearFileAndNamesIt(t *testing.T) {
	h := realGitService(t)
	dir := newFolder(t)
	writeTree(t, dir, unclearFiles())

	plain := h.inspect(dir, "")
	got := excludedReasons(plain)
	if got["web/acme.json"] != control.SourceExcludedWritableFile || got["web/token.txt"] != control.SourceExcludedSecretsFile ||
		slices.Contains(plain.PlannedFiles, "web/acme.json") || len(plain.Included) != 0 {
		t.Fatalf("without a choice: planned %q, excluded %v, included %v", plain.PlannedFiles, got, plain.Included)
	}

	res, err := h.inspectWith(dir, []string{"web/acme.json", "web/token.txt"})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"web/acme.json", "web/token.txt"} {
		if !slices.Contains(res.PlannedFiles, f) {
			t.Errorf("%s is not planned: %q", f, res.PlannedFiles)
		}
	}
	if _, left := excludedReasons(res)["web/acme.json"]; left {
		t.Errorf("a chosen file is still left out: %v", res.Excluded)
	}
	want := []control.SourceExcluded{
		{Path: "web/acme.json", Reason: control.SourceExcludedWritableFile, Stack: "web"},
		{Path: "web/token.txt", Reason: control.SourceExcludedSecretsFile, Stack: "web"},
	}
	if !slices.Equal(res.Included, want) {
		t.Fatalf("included = %+v, want %+v", res.Included, want)
	}
	if len(res.PlannedFiles) != len(plain.PlannedFiles)+2 {
		t.Errorf("planned %q against %q", res.PlannedFiles, plain.PlannedFiles)
	}
}

func TestInspectRefusesAChoiceThatIsNotUnclear(t *testing.T) {
	h := realGitService(t)
	dir := newFolder(t)
	writeTree(t, dir, unclearFiles())
	for _, bad := range []string{"web/notes.txt", "web/compose.yaml", "web/.env", "web/missing.json", "web/data", "../x"} {
		_, err := h.inspectWith(dir, []string{"web/acme.json", bad})
		wantCode(t, err, control.CodeInvalid)
		if !strings.Contains(err.Error(), "not an unclear file") {
			t.Errorf("%s: err = %v", bad, err)
		}
	}
}

func TestSourceAddTakesChosenUnclearFilesAndOnlyThem(t *testing.T) {
	notRoot(t)
	h := realGitService(t)
	dir := newFolder(t)
	writeTree(t, dir, unclearFiles())
	include := []string{"web/acme.json"}
	shown, err := h.inspectWith(dir, include)
	if err != nil {
		t.Fatal(err)
	}

	// The confirmed list and the choice belong together: a choice without its file, and a file without its
	// choice, are both a different folder than the one that was shown.
	for _, c := range []struct {
		name    string
		files   []string
		include []string
	}{
		{"choice missing", shown.PlannedFiles, nil},
		{"file missing", h.inspect(dir, "").PlannedFiles, include},
		{"not unclear", shown.PlannedFiles, []string{"web/acme.json", "web/notes.txt"}},
	} {
		var res control.SourceAddResult
		err := h.call(control.SourceAddCommand, control.SourceAddArgs{Path: dir, Files: c.files, Include: c.include}, &res)
		wantCode(t, err, control.CodeInvalid)
		noGit(t, h, dir)
		_ = c.name
	}

	var res control.SourceAddResult
	if err := h.call(control.SourceAddCommand, control.SourceAddArgs{Path: dir, Files: shown.PlannedFiles, Include: include}, &res); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("git", "-C", dir, "ls-tree", "-r", "--name-only", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	committed := strings.Fields(string(out))
	if !slices.Contains(committed, "web/acme.json") || slices.Contains(committed, "web/token.txt") || len(committed) != len(shown.PlannedFiles) || res.Files != len(committed) {
		t.Fatalf("committed %q, planned %q", committed, shown.PlannedFiles)
	}
	exclude, err := exec.Command("cat", dir+"/.git/info/exclude").Output()
	if err != nil || strings.Contains(string(exclude), "acme.json") || !strings.Contains(string(exclude), "/web/token.txt") {
		t.Fatalf("exclude = %q, %v", exclude, err)
	}
}

func TestChoiceOfUnclearFilesNeedsAFolderOfFiles(t *testing.T) {
	h := realGitService(t)
	dir := localRepo(t, localFiles())
	_, err := h.inspectWith(dir, []string{"web/acme.json"})
	wantCode(t, err, control.CodeInvalid)
	var res control.SourceAddResult
	err = h.call(control.SourceAddCommand, control.SourceAddArgs{Path: dir, Include: []string{"web/acme.json"}}, &res)
	wantCode(t, err, control.CodeInvalid)
	if len(h.sourceList().Sources) != 0 {
		t.Fatal("a folder was bound")
	}
}
