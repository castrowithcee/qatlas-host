package service

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/backup"
	"github.com/castrowithcee/qatlas-host/internal/channeltest"
	"github.com/castrowithcee/qatlas-host/internal/formatnum"
	"github.com/castrowithcee/qatlas-host/internal/formatnum/formatnumtest"
)

func TestServiceWithUnreadableCoreStateDoesNotStartAndNamesTheFile(t *testing.T) {
	const doc = `{"version":99,"sources":[]}`
	state := filepath.Join(t.TempDir(), "state")
	path := filepath.Join(state, sourcesFile)
	formatnumtest.Seed(t, path, doc)
	var logged bytes.Buffer
	ep := channeltest.Endpoint(t)
	err := Run(context.Background(), slog.New(slog.NewTextHandler(&logged, nil)), Config{Version: "9.9.9", Endpoint: ep, StateDir: state})
	formatnumtest.Refused(t, err, path)
	if !strings.Contains(logged.String(), path) {
		t.Fatalf("the service log does not name the file: %s", logged.String())
	}
	formatnumtest.Unchanged(t, path, doc)
}

func TestCheckStateNamesTheUnreadableCoreFile(t *testing.T) {
	const id = "srv-stacks"
	sources := `{"version":2,"sources":[{"id":"` + id + `","path":"/srv/stacks","kind":"local"}]}`
	for name, tc := range map[string]struct{ file, doc string }{
		"sources":       {sourcesFile, `{"version":3,"sources":[]}`},
		"legacy":        {legacyFile, `{"url":"` + exampleURL + `","dir":"/srv/stacks"}`},
		"notifications": {"notify.json", `{"schema":9}`},
		"window":        {"window.json", `{"days":["mon"]}`},
		"engine state":  {filepath.Join(sourcesDir, id, "deploy", "state.json"), `{"schema":9}`},
	} {
		t.Run(name, func(t *testing.T) {
			state := t.TempDir()
			if tc.file != sourcesFile {
				put(t, filepath.Join(state, sourcesFile), sources)
			}
			path := filepath.Join(state, tc.file)
			formatnumtest.Seed(t, path, tc.doc)
			formatnumtest.Refused(t, CheckState(state), path)
			formatnumtest.Unchanged(t, path, tc.doc)
		})
	}
	state := t.TempDir()
	if err := CheckState(state); err != nil {
		t.Fatalf("an empty state is no problem: %v", err)
	}
	put(t, filepath.Join(state, sourcesFile), sources)
	if err := CheckState(state); err != nil {
		t.Fatal(err)
	}
}

func TestCheckStateReportsAMissingPermissionApart(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads every file")
	}
	state := t.TempDir()
	put(t, filepath.Join(state, sourcesFile), `{"version":2,"sources":[]}`)
	if err := os.Chmod(state, 0); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(state, 0o700) }()
	err := CheckState(state)
	if !errors.Is(err, fs.ErrPermission) || formatnum.Is(err) {
		t.Fatalf("got %v", err)
	}
}

func TestUnreadableBackupIsNamedInItsInfo(t *testing.T) {
	ferr := &formatnum.Error{File: "/var/lib/example/backups/web/20300101T000000Z/meta.json", Field: "schema", Found: 2, Present: true, Expected: 1}
	info := backupInfo(backup.Meta{ID: "20300101T000000Z", Stack: "web", Incomplete: true, Unsupported: ferr})
	if info.Unreadable == "" || !strings.Contains(info.Unreadable, ferr.File) || !info.Incomplete {
		t.Fatalf("info = %+v", info)
	}
}
