package client

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/castrowithcee/qatlas-kit/localipc"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// StreamUsage opens a stream of the load of the host on a connection of its own and calls each with every
// measurement: the first right away, then one every seconds seconds, 0 meaning the default of the service. It
// returns the error of ctx when ctx ends, the *control.Error of the answer that ended the stream, or the error
// of each, and closes the connection. A service without the stream answers control.CodeUnknownCommand, which
// comes back as it is, so the caller can fall back to single reads.
func StreamUsage(ctx context.Context, ep localipc.ServiceEndpoint, seconds int, each func(control.HostUsageResult) error) error {
	req, err := request(ctx, control.HostUsageStreamCommand, control.HostUsageStreamArgs{IntervalSeconds: seconds})
	if err != nil {
		return err
	}
	c, err := Dial(ctx, ep)
	if err != nil {
		return err
	}
	defer c.Close()
	_ = c.conn.SetDeadline(time.Now().Add(CallTimeout))
	stop := context.AfterFunc(ctx, func() { _ = c.conn.SetDeadline(time.Unix(1, 0)) })
	defer stop()

	writeErr := control.WriteFrame(c.conn, req, control.MaxRequestBytes)
	// A measurement may take a moment on top of the interval.
	wait := time.Duration(cmp.Or(seconds, control.HostUsageStreamDefaultSeconds))*time.Second + CallTimeout
	for {
		raw, err := control.ReadFrame(c.br, control.MaxResponseBytes)
		switch {
		case ctx.Err() != nil:
			return ctx.Err()
		case err != nil && writeErr != nil:
			return fmt.Errorf("send the request: %w", writeErr)
		case errors.Is(err, io.EOF):
			return errors.New("the service ended the stream without a reason")
		case err != nil:
			return fmt.Errorf("read the stream: %w", err)
		}
		var u control.HostUsageResult
		if err := decode(raw, &u); err != nil {
			return err
		}
		if err := each(u); err != nil {
			return err
		}
		// Checked after the deadline is set: an end of ctx before it would otherwise be overwritten.
		_ = c.conn.SetReadDeadline(time.Now().Add(wait))
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
}
