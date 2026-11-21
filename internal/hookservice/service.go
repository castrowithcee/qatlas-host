package hookservice

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

const (
	maxBodyBytes  = 64 << 10
	hookInterval  = 5 * time.Second
	requestWindow = time.Minute
	maxRequests   = 600
	secretBytes   = 32

	pathPrefix = "/hook/"
	// DefaultReloadEvery is how often the hook table is read again.
	DefaultReloadEvery = 10 * time.Second
	summaryEvery       = time.Minute
	markerMode         = 0o660
)

// Options configures a Service. The zero value of an optional field means the production default.
type Options struct {
	ConfigPath string
	RunDir     string
	// OwnerUID is the user that must own the configuration file; production uses 0.
	OwnerUID    uint32
	ReloadEvery time.Duration
	Logger      *slog.Logger
	// Now is the clock; tests replace it.
	Now func() time.Time
}

type entry struct {
	hook Hook
	sum  [sha256.Size]byte
}

// Service receives the webhooks.
type Service struct {
	opts   Options
	log    *slog.Logger
	listen string

	table atomic.Pointer[[]entry]

	mu         sync.Mutex
	lastMark   map[string]time.Time
	windowFrom time.Time
	windowN    int
	lastErr    string

	rejected atomic.Int64
}

// New loads the configuration; a configuration that fails the checks stops the start.
func New(opts Options) (*Service, error) {
	if opts.ConfigPath == "" {
		opts.ConfigPath = DefaultConfigPath
	}
	if opts.RunDir == "" {
		opts.RunDir = DefaultRunDir
	}
	if opts.ReloadEvery <= 0 {
		opts.ReloadEvery = DefaultReloadEvery
	}
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	s := &Service{opts: opts, log: opts.Logger, lastMark: map[string]time.Time{}}
	cfg, err := LoadConfig(opts.ConfigPath, opts.OwnerUID)
	if err != nil {
		return nil, err
	}
	s.listen = cfg.Listen
	s.setTable(cfg.Hooks)
	return s, nil
}

func (s *Service) setTable(hooks []Hook) {
	t := make([]entry, 0, len(hooks))
	for _, h := range hooks {
		e := entry{hook: h}
		// The hash is validated hex, so the decoding cannot fail; an entry that did would never match.
		raw, _ := hex.DecodeString(strings.TrimPrefix(h.Hash, "sha256:"))
		copy(e.sum[:], raw)
		t = append(t, e)
	}
	s.table.Store(&t)
}

// Reload reads the configuration again. A refused file leaves the previous table in place. The listen address
// is not applied; it needs a restart.
func (s *Service) Reload() error {
	cfg, err := LoadConfig(s.opts.ConfigPath, s.opts.OwnerUID)
	if err != nil {
		return err
	}
	s.setTable(cfg.Hooks)
	return nil
}

func (s *Service) reloadLoop(ctx context.Context) {
	t := time.NewTicker(s.opts.ReloadEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			err := s.Reload()
			msg := ""
			if err != nil {
				msg = err.Error()
			}
			s.mu.Lock()
			changed := msg != s.lastErr
			s.lastErr = msg
			s.mu.Unlock()
			if err != nil && changed {
				s.log.Warn("hook table not reloaded, keeping the previous one", "error", msg)
			}
		}
	}
}

// find compares the hash of candidate with every entry in constant time, without stopping at a match.
func (s *Service) find(candidate string) (Hook, bool) {
	sum := sha256.Sum256([]byte(candidate))
	var found Hook
	match := 0
	for _, e := range *s.table.Load() {
		if subtle.ConstantTimeCompare(sum[:], e.sum[:]) == 1 {
			found = e.hook
			match = 1
		}
	}
	return found, match == 1
}

// wellFormed reports whether s could be a secret: 32 bytes as base64url without padding.
func wellFormed(secret string) bool {
	if base64.RawURLEncoding.EncodedLen(secretBytes) != len(secret) {
		return false
	}
	b, err := base64.RawURLEncoding.Strict().DecodeString(secret)
	return err == nil && len(b) == secretBytes
}

// admit applies the overall limit of 600 requests per minute.
func (s *Service) admit(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if now.Sub(s.windowFrom) >= requestWindow || now.Before(s.windowFrom) {
		s.windowFrom, s.windowN = now, 0
	}
	if s.windowN >= maxRequests {
		return false
	}
	s.windowN++
	return true
}

// due reports whether the hook may drop a marker now and, if so, claims the slot.
func (s *Service) due(id string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if last, ok := s.lastMark[id]; ok && now.Sub(last) < hookInterval && !now.Before(last) {
		return false
	}
	s.lastMark[id] = now
	return true
}

func (s *Service) release(id string) {
	s.mu.Lock()
	delete(s.lastMark, id)
	s.mu.Unlock()
}

// Handler is the HTTP surface: only POST /hook/<secret>.
func (s *Service) Handler() http.Handler {
	return http.HandlerFunc(s.serve)
}

func (s *Service) serve(w http.ResponseWriter, r *http.Request) {
	now := s.opts.Now()
	if !s.admit(now) {
		s.rejected.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}
	secret, ok := strings.CutPrefix(r.URL.Path, pathPrefix)
	if r.Method != http.MethodPost || !ok || !wellFormed(secret) {
		s.reject(w)
		return
	}
	hook, known := s.find(secret)
	if !known {
		s.reject(w)
		return
	}
	// Read and discard the body, at most 64 KiB; nothing of it, of the headers or of the query is kept.
	_, _ = io.Copy(io.Discard, io.LimitReader(r.Body, maxBodyBytes))
	if s.due(hook.ID, now) {
		if err := writeMarker(s.opts.RunDir, hook); err != nil {
			s.release(hook.ID)
			s.log.Error("hook marker not written", "hook", hook.ID, "stack", hook.Stack, "error", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}
	s.log.Info("hook accepted", "hook", hook.ID, "stack", hook.Stack)
	w.WriteHeader(http.StatusAccepted)
}

func (s *Service) reject(w http.ResponseWriter) {
	s.rejected.Add(1)
	w.WriteHeader(http.StatusNotFound)
}

// flushRejected logs the rejected requests since the last call as one line.
func (s *Service) flushRejected() {
	if n := s.rejected.Swap(0); n > 0 {
		s.log.Info("hook requests rejected", "count", n)
	}
}

// writeMarker creates the empty marker <runDir>/<instance>/<stack>. Instance and stack come from the loaded
// configuration and are checked against their patterns again here. A symlink at the marker is not followed.
func writeMarker(runDir string, h Hook) error {
	if err := h.validate(); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(runDir, h.Instance, h.Stack), syscall.O_CREAT|syscall.O_WRONLY|syscall.O_NOFOLLOW, markerMode)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.Mode().Perm() != markerMode {
		if err := f.Chmod(markerMode); err != nil {
			return fmt.Errorf("marker mode: %w", err)
		}
	}
	return nil
}

func (s *Service) newHTTPServer() *http.Server {
	return &http.Server{
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    8 << 10,
		ErrorLog:          slog.NewLogLogger(s.log.Handler(), slog.LevelWarn),
	}
}

// Serve answers on ln until ctx ends, reloads the hook table and summarizes rejections.
func (s *Service) Serve(ctx context.Context, ln net.Listener) error {
	srv := s.newHTTPServer()
	ctx, stop := context.WithCancel(ctx)
	defer stop()
	go s.reloadLoop(ctx)
	go func() {
		t := time.NewTicker(summaryEvery)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.flushRejected()
			}
		}
	}()
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	s.log.Info("service started", "listen", ln.Addr().String())
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdown); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	s.flushRejected()
	return nil
}

// Run loads the configuration, listens on its address and serves until ctx ends.
func Run(ctx context.Context, opts Options) error {
	s, err := New(opts)
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", s.listen)
	if err != nil {
		return fmt.Errorf("hook service: %w", err)
	}
	defer ln.Close()
	return s.Serve(ctx, ln)
}
