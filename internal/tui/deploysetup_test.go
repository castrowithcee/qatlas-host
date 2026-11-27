package tui

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/castrowithcee/qatlas-kit/i18n"
	kittui "github.com/castrowithcee/qatlas-kit/tui"
	"github.com/castrowithcee/qatlas-kit/uiprefs"

	"github.com/castrowithcee/qatlas-host/internal/client"
)

// fakeDeploy is the host side of the setup; nothing of the real host is touched. Instances are planned by user:
// instanceCommands for a user without an entry in refusals, otherwise the refusal.
type fakeDeploy struct {
	installed bool
	commands  []string
	planErr   error
	runErr    error
	ran       int
	// ranFor holds the user of every run.
	ranFor           []string
	users            []string
	instanceCommands map[string][]string
	warnings         map[string][]string
	refusals         map[string]error
}

func (f *fakeDeploy) Installed() bool { return f.installed }
func (f *fakeDeploy) Users() []string { return f.users }
func (f *fakeDeploy) Plan(_ context.Context, user string) (SetupPlan, error) {
	if user == "" {
		return SetupPlan{Commands: f.commands}, f.planErr
	}
	if err := f.refusals[user]; err != nil {
		return SetupPlan{}, err
	}
	return SetupPlan{Commands: f.instanceCommands[user], Warnings: f.warnings[user]}, nil
}
func (f *fakeDeploy) Run(_ context.Context, user string) error {
	f.ran++
	f.ranFor = append(f.ranFor, user)
	if f.runErr == nil && user == "" {
		f.installed = true
	}
	return f.runErr
}

func newFakeDeploy() *fakeDeploy {
	return &fakeDeploy{commands: []string{
		"write /etc/tmpfiles.d/qhost-deploy.conf (mode 0644): d /run/qhost-deploy 2750 root qhost -",
		"systemd-tmpfiles --create /etc/tmpfiles.d/qhost-deploy.conf",
		"systemctl enable --now qhost-deploy",
	}}
}

// notSetUpModel is the interface on a host whose deploy service does not exist, run as euid.
func notSetUpModel(t *testing.T, lang i18n.Lang, d *fakeDeploy, euid, width, rows int) (*Model, *fakeBackend) {
	t.Helper()
	return deployModel(t, lang, d, euid, width, rows, false)
}

// deployModel is the interface with the host side d; with running the deploy service answers, otherwise it does
// not exist.
func deployModel(t *testing.T, lang i18n.Lang, d *fakeDeploy, euid, width, rows int, running bool) (*Model, *fakeBackend) {
	t.Helper()
	b := demoBackend()
	if !running {
		b.statusErr = fmt.Errorf("dial: %w", client.ErrNotRunning)
	}
	m, err := newModel(config{ctx: context.Background(), backend: b, deploy: d, lang: lang, theme: kittui.ThemeTerminal,
		profile: kittui.ProfileNone, charset: kittui.CharsetUnicode, loc: time.UTC,
		prefs: uiprefs.Prefs{Theme: kittui.ThemeTerminal, Start: string(areaStacks), Icons: uiprefs.IconsUnicode,
			Language: uiprefs.LanguageEnglish, MouseEnabled: true}, accounts: testAccounts(false)})
	if err != nil {
		t.Fatal(err)
	}
	m.euid = func() int { return euid }
	m.Update(tea.WindowSizeMsg{Width: width, Height: rows})
	timersOf[m] = &timers{}
	m.after = timersOf[m].start
	load(t, m)
	m.setZone(kittui.ZoneRows)
	return m, b
}

func TestNotSetUpShowsHowToSetUpInEveryDockerArea(t *testing.T) {
	for _, area := range []string{"1", "3", "5"} {
		m, _ := notSetUpModel(t, i18n.English, newFakeDeploy(), 1000, wideWidth, goldenRows)
		send(m, area)
		screen := m.render()
		for _, want := range []string{"The deploy service is not set up yet", "G sets it up"} {
			if !strings.Contains(screen, want) {
				t.Errorf("area %s lacks %q:\n%s", area, want, screen)
			}
		}
		if strings.Contains(screen, "is not running") {
			t.Errorf("area %s shows a connection error:\n%s", area, screen)
		}
	}
}

func TestSetupAsksWithCommandsAndRunsOnlyAfterYes(t *testing.T) {
	d := newFakeDeploy()
	m, b := notSetUpModel(t, i18n.English, d, 0, wideWidth, 24)
	send(m, "G", "y")
	screen := m.render()
	for _, want := range []string{"Set up the deploy service now?", "$ systemd-tmpfiles --create", "$ systemctl enable --now qhost-deploy"} {
		if !strings.Contains(screen, want) {
			t.Errorf("%q missing:\n%s", want, screen)
		}
	}
	send(m, "n")
	if d.ran != 0 || m.modal != nil {
		t.Fatal("cancel ran the setup or left the question open")
	}
	send(m, "G", "y")
	b.statusErr = nil
	drain(m, press(m, "y"))
	if d.ran != 1 {
		t.Fatalf("setup ran %d times", d.ran)
	}
	if m.notSetUp {
		t.Fatal("the interface still says not set up")
	}
	load(t, m)
	if screen := m.render(); !strings.Contains(screen, "demo-stack") {
		t.Fatalf("no reload after the setup:\n%s", screen)
	}
}

func TestSetupFailureIsShown(t *testing.T) {
	d := newFakeDeploy()
	d.runErr = errors.New("systemd-tmpfiles: exit status 1")
	m, _ := notSetUpModel(t, i18n.English, d, 0, wideWidth, 24)
	send(m, "G", "y")
	drain(m, press(m, "y"))
	if screen := m.render(); !strings.Contains(screen, "systemd-tmpfiles: exit status 1") || !m.notSetUp {
		t.Fatalf("failure not shown:\n%s", screen)
	}
}

// A set up service that does not answer shows its cause, not the setup (see deployrepair_test.go).
func TestStoppedDeployServiceIsNotNotSetUp(t *testing.T) {
	d := newFakeDeploy()
	d.installed = true
	m, _ := notSetUpModel(t, i18n.English, d, 0, wideWidth, goldenRows)
	if m.notSetUp || m.down != downNotRunning || strings.Contains(m.render(), "is not set up yet") {
		t.Fatalf("a set up but stopped service shows the setup:\n%s", m.render())
	}
}

// The goldens of the setup are drawn in the two terminal sizes the interface is checked in.
var setupSizes = map[string][2]int{"80x24": {80, 24}, "120x40": {120, 40}}

func TestGoldenNotSetUp(t *testing.T) {
	for name, lang := range goldenLangs {
		for size, wh := range setupSizes {
			t.Run(name+"."+size, func(t *testing.T) {
				m, _ := notSetUpModel(t, lang, newFakeDeploy(), 1000, wh[0], wh[1])
				screen := m.render()
				checkScreen(t, screen, wh[0], wh[1])
				checkGolden(t, "deploy.notsetup."+name+"."+size+".golden", screen+"\n")
			})
		}
	}
}

func TestGoldenSetupConfirmation(t *testing.T) {
	for name, lang := range goldenLangs {
		for size, wh := range setupSizes {
			t.Run(name+"."+size, func(t *testing.T) {
				m, _ := notSetUpModel(t, lang, newFakeDeploy(), 0, wh[0], wh[1])
				send(m, "G", "y")
				screen := m.render()
				checkScreen(t, screen, wh[0], wh[1])
				checkGolden(t, "deploy.setup.confirm."+name+"."+size+".golden", screen+"\n")
			})
		}
	}
}

func TestGoldenSetupSudoConfirmation(t *testing.T) {
	for name, lang := range goldenLangs {
		for size, wh := range setupSizes {
			t.Run(name+"."+size, func(t *testing.T) {
				m, _ := notSetUpModel(t, lang, newFakeDeploy(), 1000, wh[0], wh[1])
				send(m, "G", "y")
				screen := m.render()
				checkScreen(t, screen, wh[0], wh[1])
				checkGolden(t, "deploy.setup.sudo."+name+"."+size+".golden", screen+"\n")
			})
		}
	}
}

// instanceDeploy is a host with the system service set up and rootless engines for demo_ops and bob. demo_ops is an
// administrator, bob is in the group docker and is refused.
func instanceDeploy() *fakeDeploy {
	d := newFakeDeploy()
	d.installed = true
	d.users = []string{"demo_ops", "bob"}
	d.instanceCommands = map[string][]string{"demo_ops": {
		"loginctl enable-linger demo_ops",
		"write /etc/tmpfiles.d/qhost-deploy-demo_ops.conf (mode 0644)",
		"systemctl enable --now " + instanceUnit("demo_ops"),
	}}
	d.warnings = map[string][]string{"demo_ops": {"demo_ops is an administrator: the damage of a taken-over instance is not limited."}}
	d.refusals = map[string]error{"bob": errors.New("the user bob is in the group docker; remove bob from the group first")}
	return d
}

// instanceUnit is the unit of the instance of user.
func instanceUnit(user string) string { return "qhost-deploy" + "@" + user + ".service" }

func instanceModel(t *testing.T, lang i18n.Lang, d *fakeDeploy, euid, width, rows int) (*Model, *fakeBackend) {
	t.Helper()
	m, b := deployModel(t, lang, d, euid, width, rows, true)
	return m, b
}

func TestInstanceSetupIsOfferedInStacksOfARunningHost(t *testing.T) {
	m, _ := instanceModel(t, i18n.English, instanceDeploy(), 0, wideWidth, goldenRows)
	if screen := m.render(); !strings.Contains(screen, "set up deploy service") {
		t.Fatalf("no hint for the setup:\n%s", screen)
	}
	send(m, "G")
	contains(t, m, "Which deploy service do you want to set up?", "system engine", "rootless engine")
}

func TestInstanceSetupListsUsersAndAsksWithWarningAndSteps(t *testing.T) {
	d := instanceDeploy()
	m, b := instanceModel(t, i18n.English, d, 0, wideWidth, 30)
	send(m, "G", "r")
	contains(t, m, "with an engine: demo_ops, bob")
	send(m, "d", "e", "m", "o", "_", "o", "p", "s", "enter")
	contains(t, m, "Set up the deploy service for the", "! demo_ops is an",
		"$ loginctl enable-linger demo_ops", "$ systemctl enable --now "+instanceUnit("demo_ops"), "These steps run as root:")
	if d.ran != 0 {
		t.Fatal("the setup ran before the answer")
	}
	send(m, "n")
	if d.ran != 0 || m.modal != nil {
		t.Fatal("cancel ran the setup or left the question open")
	}
	_ = b
	send(m, "G", "r", "d", "e", "m", "o", "_", "o", "p", "s", "enter")
	drain(m, press(m, "y"))
	if d.ran != 1 || d.ranFor[0] != "demo_ops" {
		t.Fatalf("setup ran %d times for %v", d.ran, d.ranFor)
	}
	contains(t, m, "deploy service for the rootless engine of demo_ops is set up and started")
}

func TestInstanceSetupAcceptsATypedNameOutsideTheList(t *testing.T) {
	d := instanceDeploy()
	d.users = nil
	d.instanceCommands["carol"] = []string{"loginctl enable-linger carol"}
	m, _ := instanceModel(t, i18n.English, d, 0, wideWidth, 30)
	send(m, "G", "r")
	contains(t, m, "no user with an engine found")
	send(m, "c", "a", "r", "o", "l", "enter")
	contains(t, m, "rootless engine of carol", "$ loginctl enable-linger carol")
}

func TestInstanceSetupShowsRefusalWithReason(t *testing.T) {
	d := instanceDeploy()
	m, _ := instanceModel(t, i18n.English, d, 0, wideWidth, 30)
	send(m, "G", "r", "b", "o", "b", "enter")
	if m.modal != nil || d.ran != 0 {
		t.Fatal("a refused user got a question")
	}
	contains(t, m, "the user bob is in the group docker")
}

func TestRepeatedInstanceSetupSaysThereIsNothingToDo(t *testing.T) {
	d := instanceDeploy()
	d.instanceCommands["demo_ops"] = nil
	m, _ := instanceModel(t, i18n.English, d, 0, wideWidth, 30)
	send(m, "G", "r", "d", "e", "m", "o", "_", "o", "p", "s", "enter")
	if m.modal != nil || d.ran != 0 {
		t.Fatal("a finished setup asked again")
	}
	contains(t, m, "nothing to do")
}

func TestSystemSetupOfARunningHostSaysThereIsNothingToDo(t *testing.T) {
	d := instanceDeploy()
	d.commands = nil
	m, _ := instanceModel(t, i18n.English, d, 0, wideWidth, 30)
	send(m, "G", "y")
	contains(t, m, "nothing to do")
}

func TestInstanceSetupOnAHostWithoutServiceKeepsTheSetupScreen(t *testing.T) {
	d := instanceDeploy()
	d.installed = false
	m, _ := notSetUpModel(t, i18n.English, d, 0, wideWidth, 30)
	send(m, "G", "r", "d", "e", "m", "o", "_", "o", "p", "s", "enter")
	drain(m, press(m, "y"))
	if !m.notSetUp {
		t.Fatal("an instance of a rootless engine marked the service of the system engine as set up")
	}
}

func TestGoldenInstanceSetup(t *testing.T) {
	steps := map[string][]string{
		"choose": {"G"},
		"user":   {"G", "r"},
		"ask":    {"G", "r", "d", "e", "m", "o", "_", "o", "p", "s", "enter"},
	}
	for step, keys := range steps {
		for name, lang := range goldenLangs {
			for size, wh := range setupSizes {
				t.Run(step+"."+name+"."+size, func(t *testing.T) {
					m, _ := instanceModel(t, lang, instanceDeploy(), 0, wh[0], wh[1])
					send(m, keys...)
					screen := m.render()
					checkScreen(t, screen, wh[0], wh[1])
					checkGolden(t, "deploy.instance."+step+"."+name+"."+size+".golden", screen+"\n")
				})
			}
		}
	}
}

const setupJSON = `{"set_up":true,"changed":true,"steps":4}`

func TestSetupWithoutRootRunsTheCommandThroughSudo(t *testing.T) {
	d := newFakeDeploy()
	m, b := notSetUpModel(t, i18n.English, d, 1000, wideWidth, 30)
	f := &fakeRoot{report: setupJSON}
	m.root = f.hook()
	send(m, "G", "y")
	contains(t, m, "Set up the deploy service now?", "$ systemd-tmpfiles --create",
		"$ sudo qhost deploy-service setup", "--confirm --yes --output")
	send(m, "n")
	if len(f.reqs) != 0 || d.ran != 0 || m.modal != nil {
		t.Fatalf("cancel ran something: %+v", f.reqs)
	}
	send(m, "G", "y")
	b.statusErr = nil
	send(m, "y")
	want := []string{"deploy-service", "setup", "--confirm", "--yes", "--output", "json"}
	if len(f.reqs) != 1 || !slices.Equal(f.reqs[0].Args, want) {
		t.Fatalf("requests %+v, want arguments %v", f.reqs, want)
	}
	if d.ran != 0 || m.notSetUp {
		t.Fatalf("the interface ran the setup itself (%d) or still says not set up", d.ran)
	}
}

func TestInstanceSetupWithoutRootPassesTheUser(t *testing.T) {
	d := instanceDeploy()
	m, _ := instanceModel(t, i18n.English, d, 1000, wideWidth, 30)
	f := &fakeRoot{report: setupJSON}
	m.root = f.hook()
	send(m, "G", "r", "d", "e", "m", "o", "_", "o", "p", "s", "enter")
	contains(t, m, "$ sudo qhost deploy-service setup", "--rootless demo_ops", "--confirm --yes --output")
	send(m, "y")
	want := []string{"deploy-service", "setup", "--rootless", "demo_ops", "--confirm", "--yes", "--output", "json"}
	if len(f.reqs) != 1 || !slices.Equal(f.reqs[0].Args, want) || d.ran != 0 {
		t.Fatalf("requests %+v, ran %d", f.reqs, d.ran)
	}
	contains(t, m, "rootless engine of demo_ops is set up and started")
}
