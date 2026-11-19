package hostservice

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/castrowithcee/qatlas-host/internal/channeltest"
	"github.com/castrowithcee/qatlas-host/internal/client"
	"github.com/castrowithcee/qatlas-host/internal/control"
)

func start(t *testing.T, uid uint32) (stateDir string, ep func(context.Context) error) {
	t.Helper()
	endpoint := channeltest.Endpoint(t)
	stateDir = filepath.Join(t.TempDir(), "state")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, slog.New(slog.NewTextHandler(io.Discard, nil)), Config{
			Version: "v9.9.9", Endpoint: endpoint, StateDir: stateDir,
			Now:     func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) },
			peerUID: func(net.Conn) (uint32, error) { return uid, nil },
		})
	}()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("run: %v", err)
		}
	})
	channeltest.WaitReady(t, endpoint)
	return stateDir, func(ctx context.Context) error {
		var res StatusResult
		err := client.Do(ctx, endpoint, 5*time.Second, control.StatusCommand, nil, &res)
		if err == nil && (res.Version != "v9.9.9" || !res.StartedAt.Equal(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))) {
			return errors.New("unexpected status " + res.Version + " " + res.StartedAt.String())
		}
		return err
	}
}

func TestRootGetsTheStatus(t *testing.T) {
	stateDir, status := start(t, 0)
	if err := status(context.Background()); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(stateDir); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("state directory: %v, %v", info, err)
	}
}

func TestAnotherUserIsToldItIsNotRoot(t *testing.T) {
	_, status := start(t, 1000)
	var remote *control.Error
	if err := status(context.Background()); !errors.As(err, &remote) || remote.Code != control.CodeNotRoot {
		t.Fatalf("err = %v, want code %s", err, control.CodeNotRoot)
	}
}

func TestOtherCommandsAreUnknown(t *testing.T) {
	endpoint := channeltest.Endpoint(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, slog.New(slog.NewTextHandler(io.Discard, nil)), Config{Endpoint: endpoint,
			StateDir: filepath.Join(t.TempDir(), "state"), peerUID: func(net.Conn) (uint32, error) { return 0, nil }})
	}()
	defer func() { cancel(); <-done }()
	channeltest.WaitReady(t, endpoint)
	var out json.RawMessage
	var remote *control.Error
	err := client.Do(context.Background(), endpoint, 5*time.Second, "stack.list", nil, &out)
	if !errors.As(err, &remote) || remote.Code != control.CodeUnknownCommand {
		t.Fatalf("err = %v, want code %s", err, control.CodeUnknownCommand)
	}
}
