// Package hostservice is the host service: a small service that runs as the system user qhost without access
// to the container engine. It answers the status command to root and refuses every other peer; the host
// operations follow later. It knows no package of the deploy part and never talks to the container engine.
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
	"sync"
	"time"

	"github.com/castrowithcee/qatlas-kit/localipc"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// DefaultStateDir is the state directory of the host service under systemd's StateDirectory convention.
const DefaultStateDir = "/var/lib/qhost"

// maxConns bounds the connections served at once; further ones are closed unanswered.
const maxConns = 8

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
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	l, err := cfg.Endpoint.Listen()
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	peer := cfg.peerUID
	if peer == nil {
		peer = localipc.PeerUID
	}
	s := &server{version: cfg.Version, started: now().UTC(), log: log, peer: peer, conns: map[net.Conn]struct{}{}}
	log.Info("host service started")
	s.serve(ctx, l)
	log.Info("host service stopped")
	return nil
}

type server struct {
	version string
	started time.Time
	log     *slog.Logger
	peer    func(net.Conn) (uint32, error)

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

func (s *server) handle(ctx context.Context, conn net.Conn) {
	defer s.wg.Done()
	defer func() {
		s.mu.Lock()
		delete(s.conns, conn)
		s.mu.Unlock()
		_ = conn.Close()
	}()
	uid, err := s.peer(conn)
	if err != nil {
		s.respond(conn, failure(control.ErrNoAccess()))
		linger(conn)
		return
	}
	// Only root is served, whatever the channel admitted; the answer is fixed, so nothing a peer sent is echoed.
	if uid != 0 {
		s.respond(conn, failure(control.ErrNotRoot()))
		linger(conn)
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
		if err != nil {
			if errors.Is(err, control.ErrTooLarge) {
				s.respond(conn, failure(control.NewError(control.CodeTooLarge, "request is too large")))
				linger(conn)
			} else if !errors.Is(err, io.EOF) && ctx.Err() == nil && !errors.Is(err, os.ErrDeadlineExceeded) {
				s.respond(conn, failure(control.NewError(control.CodeBadRequest, "request is incomplete")))
				linger(conn)
			}
			return
		}
		resp, keep := s.dispatch(raw)
		if !s.respond(conn, resp) {
			return
		}
		if !keep {
			linger(conn)
			return
		}
	}
}

// dispatch answers one request. keep is false when the stream cannot be trusted any longer.
func (s *server) dispatch(raw []byte) (resp control.Response, keep bool) {
	var req control.Request
	if err := json.Unmarshal(raw, &req); err != nil || req.Command == "" {
		return failure(control.NewError(control.CodeBadRequest, "request is not a valid command")), false
	}
	if req.V != control.Version {
		return failure(control.NewError(control.CodeVersion,
			fmt.Sprintf("protocol version %d is not supported; this service speaks version %d", req.V, control.Version))), false
	}
	if req.Command != control.StatusCommand {
		return failure(control.NewError(control.CodeUnknownCommand, "unknown command")), true
	}
	result, err := json.Marshal(StatusResult{Version: s.version, StartedAt: s.started})
	if err != nil {
		s.log.Error("status cannot be encoded", "error", err)
		return failure(control.NewError(control.CodeInternal, "internal error")), true
	}
	return control.Response{V: control.Version, OK: true, Result: result}, true
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
