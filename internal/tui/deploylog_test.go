package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/castrowithcee/qatlas-kit/i18n"

	"github.com/castrowithcee/qatlas-host/internal/client"
	"github.com/castrowithcee/qatlas-host/internal/control"
)

// demoDeployLog is an invented action log of the deploy service of the system engine and of alice's engine.
func demoDeployLog(f *fakeBackend) {
	at := func(min int) time.Time { return time.Date(2026, 11, 2, 9, min, 0, 0, time.UTC) }
	ada, root, unknown := uint32(1000), uint32(0), uint32(4242)
	f.deployLogs = map[string][]control.DeployLogEntry{
		control.EngineSystem: {
			{Seq: 9, Time: at(5), ClientUID: &ada, Command: "stacks.deploy", Stack: "web", Result: control.HostLogResultSuccess, Origin: "tui"},
			{Seq: 8, Time: at(4), ClientUID: &unknown, Command: "stacks.update", Stack: "db", Result: control.HostLogResultFailure, ErrorCode: "busy", Origin: "mcp"},
			{Seq: 7, Time: at(3), ClientUID: &root, Command: "log.list", Result: control.HostLogResultSuccess, Origin: "cli"},
		},
		aliceEngine: {
			{Seq: 2, Time: at(2), ClientUID: &ada, Command: "stacks.pause", Stack: "alice-app", Result: control.HostLogResultSuccess, Origin: "cli"},
		},
	}
}

// deployLogPage opens the log page with the host service and the engines named, the log of the host service
// and the deploy services read from the demo backend.
func deployLogPage(t *testing.T, m *Model, engines ...string) *fakeBackend {
	t.Helper()
	b := m.backend.(*fakeBackend)
	demoDeployLog(b)
	m.snap.engines = engines
	m.hostLog = demoHostLog()
	toLogPage(t, m)
	return b
}

func TestGoldenSystemLogDeploy(t *testing.T) {
	goldenSystem(t, "system.log.deploy", func(t *testing.T, m *Model) {
		deployLogPage(t, m, control.EngineSystem, aliceEngine)
		send(m, "H")
	})
}

func TestGoldenSystemLogSwitch(t *testing.T) {
	goldenSystem(t, "system.log.switch", func(t *testing.T, m *Model) {
		deployLogPage(t, m, control.EngineSystem, aliceEngine)
		send(m, "H", "H", "H")
	})
}

func TestGoldenSystemLogDeployDown(t *testing.T) {
	goldenSystem(t, "system.log.deploy.down", func(t *testing.T, m *Model) {
		b := deployLogPage(t, m, control.EngineSystem, aliceEngine)
		b.errs = map[string]error{"deploylog:" + aliceEngine: client.ErrNotRunning}
		send(m, "H", "H")
	})
}

func TestLogSwitchesBetweenHostAndEveryInstance(t *testing.T) {
	m, _ := systemModel(t, i18n.English, wideWidth, 40)
	b := deployLogPage(t, m, control.EngineSystem, aliceEngine)
	for i, want := range []string{"Service: deploy, engine " + control.EngineSystem, "Service: deploy, engine " + aliceEngine, "Service: host"} {
		send(m, "H")
		if screen := m.render(); !strings.Contains(screen, want) {
			t.Fatalf("step %d lacks %q:\n%s", i, want, screen)
		}
	}
	if got := strings.Join(b.deployLogArgs, ","); got != control.EngineSystem+","+aliceEngine {
		t.Errorf("engines asked %q, want the system engine, then alice's, each once", got)
	}
	if screen := m.render(); !strings.Contains(screen, "folder.set-permissions") {
		t.Errorf("the host entries are not back:\n%s", screen)
	}
}

func TestLogDeployColumnsShowUserNameOrUID(t *testing.T) {
	m, _ := systemModel(t, i18n.English, wideWidth, 40)
	deployLogPage(t, m, control.EngineSystem)
	send(m, "H")
	screen := m.render()
	for _, want := range []string{"TIME", "USER", "FUNCTION", "STACK", "RESULT", "ORIGIN", "stacks.deploy", "web", "ada", "tui",
		"4242", "failure: busy", "mcp", "root"} {
		if !strings.Contains(screen, want) {
			t.Errorf("the deploy log lacks %q:\n%s", want, screen)
		}
	}
}

func TestLogFilterAndSaveWorkOnTheChosenService(t *testing.T) {
	home := saveInto(t)
	m, _ := systemModel(t, i18n.English, wideWidth, 40)
	deployLogPage(t, m, control.EngineSystem, aliceEngine)
	send(m, "H", "H")
	send(m, "/")
	send(m, typed("alice-app")...)
	if screen := m.render(); !strings.Contains(screen, "alice-app") || strings.Contains(screen, "stacks.deploy") {
		t.Errorf("the filter does not work on the entries of the chosen instance:\n%s", screen)
	}
	send(m, "enter", "W")
	if !strings.Contains(m.result, "Log saved to") {
		t.Fatalf("result %q, notice %q: the log was not saved", m.result, m.notice)
	}
	files, _ := filepath.Glob(filepath.Join(home, "*", "qhost-actionlog-*"))
	if len(files) == 0 {
		files, _ = filepath.Glob(filepath.Join(home, "qhost-actionlog-*"))
	}
	if len(files) != 1 {
		t.Fatalf("saved files %v, want one", files)
	}
	data, _ := os.ReadFile(files[0])
	if !strings.Contains(string(data), "alice-app") || strings.Contains(string(data), "stacks.deploy") || strings.Contains(string(data), "host.change") {
		t.Errorf("the file holds entries of another service:\n%s", data)
	}
}

func TestLogSaveOfTheHostServiceHoldsHostEntries(t *testing.T) {
	saveInto(t)
	var got string
	old := saveLogFile
	saveLogFile = func(name string, data []byte) (string, error) { got = string(data); return "/home/demo/" + name, nil }
	t.Cleanup(func() { saveLogFile = old })
	m, _ := systemModel(t, i18n.English, wideWidth, 40)
	deployLogPage(t, m, control.EngineSystem)
	send(m, "W")
	if !strings.Contains(got, "host.change") || strings.Contains(got, "stacks.deploy") {
		t.Errorf("saved %q", got)
	}
}

func TestLogReloadReadsTheChosenService(t *testing.T) {
	m, _ := systemModel(t, i18n.English, wideWidth, 40)
	b := deployLogPage(t, m, control.EngineSystem, aliceEngine)
	send(m, "H", "H", "r")
	if got := strings.Join(b.deployLogArgs, ","); got != control.EngineSystem+","+aliceEngine+","+aliceEngine {
		t.Errorf("engines asked %q", got)
	}
}

func TestLogDeployNoteForInstanceThatIsDown(t *testing.T) {
	for name, c := range map[string]struct {
		err  error
		want string
	}{
		"not running": {client.ErrNotRunning, "is not set up or not running"},
		"other":       {control.NewError(control.CodeBadRequest, "peer is gone"), "cannot be asked: peer is gone"},
	} {
		m, _ := systemModel(t, i18n.English, wideWidth, 40)
		b := deployLogPage(t, m, control.EngineSystem, aliceEngine)
		b.errs = map[string]error{"deploylog:" + aliceEngine: c.err}
		send(m, "H", "H")
		screen := flat(m.render())
		if !strings.Contains(screen, c.want) || m.failure != "" {
			t.Errorf("%s: want %q, failure %q:\n%s", name, c.want, m.failure, screen)
		}
		send(m, "H")
		if !strings.Contains(m.render(), "folder.set-permissions") {
			t.Errorf("%s: the host log is lost after a down instance", name)
		}
	}
}

func TestLogHasNoSwitchWithoutDeployService(t *testing.T) {
	m, _ := systemModel(t, i18n.English, wideWidth, 40)
	deployLogPage(t, m)
	send(m, "H")
	if screen := m.render(); strings.Contains(screen, "Service:") || m.sys.hlog.svc != "" {
		t.Errorf("a host without deploy service offers a choice:\n%s", screen)
	}
}
