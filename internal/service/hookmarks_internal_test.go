package service

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type markClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *markClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *markClock) After(d time.Duration) <-chan time.Time { return make(chan time.Time) }

func (c *markClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

type markSyncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *markSyncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *markSyncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

type markRig struct {
	m     *markerService
	dir   string
	clock *markClock
	logs  *markSyncBuffer
	mu    sync.Mutex
	calls []string
	block chan struct{} // when not nil, a check waits for it to close
}

func newMarkRig(t *testing.T) *markRig {
	t.Helper()
	r := &markRig{dir: filepath.Join(t.TempDir(), "marks"), clock: &markClock{now: time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)},
		logs: &markSyncBuffer{}}
	if err := os.Mkdir(r.dir, 0o770); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(r.logs, nil))
	r.m = newMarkerService(r.dir, r.clock, 0, func(ctx context.Context, name string) {
		r.mu.Lock()
		r.calls = append(r.calls, name)
		block := r.block
		r.mu.Unlock()
		if block != nil {
			select {
			case <-block:
			case <-ctx.Done():
			}
		}
	}, log)
	return r
}

func (r *markRig) scan() {
	r.m.scan(context.Background())
}

// settle waits for the checks started so far and returns the stacks they were for.
func (r *markRig) settle() []string {
	r.m.wg.Wait()
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.calls...)
}

func (r *markRig) touch(t *testing.T, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(r.dir, name), nil, 0o660); err != nil {
		t.Fatal(err)
	}
}

func (r *markRig) entries(t *testing.T) []string {
	t.Helper()
	list, err := os.ReadDir(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range list {
		names = append(names, e.Name())
	}
	return names
}

func TestMarkerStartsOneCheckAndIsRemoved(t *testing.T) {
	r := newMarkRig(t)
	r.touch(t, "demo")
	r.scan()
	if got := r.settle(); len(got) != 1 || got[0] != "demo" {
		t.Fatalf("checks = %q", got)
	}
	if left := r.entries(t); len(left) != 0 {
		t.Fatalf("left in the folder: %q", left)
	}
	r.scan()
	if got := r.settle(); len(got) != 1 {
		t.Fatalf("a second pass checked again: %q", got)
	}
}

// Markers of one stack laid down while its check runs ask for no further check; every one of them is removed.
func TestMarkersOfOneStackMakeOneCheck(t *testing.T) {
	r := newMarkRig(t)
	r.block = make(chan struct{})
	for i := 0; i < 3; i++ {
		r.touch(t, "demo")
		r.scan()
		if left := r.entries(t); len(left) != 0 {
			t.Fatalf("pass %d left %q", i, left)
		}
	}
	close(r.block)
	if got := r.settle(); len(got) != 1 {
		t.Fatalf("checks = %q", got)
	}
}

// A long check of one stack does not hold back the markers of another.
func TestLongCheckDoesNotHoldBackOtherStacks(t *testing.T) {
	r := newMarkRig(t)
	r.block = make(chan struct{})
	r.touch(t, "slow")
	r.scan()
	r.touch(t, "other")
	r.scan()
	if left := r.entries(t); len(left) != 0 {
		t.Fatalf("left in the folder: %q", left)
	}
	close(r.block)
	if got := r.settle(); len(got) != 2 {
		t.Fatalf("checks = %q", got)
	}
}

func TestForeignEntriesAreRemovedWithoutFollowingThem(t *testing.T) {
	r := newMarkRig(t)
	outside := t.TempDir()
	target := filepath.Join(outside, "target")
	if err := os.WriteFile(target, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(target, old, old); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target, 0); err != nil {
		t.Fatal(err)
	}
	targetDir := filepath.Join(outside, "dir")
	if err := os.Mkdir(targetDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "keep"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	// A link named like a stack, a link to a folder, a bad name, a name no folder can hold as a path, an empty
	// and a non-empty folder.
	for name, link := range map[string]string{"demo": target, "dirlink": targetDir, "up": "../x"} {
		if err := os.Symlink(link, filepath.Join(r.dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	r.touch(t, "UPPER")
	r.touch(t, "x..y.")
	if err := os.Mkdir(filepath.Join(r.dir, "emptydir"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(r.dir, "full"), 0o700); err != nil {
		t.Fatal(err)
	}
	inner := filepath.Join(r.dir, "full", "inner")
	if err := os.WriteFile(inner, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	r.scan()
	if got := r.settle(); len(got) != 0 {
		t.Fatalf("checks = %q", got)
	}
	if left := r.entries(t); len(left) != 1 || left[0] != "full" {
		t.Fatalf("left in the folder: %q", left)
	}
	if b, err := os.ReadFile(inner); err != nil || string(b) != "x" {
		t.Fatalf("the non-empty folder was entered: %q %v", b, err)
	}
	fi, err := os.Stat(target)
	if err != nil || fi.Mode().Perm() != 0 || !fi.ModTime().Equal(old) || fi.Size() != 6 {
		t.Fatalf("the link target changed: %v %v", fi, err)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "keep")); err != nil {
		t.Fatalf("the target folder changed: %v", err)
	}
	// The folder that cannot go is logged once and then left alone.
	logs := r.logs.String()
	if strings.Count(logs, "foreign entry not removed") != 1 || !strings.Contains(logs, "name=full") {
		t.Fatalf("logs = %s", logs)
	}
	r.clock.advance(2 * time.Minute)
	r.scan()
	r.scan()
	if got := strings.Count(r.logs.String(), "foreign entry not removed"); got != 1 {
		t.Fatalf("the non-empty folder was logged %d times", got)
	}
}

func TestMarkerNamePattern(t *testing.T) {
	for _, n := range []string{"demo", "a", "demo-stack_1", "0abc"} {
		if !markerName.MatchString(n) {
			t.Errorf("%q refused", n)
		}
	}
	for _, n := range []string{"../x", "UPPER", "", "-a", "_a", "a b", "a.b", "a/b"} {
		if markerName.MatchString(n) {
			t.Errorf("%q accepted", n)
		}
	}
}

func TestMissingFolderIsNoErrorAndLoggedOnChange(t *testing.T) {
	r := newMarkRig(t)
	if err := os.Remove(r.dir); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		r.scan()
	}
	if got := strings.Count(r.logs.String(), "level="); got != 1 {
		t.Fatalf("%d log lines for a missing folder: %s", got, r.logs.String())
	}
	if err := os.Mkdir(r.dir, 0o770); err != nil {
		t.Fatal(err)
	}
	r.touch(t, "demo")
	r.scan()
	r.scan()
	if got := r.settle(); len(got) != 1 {
		t.Fatalf("checks = %q", got)
	}
	if !strings.Contains(r.logs.String(), "folder is available") || strings.Count(r.logs.String(), "level=") != 2 {
		t.Fatalf("logs = %s", r.logs.String())
	}
}

func TestFolderThatIsAFileIsNotRead(t *testing.T) {
	r := newMarkRig(t)
	if err := os.Remove(r.dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(r.dir, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	r.scan()
	r.scan()
	if got := strings.Count(r.logs.String(), "cannot be read"); got != 1 {
		t.Fatalf("logs = %s", r.logs.String())
	}
}

func TestFolderThatIsALinkIsNotRead(t *testing.T) {
	r := newMarkRig(t)
	real := t.TempDir()
	if err := os.WriteFile(filepath.Join(real, "demo"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(r.dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, r.dir); err != nil {
		t.Fatal(err)
	}
	r.scan()
	if got := r.settle(); len(got) != 0 {
		t.Fatalf("checks = %q", got)
	}
	if _, err := os.Stat(filepath.Join(real, "demo")); err != nil {
		t.Fatalf("the linked folder was touched: %v", err)
	}
}

func TestPassTakesAtMostOneHundredEntries(t *testing.T) {
	r := newMarkRig(t)
	for i := 0; i < 150; i++ {
		r.touch(t, fmt.Sprintf("stack-%03d", i))
	}
	r.scan()
	if got := r.settle(); len(got) != 100 || len(r.entries(t)) != 50 {
		t.Fatalf("first pass: %d checks, %d left", len(got), len(r.entries(t)))
	}
	r.scan()
	if got := r.settle(); len(got) != 150 || len(r.entries(t)) != 0 {
		t.Fatalf("second pass: %d checks, %d left", len(got), len(r.entries(t)))
	}
}

// Folders that cannot be removed do not use up the hundred entries of a pass.
func TestStuckFoldersDoNotStarveMarkers(t *testing.T) {
	r := newMarkRig(t)
	for i := 0; i < 120; i++ {
		d := filepath.Join(r.dir, fmt.Sprintf("full%03d", i))
		if err := os.Mkdir(d, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "x"), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	r.scan()
	r.touch(t, "demo")
	r.scan()
	if got := r.settle(); len(got) != 1 || got[0] != "demo" {
		t.Fatalf("checks = %q", got)
	}
}

func TestForeignLogLineComesOncePerMinute(t *testing.T) {
	r := newMarkRig(t)
	lines := func() int { return strings.Count(r.logs.String(), "foreign entries removed") }
	r.touch(t, "UPPER1")
	r.scan()
	r.touch(t, "UPPER2")
	r.clock.advance(30 * time.Second)
	r.scan()
	if lines() != 1 {
		t.Fatalf("lines = %d", lines())
	}
	r.touch(t, "UPPER3")
	r.clock.advance(31 * time.Second)
	r.scan()
	if lines() != 2 {
		t.Fatalf("lines = %d", lines())
	}
}

func TestMarkNameIsShortAndPrintable(t *testing.T) {
	got := markName("a\x00b\xffc\n" + strings.Repeat("x", 100))
	if strings.ContainsAny(got, "\x00\n\xff") || len([]rune(got)) != markNameLen+1 || !strings.HasPrefix(got, "a?b?c?xx") {
		t.Fatalf("markName = %q", got)
	}
	if markName("demo") != "demo" {
		t.Fatalf("markName shortened a short name")
	}
}

// The loop ends with its context, and only after the check it started, which ends with that context too.
func TestRunEndsWithContextAndWaitsForChecks(t *testing.T) {
	r := newMarkRig(t)
	r.block = make(chan struct{})
	r.touch(t, "demo")
	r.m.clock = systemClock{}
	r.m.interval = 5 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		r.m.run(ctx)
	}()
	for deadline := time.Now().Add(5 * time.Second); len(r.entries(t)) != 0; time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the marker was not taken")
		}
	}
	select {
	case <-done:
		t.Fatal("run ended before its context")
	case <-time.After(20 * time.Millisecond):
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("run did not end")
	}
	if got := r.settle(); len(got) != 1 {
		t.Fatalf("checks = %q", got)
	}
}

// Entries go relative to the folder that was opened: a folder swapped for a link after the opening neither
// redirects the removal nor lets the link's target be touched.
func TestRemovalIsRelativeToTheOpenedFolder(t *testing.T) {
	r := newMarkRig(t)
	r.touch(t, "demo")
	r.touch(t, "UPPER")
	other := t.TempDir()
	keep := filepath.Join(other, "UPPER")
	keepMarker := filepath.Join(other, "demo")
	for _, p := range []string{keep, keepMarker} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	moved := r.dir + ".moved"
	r.m.afterOpen = func() {
		if err := os.Rename(r.dir, moved); err != nil {
			t.Error(err)
			return
		}
		if err := os.Symlink(other, r.dir); err != nil {
			t.Error(err)
		}
	}
	r.scan()
	if got := r.settle(); len(got) != 1 || got[0] != "demo" {
		t.Fatalf("checks = %q", got)
	}
	for _, p := range []string{keep, keepMarker} {
		if b, err := os.ReadFile(p); err != nil || string(b) != "x" {
			t.Fatalf("%s in the linked folder was touched: %q %v", p, b, err)
		}
	}
	if list, _ := os.ReadDir(moved); len(list) != 0 {
		t.Fatalf("left in the opened folder: %v", list)
	}
	// The next pass finds the link and refuses it.
	r.m.afterOpen = nil
	r.scan()
	if _, err := os.Stat(keep); err != nil || len(r.settle()) != 1 {
		t.Fatalf("the link was followed: %v", err)
	}
}
