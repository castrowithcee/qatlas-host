package hostservice

import (
	"bufio"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/proposals"
)

// maxStreams bounds the streams open at once, so some of the maxConns connections stay free for other requests.
const maxStreams = 4

// usageStream is a started stream of the load of the host; it holds one of the maxStreams slots.
type usageStream struct {
	interval time.Duration
}

func realTicker(d time.Duration) (<-chan time.Time, func()) {
	t := time.NewTicker(d)
	return t.C, t.Stop
}

func (s *server) readUsage(ctx context.Context, _ json.RawMessage) (any, error) {
	return s.usageSource.Read(ctx)
}

// startUsageStream takes a slot for a stream and measures its first answer. The caller serves the rest of the
// stream and frees the slot.
func (s *server) startUsageStream(c *call) (any, error) {
	var a control.HostUsageStreamArgs
	if len(c.args) > 0 && proposals.DecodeStrict(c.args, &a) != nil {
		return nil, badRequest()
	}
	seconds := cmp.Or(a.IntervalSeconds, control.HostUsageStreamDefaultSeconds)
	if seconds < control.HostUsageStreamMinSeconds || seconds > control.HostUsageStreamMaxSeconds {
		return nil, control.NewError(control.CodeInvalid, fmt.Sprintf("the interval must be %d to %d seconds",
			control.HostUsageStreamMinSeconds, control.HostUsageStreamMaxSeconds))
	}
	s.mu.Lock()
	full := s.streams >= maxStreams
	if !full {
		s.streams++
	}
	s.mu.Unlock()
	if full {
		return nil, control.NewError(control.CodeBusy, fmt.Sprintf("%d usage streams are open, the most the service serves at once", maxStreams))
	}
	c.stream = &usageStream{interval: time.Duration(seconds) * time.Second}
	return s.usageSource.Read(context.Background())
}

// streamUsage sends first and then one measurement per tick. It ends silently when the peer closes the
// connection or a write fails, and with an error when the service stops or the peer is no administrator any
// more. The one log entry of the stream is written already; it frees the slot of the stream.
func (s *server) streamUsage(ctx context.Context, conn net.Conn, br *bufio.Reader, uid uint32, st *usageStream, first control.Response) {
	s.mu.Lock()
	s.conns[conn] = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.streams--
		s.mu.Unlock()
	}()
	if !s.respond(conn, first) || !first.OK {
		return
	}
	// The peer sends nothing after its request, so a read returns only when it closes or breaks the protocol;
	// either ends the stream at once instead of at a write up to an interval later. The read ends when handle
	// closes the connection.
	gone := make(chan struct{})
	_ = conn.SetReadDeadline(time.Time{})
	go func() {
		defer close(gone)
		_, _ = br.ReadByte()
	}()
	ticks, stop := s.ticker(st.interval)
	defer stop()
	measure := control.Command{Handle: s.readUsage}
	for {
		select {
		case <-ctx.Done():
		case <-gone:
		case <-ticks:
		}
		var resp control.Response
		select {
		case <-ctx.Done():
			resp = failure(control.NewError(control.CodeStopping, "the service stops"))
		case <-gone:
			return
		default:
			if !s.admitted(uid) {
				resp = failure(control.ErrNotAdmin())
			} else {
				resp = s.run(measure, &call{uid: uid})
			}
		}
		if !s.respond(conn, resp) || !resp.OK {
			return
		}
	}
}
