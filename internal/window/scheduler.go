package window

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/castrowithcee/qatlas-kit/configfile"

	"github.com/castrowithcee/qatlas-host/internal/formatnum"
)

const (
	fileName = "window.json"
	// fileSchema 2 added the mark of an own window; number 1 is still read and means the window follows the host.
	fileSchema = 2
	// maxWait bounds one wait, so a wall clock that jumps is noticed within it.
	maxWait = time.Hour
	// retryWait is the wait after the setting could not be read or the run could not be recorded.
	retryWait = time.Minute
	// defaultHostPoll is how often a running scheduler looks whether the host window file changed.
	defaultHostPoll = 10 * time.Second
)

// Origin tells where the window that applies to an instance comes from.
type Origin string

const (
	// OriginHost is the host window, or the default when the host has none.
	OriginHost Origin = "host"
	// OriginOwn is a window set for this instance.
	OriginOwn Origin = "own"
)

// Clock is the time source of the scheduler.
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

type realClock struct{}

func (realClock) Now() time.Time                         { return time.Now() }
func (realClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

// Job is the work of one opening. It runs in the scheduler's goroutine; open tells whether the window, as set
// now, is still open, and the job starts nothing new once it answers false. ctx ends when the service stops.
type Job func(ctx context.Context, open func() bool)

// Config is what New needs.
type Config struct {
	// StateDir is the service state directory; the setting is kept in a file there.
	StateDir string
	Job      Job
	// HostFile is the file of the host window; empty means the instance has no host window and keeps its own
	// stored one. Tests give it a file in a temporary folder.
	HostFile string
	// HostPoll is how often Run looks whether HostFile changed; zero means ten seconds.
	HostPoll time.Duration
	// Clock is replaced by tests; nil means the system clock.
	Clock  Clock
	Logger *slog.Logger
}

// Change names the parts of the window to replace; a nil part keeps its value.
type Change struct {
	Days     *Days
	Start    *int
	Duration *time.Duration
	Zone     *string
}

// Status is the window as set, whether it is open and when it opens next. OpenUntil is set while it is open;
// Next is the opening after the current one then. LastRun is when the work of an opening last began.
type Status struct {
	Window Window
	// Origin is where Window comes from. Warning is set when the host window file could not be used and the
	// default applies instead.
	Origin    Origin
	Warning   string
	Open      bool
	OpenUntil time.Time
	Next      time.Time
	LastRun   time.Time
	Running   bool
}

// Scheduler runs the job once per opening of the window. The setting and the start of the last run are kept
// in the service state, so a restart within an opening whose work already began does not begin it again.
type Scheduler struct {
	cfg     Config
	store   *configfile.Store[record]
	changed chan struct{}

	mu       sync.Mutex
	running  bool
	lastWarn string
}

// record is the stored setting with the start of the last run. Own tells that Window was set for this
// instance; otherwise the instance follows the host window and Window is only what applies while the host has none.
type record struct {
	Window  Window
	Own     bool
	LastRun time.Time
}

type recordFile struct {
	Schema   int       `json:"schema"`
	Own      bool      `json:"own,omitempty"`
	Days     []string  `json:"days"`
	Start    string    `json:"start"`
	Duration string    `json:"duration"`
	Zone     string    `json:"zone"`
	LastRun  time.Time `json:"last_run,omitzero"`
}

// New prepares the scheduler; it creates no file before the first change.
func New(cfg Config) (*Scheduler, error) {
	if cfg.StateDir == "" || cfg.Job == nil {
		return nil, errors.New("window: state directory and job are required")
	}
	if cfg.Clock == nil {
		cfg.Clock = realClock{}
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.DiscardHandler)
	}
	path := filepath.Join(cfg.StateDir, fileName)
	store, err := configfile.NewStore(path, configfile.Codec[record]{
		Validate: func(r record) error {
			if r.Window.check() != nil {
				return errMalformed
			}
			return nil
		},
		Encode: encodeRecord, Decode: func(b []byte) (record, error) { return decodeRecord(path, b) },
	})
	if err != nil {
		return nil, err
	}
	return &Scheduler{cfg: cfg, store: store, changed: make(chan struct{}, 1)}, nil
}

func encodeRecord(r record) ([]byte, error) {
	return json.MarshalIndent(recordFile{
		Schema: fileSchema, Own: r.Own, Days: r.Window.Days.Names(), Start: FormatStart(r.Window.Start),
		Duration: FormatDuration(r.Window.Duration), Zone: r.Window.Zone, LastRun: r.LastRun.UTC(),
	}, "", "  ")
}

// errMalformed is a stored setting that does not decode; it is no *InvalidError, which answers a request.
var errMalformed = errors.New("the stored window setting is malformed")

func decodeRecord(path string, b []byte) (record, error) {
	r, err := decodeFile(path, b)
	if formatnum.Is(err) {
		return record{}, err
	}
	if err != nil {
		return record{}, errMalformed
	}
	return r, nil
}

func decodeFile(path string, b []byte) (record, error) {
	if err := formatnum.CheckJSON(path, "schema", b, fileSchema, 1); err != nil {
		return record{}, err
	}
	var f recordFile
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return record{}, err
	}
	days, err := ParseDays(f.Days)
	if err != nil {
		return record{}, err
	}
	start, err := ParseStart(f.Start)
	if err != nil {
		return record{}, err
	}
	d, err := ParseDuration(f.Duration)
	if err != nil {
		return record{}, err
	}
	if !validZoneName(f.Zone) {
		return record{}, invalid("invalid zone")
	}
	return record{Window: Window{Days: days, Start: start, Duration: d, Zone: f.Zone}, Own: f.Own, LastRun: f.LastRun}, nil
}

// load returns the stored record; a missing file is the default window that never ran.
func (s *Scheduler) load() (record, error) {
	r, _, err := s.store.LoadVersioned()
	if errors.Is(err, fs.ErrNotExist) {
		return record{Window: Default()}, nil
	}
	return r, err
}

func (s *Scheduler) update(fn func(record) (record, error)) (record, error) {
	var out record
	_, err := s.store.Update(func(r record) (record, error) {
		if r.Window == (Window{}) {
			r.Window = Default()
		}
		r, err := fn(r)
		out = r
		return r, err
	})
	return out, err
}

// Status returns the window as set and its openings as of now.
func (s *Scheduler) Status() (Status, error) {
	r, err := s.load()
	if err != nil {
		return Status{}, err
	}
	return s.status(r)
}

// effective is the window that applies: the own one, else the host window, else the default. A host window
// file that cannot be used gives the default and a warning, which is logged once until it changes.
func (s *Scheduler) effective(r record) (w Window, origin Origin, warning string) {
	if r.Own {
		return r.Window, OriginOwn, ""
	}
	if s.cfg.HostFile == "" {
		return r.Window, OriginHost, ""
	}
	host, ok, err := ReadHost(s.cfg.HostFile)
	switch {
	case err != nil:
		warning = "the host window file cannot be used, so the default window applies: " + err.Error()
		s.warnOnce(warning)
		return Default(), OriginHost, warning
	case ok:
		s.warnOnce("")
		return host, OriginHost, ""
	}
	s.warnOnce("")
	return r.Window, OriginHost, ""
}

func (s *Scheduler) warnOnce(text string) {
	s.mu.Lock()
	changed := s.lastWarn != text
	s.lastWarn = text
	s.mu.Unlock()
	if changed && text != "" {
		s.cfg.Logger.Warn(text)
	}
}

func (s *Scheduler) status(r record) (Status, error) {
	now := s.cfg.Clock.Now()
	w, origin, warning := s.effective(r)
	st := Status{Window: w, Origin: origin, Warning: warning, LastRun: r.LastRun}
	cur, open, err := w.At(now)
	if err != nil {
		return Status{}, err
	}
	next, err := w.Next(now)
	if err != nil {
		return Status{}, err
	}
	if open {
		st.Open, st.OpenUntil = true, cur.End
	}
	st.Next = next.Start
	if !st.LastRun.IsZero() {
		st.LastRun = st.LastRun.In(next.Start.Location())
	}
	s.mu.Lock()
	st.Running = s.running
	s.mu.Unlock()
	return st, nil
}

// Set replaces the named parts of the window that applies and makes the result the own window of this
// instance. It returns the new status. A zone is checked against the zone database. The scheduler picks the
// change up at once: a wait is computed again, and a running job sees the new end through open.
func (s *Scheduler) Set(c Change) (Status, error) {
	if c.Days == nil && c.Start == nil && c.Duration == nil && c.Zone == nil {
		return Status{}, invalid("name at least one part of the window to change")
	}
	r, err := s.update(func(r record) (record, error) {
		w, _, _ := s.effective(r)
		w, err := c.Apply(w)
		if err != nil {
			return r, err
		}
		r.Window, r.Own = w, true
		return r, nil
	})
	if err != nil {
		return Status{}, err
	}
	s.nudge()
	return s.status(r)
}

// Reset removes the own window, so the instance follows the host window again.
func (s *Scheduler) Reset() (Status, error) {
	r, err := s.update(func(r record) (record, error) {
		r.Window, r.Own = Default(), false
		return r, nil
	})
	if err != nil {
		return Status{}, err
	}
	s.nudge()
	return s.status(r)
}

func (s *Scheduler) nudge() {
	select {
	case s.changed <- struct{}{}:
	default:
	}
}

// open tells whether the window as stored now is open. A setting that cannot be read counts as closed.
func (s *Scheduler) open() bool {
	r, err := s.load()
	if err != nil {
		return false
	}
	w, _, _ := s.effective(r)
	_, ok, err := w.At(s.cfg.Clock.Now())
	return err == nil && ok
}

// Run waits for the openings and runs the job in each until ctx ends. The job runs at most once per opening:
// when the window is open and no run began since the opening started. The start of a run is stored before the
// job begins, so a run cut short by a stop is not repeated in the same opening.
func (s *Scheduler) Run(ctx context.Context) {
	if s.cfg.HostFile != "" {
		go s.watchHost(ctx, hostStamp(s.cfg.HostFile))
	}
	for ctx.Err() == nil {
		wait := s.cycle(ctx)
		if wait <= 0 {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-s.changed:
		case <-s.cfg.Clock.After(min(wait, maxWait)):
		}
	}
}

// cycle runs the job if it is due and returns how long to wait before looking again.
func (s *Scheduler) cycle(ctx context.Context) time.Duration {
	log := s.cfg.Logger
	r, err := s.load()
	if err != nil {
		log.Error("the maintenance window setting cannot be read; no window runs until it is fixed", "error", err)
		return retryWait
	}
	now := s.cfg.Clock.Now()
	w, _, _ := s.effective(r)
	cur, open, err := w.At(now)
	if err != nil {
		log.Error("the maintenance window cannot be computed", "error", err)
		return retryWait
	}
	if open && r.LastRun.Before(cur.Start) {
		// Running is set first, so no status shows the new start without it.
		s.setRunning(true)
		defer s.setRunning(false)
		if _, err := s.update(func(r record) (record, error) {
			r.LastRun = now.UTC()
			return r, nil
		}); err != nil {
			log.Error("the start of the maintenance window run cannot be recorded; it is not run", "error", err)
			return retryWait
		}
		log.Info("maintenance window opened", "until", cur.End.Format(time.RFC3339))
		s.cfg.Job(ctx, s.open)
		log.Info("maintenance window run finished")
		return 0
	}
	next, err := w.Next(now)
	if err != nil {
		log.Error("the maintenance window cannot be computed", "error", err)
		return retryWait
	}
	return next.Start.Sub(now)
}

// watchHost wakes the scheduler when the host window file appears, changes or goes, so a new host window applies
// without a restart. It looks at the real time, not the scheduler's clock, because it watches a file.
func (s *Scheduler) watchHost(ctx context.Context, last string) {
	every := s.cfg.HostPoll
	if every <= 0 {
		every = defaultHostPoll
	}
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if now := hostStamp(s.cfg.HostFile); now != last {
				last = now
				s.nudge()
			}
		}
	}
}

// hostStamp identifies the state of the file: its modification time, size and inode, or a fixed text without it.
func hostStamp(path string) string {
	fi, err := os.Stat(path)
	if err != nil {
		return "none"
	}
	ino := uint64(0)
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		ino = st.Ino
	}
	return fmt.Sprintf("%d/%d/%d", fi.ModTime().UnixNano(), fi.Size(), ino)
}

func (s *Scheduler) setRunning(on bool) {
	s.mu.Lock()
	s.running = on
	s.mu.Unlock()
}

// CheckFormat checks the format number of the stored window setting in stateDir without changing it.
func CheckFormat(stateDir string) error {
	return formatnum.CheckFile(filepath.Join(stateDir, fileName), "schema", fileSchema, 1)
}
