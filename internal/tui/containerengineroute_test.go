package tui

import (
	"slices"
	"testing"

	"github.com/castrowithcee/qatlas-kit/i18n"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// Both engines have a container called example-cache; the action, the detail read before it and the log go to the
// engine of the marked row and to no other.
func TestContainerActionAndLogGoToTheEngineOfTheMarkedRow(t *testing.T) {
	for _, tt := range []struct{ engine, id string }{
		{ctAlice, "a11ce0000001"}, {control.EngineSystem, "feedfacecafe"},
	} {
		t.Run(tt.engine, func(t *testing.T) {
			b := twoEnginesContainers()
			b.ctLogLines = []control.ContainerLogLine{{Source: "example-cache", Stream: "stdout", Text: "hello"}}
			m := ctModel(t, b)
			selectIn(t, m, tt.engine, "example-cache")
			send(m, "Z", "y")
			if want := "ctrestart:" + tt.id + "@" + tt.engine; !slices.Contains(b.engineLog, want) || len(b.actions()) != 1 {
				t.Fatalf("engine log %v, actions %v, want %s", b.engineLog, b.actions(), want)
			}
			send(m, "enter") // closes the summary
			selectIn(t, m, tt.engine, "example-cache")
			send(m, "o")
			if len(b.ctLogEngines) == 0 || b.ctLogEngines[0] != tt.engine || b.ctLogArgs[0].Name != "example-cache" {
				t.Fatalf("log engines %v, args %+v", b.ctLogEngines, b.ctLogArgs)
			}
		})
	}
}

// The detail that is open names its engine, so an action from it reaches that engine too.
func TestContainerActionFromTheDetailGoesToItsEngine(t *testing.T) {
	b := twoEnginesContainers()
	m := ctModel(t, b)
	selectIn(t, m, ctAlice, "example-cache")
	send(m, "enter", "X", "y")
	if want := "ctstop:a11ce0000001@" + ctAlice; !slices.Contains(b.engineLog, want) {
		t.Fatalf("engine log %v, want %s", b.engineLog, want)
	}
}

// One engine runs as before: no call names an engine.
func TestContainerActionOnOneEngineNamesNone(t *testing.T) {
	b := containersBackend()
	m := ctModel(t, b)
	selectContainer(t, m, "demo-stack-app-1")
	send(m, "Z", "y")
	if want := "ctrestart:0123456789ab@"; !slices.Contains(b.engineLog, want) {
		t.Fatalf("engine log %v, want %s", b.engineLog, want)
	}
}

// Without root the command gets --engine, before the end of the flags, and the service is not called.
func TestContainerActionWithoutRootNamesTheEngine(t *testing.T) {
	b := twoEnginesContainers()
	m, f := sudoCtModel(t, b, ctReport("example-cache"))
	selectIn(t, m, ctAlice, "example-cache")
	send(m, "Z", "y")
	want := []string{"container", "restart", "--yes", "--output", "json", "--engine", ctAlice, "--", "a11ce0000001"}
	if len(f.reqs) != 1 || !slices.Equal(f.reqs[0].Args, want) || len(b.actions()) != 0 {
		t.Fatalf("requests %+v, want %q", f.reqs, want)
	}
}

// One engine hands over the command as before.
func TestContainerActionWithoutRootOnOneEngineNamesNone(t *testing.T) {
	m, f := sudoCtModel(t, containersBackend(), ctReport("demo-stack-app-1"))
	selectContainer(t, m, "demo-stack-app-1")
	send(m, "Z", "y")
	if len(f.reqs) != 1 || slices.Contains(f.reqs[0].Args, "--engine") {
		t.Fatalf("requests %+v", f.reqs)
	}
}

// A project taken over from the Containers area starts with the engine it runs in.
func TestAdoptPreselectsTheEngineOfTheProject(t *testing.T) {
	b := adoptBackend(localResult(foreignStack()))
	b.engines = []string{control.EngineSystem, ctAlice}
	for i := range b.containers.Groups {
		g := &b.containers.Groups[i]
		g.Engine = control.EngineSystem
		if g.Name == "legacy-blog" {
			g.Engine = ctAlice
		}
	}
	m := newTestModel(t, i18n.English, b, wideWidth, 40)
	send(m, "5")
	selectIn(t, m, ctAlice, "legacy-blog-db-1")
	send(m, "a")
	if got := engineChoiceOf(t, m); got != ctAlice {
		t.Fatalf("preselected %q", got)
	}
}
