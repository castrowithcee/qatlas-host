package client

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/castrowithcee/qatlas-kit/localipc"

	"github.com/castrowithcee/qatlas-host/internal/channeltest"
	"github.com/castrowithcee/qatlas-host/internal/control"
)

// fakeService answers the first request on ep with answers and then waits until the client closes. It reports
// the request it read and whether the client closed.
func fakeService(t *testing.T, answers ...control.Response) (localipc.ServiceEndpoint, <-chan control.Request, <-chan struct{}) {
	t.Helper()
	ep := channeltest.Endpoint(t)
	l, err := ep.Listen()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	got, closed := make(chan control.Request, 1), make(chan struct{})
	go func() {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		br := bufio.NewReader(conn)
		raw, err := control.ReadFrame(br, control.MaxRequestBytes)
		if err != nil {
			return
		}
		var req control.Request
		_ = json.Unmarshal(raw, &req)
		got <- req
		for _, a := range answers {
			if control.WriteFrame(conn, a, control.MaxResponseBytes) != nil {
				return
			}
		}
		if _, err := br.ReadByte(); errors.Is(err, io.EOF) {
			close(closed)
		}
	}()
	return ep, got, closed
}

func TestAServiceWithoutTheStreamAnswersUnknownCommandAsItIs(t *testing.T) {
	ep, got, _ := fakeService(t, control.Response{V: control.Version, Error: control.NewError(control.CodeUnknownCommand, "unknown command")})
	err := StreamUsage(context.Background(), ep, 7, func(control.HostUsageResult) error {
		t.Fatal("a value from an error")
		return nil
	})
	var ce *control.Error
	if !errors.As(err, &ce) || ce.Code != control.CodeUnknownCommand || ce.Message != "unknown command" {
		t.Fatalf("err = %v", err)
	}
	if req := <-got; req.Command != control.HostUsageStreamCommand || string(req.Args) != `{"interval_seconds":7}` {
		t.Fatalf("request = %+v", req)
	}
}

func TestTheEndOfTheContextEndsTheStreamAndClosesTheConnection(t *testing.T) {
	value := control.Response{V: control.Version, OK: true, Result: json.RawMessage(`{"cores":3}`)}
	ep, _, closed := fakeService(t, value, value)
	ctx, cancel := context.WithCancel(context.Background())
	var cores []int
	err := StreamUsage(ctx, ep, 0, func(u control.HostUsageResult) error {
		if cores = append(cores, u.Cores); len(cores) == 2 {
			cancel()
		}
		return nil
	})
	if !errors.Is(err, context.Canceled) || len(cores) != 2 || cores[0] != 3 {
		t.Fatalf("err = %v, cores %v", err, cores)
	}
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("the connection stays open")
	}
}
