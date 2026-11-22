package tui

import (
	"slices"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-kit/i18n"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// The unclear files of the folder of files in these tests: one a stack mounts writable, one a secrets file.
const (
	unclearData   = "demo/acme.json"
	unclearSecret = "demo/secrets/db.txt"
)

// withUnclear is b with two unclear files in the inspection of the folder of files.
func withUnclear(b *fakeBackend) *fakeBackend {
	r := b.inspects[filesDir]
	r.Excluded = append(slices.Clone(r.Excluded),
		control.SourceExcluded{Path: unclearData, Reason: control.SourceExcludedWritableFile, Stack: "demo"},
		control.SourceExcluded{Path: unclearSecret, Reason: control.SourceExcludedSecretsFile, Stack: "demo"})
	b.inspects[filesDir] = r
	return b
}

// chooseFirst opens the choice, ticks the first unclear file and saves.
func chooseFirst(m *Model) {
	send(m, "c")
	send(m, "space", "f2")
}

func TestUnclearFilesAreShownApartAndNotChosen(t *testing.T) {
	b := withUnclear(foldersBackend())
	m := newSettingsModel(t, b)
	send(m, addKeys(filesDir)...)
	contains(t, m, "Unclear files (2)", "[ ] "+unclearData+": writable-file", "[ ] "+unclearSecret+": secrets-file", "Left out (2)", "choose files")
	if strings.Contains(m.render(), "[x]") {
		t.Fatalf("an unclear file is chosen by default:\n%s", m.render())
	}
	send(m, "y")
	if len(b.sourceAdds) != 1 || len(b.sourceAdds[0].Include) != 0 || slices.Contains(b.sourceAdds[0].Files, unclearData) {
		t.Fatalf("adds = %+v", b.sourceAdds)
	}
}

func TestFolderWithoutUnclearFilesOffersNoChoice(t *testing.T) {
	m := newSettingsModel(t, foldersBackend())
	send(m, addKeys(filesDir)...)
	if strings.Contains(m.render(), "choose files") || strings.Contains(m.render(), "Unclear files") {
		t.Fatalf("a choice without unclear files:\n%s", m.render())
	}
}

func TestChoosingAnUnclearFilePlansAgainAndIsSentWithTheAdd(t *testing.T) {
	b := withUnclear(foldersBackend())
	m := newSettingsModel(t, b)
	send(m, addKeys(filesDir)...)
	chooseFirst(m)
	if got := b.inspectIncludes; len(got) != 2 || len(got[0]) != 0 || !slices.Equal(got[1], []string{unclearData}) {
		t.Fatalf("inspections chose %v", got)
	}
	contains(t, m, "[x] "+unclearData+": writable-file", "[ ] "+unclearSecret+": secrets-file", "Left out (2)", unclearData)
	noChange(t, b)
	send(m, "y")
	if len(b.sourceAdds) != 1 {
		t.Fatalf("adds = %+v", b.sourceAdds)
	}
	a := b.sourceAdds[0]
	if !slices.Equal(a.Include, []string{unclearData}) || !slices.Contains(a.Files, unclearData) || slices.Contains(a.Files, unclearSecret) {
		t.Fatalf("add = %+v", a)
	}
}

func TestChoiceCanBeTakenBackAndAFileThatChangedBreaksTheAdd(t *testing.T) {
	b := withUnclear(foldersBackend())
	m := newSettingsModel(t, b)
	send(m, addKeys(filesDir)...)
	chooseFirst(m)
	send(m, "c", "space", "f2")
	contains(t, m, "[ ] "+unclearData)
	send(m, "y")
	if len(b.sourceAdds) != 1 || len(b.sourceAdds[0].Include) != 0 {
		t.Fatalf("adds = %+v", b.sourceAdds)
	}

	b = withUnclear(foldersBackend())
	m = newSettingsModel(t, b)
	send(m, addKeys(filesDir)...)
	chooseFirst(m)
	// The list differs by the time yes is answered: nothing is applied.
	r := b.inspects[filesDir]
	r.Excluded = r.Excluded[:2]
	b.inspects[filesDir] = r
	send(m, "y")
	if len(b.sourceAdds) != 0 {
		t.Fatalf("applied after the plan changed: %+v", b.sourceAdds)
	}
}

func TestChoosingWithARemoteSendsTheChoiceToTheComparisonAndTheAdd(t *testing.T) {
	b := withUnclear(foldersBackend())
	b.dirKeys = map[string]control.KeyResult{filesDir: {Fingerprint: fingerprint}}
	cmp := cmpDiff
	cmp.Case = control.SourceCaseFiles
	cmp.PlannedFiles = []string{"demo/.env", "demo/compose.yaml", unclearData}
	cmp.Included = []control.SourceExcluded{{Path: unclearData, Reason: control.SourceExcludedWritableFile}}
	b.compare = cmp
	m := newSettingsModel(t, b)
	send(m, addKeys(filesDir)...)
	chooseFirst(m)
	send(m, "w")
	send(m, typed(remoteURL)...)
	send(m, "f2", "y")
	send(m, "y")
	if len(b.compares) == 0 || !slices.Equal(b.compares[len(b.compares)-1].Include, []string{unclearData}) {
		t.Fatalf("comparisons = %+v", b.compares)
	}
	send(m, "y")
	if len(b.sourceAdds) != 1 || !slices.Equal(b.sourceAdds[0].Include, []string{unclearData}) {
		t.Fatalf("adds = %+v", b.sourceAdds)
	}
}

func TestChoiceWithoutRootGoesThroughTheCommands(t *testing.T) {
	b := withUnclear(sudoFolders())
	chosen := b.inspects[filesDir]
	chosen.PlannedFiles = append(slices.Clone(chosen.PlannedFiles), unclearData)
	chosen.Included = []control.SourceExcluded{{Path: unclearData, Reason: control.SourceExcludedWritableFile}}
	chosen.Excluded = chosen.Excluded[: len(chosen.Excluded)-2 : len(chosen.Excluded)-2]
	chosen.Excluded = append(chosen.Excluded, control.SourceExcluded{Path: unclearSecret, Reason: control.SourceExcludedSecretsFile})
	plain := b.inspects[filesDir]
	m, f := sudoFolderModel(t, i18n.English, b, wideWidth, 60, nil)
	f.reports = func(argv []string) string {
		if len(argv) > 3 && argv[2]+" "+argv[3] == "source inspect" {
			if slices.Contains(argv, "--include") {
				return inspectJSON(t, chosen, false, nil)
			}
			return inspectJSON(t, plain, false, nil)
		}
		return filesJSON
	}
	send(m, addKeys(filesDir)...)
	contains(t, m, "[ ] "+unclearData)
	add := []string{"source", "add", "--expect-effect", effectExample, "--yes", "--output", "json", "--", filesDir}
	showsCommand(t, m, add)
	chooseFirst(m)
	inspect := []string{"source", "inspect", "--include", unclearData, "--output", "json", "--", filesDir}
	wantCommands(t, m, f, inspectArgs(filesDir), inspect)
	contains(t, m, "[x] "+unclearData)
	add = []string{"source", "add", "--expect-effect", effectExample, "--include", unclearData, "--yes", "--output", "json", "--", filesDir}
	showsCommand(t, m, add)
	untouched(t, b)
	send(m, "y")
	wantCommands(t, m, f, inspectArgs(filesDir), inspect, add)
	untouched(t, b)
}

func TestChoiceWithoutRootAndARemoteSendsItToTheComparison(t *testing.T) {
	b := withUnclear(sudoFolders())
	chosen := b.inspects[filesDir]
	chosen.Included = []control.SourceExcluded{{Path: unclearData, Reason: control.SourceExcludedWritableFile}}
	chosen.Excluded = slices.DeleteFunc(slices.Clone(chosen.Excluded), func(e control.SourceExcluded) bool { return e.Path == unclearData })
	plain := b.inspects[filesDir]
	cmp := cmpDiff
	cmp.Case = control.SourceCaseFiles
	m, f := sudoFolderModel(t, i18n.English, b, wideWidth, 60, nil)
	f.reports = func(argv []string) string {
		switch argv[2] + " " + argv[3] {
		case "source inspect":
			if slices.Contains(argv, "--include") {
				return inspectJSON(t, chosen, false, nil)
			}
			return inspectJSON(t, plain, false, nil)
		case "repo probe":
			return probeJSON
		case "source compare":
			return compareJSON(t, cmp)
		}
		return filesJSON
	}
	send(m, addKeys(filesDir)...)
	chooseFirst(m)
	send(m, "w")
	send(m, typed(remoteURL)...)
	send(m, "f2", "y", "y")
	compare := []string{"source", "compare", "--fingerprint", fingerprint, "--include", unclearData, "--output", "json", "--", filesDir, remoteURL}
	showsCommand(t, m, compare)
	send(m, "y")
	add := []string{"source", "add", "--url", remoteURL, "--fingerprint", fingerprint, "--expect-effect", effectExample,
		"--expect-state", cmp.State, "--expect-commit", cmp.Commit, "--include", unclearData, "--yes", "--output", "json", "--", filesDir}
	showsCommand(t, m, add)
}

func TestProposalApproveWithAChoiceAsRoot(t *testing.T) {
	b := withUnclear(approveBackend())
	m := proposalOpen(t, b, true, 0)
	send(m, "A")
	contains(t, m, "Unclear files (2)", "[ ] "+unclearData)
	chooseFirst(m)
	contains(t, m, "[x] "+unclearData)
	noProposalChange(t, b)
	send(m, "y")
	if len(b.proposalApproves) != 1 || len(b.sourceAdds) != 1 || !slices.Equal(b.sourceAdds[0].Include, []string{unclearData}) {
		t.Fatalf("approves %+v, adds %+v, calls %v", b.proposalApproves, b.sourceAdds, b.actions())
	}
	contains(t, m, "The stack folder "+filesDir+" is added")
}

func TestProposalApproveWithAChoiceWithoutRootBindsItToTheApproval(t *testing.T) {
	b := withUnclear(approveBackend())
	chosen := b.inspects[filesDir]
	chosen.Included = []control.SourceExcluded{{Path: unclearData, Reason: control.SourceExcludedWritableFile}}
	chosen.Excluded = slices.DeleteFunc(slices.Clone(chosen.Excluded), func(e control.SourceExcluded) bool { return e.Path == unclearData })
	m, f := sudoProposalOpen(t, i18n.English, b, 0, nil)
	f.reports = func(argv []string) string {
		if argv[2]+" "+argv[3] == "source inspect" {
			if slices.Contains(argv, "--include") {
				return inspectJSON(t, chosen, false, nil)
			}
			return inspectJSON(t, b.inspects[filesDir], false, nil)
		}
		return filesJSON
	}
	send(m, "A")
	contains(t, m, "[ ] "+unclearData)
	showsCommand(t, m, approveArgsOf("p-0005", "--expect-effect", effectExample))
	chooseFirst(m)
	inspect := []string{"source", "inspect", "--include", unclearData, "--output", "json", "--", filesDir}
	args := approveArgsOf("p-0005", "--expect-effect", effectExample, "--include", unclearData)
	showsCommand(t, m, args)
	send(m, "y")
	wantCommands(t, m, f, inspectArgs(filesDir), inspect, args)
	untouched(t, b)
}
