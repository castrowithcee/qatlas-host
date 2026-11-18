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

// A change shows the message that it needs root and leaves the interface usable.
func TestChangeWithoutRootSaysSo(t *testing.T) {
	b := noRootBackend()
	m := newActionModel(t, b)
	send(m, "d", "y")
	if screen := m.render(); !strings.Contains(screen, "this needs root; quit and run sudo qhost") {
		t.Fatalf("no message that the change needs root:\n%s", screen)
	}
	send(m, "esc", "esc")
	if screen := m.render(); !strings.Contains(screen, "demo-stack") {
		t.Fatalf("the list is gone after the refusal:\n%s", screen)
	}
}

// The update preview reads the changelog first; without root it ends there with the message instead of a
// question that could not be answered.
func TestUpdatePreviewWithoutRootEndsWithTheMessage(t *testing.T) {
	b := noRootBackend()
	m := newActionModel(t, b)
	send(m, "u")
	if m.modal != nil {
		t.Fatalf("the preview stays open without root:\n%s", m.render())
	}
	if !strings.Contains(m.render(), "this needs root; quit and run sudo qhost") {
		t.Errorf("no message that the update needs root:\n%s", m.render())
	}
	if len(b.actions()) != 0 {
		t.Errorf("calls = %v", b.actions())
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
