package tui

import (
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-kit/i18n"
)

// noRootBackend answers like the service does for an administrator without root: every read works, and the
// calls that change something, the changelog among them, are refused as not-root.
func noRootBackend() *fakeBackend {
	b := demoBackend()
	b.errs["changelog:demo-stack"] = control.ErrNotRoot()
	b.errs["deploy:demo-stack"] = control.ErrNotRoot()
	return b
}

// The stack detail reads on its own; the changelog is the only part that needs root, and says so while the
// others show their data.
func TestDetailWithoutRootShowsDataAndSaysTheChangelogNeedsRoot(t *testing.T) {
	m := newTestModel(t, i18n.English, noRootBackend(), wideWidth, 47)
	send(m, "enter")
	screen := m.render()
	for _, want := range []string{"this needs root; quit and run sudo qhost", "20261030T140100Z-0123abcd", "12.0 MiB",
		"1.4.2 -> 1.5.0"} {
		if !strings.Contains(screen, want) {
			t.Errorf("%q missing:\n%s", want, screen)
		}
	}
}

func TestGoldenDetailWithoutRoot(t *testing.T) {
	goldenScreens(t, "detail.noroot", detailRows, noRootBackend, "enter")
}

// A change of the Stacks area, which the service refuses without root, is handed to sudo instead of the message.
func TestChangeWithoutRootIsHandedToSudo(t *testing.T) {
	b := noRootBackend()
	m, f := sudoStackModel(t, i18n.English, b, wideWidth, 60)
	f.report = reportDeploy
	send(m, "d", "y")
	if screen := m.render(); strings.Contains(screen, "this needs root; quit and run sudo qhost") || len(b.actions()) != 0 {
		t.Fatalf("the change was refused instead of handed to sudo:\n%s", screen)
	}
	if len(f.execs) != 1 || !strings.Contains(strings.Join(f.execs[0].Args, " "), "sudo-fake qhost-fake stack deploy") {
		t.Fatalf("sudo was not handed the command: %v", f.execs)
	}
	send(m, "esc", "esc")
	if screen := m.render(); !strings.Contains(screen, "demo-stack") {
		t.Fatalf("the list is gone after the change:\n%s", screen)
	}
}

// The update preview reads the changelog through sudo without root, after the question that names GitHub and the
// command, instead of ending with the message.
func TestUpdatePreviewWithoutRootIsHandedToSudo(t *testing.T) {
	b := noRootBackend()
	m, f := sudoStackModel(t, i18n.English, b, wideWidth, 60)
	f.report = reportChangelog
	send(m, "u")
	if m.modal == nil || !strings.Contains(flat(m.render()), "This asks GitHub") {
		t.Fatalf("no question with the sentence on GitHub:\n%s", m.render())
	}
	if len(b.actions()) != 0 || len(f.reqs) != 0 {
		t.Fatalf("calls = %v, sudo = %v", b.actions(), f.reqs)
	}
	send(m, "y")
	if len(f.execs) != 1 || !strings.Contains(flat(m.render()), "Spring release") {
		t.Fatalf("no preview after the command:\n%s", m.render())
	}
}

// A container action, which the service refuses without root, is handed to sudo instead of the message.
func TestContainerActionWithoutRootIsHandedToSudo(t *testing.T) {
	b := containersBackend()
	b.errs["ctrestart:0123456789ab"] = control.ErrNotRoot()
	m, f := sudoCtModel(t, b, ctReport("demo-stack-app-1"))
	selectContainer(t, m, "demo-stack-app-1")
	send(m, "Z", "y")
	screen := m.render()
	if strings.Contains(screen, "this needs root; quit and run sudo qhost") || len(b.actions()) != 0 {
		t.Fatalf("the action was refused instead of handed to sudo:\n%s", screen)
	}
	if len(f.execs) != 1 || !strings.Contains(strings.Join(f.execs[0].Args, " "), "sudo-fake qhost-fake container restart") {
		t.Fatalf("sudo was not handed the command: %v", f.execs)
	}
	if !strings.Contains(screen, "1 ok, 0 failed") {
		t.Errorf("no result:\n%s", screen)
	}
}

func TestAccessErrorsAreSaidApart(t *testing.T) {
	m := newTestModel(t, i18n.English, demoBackend(), wideWidth, 20)
	if got := m.serviceText(control.ErrNoAccess()); !strings.Contains(got, "reserved for administrators (sudo)") {
		t.Errorf("no-access: %q", got)
	}
	if got := m.serviceText(control.ErrNotRoot()); got != "this needs root; quit and run sudo qhost" {
		t.Errorf("not-root: %q", got)
	}
}
