package tui

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-kit/i18n"
	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// effectExample is the effect fingerprint the invented inspections through sudo print.
const effectExample = "sha256:example-effect-for-tests-only-0000000000000000000000000000"

// permsExample is the permission plan an inspection as root prints for a folder whose permissions change.
var permsExample = map[string]any{"changed": 3, "summary": []string{"3 paths get the group of the service account with group write; the owner stays"}}

// inspectJSON is what `source inspect --output json` prints as root for res, with the fields root adds.
func inspectJSON(t *testing.T, res control.SourceInspectResult, missing bool, perms map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	var v map[string]any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	if res.Case != control.SourceCaseReject {
		v["missing"], v["effect"] = missing, effectExample
		if perms != nil {
			v["permissions"] = perms
		}
	}
	out, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// compareJSON is what `source compare --output json` prints.
func compareJSON(t *testing.T, cmp control.SourceCompareResult) string {
	t.Helper()
	out, err := json.Marshal(cmp)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// What the other commands of the flows print with --output json.
var (
	probeJSON = `{"host":"example.test","port":"22","key_type":"ssh-ed25519","public_key":"ssh-ed25519 AAAAexamplehostkey",` +
		`"fingerprint":"` + fingerprint + `","pinned":false}`
	cloneJSON = `{"url":"` + repoURL + `","dir":"` + cloneDir + `","host_key_fingerprint":"` + fingerprint + `","head":"` + commitA +
		`","cloned":true,"changed_paths":0}`
	adoptJSON = `{"url":"` + repoURL + `","dir":"` + adoptDir + `","host_key_fingerprint":"` + fingerprint + `","head":"` + commitA +
		`","cloned":false,"changed_paths":9}`
	localJSON = `{"id":"srv-repo-example","path":"` + repoDir + `","kind":"local","head":"` + commitB + `","changed_paths":0}`
	filesJSON = `{"id":"srv-files-example","path":"` + filesDir + `","kind":"remote","head":"` + commitA + `","files":2,"url":"` + remoteURL +
		`","state":"different","branch":"main","pushed":true,"changed_paths":3}`
	remoteJSON = `{"id":"srv-local-example","path":"` + localDir + `","url":"` + remoteURL + `","state":"empty","branch":"main","head":"` +
		commitB + `","pushed":true}`
)

// sudoFolderModel is the interface without root on b, with a fake sudo that prints replies[<group> <command>] for
// each command, as replies["source inspect"].
func sudoFolderModel(t *testing.T, lang i18n.Lang, b *fakeBackend, width, rows int, replies map[string]string) (*Model, *fakeRoot) {
	t.Helper()
	m, f := sudoDetailModel(t, lang, b, width, rows)
	f.reports = func(argv []string) string {
		// argv is sudo-fake qhost-fake <args>.
		if len(argv) < 4 {
			return ""
		}
		return replies[argv[2]+" "+argv[3]]
	}
	return m, f
}

// sudoFolders is foldersBackend with deploy keys for the folders that clone, adopt and compare.
func sudoFolders() *fakeBackend {
	b := remoteBackend(cmpDiff)
	key := control.KeyResult{Fingerprint: fingerprint, PublicKey: "ssh-ed25519 AAAAexamplekey qhost"}
	b.dirKeys[cloneDir], b.dirKeys[adoptDir] = key, key
	return b
}

// inspectArgs are the arguments of the inspection of dir through sudo.
func inspectArgs(dir string) []string {
	return []string{"source", "inspect", "--output", "json", "--", dir}
}

// probeArgs are the arguments of the probe of url through sudo.
func probeArgs(url string) []string { return []string{"repo", "probe", "--output", "json", "--", url} }

// untouched fails when the service was asked for a step that goes through sudo, or the host side was used at all.
// The look for the deploy key is the one read of the service.
func untouched(t *testing.T, b *fakeBackend) {
	t.Helper()
	for _, a := range b.actions() {
		for _, step := range []string{"probe:", "reposet:", "sourceremote:", "sourceadd:"} {
			if strings.HasPrefix(a, step) {
				t.Fatalf("the service was called for a step of sudo: %v", b.actions())
			}
		}
	}
	if b.called("sourceinspect") != 0 || b.called("sourcecompare") != 0 || len(b.host.calls) != 0 {
		t.Fatalf("inspections %d, comparisons %d, host %v", b.called("sourceinspect"), b.called("sourcecompare"), b.host.calls)
	}
}

// wantCommands fails unless exactly the commands want were handed to sudo, in order.
func wantCommands(t *testing.T, m *Model, f *fakeRoot, want ...[]string) {
	t.Helper()
	got := commandsOf(f)
	if len(got) != len(want) {
		t.Fatalf("commands %v, want %v\n%s", got, want, m.render())
	}
	for i := range want {
		if !slices.Equal(got[i], want[i]) {
			t.Fatalf("command %d is %v, want %v", i, got[i], want[i])
		}
	}
}

// says fails when the screen lacks one of the texts, which a dialog may wrap: its words stand in order.
func says(t *testing.T, m *Model, texts ...string) {
	t.Helper()
	for _, text := range texts {
		rest := flat(m.render())
		for _, word := range strings.Fields(text) {
			at := strings.Index(rest, word)
			if at < 0 {
				t.Fatalf("the screen lacks %q:\n%s", text, m.render())
			}
			rest = rest[at+len(word):]
		}
	}
}

// showsCommand fails when the screen lacks the command sudo qhost args.
func showsCommand(t *testing.T, m *Model, args []string) {
	t.Helper()
	if command := sudoCommand(args); !shows(m.render(), command) {
		t.Fatalf("the question lacks %q:\n%s", command, m.render())
	}
}

// A local folder is inspected through sudo without a question of its own; the last question shows the exact
// command with the effect that was shown, and only y runs it.
func TestFolderAddWithoutRootLocal(t *testing.T) {
	b := sudoFolders()
	m, f := sudoFolderModel(t, i18n.English, b, wideWidth, 40, map[string]string{
		"source inspect": inspectJSON(t, b.inspects[repoDir], false, nil), "source add": localJSON})
	send(m, addKeys(repoDir)...)
	wantCommands(t, m, f, inspectArgs(repoDir))
	contains(t, m, "Take over the repository "+repoDir, "permissions are set: no", "No permission changes")
	add := []string{"source", "add", "--expect-effect", effectExample, "--yes", "--output", "json", "--", repoDir}
	showsCommand(t, m, add)
	untouched(t, b)
	send(m, "y")
	wantCommands(t, m, f, inspectArgs(repoDir), add)
	untouched(t, b)
	contains(t, m, "The stack folder "+repoDir+" is added.")
	if m.set.add != nil || m.set.busy {
		t.Fatalf("flow %v, busy %v after the end", m.set.add, m.set.busy)
	}
}

// The plan read through sudo has no handle, but its lines are a change: the inspection says so and lists them, and a
// missing folder for a clone shows that it is created.
func TestFolderAddWithoutRootShowsThePlanAndTheMissingFolder(t *testing.T) {
	b := sudoFolders()
	m, _ := sudoFolderModel(t, i18n.English, b, wideWidth, 50, map[string]string{
		"source inspect": inspectJSON(t, b.inspects[filesDir], false, permsExample)})
	send(m, addKeys(filesDir)...)
	contains(t, m, "permissions are set: yes", "3 paths get the group of the service account")
	if strings.Contains(m.render(), "No permission changes") {
		t.Fatalf("a plan with lines shows as no change:\n%s", m.render())
	}
	showsCommand(t, m, []string{"source", "add", "--expect-effect", effectExample, "--yes", "--output", "json", "--", filesDir})

	b = sudoFolders()
	m, _ = sudoFolderModel(t, i18n.English, b, wideWidth, 50, map[string]string{
		"source inspect": inspectJSON(t, b.inspects[cloneDir], true, nil), "repo probe": probeJSON})
	send(m, addKeys(cloneDir)...)
	send(m, "y")
	send(m, typed(repoURL)...)
	send(m, "f2", "y", "y")
	contains(t, m, "Clone "+repoURL+" into "+cloneDir, "created with owner root:root")
}

// A clone asks before the probe, which asks the git server, then the host key, then the last question with the
// confirmed fingerprint and the effect.
func TestFolderAddWithoutRootClone(t *testing.T) {
	b := sudoFolders()
	m, f := sudoFolderModel(t, i18n.English, b, wideWidth, 50, map[string]string{
		"source inspect": inspectJSON(t, b.inspects[cloneDir], true, nil), "repo probe": probeJSON, "source add": cloneJSON})
	send(m, addKeys(cloneDir)...)
	send(m, "y")
	send(m, typed(repoURL)...)
	send(m, "f2")
	says(t, m, "Read the host key of the server of "+repoURL+"?", "git server of the address", "leaves the host")
	showsCommand(t, m, probeArgs(repoURL))
	wantCommands(t, m, f, inspectArgs(cloneDir))
	send(m, "y")
	wantCommands(t, m, f, inspectArgs(cloneDir), probeArgs(repoURL))
	contains(t, m, "Trust this host key for", fingerprint)
	send(m, "y")
	add := []string{"source", "add", "--url", repoURL, "--fingerprint", fingerprint, "--expect-effect", effectExample,
		"--yes", "--output", "json", "--", cloneDir}
	contains(t, m, "Clone "+repoURL+" into "+cloneDir)
	showsCommand(t, m, add)
	wantCommands(t, m, f, inspectArgs(cloneDir), probeArgs(repoURL))
	untouched(t, b)
	send(m, "y")
	wantCommands(t, m, f, inspectArgs(cloneDir), probeArgs(repoURL), add)
	untouched(t, b)
	contains(t, m, "The stack folder "+cloneDir+" is bound.")
}

// A clone with an origin takes the address of the inspection.
func TestFolderAddWithoutRootAdoptClone(t *testing.T) {
	b := sudoFolders()
	m, f := sudoFolderModel(t, i18n.English, b, wideWidth, 50, map[string]string{
		"source inspect": inspectJSON(t, b.inspects[adoptDir], false, permsExample), "repo probe": probeJSON, "source add": adoptJSON})
	send(m, addKeys(adoptDir)...)
	send(m, "y")
	showsCommand(t, m, probeArgs(repoURL))
	send(m, "y", "y")
	add := []string{"source", "add", "--url", repoURL, "--fingerprint", fingerprint, "--expect-effect", effectExample,
		"--yes", "--output", "json", "--", adoptDir}
	showsCommand(t, m, add)
	send(m, "y")
	wantCommands(t, m, f, inspectArgs(adoptDir), probeArgs(repoURL), add)
	untouched(t, b)
	contains(t, m, "The stack folder "+adoptDir+" is bound.")
}

// A folder of files with a remote asks for the comparison, then shows the command with the state and the commit
// that were shown.
func TestFolderAddWithoutRootFilesWithRemote(t *testing.T) {
	b := sudoFolders()
	cmp := cmpDiff
	cmp.Case = control.SourceCaseFiles
	m, f := sudoFolderModel(t, i18n.English, b, wideWidth, 50, map[string]string{
		"source inspect": inspectJSON(t, b.inspects[filesDir], false, permsExample), "repo probe": probeJSON,
		"source compare": compareJSON(t, cmp), "source add": filesJSON})
	send(m, addKeys(filesDir)...)
	send(m, "w")
	send(m, typed(remoteURL)...)
	send(m, "f2", "y", "y")
	compare := []string{"source", "compare", "--fingerprint", fingerprint, "--output", "json", "--", filesDir, remoteURL}
	says(t, m, "Compare "+filesDir+" with the repository at "+remoteURL+"?", "git server of the address")
	showsCommand(t, m, compare)
	wantCommands(t, m, f, inspectArgs(filesDir), probeArgs(remoteURL))
	send(m, "y")
	add := []string{"source", "add", "--url", remoteURL, "--fingerprint", fingerprint, "--expect-effect", effectExample,
		"--expect-state", control.SourceRemoteDifferent, "--expect-commit", commitA, "--yes", "--output", "json", "--", filesDir}
	showsCommand(t, m, add)
	send(m, "y")
	wantCommands(t, m, f, inspectArgs(filesDir), probeArgs(remoteURL), compare, add)
	untouched(t, b)
	contains(t, m, "The stack folder "+filesDir+" is added.")
}

// Adding a remote probes and compares through sudo and runs source remote with the state of the comparison; a remote
// without a commit has no --expect-commit.
func TestRemoteWithoutRootRunsSourceRemote(t *testing.T) {
	for name, tt := range map[string]struct {
		cmp    control.SourceCompareResult
		expect []string
	}{
		"empty":     {cmpEmpty, []string{"--expect-state", control.SourceRemoteEmpty}},
		"different": {cmpDiff, []string{"--expect-state", control.SourceRemoteDifferent, "--expect-commit", commitA}},
	} {
		t.Run(name, func(t *testing.T) {
			b := sudoFolders()
			m, f := sudoFolderModel(t, i18n.English, b, wideWidth, 50, map[string]string{
				"repo probe": probeJSON, "source compare": compareJSON(t, tt.cmp), "source remote": remoteJSON})
			send(m, sudoRemoteKeys()...)
			compare := []string{"source", "compare", "--fingerprint", fingerprint, "--output", "json", "--", localDir, remoteURL}
			showsCommand(t, m, compare)
			send(m, "y")
			remote := append(append([]string{"source", "remote", "--fingerprint", fingerprint}, tt.expect...),
				"--yes", "--output", "json", "--", localDir, remoteURL)
			showsCommand(t, m, remote)
			wantCommands(t, m, f, probeArgs(remoteURL), compare)
			send(m, "y")
			wantCommands(t, m, f, probeArgs(remoteURL), compare, remote)
			untouched(t, b)
			says(t, m, localDir+" runs against "+remoteURL+" now.")
		})
	}
}

// sudoRemoteKeys are remoteKeys without root: the probe is asked first, so it takes one more yes before the host key.
func sudoRemoteKeys() []string {
	return seq(toFolders, keys("up", "up", "O"), typed(remoteURL), keys("f2", "y", "y"))
}

// A foreign project is inspected through sudo and taken over with source add after the last question.
func TestAdoptWithoutRootRunsSourceAdd(t *testing.T) {
	b := sameFiles()
	res := b.inspects[foreignDir]
	m, f := sudoFolderModel(t, i18n.English, b, ctWide, ctWideRows, map[string]string{
		"source inspect": inspectJSON(t, res, false, nil),
		"source add":     `{"id":"srv-demo-foreign","path":"` + foreignDir + `","kind":"local","head":"` + commitB + `","changed_paths":0}`})
	send(m, "5")
	if !m.ctList.Select(kittui.ItemID("legacy-blog-db-1")) {
		t.Fatal("the container of the foreign project is not in the list")
	}
	send(m, "n")
	wantCommands(t, m, f, inspectArgs(foreignDir))
	add := []string{"source", "add", "--expect-effect", effectExample, "--yes", "--output", "json", "--", foreignDir}
	contains(t, m, "legacy-blog")
	showsCommand(t, m, add)
	send(m, "y")
	wantCommands(t, m, f, inspectArgs(foreignDir), add)
	if len(b.sourceAdds) != 0 || len(b.host.calls) != 0 {
		t.Fatalf("the service or the host was called directly: %v %v", b.sourceAdds, b.host.calls)
	}
	contains(t, m, "The stack folder "+foreignDir+" is added.")
}

// A last command that refuses because the state changed says so in plain words and ends the flow.
func TestFolderAddWithoutRootStateChanged(t *testing.T) {
	for name, tt := range map[string]struct {
		lead func(t *testing.T, b *fakeBackend) map[string]string
		to   []string
		want []string
	}{
		"local": {func(t *testing.T, b *fakeBackend) map[string]string {
			return map[string]string{"source inspect": inspectJSON(t, b.inspects[repoDir], false, nil)}
		}, addKeys(repoDir), []string{"The folder " + repoDir + " changed since it was shown. Nothing was changed.",
			"Add the folder again to see its current state."}},
		"remote": {func(t *testing.T, b *fakeBackend) map[string]string {
			return map[string]string{"repo probe": probeJSON, "source compare": compareJSON(t, cmpDiff)}
		}, seq(sudoRemoteKeys(), keys("y")), []string{"The folder " + localDir + " or its remote changed since it was shown. Nothing was changed.",
			"Add the remote again to see the current comparison."}},
	} {
		t.Run(name, func(t *testing.T) {
			b := sudoFolders()
			m, f := sudoFolderModel(t, i18n.English, b, wideWidth, 50, tt.lead(t, b))
			send(m, tt.to...)
			f.stderr = "qhost: state-changed: the folder changed since it was inspected; nothing was changed\n"
			f.runErr = exitCode(t, "1")
			send(m, "y")
			says(t, m, tt.want...)
			if m.set.add != nil || m.set.busy {
				t.Fatalf("flow %v, busy %v after the refusal", m.set.add, m.set.busy)
			}
			untouched(t, b)
		})
	}
}

// Any other failure of the last command shows what qhost reported.
func TestFolderAddWithoutRootShowsOtherFailures(t *testing.T) {
	b := sudoFolders()
	m, f := sudoFolderModel(t, i18n.English, b, wideWidth, 50, map[string]string{
		"source inspect": inspectJSON(t, b.inspects[repoDir], false, nil)})
	send(m, addKeys(repoDir)...)
	f.stderr = "qhost: invalid-request: example refusal of the command\n"
	f.runErr = exitCode(t, "1")
	send(m, "y")
	contains(t, m, "qhost reported: example refusal of the command")
}

// An inspection that gives no answer, as when the password prompt was given up, stays in the form and changes
// nothing.
func TestFolderAddWithoutRootFailedInspection(t *testing.T) {
	b := sudoFolders()
	m, f := sudoFolderModel(t, i18n.English, b, wideWidth, 40, nil)
	f.stderr = "qhost: invalid-request: the path must be absolute and clean\n"
	f.runErr = exitCode(t, "1")
	send(m, addKeys(repoDir)...)
	if m.set.form == nil || m.set.form.kind != formFolder || m.set.busy {
		t.Fatalf("form %v, busy %v", m.set.form, m.set.busy)
	}
	contains(t, m, "qhost reported: the path must be absolute and clean")
	untouched(t, b)
}

// Cancelling any question runs nothing further and changes nothing.
func TestFolderAddWithoutRootCancelRunsNothing(t *testing.T) {
	for _, tt := range []struct {
		name string
		to   []string
		ran  int
	}{
		{"local", seq(addKeys(repoDir), keys("n")), 1},
		{"local esc", seq(addKeys(repoDir), keys("esc")), 1},
		{"files", seq(addKeys(filesDir), keys("n")), 1},
		{"clone probe", seq(addKeys(cloneDir), keys("y"), typed(repoURL), keys("f2", "n")), 1},
		{"clone host key", seq(addKeys(cloneDir), keys("y"), typed(repoURL), keys("f2", "y", "n")), 2},
		{"clone last question", seq(addKeys(cloneDir), keys("y"), typed(repoURL), keys("f2", "y", "y", "n")), 2},
		{"adopt last question", seq(addKeys(adoptDir), keys("y", "y", "y", "esc")), 2},
		{"files compare", seq(addKeys(filesDir), keys("w"), typed(remoteURL), keys("f2", "y", "y", "n")), 2},
		{"remote probe", seq(toFolders, keys("up", "up", "O"), typed(remoteURL), keys("f2", "n")), 0},
		{"remote compare", seq(sudoRemoteKeys(), keys("n")), 1},
		{"remote last question", seq(sudoRemoteKeys(), keys("y", "n")), 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := sudoFolders()
			cmp := cmpDiff
			if strings.HasPrefix(tt.name, "files") {
				cmp.Case = control.SourceCaseFiles
			}
			replies := map[string]string{"repo probe": probeJSON, "source compare": compareJSON(t, cmp)}
			for _, dir := range []string{repoDir, filesDir, cloneDir, adoptDir} {
				if strings.Contains(strings.Join(tt.to, ""), dir) {
					replies["source inspect"] = inspectJSON(t, b.inspects[dir], dir == cloneDir, nil)
				}
			}
			m, f := sudoFolderModel(t, i18n.English, b, wideWidth, 50, replies)
			send(m, tt.to...)
			got := commandsOf(f)
			if len(got) != tt.ran {
				t.Fatalf("commands %v, want %d", got, tt.ran)
			}
			for _, c := range got {
				if c[0] == "source" && (c[1] == "add" || c[1] == "remote") {
					t.Fatalf("a change ran after a cancel: %v", got)
				}
			}
			if m.set.busy {
				t.Fatal("busy after a cancel")
			}
			untouched(t, b)
		})
	}
}

// As root nothing goes through sudo: the flow uses the host side and the service as before.
func TestFolderAddAsRootRunsNoSudo(t *testing.T) {
	b := sudoFolders()
	m := newSettingsModel(t, b)
	f := &fakeRoot{root: true}
	m.root = f.hook()
	send(m, addKeys(repoDir)...)
	if strings.Contains(m.render(), "sudo qhost") {
		t.Fatalf("the question shows a command as root:\n%s", m.render())
	}
	send(m, "y")
	if len(f.reqs) != 0 || len(b.sourceAdds) != 1 {
		t.Fatalf("requests %+v, adds %v", f.reqs, b.sourceAdds)
	}
}
