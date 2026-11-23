package engine

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/formatnum/formatnumtest"
	"github.com/castrowithcee/qatlas-host/internal/stacks"
)

func TestUnknownStateFormatNumberIsRefusedAndKept(t *testing.T) {
	for name, doc := range formatnumtest.Cases("schema", stateSchema, `"stacks":{}`) {
		t.Run(name, func(t *testing.T) {
			r := newRig(t, "web")
			path := filepath.Join(r.state, engineDir, stateFile)
			formatnumtest.Seed(t, path, doc)

			_, err := r.eng.Runs("")
			formatnumtest.Refused(t, err, path)
			formatnumtest.Refused(t, r.eng.UpdateStoppedContainers("web", nil, nil), path)
			if _, err := r.eng.Poll(context.Background()); err == nil {
				// A poll that finds work records it in the state; it must not get that far.
				r.eng.Wait()
			}
			formatnumtest.Unchanged(t, path, doc)
		})
	}
}

func TestRunLogsCarryAFormatNumber(t *testing.T) {
	s := logStore{dir: t.TempDir()}
	id := "20260102T030405Z-0a1b2c3d"
	w, err := s.create(id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("line\n")); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	got, err := s.read(id)
	if err != nil || got != "line\n" {
		t.Fatalf("read = %q, %v", got, err)
	}
	p, _ := s.path(id)
	formatnumtest.Unchanged(t, p, "schema 1\nline\n")
}

func TestRunLogWithoutHeaderIsFormat1AndKept(t *testing.T) {
	id := "20260102T030405Z-0a1b2c3d"
	doc := "2026-01-02T03:04:05Z step an earlier log\n2026-01-02T03:04:06Z info second line\n"
	e := &Engine{logs: logStore{dir: t.TempDir()}}
	p, _ := e.logs.path(id)
	formatnumtest.Seed(t, p, doc)

	if got, err := e.Log(id); err != nil || got != doc {
		t.Fatalf("log = %q, %v", got, err)
	}
	if err := e.LogFormat(id); err != nil {
		t.Fatal(err)
	}
	formatnumtest.Unchanged(t, p, doc)

	if err := e.logs.appendLine(id, "2026-01-02T03:04:07Z", "info", "more"); err != nil {
		t.Fatal(err)
	}
	formatnumtest.Unchanged(t, p, doc+"2026-01-02T03:04:07Z info more\n")

	e.logs.remove(id)
	if _, err := os.Lstat(p); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("retention kept a readable log: %v", err)
	}
}

func TestUnknownRunLogFormatNumberIsRefusedAndKept(t *testing.T) {
	id := "20260102T030405Z-0a1b2c3d"
	for name, doc := range map[string]string{
		"higher":     "schema 2\n2026-01-02T03:04:05Z info something new\n",
		"unreadable": "schema x\n2026-01-02T03:04:05Z info damaged\n",
	} {
		t.Run(name, func(t *testing.T) {
			s := logStore{dir: t.TempDir()}
			p, _ := s.path(id)
			formatnumtest.Seed(t, p, doc)
			_, err := s.read(id)
			formatnumtest.Refused(t, err, p)
			err = s.appendLine(id, "2026-01-02T03:04:06Z", "info", "more")
			formatnumtest.Refused(t, err, p)
			s.remove(id)
			formatnumtest.Unchanged(t, p, doc)
		})
	}
}

func TestSettingsWithAnUnknownFormatNumberAreNotReplacedOrRemoved(t *testing.T) {
	for name, doc := range map[string]string{"higher": "version: 2\nfuture: true\n", "missing": "update:\n  mode: manual\n"} {
		t.Run(name, func(t *testing.T) {
			err := refuseUnknownFormat("web", []byte(doc))
			if !errors.Is(err, ErrUnreadable) || !strings.Contains(err.Error(), stacks.SettingsPath("web")) {
				t.Fatalf("%v", err)
			}
		})
	}
	if err := refuseUnknownFormat("web", []byte("update: [broken")); err != nil {
		t.Fatalf("broken content that is no format problem may be replaced: %v", err)
	}
	if err := refuseUnknownFormat("web", []byte("version: 1\n")); err != nil {
		t.Fatal(err)
	}
}

func TestLogFormatNamesAnUnreadableRunLogOnly(t *testing.T) {
	e := &Engine{logs: logStore{dir: t.TempDir()}}
	bad, good, absent := "20260102T030405Z-0a1b2c3d", "20260102T030406Z-0a1b2c3d", "20260102T030407Z-0a1b2c3d"
	p, _ := e.logs.path(bad)
	formatnumtest.Seed(t, p, "schema 2\nnew\n")
	q, _ := e.logs.path(good)
	formatnumtest.Seed(t, q, "schema 1\nold\n")
	formatnumtest.Refused(t, e.LogFormat(bad), p)
	if err := e.LogFormat(good); err != nil {
		t.Fatal(err)
	}
	if err := e.LogFormat(absent); err != nil {
		t.Fatal(err)
	}
	formatnumtest.Unchanged(t, p, "schema 2\nnew\n")
}
