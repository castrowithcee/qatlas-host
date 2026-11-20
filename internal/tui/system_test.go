package tui

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"golang.org/x/crypto/ssh"

	"github.com/castrowithcee/qatlas-kit/i18n"
	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/host/hostadmin"
	"github.com/castrowithcee/qatlas-host/internal/host/hostchange"
)

// flat is a screen with the frame of a dialog taken out and every run of blanks one space, so a text the dialog
// wrapped reads as one line.
func flat(screen string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(screen, "║", " ")), " ")
}

// testAccounts are the invented account files of testdata/host; nothing of the machine is read.
func testAccounts(privileged bool) *hostadmin.Host {
	return &hostadmin.Host{PasswdPath: "testdata/host/passwd", GroupPath: "testdata/host/group", ShadowPath: "testdata/host/shadow",
		Privileged: privileged, Now: func() time.Time { return time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC) },
		ProtectedRoots: []string{"/", "/etc", "/usr", "/boot", "/proc", "/sys"}}
}

// demoKey is an invented public key from a fixed seed, so its fingerprint stays the same.
func demoKey(t *testing.T, seed byte) string {
	t.Helper()
	pub, err := ssh.NewPublicKey(ed25519.NewKeyFromSeed(bytes.Repeat([]byte{seed}, ed25519.SeedSize)).Public())
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(pub))) + " demo@example.test"
}

// fakeRoot stands in for sudo: it builds commands the way the hook of the command line does, never runs one, and
// answers with report. root makes it a hook of an interface that runs as root, which needs no password check.
type fakeRoot struct {
	root bool
	// report is what a run prints; runErr the error it ends with. reports, where set, gives the report of a
	// command by its argument vector and takes the place of report.
	report  string
	reports func(argv []string) string
	runErr  error
	// stderr is what a run of the command writes to standard error.
	stderr string
	// reqs are the requests asked for, built the commands built, execs the commands handed the terminal and bgs
	// those run in the background with what they read on standard input.
	reqs   []HostRequest
	built  []*exec.Cmd
	execs  []*exec.Cmd
	bgs    []*exec.Cmd
	stdins [][]byte
}

func (f *fakeRoot) hook() hostRoot {
	return hostRoot{run: f.run, exec: f.exec, bg: f.bg}
}

func (f *fakeRoot) run(req HostRequest) (*exec.Cmd, error) {
	f.reqs = append(f.reqs, req)
	if req.Validate {
		if f.root {
			return nil, nil
		}
		return exec.Command("sudo-fake", "-v"), nil
	}
	var cmd *exec.Cmd
	switch {
	case f.root:
		cmd = exec.Command("qhost-fake", req.Args...)
	case len(req.Secret) > 0:
		cmd = exec.Command("sudo-fake", append([]string{"-n", "qhost-fake"}, req.Args...)...)
	default:
		cmd = exec.Command("sudo-fake", append([]string{"qhost-fake"}, req.Args...)...)
	}
	if !f.root {
		cmd.Env = []string{"LC_ALL=C"}
	}
	if len(req.Secret) > 0 {
		cmd.Stdin = bytes.NewReader(req.Secret)
	}
	f.built = append(f.built, cmd)
	return cmd, nil
}

func (f *fakeRoot) exec(cmd *exec.Cmd, fn tea.ExecCallback) tea.Cmd {
	f.execs = append(f.execs, cmd)
	if slices.Contains(cmd.Args, "-v") {
		// The password check succeeds; only the command itself ends as the test says.
		return func() tea.Msg { return fn(nil) }
	}
	_, _ = io.WriteString(cmd.Stdout, f.reportOf(cmd))
	_, _ = io.WriteString(cmd.Stderr, f.stderr)
	return func() tea.Msg { return fn(f.runErr) }
}

func (f *fakeRoot) bg(cmd *exec.Cmd) error {
	f.bgs = append(f.bgs, cmd)
	var in []byte
	if cmd.Stdin != nil {
		in, _ = io.ReadAll(cmd.Stdin)
	}
	f.stdins = append(f.stdins, in)
	_, _ = io.WriteString(cmd.Stdout, f.reportOf(cmd))
	_, _ = io.WriteString(cmd.Stderr, f.stderr)
	return f.runErr
}

// reportOf is what the command prints.
func (f *fakeRoot) reportOf(cmd *exec.Cmd) string {
	if f.reports != nil {
		return f.reports(cmd.Args)
	}
	return f.report
}

// lastArgs are the arguments of the last command asked for after its command group, such as "host".
func (f *fakeRoot) lastArgs(t *testing.T) []string {
	t.Helper()
	for i := len(f.reqs) - 1; i >= 0; i-- {
		if !f.reqs[i].Validate {
			return f.reqs[i].Args[1:]
		}
	}
	t.Fatal("no command was asked for")
	return nil
}

func appliedReport(action string) string {
	return `{"journal_id":"20261102T090000-0001","action":"` + action + `","sentence":"x","commands":[],"result":"applied",` +
		`"check":"passed","check_error":"","restored":false,"reversible":true,"error":"","warning":""}`
}

// systemModel is the interface on the System area of the invented host, run as the user ada (UID 1000) without root.
func systemModel(t *testing.T, lang i18n.Lang, width, rows int) (*Model, *fakeRoot) {
	t.Helper()
	m := newTestModel(t, lang, demoBackend(), width, rows)
	m.euid = func() int { return 1000 }
	m.human = func() int { return 1000 }
	f := &fakeRoot{report: appliedReport("demo")}
	m.root = f.hook()
	send(m, "7")
	if !m.sys.loaded {
		t.Fatalf("the users were not read: %v", m.sys.err)
	}
	return m, f
}

// toUser marks the user name in the list of the users page.
func toUser(t *testing.T, m *Model, name string) {
	t.Helper()
	if !m.sys.userList.Select(kittui.ItemID(name)) {
		t.Fatalf("no row for %s", name)
	}
}

// pickChangeRow opens the choice of a change for the marked user and picks the row id.
func pickChangeRow(t *testing.T, m *Model, id string) {
	t.Helper()
	send(m, "e")
	if m.sys.changes == nil || !m.sys.changes.Select(kittui.ItemID(id)) {
		t.Fatalf("the change %s is not offered", id)
	}
	send(m, "enter")
}

func TestSystemIsTheSeventhAreaAndTheOtherKeysStay(t *testing.T) {
	m := newTestModel(t, i18n.English, demoBackend(), wideWidth, goldenRows)
	for i, want := range []kittui.AreaID{areaStacks, areaDetail, areaRuns, areaSettings, areaContainers, areaProposals, areaSystem} {
		m.setZone(kittui.ZoneTabs)
		send(m, areaKeys[i])
		if m.area != want {
			t.Fatalf("key %s: area %s, want %s", areaKeys[i], m.area, want)
		}
	}
}

func TestSystemListsTheUsersAndGroupsOfTheHost(t *testing.T) {
	m, _ := systemModel(t, i18n.English, wideWidth, 40)
	screen := m.render()
	for _, want := range []string{"ada", "ben", "cleo", "System accounts", "root", "yes, without qhost", "Login state and SSH keys are shown to root only."} {
		if !strings.Contains(screen, want) {
			t.Errorf("the users page lacks %q:\n%s", want, screen)
		}
	}
	m.setZone(kittui.ZoneSubtabs)
	send(m, "right", "down")
	if m.sys.page != pageGroups {
		t.Fatalf("page %s, want groups", m.sys.page)
	}
	screen = m.render()
	for _, want := range []string{"media", "backup-ops", "System groups", "sudo"} {
		if !strings.Contains(screen, want) {
			t.Errorf("the groups page lacks %q:\n%s", want, screen)
		}
	}
}

// A host without the deploy service still shows its users: the area needs no service.
func TestSystemIsNotHiddenByTheMissingDeployService(t *testing.T) {
	m, _ := notSetUpModel(t, i18n.English, newFakeDeploy(), 1000, wideWidth, 40)
	send(m, "7")
	screen := m.render()
	if strings.Contains(screen, "The deploy service is not set up yet") || !strings.Contains(screen, "cleo") {
		t.Fatalf("the System area is hidden by the setup:\n%s", screen)
	}
}

// The initial password goes to the child on standard input only: never into its arguments or its environment,
// never onto the screen, and the buffers that held it are overwritten once the run is over.
func TestInitialPasswordIsNeverOnTheCommandLineOfTheChild(t *testing.T) {
	m, f := systemModel(t, i18n.English, wideWidth, 40)
	f.report = appliedReport(hostchange.ActionUserCreate)
	send(m, seq(keys("a"), typed("dora"), keys("down"), typed(hiddenA), keys("down"), typed(hiddenA), keys("f2"))...)
	if m.modal == nil {
		t.Fatalf("no confirmation:\n%s", m.render())
	}
	noLeak(t, m.render(), hiddenA)
	send(m, "y")
	if len(f.bgs) != 1 || len(f.execs) != 1 || !slices.Contains(f.execs[0].Args, "-v") {
		t.Fatalf("want sudo -v on the terminal, then one run without it: execs %d, runs %d", len(f.execs), len(f.bgs))
	}
	child := f.bgs[0]
	for _, part := range append(slices.Clone(child.Args), child.Env...) {
		if strings.Contains(part, hiddenA) {
			t.Fatalf("the password is in the command line or environment of the child: %q", child.Args)
		}
	}
	want := []string{"-n", "qhost-fake", "host", "apply", "--confirm", "--output", "json", "--", hostchange.ActionUserCreate, "dora"}
	if !slices.Equal(child.Args[1:], want) {
		t.Fatalf("arguments %q, want %q", child.Args[1:], want)
	}
	var in hostchange.CreateInput
	if err := json.Unmarshal(f.stdins[0], &in); err != nil || in.Password != hiddenA {
		t.Fatalf("the standard input does not carry the password: %v", err)
	}
	for _, req := range f.reqs {
		if len(req.Secret) > 0 && bytes.ContainsFunc(req.Secret, func(r rune) bool { return r != 0 }) {
			t.Fatal("the secret was not overwritten after the run")
		}
	}
	screen := m.render()
	noLeak(t, screen, hiddenA)
	if !strings.Contains(screen, "Done: Create the user dora") {
		t.Fatalf("no result:\n%s", screen)
	}
}

func TestCreateConfirmationShowsTheCommandsWithoutTheSecret(t *testing.T) {
	m, f := systemModel(t, i18n.English, wideWidth, 40)
	send(m, seq(keys("a"), typed("dora"), keys("down"), typed(hiddenA), keys("down"), typed(hiddenA), keys("down", "right", "tab", "tab", "tab"))...)
	m.Update(tea.PasteMsg{Content: demoKey(t, 7)})
	send(m, "f2")
	screen := flat(m.render())
	for _, want := range []string{"useradd -m -d /home/dora -U -s /bin/bash -G sudo,qhost dora", "chpasswd < <password>", "chage -d 0 dora",
		"<ssh key SHA256:", "Admin and Docker rights allow practically everything"} {
		if !strings.Contains(screen, want) {
			t.Errorf("the confirmation lacks %q:\n%s", want, screen)
		}
	}
	noLeak(t, screen, hiddenA, demoKey(t, 7))
	send(m, "n")
	if len(f.reqs) != 0 {
		t.Fatalf("a cancelled change asked for %d commands", len(f.reqs))
	}
}

func TestCreateFormRefusesWhatTheRootActionWouldRefuse(t *testing.T) {
	m, _ := systemModel(t, i18n.English, wideWidth, 40)
	send(m, seq(keys("a"), typed("cleo"), keys("down"), typed(hiddenA), keys("down"), typed(hiddenB), keys("f2"))...)
	screen := m.render()
	if m.modal != nil || !strings.Contains(screen, "This user exists already.") || !strings.Contains(screen, "The two passwords differ.") {
		t.Fatalf("the form did not refuse:\n%s", screen)
	}
	noLeak(t, screen, hiddenA, hiddenB)
}

// The Admin switch sets and takes the administrator group and the group qhost together.
func TestAdminSwitchChangesBothGroups(t *testing.T) {
	m, f := systemModel(t, i18n.English, wideWidth, 40)
	toUser(t, m, "cleo")
	pickChangeRow(t, m, chgAdminOn)
	if screen := flat(m.render()); !strings.Contains(screen, "usermod -aG sudo,qhost cleo") {
		t.Fatalf("the confirmation lacks both groups:\n%s", screen)
	}
	send(m, "y")
	if got := f.lastArgs(t); !slices.Equal(got[len(got)-3:], []string{hostchange.ActionUserSetAdmin, "cleo", "on"}) {
		t.Fatalf("arguments %q", got)
	}
	send(m, "esc")
	if len(f.execs) != 1 || len(f.bgs) != 0 {
		t.Fatalf("a change without a secret runs once on the terminal: execs %d, runs %d", len(f.execs), len(f.bgs))
	}
	// ben is an administrator without qhost: taking it takes only the administrator group.
	toUser(t, m, "ben")
	pickChangeRow(t, m, chgAdminOff)
	if screen := flat(m.render()); !strings.Contains(screen, "gpasswd -d ben sudo") || strings.Contains(screen, "gpasswd -d ben qhost") {
		t.Fatalf("the confirmation of ben:\n%s", screen)
	}
}

func TestChangesOfRootAndSystemAccountsAskInRed(t *testing.T) {
	m, _ := systemModel(t, i18n.English, wideWidth, 40)
	toUser(t, m, "root")
	pickChangeRow(t, m, chgLock)
	if screen := m.render(); !strings.Contains(screen, "root is the account with all rights on this host.") {
		t.Fatalf("no risk for root:\n%s", screen)
	}
	send(m, "n")
	toUser(t, m, "daemon")
	pickChangeRow(t, m, chgDockerOn)
	if screen := m.render(); !strings.Contains(screen, "Packages or services may depend on this system account.") {
		t.Fatalf("no risk for a system account:\n%s", screen)
	}
}

func TestOwnAccountIsNotLockedOrDeleted(t *testing.T) {
	m, f := systemModel(t, i18n.English, wideWidth, 40)
	toUser(t, m, "ada")
	pickChangeRow(t, m, chgLock)
	if m.modal != nil || !strings.Contains(m.render(), "You cannot lock yourself.") {
		t.Fatalf("locking oneself was offered:\n%s", m.render())
	}
	send(m, "x")
	if m.set.form != nil || len(f.reqs) != 0 {
		t.Fatal("deleting oneself was offered")
	}
}

func TestDeleteKeepsTheHomeUnlessChosen(t *testing.T) {
	m, f := systemModel(t, i18n.English, wideWidth, 40)
	toUser(t, m, "cleo")
	send(m, "x", "f2")
	if screen := flat(m.render()); !strings.Contains(screen, "The home and its files stay.") {
		t.Fatalf("the delete question:\n%s", screen)
	}
	send(m, "y")
	if got := f.lastArgs(t); got[len(got)-1] != "cleo" {
		t.Fatalf("a delete that keeps the home: %q", got)
	}
	send(m, "esc")
	toUser(t, m, "cleo")
	send(m, "x", "right", "f2")
	if screen := flat(m.render()); !strings.Contains(screen, "This cannot be undone: the home /home/cleo") {
		t.Fatalf("the delete of the home:\n%s", screen)
	}
	send(m, "y")
	if got := f.lastArgs(t); got[len(got)-1] != "with-home" {
		t.Fatalf("a delete with the home: %q", got)
	}
}

func TestReportOfTheRootCommandIsShown(t *testing.T) {
	for _, tt := range []struct {
		name, report string
		want         []string
	}{
		{"refused", `{"journal_id":"","action":"user.lock","result":"refused","error":"nothing to change: cleo is locked already"}`,
			[]string{"Not changed: nothing to change: cleo is locked already", "A rule of the host refused"}},
		{"warning", `{"journal_id":"20261102T090000-0002","action":"user.lock","result":"applied","check":"passed","reversible":true,` +
			`"warning":"the change stands but has no entry in the action log: the host service is not running"}`,
			[]string{"Done: Lock cleo", "Warning: the change stands but has no entry in the action log"}},
		{"restored", `{"journal_id":"20261102T090000-0003","action":"user.lock","result":"restored","check":"failed","check_error":"still unlocked","error":"exit status 1"}`,
			[]string{"Failed, previous state restored", "Check failed: still unlocked"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m, f := systemModel(t, i18n.English, wideWidth, 40)
			f.report = tt.report
			toUser(t, m, "cleo")
			pickChangeRow(t, m, chgLock)
			send(m, "y")
			screen := flat(m.render())
			for _, want := range tt.want {
				if !strings.Contains(screen, want) {
					t.Errorf("the result lacks %q:\n%s", want, screen)
				}
			}
		})
	}
}

func TestRunWithoutReportIsExplained(t *testing.T) {
	m, f := systemModel(t, i18n.English, wideWidth, 40)
	f.report, f.runErr = "", exitError(t, 1)
	toUser(t, m, "cleo")
	pickChangeRow(t, m, chgUnlock)
	send(m, "y")
	if screen := m.render(); !strings.Contains(screen, "sudo did not run it") {
		t.Fatalf("no explanation:\n%s", screen)
	}
	m2, f2 := systemModel(t, i18n.English, wideWidth, 40)
	m2.root.run = func(HostRequest) (*exec.Cmd, error) { return nil, ErrNoSudo }
	toUser(t, m2, "cleo")
	pickChangeRow(t, m2, chgUnlock)
	send(m2, "y")
	if screen := m2.render(); !strings.Contains(screen, "sudo is not installed") || len(f2.execs) != 0 {
		t.Fatalf("no explanation without sudo:\n%s", screen)
	}
}

// exitError is the error of a real process that ended with code; the shell is no sudo and changes nothing.
func exitError(t *testing.T, code int) error {
	t.Helper()
	err := exec.Command("sh", "-c", "exit "+string(rune('0'+code))).Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("no exit error: %v", err)
	}
	return err
}

// Started as root, the interface needs no password check: a secret goes straight to the background run.
func TestRootInterfaceRunsWithoutSudo(t *testing.T) {
	m, f := systemModel(t, i18n.English, wideWidth, 40)
	f.root = true
	f.report = appliedReport(hostchange.ActionUserSetPassword)
	toUser(t, m, "cleo")
	pickChangeRow(t, m, chgSecret)
	send(m, seq(typed(hiddenA), keys("down"), typed(hiddenA), keys("f2", "y"))...)
	if len(f.execs) != 0 || len(f.bgs) != 1 || f.bgs[0].Args[0] != "qhost-fake" {
		t.Fatalf("as root: execs %d, runs %d", len(f.execs), len(f.bgs))
	}
	if string(f.stdins[0]) != hiddenA {
		t.Fatal("the password did not reach standard input")
	}
	if got := f.lastArgs(t); !slices.Equal(got[len(got)-3:], []string{hostchange.ActionUserSetPassword, "cleo", "must-change"}) {
		t.Fatalf("arguments %q", got)
	}
}

func TestCancelledQuestionOverwritesItsSecret(t *testing.T) {
	m, _ := systemModel(t, i18n.English, wideWidth, 40)
	secret := []byte(hiddenA)
	m.confirmHost(hostPlanned{ask: hostAsk{id: hostchange.ActionUserSetPassword, args: []string{"cleo"}, secret: secret, question: "q"},
		commands: []string{"chpasswd < <password>"}})
	send(m, "n")
	if bytes.ContainsFunc(secret, func(r rune) bool { return r != 0 }) {
		t.Fatal("the secret of a cancelled question was kept")
	}
}

// Without root the SSH keys are unknown: removing one reads them as root first, through the read command.
func TestRemoveKeyReadsTheKeysAsRootFirst(t *testing.T) {
	m, f := systemModel(t, i18n.English, wideWidth, 40)
	f.report = `{"name":"cleo","ssh_keys":[{"type":"ssh-ed25519","comment":"demo@example.test","fingerprint":"SHA256:` +
		strings.Repeat("A", 43) + `"}]}`
	toUser(t, m, "cleo")
	pickChangeRow(t, m, chgRemoveKey)
	if got := f.lastArgs(t); !slices.Equal(got, []string{"user", "--output", "json", "--", "cleo"}) {
		t.Fatalf("read arguments %q", got)
	}
	if m.set.form == nil || m.set.form.kind != formUserKeyRemove {
		t.Fatalf("no form after the read:\n%s", m.render())
	}
	f.report = appliedReport(hostchange.ActionUserRemoveKey)
	send(m, "f2", "y")
	if got := f.lastArgs(t); got[len(got)-1] != "SHA256:"+strings.Repeat("A", 43) {
		t.Fatalf("arguments %q", got)
	}
}

func TestAddGroupsTicksSeveral(t *testing.T) {
	m, f := systemModel(t, i18n.English, wideWidth, 40)
	toUser(t, m, "ben")
	pickChangeRow(t, m, chgAddGroups)
	send(m, "space", "right", "space", "f2")
	screen := m.render()
	if !strings.Contains(screen, "usermod -aG") || !strings.Contains(screen, "Packages or services may depend on a system group") {
		t.Fatalf("the question:\n%s", screen)
	}
	send(m, "y")
	if got := f.lastArgs(t); !slices.Contains(got, hostchange.ActionUserAddGroups) || len(got) != 9 {
		t.Fatalf("arguments %q", got)
	}
}

// A run without a prompt that finds no password held by sudo says so; what it wrote never shows the secret.
func TestSecretRunWithoutHeldPasswordIsExplained(t *testing.T) {
	for _, tt := range []struct{ stderr, want string }{
		{"sudo: a password is required", "sudo no longer holds your password"},
		{"error: cannot read " + hiddenA, "qhost answered without a report"},
	} {
		m, f := systemModel(t, i18n.English, wideWidth, 40)
		f.report, f.runErr, f.stderr = "", exitError(t, 1), tt.stderr
		toUser(t, m, "cleo")
		pickChangeRow(t, m, chgSecret)
		send(m, seq(typed(hiddenA), keys("down"), typed(hiddenA), keys("f2", "y"))...)
		screen := flat(m.render())
		noLeak(t, screen, hiddenA)
		if !strings.Contains(screen, tt.want) {
			t.Errorf("no explanation %q:\n%s", tt.want, screen)
		}
	}
}
