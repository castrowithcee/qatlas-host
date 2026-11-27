package hostservice

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/castrowithcee/qatlas-host/internal/client"
	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/host/auditlog"
	"github.com/castrowithcee/qatlas-host/internal/host/usage"
)

const testMemTotal = 4000 * 1024

// fakeTicker is the clock of every stream of a test service: a tick reaches the stream that takes it first,
// and asked holds the intervals the streams asked for.
type fakeTicker struct {
	ticks chan time.Time
	asked chan time.Duration
}

// startStreams starts a host service for the peer uid that measures an invented host and ticks only when the
// test says so.
func startStreams(t *testing.T, uid uint32) (env, *fakeTicker) {
	t.Helper()
	root := t.TempDir()
	meminfo := "MemTotal: 4000 kB\nMemFree: 1000 kB\nMemAvailable: 3000 kB\nSwapTotal: 0 kB\nSwapFree: 0 kB\n"
	if err := os.WriteFile(filepath.Join(root, "meminfo"), []byte(meminfo), 0o600); err != nil {
		t.Fatal(err)
	}
	src := &usage.Source{ProcRoot: root, Mounts: filepath.Join(root, "no-mounts"),
		Statfs: func(string) (usage.FSStat, error) { return usage.FSStat{}, errors.New("no file systems") },
		Pause:  func(context.Context) error { return nil }}
	ft := &fakeTicker{ticks: make(chan time.Time), asked: make(chan time.Duration, 16)}
	e := startWith(t, func(net.Conn) (uint32, error) { return uid, nil }, func(c *Config) {
		c.usage = src
		c.ticker = func(d time.Duration) (<-chan time.Time, func()) {
			ft.asked <- d
			return ft.ticks, func() {}
		}
	})
	return e, ft
}

func (ft *fakeTicker) tick(t *testing.T) {
	t.Helper()
	select {
	case ft.ticks <- time.Time{}:
	case <-time.After(5 * time.Second):
		t.Fatal("no stream waits for a tick")
	}
}

func (ft *fakeTicker) wantInterval(t *testing.T, want time.Duration) {
	t.Helper()
	select {
	case got := <-ft.asked:
		if got != want {
			t.Fatalf("interval = %s, want %s", got, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the stream asked for no ticker")
	}
}

// openStream is a stream a test client holds open.
type openStream struct {
	values chan control.HostUsageResult
	end    chan error
	cancel context.CancelFunc
}

func (e env) stream(t *testing.T, seconds int) *openStream {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	o := &openStream{values: make(chan control.HostUsageResult, 16), end: make(chan error, 1), cancel: cancel}
	go func() {
		o.end <- client.StreamUsage(ctx, e.endpoint, seconds, func(u control.HostUsageResult) error {
			o.values <- u
			return nil
		})
	}()
	return o
}

func (o *openStream) next(t *testing.T) control.HostUsageResult {
	t.Helper()
	select {
	case u := <-o.values:
		return u
	case err := <-o.end:
		t.Fatalf("the stream ended: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("no measurement")
	}
	return control.HostUsageResult{}
}

func (o *openStream) result(t *testing.T) error {
	t.Helper()
	select {
	case err := <-o.end:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("the stream does not end")
	}
	return nil
}

// usageEntries counts the entries of usage reads in the log, and those of them that failed with code.
func (e env) usageEntries(t *testing.T, code string) (all, failed int) {
	t.Helper()
	for _, r := range e.entries(t) {
		if r.Entry.Action == auditlog.OperationUsageRead {
			all++
			if code != "" && r.Entry.ErrorCode == code {
				failed++
			}
		}
	}
	return all, failed
}

func wantQuietLog(t *testing.T, e env) {
	t.Helper()
	if logs := e.logs.String(); strings.Contains(logs, "level=ERROR") || strings.Contains(logs, "level=WARN") {
		t.Fatalf("service log:\n%s", logs)
	}
}

func TestAUsageStreamAnswersRightAwayAndPerTickWithOneLogEntry(t *testing.T) {
	e, ft := startStreams(t, 4100)
	o := e.stream(t, 3)
	if u := o.next(t); u.Memory == nil || u.Memory.TotalBytes != testMemTotal {
		t.Fatalf("first measurement = %+v", u.Memory)
	}
	ft.wantInterval(t, 3*time.Second)
	for range 3 {
		ft.tick(t)
		if u := o.next(t); u.Memory == nil || u.Memory.TotalBytes != testMemTotal {
			t.Fatalf("measurement = %+v", u.Memory)
		}
	}
	if all, _ := e.usageEntries(t, ""); all != 1 {
		t.Fatalf("%d log entries for one stream", all)
	}
	if got := e.entries(t)[0].Entry; got.Result != auditlog.ResultSuccess || got.ClientUID == nil || *got.ClientUID != 4100 {
		t.Fatalf("entry = %+v", got)
	}
	o.cancel()
	if err := o.result(t); !errors.Is(err, context.Canceled) {
		t.Fatalf("end = %v", err)
	}

	def := e.stream(t, 0)
	def.next(t)
	ft.wantInterval(t, control.HostUsageStreamDefaultSeconds*time.Second)
}

func TestAUsageStreamRefusesAnIntervalOutOfBounds(t *testing.T) {
	e, _ := startStreams(t, 0)
	for _, seconds := range []int{-1, 1, 61} {
		wantCode(t, e.stream(t, seconds).result(t), control.CodeInvalid)
	}
	if all, failed := e.usageEntries(t, control.CodeInvalid); all != 3 || failed != 3 {
		t.Fatalf("entries %d, failed %d", all, failed)
	}
}

func TestARevokedAdministratorLosesTheOpenStream(t *testing.T) {
	e, ft := startStreams(t, 4100)
	o := e.stream(t, 0)
	o.next(t)
	revoked := strings.Replace(inventedGroups, "qhost:x:990:example-admin,", "qhost:x:990:", 1)
	if err := os.WriteFile(e.group, []byte(revoked), 0o600); err != nil {
		t.Fatal(err)
	}
	ft.tick(t)
	wantCode(t, o.result(t), control.CodeNotAdmin)
	if all, _ := e.usageEntries(t, ""); all != 1 {
		t.Fatalf("%d log entries for one stream", all)
	}
}

// The fifth stream is refused while four are open, a plain request still gets through, and a stream the
// client closes frees its slot without a tick and without a word in the service log.
func TestAtMostFourStreamsAreOpenAndAClosedOneFreesItsSlot(t *testing.T) {
	e, _ := startStreams(t, 0)
	open := make([]*openStream, maxStreams)
	for i := range open {
		open[i] = e.stream(t, 0)
		open[i].next(t)
	}
	wantCode(t, e.stream(t, 0).result(t), control.CodeBusy)
	if err := e.status(); err != nil {
		t.Fatalf("status beside four streams: %v", err)
	}
	open[0].cancel()
	if err := open[0].result(t); !errors.Is(err, context.Canceled) {
		t.Fatalf("end = %v", err)
	}
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		o := e.stream(t, 0)
		select {
		case <-o.values:
		case err := <-o.end:
			wantCode(t, err, control.CodeBusy)
			if time.Now().After(deadline) {
				t.Fatal("the closed stream keeps its slot")
			}
			continue
		}
		break
	}
	if _, busy := e.usageEntries(t, control.CodeBusy); busy == 0 {
		t.Fatal("the refused stream left no entry")
	}
	e.stop()
	wantQuietLog(t, e)
}

// The stop sets a past deadline on every connection; a stream must still deliver its error. The race shows only
// now and then, so the test repeats it.
func TestAStopEndsOpenStreamsWithAnError(t *testing.T) {
	for range 10 {
		e, _ := startStreams(t, 0)
		streams := []*openStream{e.stream(t, 0), e.stream(t, 0)}
		for _, o := range streams {
			o.next(t)
		}
		e.stop()
		for _, o := range streams {
			wantCode(t, o.result(t), control.CodeStopping)
		}
		wantQuietLog(t, e)
	}
}
