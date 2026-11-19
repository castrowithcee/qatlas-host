// Package hostservice is the host service: a small service that runs as the system user qhost without root and
// without access to the container engine. It serves root and the administrators of the service group, checks
// the administrator right in the group database on every request, and keeps the action log as a hash chain in
// its state directory; it is the only writer of that log. It knows no package of the deploy part and never
// talks to the container engine.
package hostservice

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/castrowithcee/qatlas-kit/localipc"

	"github.com/castrowithcee/qatlas-host/internal/admins"
	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/host/auditlog"
)

// DefaultStateDir is the state directory of the host service under systemd's StateDirectory convention.
const DefaultStateDir = "/var/lib/qhost"

// LogDir is the directory of the action log inside the state directory stateDir.
func LogDir(stateDir string) string { return filepath.Join(stateDir, "logs") }

// maxConns bounds the connections served at once; further ones are closed unanswered.
const maxConns = 8

// noUID stands in for the user of a peer whose credentials could not be read.
const noUID = ^uint32(0)

// StatusResult answers control.StatusCommand.
type StatusResult struct {
	Version   string    `json:"version"`
	StartedAt time.Time `json:"started_at"`
}

// Config is what Run needs from its caller.
type Config struct {
	// Version is reported by the status command.
	Version string
	// Endpoint is the channel to listen on; control.HostEndpoint() in production.
	Endpoint localipc.ServiceEndpoint
	// StateDir is the state directory; it is created when missing. DefaultStateDir in production.
	StateDir string
	// GroupFile and PasswdFile are the group database of the administrator check; empty means /etc/group and
	// /etc/passwd.
	GroupFile, PasswdFile string
	// Now returns the current time; nil means the system clock.
	Now func() time.Time

	// peerUID returns the user of a peer; nil means the user the kernel reports. Tests set it.
	peerUID func(net.Conn) (uint32, error)
}

// Run serves the channel until ctx ends.
func Run(ctx context.Context, log *slog.Logger, cfg Config) error {
	if cfg.StateDir == "" {
		return errors.New("a state directory is required")
	}
	if err := os.MkdirAll(cfg.StateDir, 0o700); err != nil {
		return fmt.Errorf("prepare the state directory: %w", err)
	}
	audit, err := auditlog.Open(LogDir(cfg.StateDir))
	if err != nil {
		return err
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	peer := cfg.peerUID
	if peer == nil {
		peer = localipc.PeerUID
	}
	s := &server{
		version: cfg.Version, started: now().UTC(), log: log, peer: peer, audit: audit,
		logDir: LogDir(cfg.StateDir),
		db:     admins.Database{Group: cfg.GroupFile, Passwd: cfg.PasswdFile},
		conns:  map[net.Conn]struct{}{},
	}
	s.commands = s.registry()
	l, err := cfg.Endpoint.ListenWith(localipc.ListenOptions{Refused: s.refused})
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	log.Info("host service started")
	s.serve(ctx, l)
	log.Info("host service stopped")
	return nil
}

// auditLogger is where the server records requests; *auditlog.Logger satisfies it.
type auditLogger interface {
	Append(auditlog.Fields) error
}

type server struct {
	version  string
	started  time.Time
	log      *slog.Logger
	peer     func(net.Conn) (uint32, error)
	audit    auditLogger
	logDir   string
	db       admins.Database
	commands map[string]command

	mu    sync.Mutex
	conns map[net.Conn]struct{}
	wg    sync.WaitGroup
}

// serve accepts connections on l until ctx ends, then closes l and every open connection and waits for the
// handlers. It owns l.
func (s *server) serve(ctx context.Context, l net.Listener) {
	stop := context.AfterFunc(ctx, func() {
		_ = l.Close()
		s.mu.Lock()
		for c := range s.conns {
			_ = c.SetDeadline(time.Unix(1, 0))
		}
		s.mu.Unlock()
	})
	defer stop()
	defer s.wg.Wait()
	for {
		conn, err := l.Accept()
		if err != nil {
			var refused *localipc.PeerError
			switch {
			case errors.As(err, &refused):
				s.log.Warn("refused peer", "pid", refused.PID)
				continue
			case ctx.Err() != nil || errors.Is(err, net.ErrClosed):
				return
			}
			s.log.Error("accept failed", "error", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(100 * time.Millisecond):
			}
			continue
		}
		if ctx.Err() != nil || !s.track(conn) {
			_ = conn.Close()
			continue
		}
		go s.handle(ctx, conn)
	}
}

// refused records and answers a peer the channel turned away: one that is neither root nor carries the service
// group. It runs under the channel's own short deadline.
func (s *server) refused(conn net.Conn, _ error) {
	uid, err := localipc.PeerUID(conn)
	if err != nil {
		uid = noUID
	}
	s.record(auditlog.Fields{ClientUID: uid, Action: auditlog.OperationControlRefuse}, time.Now(), control.CodeNoAccess)
	_ = control.WriteFrame(conn, failure(control.ErrNoAccess()), control.MaxResponseBytes)
}

func (s *server) track(conn net.Conn) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.conns) >= maxConns {
		return false
	}
	s.conns[conn] = struct{}{}
	s.wg.Add(1)
	return true
}

// admitted reports whether uid may use the service now: root, or an administrator of the service group per the
// group database as it is now. Any doubt means no.
func (s *server) admitted(uid uint32) bool {
	if uid == 0 {
		return true
	}
	ok, err := s.db.Admin(uid)
	return err == nil && ok
}

// refuse records the refusal of a peer, answers it with e and ends the answer.
func (s *server) refuse(conn net.Conn, uid uint32, start time.Time, e *control.Error) {
	s.record(auditlog.Fields{ClientUID: uid, Action: auditlog.OperationControlRefuse}, start, e.Code)
	s.respond(conn, failure(e))
	linger(conn)
}

func (s *server) handle(ctx context.Context, conn net.Conn) {
	defer s.wg.Done()
	defer func() {
		s.mu.Lock()
		delete(s.conns, conn)
		s.mu.Unlock()
		_ = conn.Close()
	}()
	// The peer is checked before anything is read from it; the answers are fixed, so nothing a peer sent is echoed.
	uid, err := s.peer(conn)
	if err != nil {
		s.refuse(conn, noUID, time.Now(), control.ErrNoAccess())
		return
	}
	if !s.admitted(uid) {
		s.refuse(conn, uid, time.Now(), control.ErrNotAdmin())
		return
	}
	br := bufio.NewReaderSize(conn, 4096)
	for first := true; ; first = false {
		wait := control.IdleTimeout
		if first {
			wait = control.ReadTimeout
		}
		_ = conn.SetReadDeadline(time.Now().Add(wait))
		raw, err := control.ReadFrame(br, control.MaxRequestBytes)
		start := time.Now()
		if err != nil {
			var e *control.Error
			if errors.Is(err, control.ErrTooLarge) {
				e = control.NewError(control.CodeTooLarge, "request is too large")
			} else if !errors.Is(err, io.EOF) && ctx.Err() == nil && !errors.Is(err, os.ErrDeadlineExceeded) {
				e = control.NewError(control.CodeBadRequest, "request is incomplete")
			}
			if e != nil {
				s.record(auditlog.Fields{ClientUID: uid, Action: auditlog.OperationControlRequest}, start, e.Code)
				s.respond(conn, failure(e))
				linger(conn)
			}
			return
		}
		// The administrator right is read again for every request: a revocation applies to an open connection too.
		if !s.admitted(uid) {
			clear(raw)
			s.refuse(conn, uid, start, control.ErrNotAdmin())
			return
		}
		fields, code, resp, keep := s.dispatch(uid, raw)
		clear(raw) // a request may carry a secret
		s.record(fields, start, code)
		if !s.respond(conn, resp) {
			return
		}
		if !keep {
			linger(conn)
			return
		}
	}
}

// dispatch answers one request of the user uid and returns the fields of its log entry, which name the command
// and never an argument, and its error code, empty for success. keep is false when the stream cannot be trusted
// any longer.
func (s *server) dispatch(uid uint32, raw []byte) (fields auditlog.Fields, code string, resp control.Response, keep bool) {
	fields = auditlog.Fields{ClientUID: uid, Action: auditlog.OperationControlRequest}
	fail := func(e *control.Error, keep bool) (auditlog.Fields, string, control.Response, bool) {
		return fields, e.Code, failure(e), keep
	}
	var req control.Request
	if err := json.Unmarshal(raw, &req); err != nil || req.Command == "" {
		return fail(control.NewError(control.CodeBadRequest, "request is not a valid command"), false)
	}
	if req.V != control.Version {
		return fail(control.NewError(control.CodeVersion,
			fmt.Sprintf("protocol version %d is not supported; this service speaks version %d", req.V, control.Version)), false)
	}
	cmd, ok := s.commands[req.Command]
	if !ok {
		return fail(control.NewError(control.CodeUnknownCommand, "unknown command"), true)
	}
	if cmd.rootOnly && uid != 0 {
		fields.Action = auditlog.OperationControlRefuse
		return fail(control.ErrNotRoot(), true)
	}
	fields.Action = cmd.action
	c := &call{args: req.Args}
	if resp = s.run(cmd, c); resp.Error != nil {
		return fields, resp.Error.Code, resp, true
	}
	if c.change != nil {
		// The entry of a change names the human behind sudo as its client, not root who reported it.
		fields.ClientUID, fields.Host, code = c.changeUID, c.change, c.changeCode
	}
	return fields, code, resp, true
}

func (s *server) run(cmd command, c *call) (resp control.Response) {
	defer func() {
		if r := recover(); r != nil {
			s.log.Error("command panicked", "panic", fmt.Sprint(r))
			resp = failure(control.NewError(control.CodeInternal, "internal error"))
		}
	}()
	value, err := cmd.handle(c)
	if err != nil {
		var ce *control.Error
		if errors.As(err, &ce) {
			return failure(ce)
		}
		s.log.Error("command failed", "error", err)
		return failure(control.NewError(control.CodeInternal, "internal error"))
	}
	result, err := json.Marshal(value)
	if err != nil {
		s.log.Error("command result cannot be encoded", "error", err)
		return failure(control.NewError(control.CodeInternal, "internal error"))
	}
	return control.Response{V: control.Version, OK: true, Result: result}
}

// record appends the one entry a request leaves; code is empty for success. A failed write is reported and
// changes nothing else.
func (s *server) record(f auditlog.Fields, start time.Time, code string) {
	f.Result, f.Duration = auditlog.ResultSuccess, time.Since(start)
	if code != "" {
		f.Result, f.ErrorCode = auditlog.ResultFailure, code
	}
	if err := s.audit.Append(f); err != nil {
		s.log.Warn("cannot write the action log", "error", err.Error())
	}
}

func failure(e *control.Error) control.Response {
	return control.Response{V: control.Version, Error: e}
}

func (s *server) respond(conn net.Conn, resp control.Response) bool {
	_ = conn.SetWriteDeadline(time.Now().Add(control.WriteTimeout))
	return control.WriteFrame(conn, resp, control.MaxResponseBytes) == nil
}

// lingerWait bounds how long a refused or broken peer may keep sending after its answer.
const lingerWait = time.Second

// linger ends the answer and discards what the peer still sends for a moment, so closing does not reset the
// connection before the peer has read the answer.
func linger(conn net.Conn) {
	if cw, ok := conn.(interface{ CloseWrite() error }); ok {
		_ = cw.CloseWrite()
	}
	_ = conn.SetReadDeadline(time.Now().Add(lingerWait))
	_, _ = io.Copy(io.Discard, io.LimitReader(conn, control.MaxRequestBytes))
}
