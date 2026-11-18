package service

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

// maxConns bounds the connections served at once; further ones are closed unanswered.
const maxConns = 32

// PeerCheck admits the peer of a connection and returns its user ID. It runs once per connection, before
// anything is read; what that user may do is decided again for every request.
type PeerCheck func(conn net.Conn) (uid uint32, err error)

// kernelPeer is the default PeerCheck: it admits a peer whose user the kernel reports and returns that user.
func kernelPeer(conn net.Conn) (uint32, error) {
	uid, err := localipc.PeerUID(conn)
	if err != nil {
		return 0, control.ErrNoAccess()
	}
	return uid, nil
}

// reading and writing mark a handler with the access its command needs; see control.Access.
func reading(h control.Handler) control.Command {
	return control.Command{Access: control.AccessRead, Handle: h}
}

func writing(h control.Handler) control.Command {
	return control.Command{Access: control.AccessWrite, Handle: h}
}

func proposing(h control.Handler) control.Command {
	return control.Command{Access: control.AccessPropose, Handle: h}
}

// authorizer decides whether the user uid may run a command that needs access; it answers a refusal as a
// *control.Error.
type authorizer func(uid uint32, access control.Access) error

type server struct {
	registry  *control.Registry
	check     PeerCheck
	authorize authorizer
	log       *slog.Logger

	mu    sync.Mutex
	conns map[net.Conn]struct{}
	wg    sync.WaitGroup
}

// serve accepts connections on l until ctx ends, then closes l and every open connection and waits for the
// handlers. It owns l.
func serve(ctx context.Context, l net.Listener, registry *control.Registry, check PeerCheck, authorize authorizer, log *slog.Logger) {
	s := &server{registry: registry, check: check, authorize: authorize, log: log, conns: map[net.Conn]struct{}{}}
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
				log.Warn("refused peer", "pid", refused.PID)
				continue
			case ctx.Err() != nil || errors.Is(err, net.ErrClosed):
				return
			}
			log.Error("accept failed", "error", err)
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
	uid, err := s.check(conn)
	if err == nil {
		// A peer without any access is refused before anything is read.
		err = s.authorize(uid, control.AccessRead)
	}
	if err != nil {
		s.respond(conn, errorResponse(peerRefusal(err)))
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
				s.respond(conn, errorResponse(control.NewError(control.CodeTooLarge, "request is too large")))
				linger(conn)
			} else if !errors.Is(err, io.EOF) && ctx.Err() == nil && !errors.Is(err, os.ErrDeadlineExceeded) {
				s.respond(conn, errorResponse(control.NewError(control.CodeBadRequest, "request is incomplete")))
				linger(conn)
			}
			return
		}
		resp, keep := s.dispatch(ctx, uid, raw)
		if !s.respond(conn, resp) {
			return
		}
		if !keep {
			linger(conn)
			return
		}
	}
}

// peerRefusal is the answer to a refused peer; an error without a fixed answer refuses all access.
func peerRefusal(err error) *control.Error {
	var ce *control.Error
	if !errors.As(err, &ce) {
		ce = control.ErrNoAccess()
	}
	return ce
}

// dispatch answers one request of the user uid. keep is false when the stream cannot be trusted any longer or
// the user lost all access.
func (s *server) dispatch(ctx context.Context, uid uint32, raw []byte) (resp control.Response, keep bool) {
	var req control.Request
	if err := json.Unmarshal(raw, &req); err != nil || req.Command == "" {
		return errorResponse(control.NewError(control.CodeBadRequest, "request is not a valid command")), false
	}
	if req.V != control.Version {
		return errorResponse(control.NewError(control.CodeVersion,
			fmt.Sprintf("protocol version %d is not supported; this service speaks version %d", req.V, control.Version))), false
	}
	// Access is decided before anything of the command runs, from the group database as it is now. An unknown
	// command needs read access, so a peer without access learns nothing.
	command, ok := s.registry.Lookup(req.Command)
	access := control.AccessRead
	if ok {
		access = command.AccessFor(req.Args)
	}
	if err := s.authorize(uid, access); err != nil {
		ce := peerRefusal(err)
		return errorResponse(ce), ce.Code != control.CodeNoAccess
	}
	if !ok {
		return errorResponse(control.NewError(control.CodeUnknownCommand, "unknown command")), true
	}
	return s.run(control.WithPeer(ctx, uid), command.Handle, req.Args), true
}

func (s *server) run(ctx context.Context, handler control.Handler, args json.RawMessage) (resp control.Response) {
	defer func() {
		if r := recover(); r != nil {
			s.log.Error("command panicked", "panic", fmt.Sprint(r))
			resp = errorResponse(control.NewError(control.CodeInternal, "internal error"))
		}
	}()
	value, err := handler(ctx, args)
	if err != nil {
		var ce *control.Error
		if errors.As(err, &ce) {
			return errorResponse(ce)
		}
		s.log.Error("command failed", "error", err)
		return errorResponse(control.NewError(control.CodeInternal, "internal error"))
	}
	result, err := json.Marshal(value)
	if err != nil {
		s.log.Error("command result cannot be encoded", "error", err)
		return errorResponse(control.NewError(control.CodeInternal, "internal error"))
	}
	return control.Response{V: control.Version, OK: true, Result: result}
}

func errorResponse(e *control.Error) control.Response {
	return control.Response{V: control.Version, Error: e}
}

func (s *server) respond(conn net.Conn, resp control.Response) bool {
	_ = conn.SetWriteDeadline(time.Now().Add(control.WriteTimeout))
	if err := control.WriteFrame(conn, resp, control.MaxResponseBytes); err != nil {
		if errors.Is(err, control.ErrTooLarge) {
			err = control.WriteFrame(conn, errorResponse(control.NewError(control.CodeTooLarge, "answer is too large")),
				control.MaxResponseBytes)
		}
		return err == nil
	}
	return true
}

// lingerWait bounds how long a refused or broken peer may keep sending after its answer.
const lingerWait = time.Second

// linger ends the answer and discards what the peer still sends for a moment. Closing at once would reset
// the connection and could destroy the answer before the peer reads it.
func linger(conn net.Conn) {
	if cw, ok := conn.(interface{ CloseWrite() error }); ok {
		_ = cw.CloseWrite()
	}
	_ = conn.SetReadDeadline(time.Now().Add(lingerWait))
	_, _ = io.Copy(io.Discard, io.LimitReader(conn, control.MaxRequestBytes))
}
