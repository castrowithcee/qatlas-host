package service

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/castrowithcee/qatlas-kit/configfile"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/credentials"
	"github.com/castrowithcee/qatlas-host/internal/engine"
	"github.com/castrowithcee/qatlas-host/internal/formatnum/formatnumtest"
)

func TestUnknownSourcesFormatNumberIsRefusedAndKept(t *testing.T) {
	for name, doc := range formatnumtest.Cases("version", sourcesVersion, `"sources":[]`) {
		t.Run(name, func(t *testing.T) {
			state := filepath.Join(t.TempDir(), "state")
			path := filepath.Join(state, sourcesFile)
			formatnumtest.Seed(t, path, doc)
			keys, err := credentials.Open(state, nil)
			if err != nil {
				t.Fatal(err)
			}
			_, err = openSources(state, keys, nil, engine.Config{}, slog.New(slog.DiscardHandler))
			formatnumtest.Refused(t, err, path)
			if dirs := BoundDirs(state); len(dirs) != 0 {
				t.Fatalf("bound dirs = %v", dirs)
			}

			// An earlier binding does not get to replace the file, and stays itself.
			legacy := `{"version":1,"url":"` + exampleURL + `","dir":"/srv/stacks"}`
			legacyPath := filepath.Join(state, legacyFile)
			put(t, legacyPath, legacy)
			store, err := configfile.NewStore(path, sourcesCodecAt(path))
			if err != nil {
				t.Fatal(err)
			}
			formatnumtest.Refused(t, migrateLegacy(state, store, slog.New(slog.DiscardHandler)), path)
			formatnumtest.Unchanged(t, path, doc)
			formatnumtest.Unchanged(t, legacyPath, legacy)
		})
	}
}

func TestUnknownLegacyBindingFormatNumberIsKept(t *testing.T) {
	for name, doc := range formatnumtest.Cases("version", legacyVersion, `"url":"`+exampleURL+`","dir":"/srv/stacks"`) {
		t.Run(name, func(t *testing.T) {
			state := t.TempDir()
			path := filepath.Join(state, legacyFile)
			formatnumtest.Seed(t, path, doc)
			_, err := readLegacy(state)
			formatnumtest.Refused(t, err, path)
			store, err := configfile.NewStore(filepath.Join(state, sourcesFile), sourcesCodecAt(filepath.Join(state, sourcesFile)))
			if err != nil {
				t.Fatal(err)
			}
			if err := migrateLegacy(state, store, slog.New(slog.DiscardHandler)); err != nil {
				t.Fatal(err)
			}
			if exists(filepath.Join(state, sourcesFile)) {
				t.Fatal("sources were made from a binding that was not read")
			}
			formatnumtest.Unchanged(t, path, doc)
		})
	}
}

// The transition from the earlier format of the sources keeps working: the binding is read, becomes the first
// source and its file goes away.
func TestEarlierBindingStillBecomesTheFirstSource(t *testing.T) {
	state := t.TempDir()
	put(t, filepath.Join(state, legacyFile), `{"version":1,"url":"`+exampleURL+`","dir":"/srv/stacks"}`)
	store, err := configfile.NewStore(filepath.Join(state, sourcesFile), sourcesCodecAt(filepath.Join(state, sourcesFile)))
	if err != nil {
		t.Fatal(err)
	}
	if err := migrateLegacy(state, store, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatal(err)
	}
	got, _, err := store.LoadVersioned()
	if err != nil || got.Version != sourcesVersion || len(got.Sources) != 1 || got.Sources[0].Path != "/srv/stacks" ||
		got.Sources[0].URL != exampleURL || exists(filepath.Join(state, legacyFile)) {
		t.Fatalf("sources = %+v, %v", got, err)
	}
}

func TestLogStoreWritesAndRequiresItsFormatNumber(t *testing.T) {
	st, svc, _ := newTestStore(t)
	slog.New(st.handler()).Warn("first")
	files := st.files()
	if len(files) != 1 {
		t.Fatalf("files = %v", files)
	}
	data, err := os.ReadFile(filepath.Join(st.dir, files[0]))
	if err != nil || !bytes.HasPrefix(data, logHeader) {
		t.Fatalf("file = %q, %v", data, err)
	}
	if got := messages(askLog(t, svc, control.ServiceLogArgs{})); got != "first" {
		t.Fatalf("messages = %q", got)
	}
}

func TestLogFileWithoutFormatNumberIsReadAsFormat1AndKept(t *testing.T) {
	longMsg := strings.Repeat("x", 1500)
	doc := `{"time":"2026-01-02T03:04:05Z","level":"INFO","msg":"` + longMsg + `","svc":"qhost"}` + "\n"
	st, svc, _ := newTestStore(t)
	path := filepath.Join(st.dir, "00000000000000000001.jsonl")
	formatnumtest.Seed(t, path, doc)
	if err := logFormat(path); err != nil {
		t.Fatal(err)
	}
	if got := messages(askLog(t, svc, control.ServiceLogArgs{})); got != longMsg {
		t.Fatalf("an earlier log was not read in full: %d bytes", len(got))
	}
	formatnumtest.Unchanged(t, path, doc)
}

func TestUnknownLogFormatNumberIsNotReadAppendedPrunedOrRemoved(t *testing.T) {
	line := `{"time":"2026-01-02T03:04:05Z","level":"INFO","msg":"kept","svc":"qhost"}` + "\n"
	for name, doc := range map[string]string{
		"higher": `{"schema":2,"added_later":true}` + "\n" + line,
	} {
		t.Run(name, func(t *testing.T) {
			st, svc, _ := newTestStore(t)
			st.maxFiles, st.maxAge = 1, time.Nanosecond
			name := "00000000000000000001.jsonl"
			path := filepath.Join(st.dir, name)
			formatnumtest.Seed(t, path, doc)
			formatnumtest.Refused(t, logFormat(path), path)

			if got := messages(askLog(t, svc, control.ServiceLogArgs{})); got != "" {
				t.Fatalf("an unreadable file was read: %q", got)
			}
			lg := slog.New(st.handler())
			for range 3 {
				lg.Info("new")
			}
			if got := messages(askLog(t, svc, control.ServiceLogArgs{})); got != "new,new,new" {
				t.Fatalf("messages = %q", got)
			}
			formatnumtest.Unchanged(t, path, doc)
		})
	}
}
