package tui

import (
	"testing"

	"github.com/castrowithcee/qatlas-kit/i18n"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

const (
	ctAlice = "rootless:alice"
	ctCarol = "rootless:carol"
)

// ctMark marks the groups with the engine they run in.
func ctMark(engine string, readOnly bool, groups ...control.ContainerGroup) []control.ContainerGroup {
	for i := range groups {
		groups[i].Engine, groups[i].ReadOnly = engine, readOnly
	}
	return groups
}

// twoEnginesContainers is the container list a routing backend delivers for the system engine and the engine of
// alice. Both have a container called example-cache, and the details tell them apart by the image.
func twoEnginesContainers() *fakeBackend {
	b := containersBackend()
	b.engines = []string{control.EngineSystem, ctAlice}
	system := ctMark(control.EngineSystem, false, b.containers.Groups...)
	photo := summary("photo-album-web-1", "example.invalid/photo:3", control.ContainerOK, "running", 600, 0)
	cache := summary("example-cache", "example.invalid/alice-cache:8", control.ContainerOK, "running", 30, 0)
	b.containers.Groups = append(system, ctMark(ctAlice, false,
		control.ContainerGroup{Kind: control.ContainerKindOwn, Name: "photo-album", State: control.ContainerOK, Containers: []control.ContainerSummary{photo}},
		control.ContainerGroup{Kind: control.ContainerKindSingle, Name: "example-cache", State: control.ContainerOK, Containers: []control.ContainerSummary{cache}},
	)...)
	b.containerDetails["example-cache@"+control.EngineSystem] = b.containerDetails["example-cache"]
	b.containerDetails["example-cache@"+ctAlice] = control.ContainerDetail{ContainerSummary: cache, ID: "a11ce0000001", Kind: control.ContainerKindSingle}
	return b
}

// withUnattended adds an engine without a deploy service, as root reads it.
func withUnattended(b *fakeBackend) *fakeBackend {
	web := summary("old-shop-web-1", "example.invalid/shop:1", control.ContainerStopped, "exited", 0, 0)
	ghost := summary("ghost", "example.invalid/ghost:1", control.ContainerOK, "running", 20, 0)
	b.containers.Groups = append(b.containers.Groups, ctMark(ctCarol, true,
		control.ContainerGroup{Kind: control.ContainerKindForeign, Name: "old-shop", State: control.ContainerStopped, WorkingDir: "/srv/example/shop",
			Containers: []control.ContainerSummary{web}},
		control.ContainerGroup{Kind: control.ContainerKindSingle, Name: "ghost", State: control.ContainerOK, Containers: []control.ContainerSummary{ghost}},
	)...)
	return b
}

// selectIn moves the mark of the Containers area to the container name of engine.
func selectIn(t *testing.T, m *Model, engine, name string) {
	t.Helper()
	send(m, "home")
	for range 40 {
		if id, ok := m.ctList.Selected(); ok && m.ctRows[id].name == name && m.ctRows[id].engine == engine {
			return
		}
		send(m, "down")
	}
	t.Fatalf("container %s of %s is not reachable in the list", name, engine)
}

func TestContainersSectionPerEngine(t *testing.T) {
	m := newTestModel(t, i18n.English, twoEnginesContainers(), ctWide, ctWideRows)
	send(m, "5")
	contains(t, m, "Engine system", "Engine rootless:alice", "demo-stack-app-1", "photo-album-web-1")
	if len(m.ct.secs) != 2 || m.ct.secs[0].engine != control.EngineSystem || m.ct.secs[1].engine != ctAlice {
		t.Fatalf("sections = %+v", m.ct.secs)
	}
	// The cursor passes from the last row of a section to the first row of the next, and back.
	selectIn(t, m, control.EngineSystem, "example-cache")
	for range 3 {
		send(m, "down")
	}
	if m.ct.engine != ctAlice {
		t.Fatalf("marked engine %q after moving down from the system section", m.ct.engine)
	}
	for range 5 {
		if m.ct.engine == control.EngineSystem {
			break
		}
		send(m, "up")
	}
	if id, _ := m.ctList.Selected(); m.ct.engine != control.EngineSystem || m.ctRows[id].name != "example-cache" {
		t.Fatalf("marked engine %q, row %q after moving back up", m.ct.engine, id)
	}
}

func TestContainersOneEngineStaysAsToday(t *testing.T) {
	b := containersBackend()
	b.engines = []string{control.EngineSystem}
	ctMark(control.EngineSystem, false, b.containers.Groups...)
	m := newTestModel(t, i18n.English, b, ctWide, ctWideRows)
	send(m, "5")
	contains(t, m, "demo-stack-app-1")
	noLeak(t, m.render(), "Engine system", "read-only", "sudo")
	if len(m.ct.secs) != 0 {
		t.Fatalf("sections = %+v", m.ct.secs)
	}
}

func TestContainersReadOnlySectionHasNoActions(t *testing.T) {
	b := withUnattended(twoEnginesContainers())
	m := newTestModel(t, i18n.English, b, ctWide, ctWideRows)
	m.euid = func() int { return 0 }
	send(m, "5")
	contains(t, m, "Engine rootless:carol, read-only", "no deploy service", "old-shop-web-1", "ghost")
	selectIn(t, m, ctCarol, "ghost")
	send(m, "S", "X", "Z", "o", "enter", "A")
	if m.modal != nil || m.ctv.open || m.clog.open || len(b.actions()) != 0 || b.called("container") != 0 || b.called("containerlogs") != 0 {
		t.Fatalf("a key acted on a read-only row: modal %v, detail %v, log %v, actions %v, reads %d, logs %d", m.modal != nil, m.ctv.open, m.clog.open, b.actions(), b.called("container"), b.called("containerlogs"))
	}
	// The same keys act in a section that has a deploy service.
	selectIn(t, m, ctAlice, "photo-album-web-1")
	b.containerDetails["photo-album-web-1"] = control.ContainerDetail{ContainerSummary: summary("photo-album-web-1", "x", control.ContainerOK, "running", 1, 0),
		ID: "a11ce0000002", Kind: control.ContainerKindOwn}
	send(m, "X")
	if m.modal == nil {
		t.Fatal("stop opened no question for a container with a deploy service")
	}
}

func TestContainersSameNameInTwoEnginesStaysApart(t *testing.T) {
	b := twoEnginesContainers()
	m := newTestModel(t, i18n.English, b, ctWide, ctWideRows)
	send(m, "5")
	selectIn(t, m, ctAlice, "example-cache")
	send(m, "enter")
	if m.ctv.engine != ctAlice {
		t.Fatalf("the detail is of engine %q", m.ctv.engine)
	}
	contains(t, m, "example.invalid/alice-cache:8")
	noLeak(t, m.render(), "example.invalid/cache:7")
	send(m, "esc")
	selectIn(t, m, control.EngineSystem, "example-cache")
	send(m, "enter")
	contains(t, m, "example.invalid/cache:7")
	noLeak(t, m.render(), "alice-cache")
}

func TestContainersUnreachableInstanceKeepsTheOtherSections(t *testing.T) {
	b := twoEnginesContainers()
	b.containers.Groups = b.containers.Groups[:len(b.containers.Groups)-2]
	b.containersDown = []control.EngineNotChecked{{Engine: ctAlice, Reason: "not running"}}
	m := newTestModel(t, i18n.English, b, ctWide, ctWideRows)
	send(m, "5")
	contains(t, m, "Engine system", "demo-stack-app-1", "Engine rootless:alice", "not running")
}

func TestContainersWithoutSudoSayWhatIsNotShown(t *testing.T) {
	b := containersBackend()
	b.containersUnread = true
	m := newTestModel(t, i18n.English, b, ctNarrow, ctNarrowRows)
	send(m, "5")
	contains(t, m, "demo-stack-app-1", "Engines without a deploy service are shown only with sudo.")
}

func TestGoldenContainersTwoEngines(t *testing.T) {
	goldenContainers(t, "containers.engines", twoEnginesContainers, "5")
}

func TestGoldenContainersEngineWithoutInstance(t *testing.T) {
	goldenContainers(t, "containers.readonly", func() *fakeBackend { return withUnattended(twoEnginesContainers()) }, "5")
}

func TestGoldenContainersNoSudo(t *testing.T) {
	goldenContainers(t, "containers.nosudo", func() *fakeBackend {
		b := twoEnginesContainers()
		b.containersUnread = true
		return b
	}, "5")
}

func TestGoldenContainersNoSudoOneEngine(t *testing.T) {
	goldenContainers(t, "containers.nosudo.single", func() *fakeBackend {
		b := containersBackend()
		b.containersUnread = true
		return b
	}, "5")
}
