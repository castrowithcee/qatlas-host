package tui

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-kit/i18n"
	"github.com/castrowithcee/qatlas-kit/localipc"
	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/client"
)

var downCauses = map[string]error{
	"notrunning": fmt.Errorf("dial: %w", client.ErrNotRunning),
	"refused":    fmt.Errorf("dial: %w", localipc.ErrRefused),
}

// downModel is the interface on a host whose deploy service is set up and answered until it stopped answering for
// the reason cause; the refresh that finds it has run, with the question to the setup that follows it.
func downModel(t *testing.T, lang i18n.Lang, d *fakeDeploy, euid, width, rows int, cause error) (*Model, *fakeBackend) {
	t.Helper()
	d.installed = true
	m, b := deployModel(t, lang, d, euid, width, rows, true)
	b.statusErr = cause
	drain(m, m.reload())
	return m, b
}

func TestUnreachableServiceShowsTheCauseInEveryDockerArea(t *testing.T) {
	want := map[string]map[i18n.Lang][]string{
		"notrunning": {
			i18n.English: {"is set up, but it is not running", "G repairs it", "journalctl -u qhost-deploy"},
			i18n.German:  {"ist eingerichtet, läuft aber nicht", "G repariert ihn", "journalctl -u qhost-deploy"},
		},
		"refused": {
			i18n.English: {"refused this connection", "G repairs it", "System, Log"},
			i18n.German:  {"diese Verbindung abgelehnt", "G repariert ihn", "System, Log"},
		},
	}
	for name, cause := range downCauses {
		for lang, texts := range want[name] {
			for _, area := range []string{"1", "2", "3", "5", "6"} {
				m, _ := downModel(t, lang, newFakeDeploy(), 1000, wideWidth, goldenRows, cause)
				send(m, area)
				containsFlat(t, m, texts...)
				if strings.Contains(m.render(), "cannot talk to the service") {
					t.Errorf("%s area %s shows the raw error:\n%s", name, area, m.render())
				}
			}
		}
	}
}

func TestUnreachableServiceKeepsSettingsAndSystemUsable(t *testing.T) {
	m, _ := downModel(t, i18n.English, newFakeDeploy(), 1000, wideWidth, 40, downCauses["refused"])
	send(m, "7")
	if screen := m.render(); strings.Contains(screen, "G repairs it") || !strings.Contains(screen, "cleo") {
		t.Fatalf("the System area is hidden by the cause:\n%s", screen)
	}
	send(m, "4")
	if strings.Contains(m.render(), "G repairs it") {
		t.Fatalf("the Settings area is hidden by the cause:\n%s", m.render())
	}
}

// Failures that say nothing about the service not answering stay errors of the message line.
func TestOtherFailuresAreNoUnreachableService(t *testing.T) {
	m, _ := downModel(t, i18n.English, newFakeDeploy(), 1000, wideWidth, goldenRows, errors.New("boom"))
	if m.down != downNone || strings.Contains(m.render(), "G repairs it") {
		t.Fatalf("an unrelated failure shows the repair:\n%s", m.render())
	}
}

// Without any deploy service the cause is the setup, as before.
func TestRefusedChannelWithoutAServiceIsNoRepair(t *testing.T) {
	m, _ := deployModel(t, i18n.English, newFakeDeploy(), 1000, wideWidth, goldenRows, true)
	m.deploy.(*fakeDeploy).installed = false
	m.backend.(*fakeBackend).statusErr = downCauses["refused"]
	drain(m, m.reload())
	if m.down != downNone || m.setupShown() {
		t.Fatalf("a host without a service offers the repair:\n%s", m.render())
	}
}

func TestRepairAsksWithTheCommandsAndRunsOnlyAfterYes(t *testing.T) {
	d := newFakeDeploy()
	m, b := downModel(t, i18n.English, d, 0, wideWidth, 24, downCauses["refused"])
	send(m, "G")
	contains(t, m, "Repair the deploy service now?", "$ systemd-tmpfiles --create", "$ systemctl enable --now qhost-deploy")
	send(m, "n")
	if d.ran != 0 || m.modal != nil {
		t.Fatal("cancel ran the repair or left the question open")
	}
	send(m, "G")
	b.statusErr = nil
	drain(m, press(m, "y"))
	if d.ran != 1 || len(d.ranFor) != 1 || d.ranFor[0] != "" {
		t.Fatalf("the repair ran %d times for %v, want once for the system engine", d.ran, d.ranFor)
	}
	if m.down != downNone {
		t.Fatal("the interface still says the service does not answer")
	}
	load(t, m)
	contains(t, m, "demo-stack")
}

func TestRepairWithoutRootRunsTheSetupCommandThroughSudo(t *testing.T) {
	d := newFakeDeploy()
	m, b := downModel(t, i18n.English, d, 1000, wideWidth, 30, downCauses["notrunning"])
	f := &fakeRoot{report: setupJSON}
	m.root = f.hook()
	send(m, "G")
	contains(t, m, "Repair the deploy service now?", "$ sudo qhost deploy-service setup")
	b.statusErr = nil
	send(m, "y")
	want := []string{"deploy-service", "setup", "--confirm", "--yes", "--output", "json"}
	if len(f.reqs) != 1 || strings.Join(f.reqs[0].Args, " ") != strings.Join(want, " ") || d.ran != 0 {
		t.Fatalf("requests %+v, ran %d", f.reqs, d.ran)
	}
	if m.down != downNone {
		t.Fatal("the interface still says the service does not answer")
	}
}

func TestRepairWithoutStepsSaysSoAndNamesTheServiceLog(t *testing.T) {
	d := newFakeDeploy()
	d.commands = nil
	for lang, want := range map[i18n.Lang][]string{
		i18n.English: {"no step left to repair", "journalctl -u qhost-deploy"},
		i18n.German:  {"keinen Schritt zum Reparieren", "journalctl -u qhost-deploy"},
	} {
		m, _ := downModel(t, lang, d, 0, wideWidth, goldenRows, downCauses["refused"])
		containsFlat(t, m, want...)
		if strings.Contains(m.render(), "repairs it") || strings.Contains(m.render(), "repariert ihn") {
			t.Fatalf("an empty repair is offered:\n%s", m.render())
		}
		if slices.ContainsFunc(m.footer(), func(h kittui.Hint) bool { return h.Action == actSetupDeploy }) {
			t.Fatal("the footer offers G for an empty repair")
		}
		send(m, "G")
		if m.modal != nil || d.ran != 0 {
			t.Fatal("G opened a question for an empty repair")
		}
		containsFlat(t, m, map[i18n.Lang]string{i18n.English: "nothing to repair", i18n.German: "nichts zu reparieren"}[lang])
	}
}

func TestGoldenUnreachableService(t *testing.T) {
	for cause, err := range downCauses {
		for name, lang := range goldenLangs {
			for size, wh := range setupSizes {
				t.Run(cause+"."+name+"."+size, func(t *testing.T) {
					m, _ := downModel(t, lang, newFakeDeploy(), 1000, wh[0], wh[1], err)
					screen := m.render()
					checkScreen(t, screen, wh[0], wh[1])
					checkGolden(t, "deploy.down."+cause+"."+name+"."+size+".golden", screen+"\n")
				})
			}
		}
	}
}

func TestGoldenRepairConfirmation(t *testing.T) {
	for name, lang := range goldenLangs {
		for size, wh := range setupSizes {
			t.Run(name+"."+size, func(t *testing.T) {
				m, _ := downModel(t, lang, newFakeDeploy(), 0, wh[0], wh[1], downCauses["refused"])
				send(m, "G")
				screen := m.render()
				checkScreen(t, screen, wh[0], wh[1])
				checkGolden(t, "deploy.repair.confirm."+name+"."+size+".golden", screen+"\n")
			})
		}
	}
}
