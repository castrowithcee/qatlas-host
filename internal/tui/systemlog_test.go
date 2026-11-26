package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/castrowithcee/qatlas-kit/i18n"
	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/client"
	"github.com/castrowithcee/qatlas-host/internal/control"
)

// fakeHostLog stands in for the channel of the host service with an invented action log.
type fakeHostLog struct {
	list            control.HostLogListResult
	verify          control.HostLogVerifyResult
	listErr, verErr error
	lists, verifies int
	args            []control.HostLogListArgs
}

func (f *fakeHostLog) LogList(_ context.Context, a control.HostLogListArgs) (control.HostLogListResult, error) {
	f.lists++
	f.args = append(f.args, a)
	return f.list, f.listErr
}

func (f *fakeHostLog) LogVerify(context.Context) (control.HostLogVerifyResult, error) {
	f.verifies++
	return f.verify, f.verErr
}

// demoHostLog is an invented action log of three entries, the newest a change of the host by ada, and an intact chain.
func demoHostLog() *fakeHostLog {
	ada, root := uint32(1000), uint32(0)
	at := func(minute int) time.Time { return time.Date(2026, 11, 2, 9, minute, 0, 0, time.UTC) }
	return &fakeHostLog{
		list: control.HostLogListResult{Entries: []control.HostLogEntry{
			{Seq: 42, Time: at(5), ClientUID: &ada, Action: "host.change", Result: control.HostLogResultSuccess, DurationMS: 120,
				Change: &control.HostLogChange{ActionID: "folder.set-permissions", JournalID: demoChangeID}},
			{Seq: 41, Time: at(4), ClientUID: &root, Action: "control.refuse", Result: control.HostLogResultFailure, ErrorCode: "not-admin"},
			{Seq: 40, Time: at(3), ClientUID: &ada, Action: "log.list", Result: control.HostLogResultSuccess, DurationMS: 2},
		}},
		verify: control.HostLogVerifyResult{Intact: true, Days: []control.HostLogDay{
			{Date: "2026-11-01", Entries: 30, Intact: true}, {Date: "2026-11-02", Entries: 12, Intact: true}}},
	}
}

// toLogPage opens the log page of the System area.
func toLogPage(t *testing.T, m *Model) {
	t.Helper()
	m.setZone(kittui.ZoneSubtabs)
	send(m, "left")
	if m.sys.page != pageLog {
		t.Fatalf("page %s, want the log page", m.sys.page)
	}
	send(m, "down")
}

func TestLogShowsTheActionLogUnderTheCheckOfItsChain(t *testing.T) {
	m, f := systemModel(t, i18n.English, wideWidth, 40)
	h := demoHostLog()
	m.hostLog = h
	toLogPage(t, m)
	screen := m.render()
	for _, want := range []string{"Chain of hashes intact: 2 days with 42 entries checked.", "2026-11-02 09:05:00", "host.change",
		"folder.set-permissions " + demoChangeID, "failure: not-admin", "root"} {
		if !strings.Contains(screen, want) {
			t.Errorf("the log page lacks %q:\n%s", want, screen)
		}
	}
	if len(f.reqs) != 0 {
		t.Errorf("reading the action log ran something as root: %v", f.reqs)
	}
	if h.lists != 1 || h.verifies != 1 || h.args[0].Limit != hostLogLimit {
		t.Errorf("lists %d verifies %d args %+v, want one of each", h.lists, h.verifies, h.args)
	}
}

func TestLogIsReadOnceOnOpenAndAgainOnReload(t *testing.T) {
	m, _ := systemModel(t, i18n.English, wideWidth, 40)
	h := demoHostLog()
	m.hostLog = h
	toLogPage(t, m)
	m.setZone(kittui.ZoneSubtabs)
	send(m, "left", "right")
	if h.lists != 1 {
		t.Fatalf("the action log was read %d times, want once: every read is an entry of it", h.lists)
	}
	send(m, "R")
	if h.lists != 2 || h.verifies != 2 {
		t.Fatalf("r read the action log %d and checked it %d times, want twice each", h.lists, h.verifies)
	}
}

func TestBrokenChainNamesTheDayAndTheLine(t *testing.T) {
	m, _ := systemModel(t, i18n.English, wideWidth, 40)
	h := demoHostLog()
	h.verify = control.HostLogVerifyResult{Intact: false, Days: []control.HostLogDay{{Date: "2026-11-01", Entries: 30, Intact: false}},
		Problems: []control.HostLogProblem{{Date: "2026-11-01", Seq: 17, Line: 9, Code: "chain-broken"}}, MoreProblems: true}
	m.hostLog = h
	toLogPage(t, m)
	want := "Chain of hashes broken on 2026-11-01, line 9: the entry does not follow from the one before. Further days are broken too."
	if screen := flat(m.render()); !strings.Contains(screen, want) {
		t.Fatalf("the page lacks %q:\n%s", want, screen)
	}
}

func TestLogExplainsAHostServiceThatCannotBeReached(t *testing.T) {
	for name, c := range map[string]struct {
		err  error
		want string
	}{
		"not running": {client.ErrNotRunning, "The host service is not running. Start it with sudo systemctl start qhost."},
		"no access":   {control.ErrNotAdmin(), "qhost is reserved for administrators (sudo)."},
	} {
		m, _ := systemModel(t, i18n.English, wideWidth, 40)
		m.hostLog = &fakeHostLog{listErr: c.err, verErr: c.err}
		toLogPage(t, m)
		if screen := flat(m.render()); !strings.Contains(screen, c.want) {
			t.Errorf("%s: the page lacks %q:\n%s", name, c.want, screen)
		}
	}
	m, _ := systemModel(t, i18n.English, wideWidth, 40)
	toLogPage(t, m)
	if screen := flat(m.render()); !strings.Contains(screen, "the interface has no way to reach the host service") {
		t.Errorf("an interface without a host channel does not say so:\n%s", screen)
	}
}
