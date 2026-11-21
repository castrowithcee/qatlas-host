package service

import (
	"context"
	"io"
	"log/slog"
	"os"
	"regexp"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/sys/unix"

	"github.com/castrowithcee/qatlas-host/internal/engine"
)

const (
	// markInterval is the time between looks into the marker folder; a marker is acted on within it.
	markInterval = 2 * time.Second
	// markBatch is the most entries one pass takes; the rest waits for the next pass.
	markBatch = 100
	// markLogEvery is the shortest time between two log lines about foreign entries.
	markLogEvery = time.Minute
	// markNameLen is the most characters of an entry name a log line shows.
	markNameLen = 40
	// markStuckMax bounds the memory of folders that could not be removed.
	markStuckMax = 1000
)

// markerName is what a stack name looks like: the Compose project name pattern.
var markerName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// markerService takes the markers the webhook receiver leaves in its folder: one file per stack, named like the
// stack. Everything in the folder is untrusted: an entry is never followed and never opened, its type comes from
// the directory listing, and it is removed with unlinkat relative to the opened folder, which removes a link itself
// and not its target. A marker is removed before the stack is checked, and several markers of one stack in a
// pass make one check.
type markerService struct {
	dir      string
	clock    engine.Clock
	interval time.Duration
	check    func(ctx context.Context, name string)
	log      *slog.Logger

	// afterOpen, when set, runs once the folder is open and before it is read. Tests only.
	afterOpen func()

	wg sync.WaitGroup // the checks started; run waits for them

	// Only the loop and, for active, the check goroutines touch these.
	state   string // "", "ok", "missing" or "unreadable": what the last pass found of the folder
	lastLog time.Time
	stuck   map[string]bool // folders in the marker folder that are not empty, so removal fails

	mu     sync.Mutex
	active map[string]bool // stacks whose check is running
}

func newMarkerService(dir string, clock engine.Clock, interval time.Duration, check func(context.Context, string),
	log *slog.Logger) *markerService {
	if clock == nil {
		clock = systemClock{}
	}
	if interval <= 0 {
		interval = markInterval
	}
	return &markerService{dir: dir, clock: clock, interval: interval, check: check, log: log,
		stuck: map[string]bool{}, active: map[string]bool{}}
}

// run takes the markers until ctx ends and returns once the checks it started have ended, which they do with ctx.
func (m *markerService) run(ctx context.Context) {
	defer m.wg.Wait()
	for ctx.Err() == nil {
		m.scan(ctx)
		select {
		case <-m.clock.After(m.interval):
		case <-ctx.Done():
		}
	}
}

// scan makes one pass over the folder: markers start a check each, anything else is removed. The folder is
// opened once, without following a link, and everything after that happens relative to that descriptor, so a
// folder swapped for a link meanwhile changes nothing: no path of the folder is resolved a second time.
func (m *markerService) scan(ctx context.Context) {
	f := m.open()
	if f == nil {
		return
	}
	defer func() { _ = f.Close() }()
	fd := int(f.Fd())
	if m.afterOpen != nil {
		m.afterOpen()
	}
	var (
		names     []string
		seen      = map[string]bool{}
		foreign   int
		firstName string
		stuckNow  = map[string]bool{}
		handled   int
		complete  bool
	)
	for handled < markBatch {
		entries, err := f.Readdirnames(markBatch)
		for _, name := range entries {
			if m.stuck[name] {
				stuckNow[name] = true
				continue
			}
			handled++
			var st unix.Stat_t
			if err := unix.Fstatat(fd, name, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
				continue // gone meanwhile
			}
			kind := st.Mode & unix.S_IFMT
			if kind == unix.S_IFREG && markerName.MatchString(name) {
				// Removing first means a marker laid down while the check runs asks for the next one.
				if err := unix.Unlinkat(fd, name, 0); err != nil {
					if err != unix.ENOENT {
						m.problem("marker not removed", name, err)
					}
					continue
				}
				if !seen[name] {
					seen[name] = true
					names = append(names, name)
				}
				continue
			}
			flags := 0
			if kind == unix.S_IFDIR {
				flags = unix.AT_REMOVEDIR
			}
			if err := unix.Unlinkat(fd, name, flags); err != nil {
				// Only an empty folder goes. A folder with content is left alone and never entered, and it
				// is remembered so it neither counts against the pass nor fills the log again.
				if kind == unix.S_IFDIR && len(m.stuck) < markStuckMax {
					m.stuck[name] = true
					stuckNow[name] = true
				}
				m.problem("foreign entry not removed", name, err)
				continue
			}
			foreign++
			if firstName == "" {
				firstName = name
			}
		}
		if err == io.EOF {
			complete = true
			break
		}
		if err != nil {
			m.problem("marker folder not read completely", "", err)
			break
		}
	}
	if complete {
		m.stuck = stuckNow // forget folders that are gone
	}
	if foreign > 0 {
		m.logRare("foreign entries removed from the webhook marker folder", "name", markName(firstName), "count", foreign)
	}
	for _, name := range names {
		m.start(ctx, name)
	}
}

// open returns the marker folder, or nil and notes the state. A link or a file in its place cannot be opened.
func (m *markerService) open() *os.File {
	fd, err := unix.Open(m.dir, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	switch {
	case err == unix.ENOENT:
		m.setState("missing", nil)
		return nil
	case err != nil:
		m.setState("unreadable", err)
		return nil
	}
	m.setState("ok", nil)
	return os.NewFile(uintptr(fd), m.dir)
}

// setState logs a change of what the folder offers, and nothing while it stays the same. A folder that is
// there from the start is not worth a line.
func (m *markerService) setState(state string, err error) {
	if state == m.state {
		return
	}
	prev := m.state
	m.state = state
	switch {
	case state == "ok" && prev != "":
		m.log.Info("webhook marker folder is available", "dir", m.dir)
	case state == "missing":
		m.log.Info("webhook marker folder does not exist; no markers are taken", "dir", m.dir)
	case state == "unreadable":
		args := []any{"dir", m.dir}
		if err != nil {
			args = append(args, "error", err)
		}
		m.log.Warn("webhook marker folder cannot be read; no markers are taken", args...)
	}
}

func (m *markerService) problem(msg, name string, err error) {
	m.logRare(msg, "name", markName(name), "error", err)
}

// logRare writes at most one line a minute about the folder's entries.
func (m *markerService) logRare(msg string, args ...any) {
	now := m.clock.Now()
	if !m.lastLog.IsZero() && now.Sub(m.lastLog) < markLogEvery {
		return
	}
	m.lastLog = now
	m.log.Warn(msg, args...)
}

// start checks the stack in a goroutine of its own, so a long update does not hold back the markers of other
// stacks. A stack whose check is running is not started again: the check joins the one running anyway.
func (m *markerService) start(ctx context.Context, name string) {
	m.mu.Lock()
	if m.active[name] {
		m.mu.Unlock()
		return
	}
	m.active[name] = true
	m.mu.Unlock()
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		defer func() {
			m.mu.Lock()
			delete(m.active, name)
			m.mu.Unlock()
		}()
		m.check(ctx, name)
	}()
}

// markName shortens an entry name for a log line and replaces what is not printable.
func markName(name string) string {
	var out []rune
	for i, n := 0, 0; i < len(name); i += n {
		r, size := utf8.DecodeRuneInString(name[i:])
		n = size
		if len(out) == markNameLen {
			out = append(out, '…')
			break
		}
		if (r == utf8.RuneError && size == 1) || !unicode.IsPrint(r) {
			r = '?'
		}
		out = append(out, r)
	}
	return string(out)
}
