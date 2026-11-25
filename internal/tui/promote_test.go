package tui

import (
	"slices"
	"testing"

	"github.com/castrowithcee/qatlas-kit/i18n"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// The way to the choice: the detail of the first stack, demo-stack, and the key. The sources are the other stacks
// by name, so the first is mail-relay.
var promoteFromDetail = keys("enter", "M")

func TestPromoteShowsVersionsAndRunsBoundToTheCommit(t *testing.T) {
	b := settingsBackend()
	m := newSettingsModel(t, b)
	send(m, promoteFromDetail...)
	contains(t, m, "Take over the versions of", "mail-relay", "photo-album")
	send(m, "enter")
	contains(t, m, "Set the image versions of demo-stack", "APP_VERSION: 1.4.2 -> 1.5.0", "Source mail-relay at commit 89abcdef0123")
	if len(b.promotes) != 0 {
		t.Fatal("run before the confirmation")
	}
	send(m, "y")
	if len(b.promotes) != 1 || b.promotes[0].Stack != "demo-stack" || b.promotes[0].Commit != promoteCommit {
		t.Fatalf("promotes %+v", b.promotes)
	}
	if !slices.Equal(b.promoteReads, []string{"mail-relay", "mail-relay"}) {
		t.Fatalf("the source is read for the preview and again for the run, got %v", b.promoteReads)
	}
	contains(t, m, "took over the versions", "APP_VERSION 1.4.2 -> 1.5.0")
}

func TestPromoteCancelChangesNothing(t *testing.T) {
	b := settingsBackend()
	m := newSettingsModel(t, b)
	send(m, promoteFromDetail...)
	send(m, "enter", "n")
	if m.modal != nil || len(b.promotes) != 0 {
		t.Fatal("cancel changed something")
	}
}

func TestPromoteNamesTheObservationTime(t *testing.T) {
	b := settingsBackend()
	b.promoteCheck = &control.PromoteCheckResult{Stack: "demo-stack", Source: "mail-relay", SourceCommit: promoteCommit,
		Steps:   []control.PromoteStepInfo{{Variable: "APP_VERSION", From: "1.4.2", To: "1.5.0"}},
		Observe: &control.ObserveInfo{Commit: promoteCommit, Until: at(31, 5, 0)}}
	m := newSettingsModel(t, b)
	send(m, promoteFromDetail...)
	send(m, "enter")
	contains(t, m, "under observation until")
}

func TestPromoteNothingToDoAndRefusal(t *testing.T) {
	b := settingsBackend()
	b.promoteCheck = &control.PromoteCheckResult{Stack: "demo-stack", Source: "mail-relay", SourceCommit: promoteCommit}
	m := newSettingsModel(t, b)
	send(m, promoteFromDetail...)
	send(m, "enter")
	if m.modal != nil {
		t.Fatal("asked although nothing is to do")
	}
	contains(t, m, "Nothing to take over")

	b = settingsBackend()
	b.errs = map[string]error{"promotecheck:demo-stack": &control.Error{Code: control.CodeNotUpdatable, Message: "the source mail-relay is not healthy"}}
	m = newSettingsModel(t, b)
	send(m, promoteFromDetail...)
	send(m, "enter")
	if m.modal != nil {
		t.Fatal("asked although the service refused")
	}
	contains(t, m, "the source mail-relay is not healthy")
}

func TestPromoteFailureNamesTheRun(t *testing.T) {
	b := settingsBackend()
	b.promoteRes = &control.PromoteRunResult{PromoteCheckResult: control.PromoteCheckResult{Stack: "demo-stack",
		Steps: []control.PromoteStepInfo{{Variable: "APP_VERSION", From: "1.4.2", To: "1.5.0"}}}, Run: "20261101T040000Z-00000004", Result: "rolled-back"}
	m := newSettingsModel(t, b)
	send(m, promoteFromDetail...)
	send(m, "enter", "y")
	contains(t, m, "did not succeed", "20261101T040000Z-00000004")
}

func TestPromoteWithoutRootRunsTheCommand(t *testing.T) {
	m, f := sudoDetailModel(t, i18n.English, settingsBackend(), wideWidth, 40)
	f.report = `[{"stack":"demo-stack","variable":"APP_VERSION","from":"1.4.2","to":"1.5.0","run":"20261101T040000Z-00000003","commit":"0123456789abcdef","result":"succeeded","error":""}]`
	send(m, promoteFromDetail...)
	send(m, "enter")
	contains(t, m, "sudo qhost stack promote --confirm")
	if len(f.reqs) != 0 {
		t.Fatalf("sudo ran before the answer: %+v", f.reqs)
	}
	send(m, "y")
	want := []string{"stack", "promote", "--confirm", "--output", "json", "--", "mail-relay", "demo-stack"}
	if len(f.reqs) == 0 || !slices.Equal(f.reqs[len(f.reqs)-1].Args, want) {
		t.Fatalf("requests %+v, want the arguments %v", f.reqs, want)
	}
	contains(t, m, "took over the versions", "APP_VERSION 1.4.2 -> 1.5.0")
}

func TestPromoteGerman(t *testing.T) {
	m := newTestModel(t, i18n.German, settingsBackend(), wideWidth, 40)
	send(m, promoteFromDetail...)
	send(m, "enter")
	contains(t, m, "Versionen übernehmen", "Quelle mail-relay bei Commit")
}

func TestGoldenPromoteConfirm(t *testing.T) {
	goldenScreens(t, "promote.confirm", detailEditRows, settingsBackend, seq(promoteFromDetail, keys("enter"))...)
}
