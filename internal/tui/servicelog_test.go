package tui

import (
	"errors"
	"testing"
	"time"

	"github.com/castrowithcee/qatlas-kit/i18n"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

func svcEntry(sec int, level, msg string, attrs ...control.ServiceLogAttr) control.ServiceLogEntry {
	return control.ServiceLogEntry{Time: clogStart.Add(time.Duration(sec) * time.Second), Level: level,
		Source: control.ServiceLogSourceHost, Message: msg, Attrs: attrs}
}

// svcBackend is demoBackend with a service log of four entries on three levels.
func svcBackend() *fakeBackend {
	b := demoBackend()
	b.svcLog = []control.ServiceLogEntry{
		svcEntry(0, control.LogLevelInfo, "service started", control.ServiceLogAttr{Key: "version", Value: "1.2.3"}),
		svcEntry(1, control.LogLevelDebug, "poll tick"),
		svcEntry(2, control.LogLevelWarn, "maintenance window without a bound source", control.ServiceLogAttr{Key: "window", Value: "night shift"}),
		svcEntry(3, control.LogLevelError, "registry unreachable"),
	}
	return b
}

func openServiceLog(t *testing.T, b *fakeBackend) *Model {
	t.Helper()
	m := newTestModel(t, i18n.English, b, wideWidth, goldenRows)
	send(m, "3", "v")
	if !m.clogShown() || !m.clog.service {
		t.Fatalf("clog = %+v", m.clog)
	}
	return m
}

func TestServiceLogOpensFromRunsWithOneRequest(t *testing.T) {
	b := svcBackend()
	m := openServiceLog(t, b)
	if len(b.svcArgs) != 1 || b.svcArgs[0].Tail != clogTail || b.svcArgs[0].Cursor != "" || b.svcArgs[0].Level != "" {
		t.Fatalf("requests %+v", b.svcArgs)
	}
	contains(t, m, "Service log", "INFO service started version=1.2.3", "WARN maintenance window without a bound source window=\"night shift\"",
		"ERROR registry unreachable")
	send(m, "esc")
	if m.clog.open || m.area != areaRuns {
		t.Fatalf("open %v, area %s", m.clog.open, m.area)
	}
}

func TestServiceLogLevelKeyAsksForTheMinimumLevel(t *testing.T) {
	b := svcBackend()
	m := openServiceLog(t, b)
	send(m, "!") // info
	send(m, "!") // warn
	last := b.svcArgs[len(b.svcArgs)-1]
	if last.Level != control.LogLevelWarn || last.Cursor != "" || last.Tail != clogTail {
		t.Fatalf("request %+v", last)
	}
	if m.clog.view.Len() != 2 {
		t.Fatalf("lines %d, want warn and error", m.clog.view.Len())
	}
	contains(t, m, "level from warn", "maintenance window without a bound source", "registry unreachable")
	noLeak(t, m.render(), "service started", "poll tick")
	send(m, "!", "!") // error, then all again
	if m.clog.level != "" || m.clog.view.Len() != 4 {
		t.Fatalf("level %q, lines %d", m.clog.level, m.clog.view.Len())
	}
}

func TestServiceLogFilterSearchesTheText(t *testing.T) {
	b := svcBackend()
	m := openServiceLog(t, b)
	send(m, "/", "u", "n", "r", "e", "a", "c", "h", "enter")
	if m.clog.view.Query() != "unreach" {
		t.Fatalf("query %q", m.clog.view.Query())
	}
	contains(t, m, "registry unreachable")
}

func TestServiceLogFollowsFromTheCursor(t *testing.T) {
	b := svcBackend()
	m := openServiceLog(t, b)
	b.svcLog = append(b.svcLog, svcEntry(4, control.LogLevelInfo, "window finished"))
	before := len(b.svcArgs)
	fireUntil(t, m, func() bool { return len(b.svcArgs) > before })
	got := b.svcArgs[before]
	if got.Cursor != "4" || got.Tail != 0 || !got.Since.IsZero() {
		t.Fatalf("follow request %+v", got)
	}
	contains(t, m, "window finished")
	if m.clog.view.Len() != 5 {
		t.Fatalf("lines %d, want 5", m.clog.view.Len())
	}
}

func TestServiceLogReadsFurtherPages(t *testing.T) {
	b := svcBackend()
	b.svcPage = 3
	m := openServiceLog(t, b)
	if len(b.svcArgs) != 2 || b.svcArgs[1].Cursor == "" || b.svcArgs[1].Tail != 0 || m.clog.view.Len() != 4 {
		t.Fatalf("requests %+v, lines %d", b.svcArgs, m.clog.view.Len())
	}
}

func TestServiceLogShowsAFailedRead(t *testing.T) {
	b := svcBackend()
	b.errs = map[string]error{"servicelog:": errors.New("boom")}
	m := openServiceLog(t, b)
	if m.clog.err == nil {
		t.Fatal("no error kept")
	}
}

func TestGoldenServiceLog(t *testing.T) {
	goldenScreens(t, "servicelog", goldenRows, svcBackend, "3", "v")
}

func TestGoldenServiceLogLevel(t *testing.T) {
	goldenScreens(t, "servicelog.level", goldenRows, svcBackend, "3", "v", "!", "!")
}
