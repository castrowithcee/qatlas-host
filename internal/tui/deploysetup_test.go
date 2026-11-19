package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/castrowithcee/qatlas-kit/i18n"
	kittui "github.com/castrowithcee/qatlas-kit/tui"
	"github.com/castrowithcee/qatlas-kit/uiprefs"

	"github.com/castrowithcee/qatlas-host/internal/client"
)

// fakeDeploy is the host side of the setup; nothing of the real host is touched.
type fakeDeploy struct {
	installed bool
	commands  []string
	planErr   error
	runErr    error
	ran       int
}

func (f *fakeDeploy) Installed() bool { return f.installed }
func (f *fakeDeploy) Plan(context.Context) ([]string, error) {
	return f.commands, f.planErr
}
func (f *fakeDeploy) Run(context.Context) error {
	f.ran++
	if f.runErr == nil {
		f.installed = true
	}
	return f.runErr
}

func newFakeDeploy() *fakeDeploy {
	return &fakeDeploy{commands: []string{
		"groupadd --system qhost-deploy",
		"useradd --system --gid qhost-deploy --no-create-home --home-dir /var/lib/qhost-deploy --shell /usr/sbin/nologin qhost-deploy",
		"usermod --append --groups docker qhost-deploy",
		"systemctl enable --now qhost-deploy",
	}}
}

// notSetUpModel is the interface on a host whose deploy service does not exist, run as euid.
func notSetUpModel(t *testing.T, lang i18n.Lang, d *fakeDeploy, euid, width, rows int) (*Model, *fakeBackend) {
	t.Helper()
	b := demoBackend()
	b.statusErr = fmt.Errorf("dial: %w", client.ErrNotRunning)
	m, err := newModel(config{ctx: context.Background(), backend: b, deploy: d, lang: lang, theme: kittui.ThemeTerminal,
		profile: kittui.ProfileNone, charset: kittui.CharsetUnicode, loc: time.UTC,
		prefs: uiprefs.Prefs{Theme: kittui.ThemeTerminal, Start: string(areaStacks), Icons: uiprefs.IconsUnicode,
			Language: uiprefs.LanguageEnglish, MouseEnabled: true}})
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
		for _, want := range []string{"The deploy service is not set up yet", "G sets it up", "Setting up needs root: quit and run sudo qhost."} {
			if !strings.Contains(screen, want) {
				t.Errorf("area %s lacks %q:\n%s", area, want, screen)
			}
		}
		if strings.Contains(screen, "is not running") {
			t.Errorf("area %s shows a connection error:\n%s", area, screen)
		}
	}
}

func TestNotSetUpWithoutRootNeedsSudo(t *testing.T) {
	d := newFakeDeploy()
	m, _ := notSetUpModel(t, i18n.English, d, 1000, wideWidth, goldenRows)
	send(m, "G")
	if m.modal != nil || d.ran != 0 {
		t.Fatal("the setup was offered without root")
	}
	if screen := m.render(); !strings.Contains(screen, "needs root") {
		t.Fatalf("no hint about sudo:\n%s", screen)
	}
}

func TestSetupAsksWithCommandsAndRunsOnlyAfterYes(t *testing.T) {
	d := newFakeDeploy()
	m, b := notSetUpModel(t, i18n.English, d, 0, wideWidth, 24)
	send(m, "G")
	screen := m.render()
	for _, want := range []string{"Set up the deploy service now?", "$ groupadd --system qhost-deploy", "$ systemctl enable --now qhost-deploy"} {
		if !strings.Contains(screen, want) {
			t.Errorf("%q missing:\n%s", want, screen)
		}
	}
	send(m, "n")
	if d.ran != 0 || m.modal != nil {
		t.Fatal("cancel ran the setup or left the question open")
	}
	send(m, "G")
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
	d.runErr = errors.New("useradd: exit status 9")
	m, _ := notSetUpModel(t, i18n.English, d, 0, wideWidth, 24)
	send(m, "G")
	drain(m, press(m, "y"))
	if screen := m.render(); !strings.Contains(screen, "useradd: exit status 9") || !m.notSetUp {
		t.Fatalf("failure not shown:\n%s", screen)
	}
}

func TestStoppedDeployServiceIsNotNotSetUp(t *testing.T) {
	d := newFakeDeploy()
	d.installed = true
	m, _ := notSetUpModel(t, i18n.English, d, 0, wideWidth, goldenRows)
	if m.notSetUp || !strings.Contains(m.render(), "is not running") {
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
				send(m, "G")
				screen := m.render()
				checkScreen(t, screen, wh[0], wh[1])
				checkGolden(t, "deploy.setup.confirm."+name+"."+size+".golden", screen+"\n")
			})
		}
	}
}
