package tui

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-kit/i18n"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// What the commands print with --output json, by the verb after "stack".
const (
	reportDeploy = `{"run":"20261030T160000Z-0123abcd","stack":"demo-stack","commit":"0123456789abcdef0123"}`
	reportHold   = `{"stack":"%s","active":true,"changed":true}`
	reportResume = `{"stack":"photo-album","repair":"released"}`
	reportAction = `{"stack":"%s","action":"%s","stopped":false}`
	reportUpdate = `[{"stack":"demo-stack","variable":"APP_VERSION","from":"1.4.2","to":"1.5.0","run":"20261030T160000Z-0123abcd",` +
		`"commit":"abcdef0123456789","result":"succeeded","error":""}]`
	reportUpdates = `[{"stack":"demo-stack","update":"automatic","variable":"APP_VERSION","current":"1.4.2","available":"1.5.0",` +
		`"major_available":"","status":"update","checked":"2026-10-30T03:05:00Z","error":""},` +
		`{"stack":"photo-album","update":"automatic","variable":"ALBUM_VERSION","current":"2.0.1","available":"","status":"error",` +
		`"checked":"2026-10-30T03:05:00Z","error":"registry unreachable"}]`
	reportChangelog = `[{"stack":"demo-stack","variable":"APP_VERSION","current":"1.4.2","target":"1.5.0","status":"release",` +
		`"version":"1.5.0","published":"2026-10-29T09:00:00Z","name":"Spring release","url":"","source":"example/app","link":"https://example.invalid/app/releases",` +
		`"reason":"","omitted":null,"body":"Adds a dark theme.\n\n- faster start"}]`
)

// stackReports answers each command of the Stacks area the way qhost does.
func stackReports(argv []string) string {
	at := slices.Index(argv, "stack")
	if at < 0 {
		return ""
	}
	verb, stack := argv[at+1], argv[len(argv)-1]
	switch verb {
	case "deploy":
		return reportDeploy
	case "pause", "unpause", "pin", "unpin":
		return strings.Replace(reportHold, "%s", stack, 1)
	case "resume":
		return reportResume
	case "start", "stop", "restart":
		return fmt.Sprintf(reportAction, stack, verb)
	case "update":
		return reportUpdate
	case "updates":
		if slices.Contains(argv, "--changelog") {
			return reportChangelog
		}
		return reportUpdates
	}
	return ""
}

// sudoStackModel is the Stacks area of an interface run by the administrator ada (UID 1000) without root, with a
// fake sudo that prints what qhost prints.
func sudoStackModel(t *testing.T, lang i18n.Lang, b *fakeBackend, width, rows int) (*Model, *fakeRoot) {
	t.Helper()
	m := newTestModel(t, lang, b, width, rows)
	m.euid = func() int { return 1000 }
	f := &fakeRoot{reports: stackReports}
	m.root = f.hook()
	return m, f
}

// selectStack moves the mark of the Stacks list to the stack name.
func selectStack(t *testing.T, m *Model, name string) {
	t.Helper()
	send(m, "home")
	for range 20 {
		if id, ok := m.list.Selected(); ok && m.rows[id].name == name {
			return
		}
		send(m, "down")
	}
	t.Fatalf("stack %s is not reachable in the list", name)
}

// shows reports whether the screen shows the command, a dialog wrapping it over other text of the screen: its words
// stand in order.
func shows(screen, command string) bool {
	rest := flat(screen)
	for _, word := range strings.Fields("$ " + command) {
		at := strings.Index(rest, word)
		if at < 0 {
			return false
		}
		rest = rest[at+len(word):]
	}
	return true
}

// commandsOf are the arguments after qhost of the commands handed to sudo, without the password checks.
func commandsOf(f *fakeRoot) [][]string {
	var out [][]string
	for _, r := range f.reqs {
		if !r.Validate {
			out = append(out, r.Args)
		}
	}
	return out
}

// Each action of the Stacks area shows the exact command, runs nothing before y, then hands the terminal to sudo
// with that command and shows its answer. The service is not called.
func TestStackActionWithoutRootRunsTheCommandThroughSudo(t *testing.T) {
	for _, tt := range []struct {
		name, key, stack string
		args             []string
		done             string
	}{
		{"deploy", "d", "demo-stack", []string{"stack", "deploy", "--output", "json", "--", "demo-stack"}, "run 20261030T160000Z-0123abcd started at commit 0123456789ab"},
		{"pause", "p", "demo-stack", []string{"stack", "pause", "--output", "json", "--", "demo-stack"}, "paused"},
		{"unpause", "P", "demo-stack", []string{"stack", "unpause", "--output", "json", "--", "demo-stack"}, "no longer paused"},
		{"pin", "i", "demo-stack", []string{"stack", "pin", "--output", "json", "--", "demo-stack"}, "pinned"},
		{"unpin", "I", "demo-stack", []string{"stack", "unpin", "--output", "json", "--", "demo-stack"}, "no longer pinned"},
		{"resume", "g", "photo-album", []string{"stack", "resume", "--output", "json", "--", "photo-album"}, "pause or repair halt cleared"},
		{"start", "S", "demo-stack", []string{"stack", "start", "--yes", "--output", "json", "--", "demo-stack"}, "started, the stopped mark is removed"},
		{"stop", "X", "demo-stack", []string{"stack", "stop", "--yes", "--output", "json", "--", "demo-stack"}, "stopped, it stays stopped"},
		{"restart", "Z", "demo-stack", []string{"stack", "restart", "--yes", "--output", "json", "--", "demo-stack"}, "restarted"},
		{"check", "c", "", []string{"stack", "updates", "--refresh", "--output", "json"}, "2 stacks: 1 with an update available, 1 could not be checked"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := demoBackend()
			m, f := sudoStackModel(t, i18n.English, b, wideWidth, 60)
			if tt.stack != "" {
				selectStack(t, m, tt.stack)
			}
			send(m, tt.key)
			command := "sudo qhost " + strings.Join(tt.args, " ")
			if screen := m.render(); !shows(screen, command) || !strings.Contains(flat(screen), "yes, with sudo") {
				t.Fatalf("the question lacks %q:\n%s", command, m.render())
			}
			if len(f.reqs) != 0 {
				t.Fatalf("something ran before the answer: %+v", f.reqs)
			}
			send(m, "y")
			if got := commandsOf(f); len(got) != 1 || !slices.Equal(got[0], tt.args) {
				t.Fatalf("commands %v, want %v", got, tt.args)
			}
			if len(f.execs) != 1 || !slices.Equal(f.execs[0].Args, append([]string{"sudo-fake", "qhost-fake"}, tt.args...)) {
				t.Fatalf("the terminal was not handed to sudo: %v", f.execs)
			}
			if env := f.execs[0].Env; !slices.Equal(env, []string{"LC_ALL=C"}) || len(f.reqs[0].Secret) != 0 {
				t.Errorf("environment %v, secret %q", env, f.reqs[0].Secret)
			}
			if got := b.actions(); len(got) != 0 {
				t.Fatalf("the service was called directly: %v", got)
			}
			screen := flat(m.render())
			for _, w := range []string{tt.done, "finished"} {
				if !strings.Contains(screen, w) {
					t.Errorf("the result lacks %q:\n%s", w, m.render())
				}
			}
			if m.run != nil {
				t.Error("the action is still marked as running")
			}
		})
	}
}

// Ticked stacks get one command each, all shown in the question and run one after another.
func TestStackActionOfSeveralStacksRunsOneCommandEach(t *testing.T) {
	b := demoBackend()
	m, f := sudoStackModel(t, i18n.English, b, wideWidth, 60)
	send(m, "home", "space", "down", "space", "p")
	for _, want := range []string{"sudo qhost stack pause --output json -- demo-stack", "sudo qhost stack pause --output json -- mail-relay"} {
		if !shows(m.render(), want) {
			t.Fatalf("the question lacks %q:\n%s", want, m.render())
		}
	}
	send(m, "y")
	want := [][]string{{"stack", "pause", "--output", "json", "--", "demo-stack"}, {"stack", "pause", "--output", "json", "--", "mail-relay"}}
	if got := commandsOf(f); !slices.EqualFunc(got, want, slices.Equal) {
		t.Fatalf("commands %v, want %v", got, want)
	}
	if !strings.Contains(flat(m.render()), "2 ok, 0 failed") {
		t.Errorf("no result:\n%s", m.render())
	}
}

// A start, stop or restart that failed ends the sequence, and the message of qhost is shown.
func TestStackActionFailureOfTheCommandIsShownAndEndsTheSequence(t *testing.T) {
	m, f := sudoStackModel(t, i18n.English, demoBackend(), wideWidth, 60)
	f.stderr = "qhost: conflict: a run of stack demo-stack is active\n"
	f.runErr = exitCode(t, "1")
	f.reports = func([]string) string { return "" }
	send(m, "home", "space", "down", "space", "X", "y")
	screen := flat(m.render())
	for _, want := range []string{"qhost reported: a run of stack demo-stack is active", "0 ok, 1 failed", "not run"} {
		if !strings.Contains(screen, want) {
			t.Errorf("the result lacks %q:\n%s", want, m.render())
		}
	}
	if got := commandsOf(f); len(got) != 1 {
		t.Errorf("commands after the failure: %v", got)
	}
}

// An update asks for the release notes first: it says that GitHub is asked, shows the command and runs nothing
// before y. Then the preview, then the question of the update itself.
func TestUpdateWithoutRootAsksForTheNotesThenForTheUpdate(t *testing.T) {
	b := demoBackend()
	m, f := sudoStackModel(t, i18n.English, b, wideWidth, 60)
	send(m, "u")
	if screen := flat(m.render()); !strings.Contains(screen, "This asks GitHub, so the request leaves the host.") ||
		!shows(m.render(), "sudo qhost stack updates --changelog --output json -- demo-stack") {
		t.Fatalf("the question lacks the sentence on GitHub or the command:\n%s", m.render())
	}
	if len(f.reqs) != 0 {
		t.Fatalf("sudo ran before the answer: %+v", f.reqs)
	}
	send(m, "y")
	notes := []string{"stack", "updates", "--changelog", "--output", "json", "--", "demo-stack"}
	if got := commandsOf(f); len(got) != 1 || !slices.Equal(got[0], notes) {
		t.Fatalf("commands %v, want %v", got, notes)
	}
	screen := flat(m.render())
	for _, want := range []string{"Update demo-stack?", "Spring release", "Adds a dark theme.", "1.4.2"} {
		if !strings.Contains(screen, want) {
			t.Fatalf("the preview lacks %q:\n%s", want, m.render())
		}
	}
	send(m, "y")
	update := []string{"stack", "update", "--output", "json", "--", "demo-stack"}
	if !shows(m.render(), "sudo qhost "+strings.Join(update, " ")) || len(commandsOf(f)) != 1 {
		t.Fatalf("the update itself is not asked for with its command:\n%s", m.render())
	}
	send(m, "y")
	if got := commandsOf(f); len(got) != 2 || !slices.Equal(got[1], update) {
		t.Fatalf("commands %v, want the update last", got)
	}
	if screen = flat(m.render()); !strings.Contains(screen, "APP_VERSION: 1.4.2 -> 1.5.0") || !strings.Contains(screen, "1 ok, 0 failed") {
		t.Errorf("no result:\n%s", m.render())
	}
	if got := b.actions(); len(got) != 0 {
		t.Errorf("the service was called directly: %v", got)
	}
}

// Declining the notes opens no preview and runs nothing.
func TestUpdateWithoutRootDeclinedRunsNothing(t *testing.T) {
	m, f := sudoStackModel(t, i18n.English, demoBackend(), wideWidth, 60)
	send(m, "u", "n")
	if m.modal != nil || len(f.reqs) != 0 {
		t.Fatalf("modal %v, requests %+v", m.modal != nil, f.reqs)
	}
}

// A stack without an update has nothing to read from GitHub: the preview opens at once, and the update is asked for.
func TestUpdateWithoutRootOfAStackWithoutUpdateSkipsTheNotes(t *testing.T) {
	m, f := sudoStackModel(t, i18n.English, demoBackend(), wideWidth, 60)
	selectStack(t, m, "mail-relay")
	send(m, "u")
	if !strings.Contains(flat(m.render()), "Update mail-relay?") || len(f.reqs) != 0 {
		t.Fatalf("no preview:\n%s", m.render())
	}
	send(m, "y")
	if !shows(m.render(), "sudo qhost stack update --output json -- mail-relay") {
		t.Fatalf("no question with the command:\n%s", m.render())
	}
}

// Without root the result of an update that was refused is read from the rows of the command, which ends with 1.
func TestUpdateWithoutRootReadsARefusalFromTheRows(t *testing.T) {
	m, f := sudoStackModel(t, i18n.English, demoBackend(), wideWidth, 60)
	f.reports = func(argv []string) string {
		return `[{"stack":"demo-stack","variable":"","from":"","to":"","run":"","commit":"","result":"refused","error":"the stack has local changes"}]`
	}
	f.runErr = exitCode(t, "1")
	f.stderr = "qhost: runtime: 1 of 1 stack(s) were not updated completely\n"
	send(m, "u", "y", "y", "y")
	screen := flat(m.render())
	for _, want := range []string{"refused: the stack has local changes", "0 ok, 1 failed"} {
		if !strings.Contains(screen, want) {
			t.Errorf("the result lacks %q:\n%s", want, m.render())
		}
	}
}

// The notes that GitHub did not answer, and the failure of the command, are said.
func TestUpdateWithoutRootNotesFailureIsShown(t *testing.T) {
	m, f := sudoStackModel(t, i18n.English, demoBackend(), wideWidth, 60)
	f.stderr = "qhost: permission: the service refused\n"
	f.runErr = exitCode(t, "1")
	f.reports = func([]string) string { return "" }
	send(m, "u", "y")
	if !strings.Contains(flat(m.render()), "qhost reported: the service refused") {
		t.Fatalf("no failure:\n%s", m.render())
	}
	if m.modal != nil && m.modal.kind == modalPreview {
		t.Error("a preview opened without notes")
	}
}

// The check says that the registries are asked before the command.
func TestCheckWithoutRootSaysThatTheRegistriesAreAsked(t *testing.T) {
	m, _ := sudoStackModel(t, i18n.English, demoBackend(), wideWidth, 60)
	send(m, "c")
	if !strings.Contains(flat(m.render()), "This asks the registries of the images, so the requests leave the host.") {
		t.Fatalf("no sentence on the registries:\n%s", m.render())
	}
}

// Cancelling any of the questions runs nothing.
func TestStackActionWithoutRootCancelledRunsNothing(t *testing.T) {
	for _, key := range []string{"d", "p", "S", "c"} {
		for _, no := range []string{"n", "esc", "enter"} {
			m, f := sudoStackModel(t, i18n.English, demoBackend(), wideWidth, 60)
			send(m, key)
			if m.modal == nil {
				t.Fatalf("%s opened no question", key)
			}
			send(m, no)
			if len(f.reqs) != 0 || m.run != nil {
				t.Fatalf("key %s answer %q ran something: %+v", key, no, f.reqs)
			}
		}
	}
}

// sudoDetailModel is the interface of ada without root on the stack settings of demo-stack.
func sudoDetailModel(t *testing.T, lang i18n.Lang, b *fakeBackend, width, rows int) (*Model, *fakeRoot) {
	t.Helper()
	m := newTestModel(t, lang, b, width, rows)
	m.euid = func() int { return 1000 }
	f := &fakeRoot{}
	m.root = f.hook()
	return m, f
}

// Saving the update rules asks with the command, reads the settings the service holds now and hands them on
// standard input after the password check; nothing of them is in the arguments.
func TestSettingsSaveWithoutRootHandsTheSettingsOnStandardInput(t *testing.T) {
	b := updateBackend()
	m, f := sudoDetailModel(t, i18n.English, b, wideWidth, 40)
	f.report = `{"stack":"demo-stack","changed":true,"commit":"0123456789abcdef","settings":{"version":1}}`
	send(m, seq(toUpdateForm, keys("f2"))...)
	command := "sudo qhost stack settings set --file - --output json -- demo-stack"
	if !shows(m.render(), command) {
		t.Fatalf("the question lacks %q:\n%s", command, m.render())
	}
	if len(f.reqs) != 0 {
		t.Fatalf("sudo ran before the answer: %+v", f.reqs)
	}
	send(m, "y")
	if len(f.reqs) != 2 || !f.reqs[0].Validate || f.reqs[1].Validate {
		t.Fatalf("requests %+v, want the password check then the command", f.reqs)
	}
	args := []string{"stack", "settings", "set", "--file", "-", "--output", "json", "--", "demo-stack"}
	if !slices.Equal(f.reqs[1].Args, args) {
		t.Fatalf("arguments %v, want %v", f.reqs[1].Args, args)
	}
	if len(f.bgs) != 1 || !slices.Equal(f.bgs[0].Args, append([]string{"sudo-fake", "-n", "qhost-fake"}, args...)) ||
		!slices.Equal(f.bgs[0].Env, []string{"LC_ALL=C"}) {
		t.Fatalf("the command was not run without a prompt: %v", f.bgs)
	}
	var sent control.StackSettings
	if err := json.Unmarshal(f.stdins[0], &sent); err != nil || sent.Update == nil || sent.Update.Mode != "automatic" ||
		sent.Backup == nil || sent.Compose == nil {
		t.Fatalf("standard input %s: %v", f.stdins[0], err)
	}
	if got := b.settingsSets; len(got) != 0 {
		t.Fatalf("the service was called directly: %v", got)
	}
	contains(t, m, "Update rules of the stack demo-stack saved.", "0123456789ab")
}

// The message of qhost about the settings stands at the form, which stays open.
func TestSettingsSaveWithoutRootShowsTheMessageOfTheCommand(t *testing.T) {
	m, f := sudoDetailModel(t, i18n.English, updateBackend(), wideWidth, 40)
	f.stderr = "qhost: invalid: stack settings: update.observe: too large\n"
	f.runErr = exitCode(t, "1")
	send(m, seq(toUpdateForm, keys("f2", "y"))...)
	contains(t, m, "qhost reported: stack settings: update.observe: too large", "Correct the form")
	if m.set.form == nil || m.set.busy {
		t.Fatalf("form %v, busy %v", m.set.form, m.set.busy)
	}
}

// Backup, check and compose go the same way.
func TestOtherSettingsFormsWithoutRootUseTheSameCommand(t *testing.T) {
	for _, tt := range []struct {
		name string
		keys []string
		done string
		part func(control.StackSettings) bool
	}{
		{"backup", toBackupForm, "Backup of the stack demo-stack saved.", func(s control.StackSettings) bool { return s.Backup != nil && s.Update != nil }},
		{"compose", toComposeForm, "Compose files of the stack demo-stack saved.", func(s control.StackSettings) bool { return s.Compose != nil && s.Backup != nil }},
		{"check", keys("enter", "k"), "Check of the stack demo-stack saved.", func(s control.StackSettings) bool { return s.Check != nil && s.Backup != nil }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m, f := sudoDetailModel(t, i18n.English, updateBackend(), wideWidth, 40)
			f.report = `{"stack":"demo-stack","changed":true,"commit":"0123456789abcdef","settings":{"version":1}}`
			send(m, seq(tt.keys, keys("f2", "y"))...)
			if len(f.reqs) != 2 || !slices.Equal(f.reqs[1].Args, []string{"stack", "settings", "set", "--file", "-", "--output", "json", "--", "demo-stack"}) {
				t.Fatalf("requests %+v", f.reqs)
			}
			var sent control.StackSettings
			if err := json.Unmarshal(f.stdins[0], &sent); err != nil || !tt.part(sent) {
				t.Fatalf("standard input %s: %v", f.stdins[0], err)
			}
			contains(t, m, tt.done)
		})
	}
}

// Removing the settings, restoring a backup and deleting a secret ask with their command and show its answer.
func TestDetailChangesWithoutRootRunTheirCommand(t *testing.T) {
	for _, tt := range []struct {
		name, report, done string
		keys               []string
		args               []string
	}{
		{"remove settings", `{"stack":"demo-stack","removed":true,"commit":"0123456789abcdef"}`, "Settings of the stack demo-stack removed.",
			keys("enter", "E"), []string{"stack", "settings", "remove", "--output", "json", "--", "demo-stack"}},
		{"restore", `{"backup":"` + olderBackup + `","stack":"demo-stack","restored":true}`, "Backup " + olderBackup + " restored for the stack demo-stack.",
			seq(toRestore, keys("enter")), []string{"stack", "backup", "restore", "--yes", "--output", "json", "--", "demo-stack", olderBackup}},
		{"secret delete", `{"stack":"demo-stack","key":"API_SIGNING_KEY","deleted":true}`, "Secret API_SIGNING_KEY of the stack demo-stack deleted.",
			seq(toSecretPick, keys("enter")), []string{"stack", "secret", "delete", "--output", "json", "--", "demo-stack", "API_SIGNING_KEY"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := settingsBackend()
			m, f := sudoDetailModel(t, i18n.English, b, wideWidth, 40)
			f.report = tt.report
			send(m, tt.keys...)
			if command := "sudo qhost " + strings.Join(tt.args, " "); !shows(m.render(), command) {
				t.Fatalf("the question lacks %q:\n%s", command, m.render())
			}
			if len(f.reqs) != 0 {
				t.Fatalf("sudo ran before the answer: %+v", f.reqs)
			}
			send(m, "y")
			if got := commandsOf(f); len(got) != 1 || !slices.Equal(got[0], tt.args) {
				t.Fatalf("commands %v, want %v", got, tt.args)
			}
			if len(f.execs) != 1 || len(f.bgs) != 0 {
				t.Fatalf("no terminal handover: execs %d, background %d", len(f.execs), len(f.bgs))
			}
			if got := b.actions(); len(got) != 0 {
				t.Fatalf("the service was called directly: %v", got)
			}
			contains(t, m, tt.done)
		})
	}
}

// The message of qhost about a change of the detail is shown.
func TestDetailChangeWithoutRootShowsTheMessageOfTheCommand(t *testing.T) {
	m, f := sudoDetailModel(t, i18n.English, settingsBackend(), wideWidth, 40)
	f.stderr = "qhost: not-found: no such backup\n"
	f.runErr = exitCode(t, "1")
	send(m, seq(toRestore, keys("enter", "y"))...)
	contains(t, m, "qhost reported: no such backup")
}

// Cancelling a change of the detail runs nothing.
func TestDetailChangeWithoutRootCancelledRunsNothing(t *testing.T) {
	for _, ks := range [][]string{keys("enter", "E"), seq(toRestore, keys("enter")), seq(toSecretPick, keys("enter"))} {
		m, f := sudoDetailModel(t, i18n.English, settingsBackend(), wideWidth, 40)
		send(m, ks...)
		send(m, "n")
		if len(f.reqs) != 0 || m.set.busy {
			t.Fatalf("keys %v ran something: %+v", ks, f.reqs)
		}
	}
}

// goldenSudo is the screen of the Stacks area or the detail of an interface without root after ks, in both languages
// and both sizes.
func goldenSudo(t *testing.T, prefix string, setup func() *fakeBackend, ks ...string) {
	t.Helper()
	for lname, lang := range goldenLangs {
		for size, dim := range goldenFormSizes {
			t.Run(lname+"."+size, func(t *testing.T) {
				m, _ := sudoStackModel(t, lang, setup(), dim[0], dim[1])
				send(m, ks...)
				screen := m.render()
				checkScreen(t, screen, dim[0], dim[1])
				checkGolden(t, prefix+"."+lname+"."+size+".golden", screen+"\n")
			})
		}
	}
}

func TestGoldenSudoDeployQuestion(t *testing.T) {
	goldenSudo(t, "stacks.deploy.sudo", demoBackend, "d")
}

func TestGoldenSudoPauseOfSeveralStacksQuestion(t *testing.T) {
	goldenSudo(t, "stacks.pause.sudo", demoBackend, "home", "space", "down", "space", "p")
}

func TestGoldenSudoChangelogQuestion(t *testing.T) {
	goldenSudo(t, "stacks.changelog.sudo", demoBackend, "u")
}

func TestGoldenSudoCheckQuestion(t *testing.T) {
	goldenSudo(t, "stacks.check.sudo", demoBackend, "c")
}

func TestGoldenSudoDeploySummary(t *testing.T) {
	goldenSudo(t, "stacks.deploy.sudosummary", demoBackend, "d", "y")
}

func TestGoldenSudoUpdateSettingsQuestion(t *testing.T) {
	goldenSudo(t, "detail.update.sudo", updateBackend, seq(toUpdateForm, keys("f2"))...)
}

func TestGoldenSudoRestoreQuestion(t *testing.T) {
	goldenSudo(t, "detail.restore.sudo", settingsBackend, seq(toRestore, keys("enter"))...)
}
