package tui

import (
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

func TestStackDetailShowsImageFingerprintsAndMissingOnes(t *testing.T) {
	b := demoBackend()
	b.status.Engine.Stacks[0].Images = []control.FingerprintInfo{
		{Service: "app", Image: "reg.example.test/app:1", RepoDigest: "sha256:" + strings.Repeat("a", 64), ImageID: "sha256:" + strings.Repeat("b", 64)},
		{Service: "worker", Image: "local/worker"},
	}
	m := newSettingsModel(t, b)
	send(m, toDetail...)
	contains(t, m, "Images", "reg.example.test/app:1", "digest sha256:aaaaaaaaaaaa  id sha256:bbbbbbbbbbbb", "digest missing  id missing")
}

func TestStackDetailListsEachVersionVariableOnceBeforeTheFirstCheck(t *testing.T) {
	m := newSettingsModel(t, demoBackend())
	m.snap.stacks.Stacks = []control.StackInfo{{Name: "s", Images: []control.ImageInfo{
		{Service: "web", Image: "reg.example.test/app", Var: "APP", Version: "1.0"},
		{Service: "worker", Image: "reg.example.test/app", Var: "APP", Version: "1.0"},
	}}}
	items, _ := m.versionItems("", "s")
	if len(items) != 1 || items[0].variable != "APP" || items[0].current != "1.0" {
		t.Fatalf("items = %+v", items)
	}
}
