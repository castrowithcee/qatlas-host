package tui

import (
	"slices"
	"testing"

	"github.com/castrowithcee/qatlas-kit/i18n"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// windowEnginesBackend is kindsBackend on a host with two engines, the window proposal submitted to engine.
func windowEnginesBackend(engine string) *fakeBackend {
	b := kindsBackend()
	b.engines = []string{control.EngineSystem, aliceEngine}
	b.proposals[0].Engine = engine
	b.windowBy = map[string]control.WindowResult{
		control.EngineSystem: {Days: []string{"mon"}, Start: "03:00", Duration: "2h", Zone: "local", Origin: control.WindowOriginHost},
		aliceEngine:          {Days: []string{"mon"}, Start: "05:00", Duration: "2h", Zone: "local", Origin: control.WindowOriginOwn},
	}
	return b
}

func TestKindWindowOfTheSystemServiceChangesTheHostWindow(t *testing.T) {
	b := windowEnginesBackend(control.EngineSystem)
	m := proposalOpen(t, b, true, kindWindowRow)
	contains(t, m, "The maintenance window of the host would change:", "before: \"03:00\"")
	send(m, "A", "y")
	if len(b.hostWindows) != 1 || len(b.windows) != 0 {
		t.Fatalf("host windows %v, own windows %v", b.hostWindows, b.windows)
	}
}

func TestKindWindowOfARootlessInstanceChangesItsOwnWindow(t *testing.T) {
	b := windowEnginesBackend(aliceEngine)
	m := proposalOpen(t, b, true, kindWindowRow)
	contains(t, m, "The own maintenance window of this instance would change:", "before: \"05:00\"")
	send(m, "A", "y")
	if len(b.windows) != 1 || len(b.hostWindows) != 0 {
		t.Fatalf("host windows %v, own windows %v", b.hostWindows, b.windows)
	}
	if !slices.Contains(b.engineLog, "windowset:@"+aliceEngine) {
		t.Fatalf("not addressed to the instance: %v", b.engineLog)
	}
}

func TestKindWindowApproveWithoutRootNamesTheEngineOnlyForARootlessInstance(t *testing.T) {
	for engine, want := range map[string][]string{
		control.EngineSystem: approveArgsOf("p-0012"),
		aliceEngine:          append(approveArgsOf("p-0012"), "--engine", aliceEngine),
	} {
		m, f := sudoProposalOpen(t, i18n.English, windowEnginesBackend(engine), kindWindowRow, nil)
		send(m, "A")
		showsCommand(t, m, want)
		if got := shows(m.render(), "--engine"); got != (engine != control.EngineSystem) {
			t.Fatalf("engine %s: --engine shown = %v", engine, got)
		}
		_ = f
	}
}
