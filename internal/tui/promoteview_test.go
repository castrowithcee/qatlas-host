package tui

import (
	"context"
	"testing"
	"time"

	"github.com/castrowithcee/qatlas-kit/i18n"
	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// sourceRowOf is the row of demo-stack after a refresh, with the given promote source in its settings.
func sourceRowOf(t *testing.T, b *fakeBackend, source string) stackRow {
	t.Helper()
	b.stacks.Stacks[0].PromoteSource = source
	msg := fetch(context.Background(), b)
	for _, r := range buildRows(msg.snap) {
		if r.name == "demo-stack" {
			return r
		}
	}
	t.Fatal("no row for demo-stack")
	return stackRow{}
}

func TestRowShowsTheVersionOfAProvenSourceBesideItsOwn(t *testing.T) {
	m := newSettingsModel(t, settingsBackend())
	r := sourceRowOf(t, settingsBackend(), "mail-relay")
	if got := cells(r, m.text, time.UTC, map[string]kittui.TextStyle{})[1]; got != "1.4.2 (source 1.5.0)" {
		t.Fatalf("version cell %q", got)
	}
	if got := cells(sourceRowOf(t, settingsBackend(), ""), m.text, time.UTC, map[string]kittui.TextStyle{})[1]; got != "1.4.2" {
		t.Fatalf("version cell without a source %q", got)
	}
}

func TestRowSaysWhyNoVersionOfTheSourceIsShown(t *testing.T) {
	m := newSettingsModel(t, settingsBackend())
	if got := sourceRowOf(t, settingsBackend(), "gone-stack").promote.cell(m.text); got != "missing" {
		t.Fatalf("a source no stack has: %q", got)
	}
	b := settingsBackend()
	for _, tc := range []struct {
		name string
		src  control.PromoteSource
		want string
	}{
		{"failed run", control.PromoteSource{Stack: "mail-relay", Commit: promoteCommit, LastResult: "failed"}, "not proven"},
		{"failed commit", control.PromoteSource{Stack: "mail-relay", Commit: promoteCommit, LastResult: "succeeded", FailedCommit: commitB}, "not proven"},
		{"unhealthy", control.PromoteSource{Stack: "mail-relay", Commit: promoteCommit, LastResult: "succeeded", Unhealthy: true}, "not proven"},
		{"never deployed", control.PromoteSource{Stack: "mail-relay"}, "not proven"},
	} {
		si := control.StackInfo{Name: "demo-stack", PromoteSource: "mail-relay", Images: b.stacks.Stacks[0].Images}
		if got := viewOf(si, sourceRead{src: tc.src}).cell(m.text); got != tc.want {
			t.Errorf("%s: cell %q, want %q", tc.name, got, tc.want)
		}
	}
	if got := viewOf(control.StackInfo{PromoteSource: "x"}, sourceRead{unreadable: true}).cell(m.text); got != "unreadable" {
		t.Errorf("unreadable source: %q", got)
	}
}

func TestDetailShowsTheSourceVersionAndGermanTexts(t *testing.T) {
	b := settingsBackend()
	b.stacks.Stacks[0].PromoteSource = "mail-relay"
	m := newSettingsModel(t, b)
	send(m, "enter")
	containsFlat(t, m, "Source stack for promotion: mail-relay", "APP_VERSION 1.5.0")

	b = settingsBackend()
	b.stacks.Stacks[0].PromoteSource = "gone-stack"
	de := newTestModel(t, i18n.German, b, wideWidth, 40)
	send(de, "enter")
	containsFlat(t, de, "Quell-Stack für die Übernahme: gone-stack", "fehlt")
}

func TestRefreshReadsASharedSourceOnce(t *testing.T) {
	b := settingsBackend()
	b.stacks.Stacks[0].PromoteSource = "mail-relay"
	b.stacks.Stacks[4].PromoteSource = "mail-relay"
	b.stacks.Stacks[2].PromoteSource = "gone-stack"
	got := readSources(context.Background(), b, b.stacks)
	if len(b.promoteReads) != 1 || len(got) != 3 || !got[stackRef{"", "photo-album"}].missing {
		t.Fatalf("reads %v, sources %+v", b.promoteReads, got)
	}
}
