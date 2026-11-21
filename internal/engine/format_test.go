package engine

import (
	"context"
	"errors"
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

func TestUnknownRunLogFormatNumberIsRefusedAndKept(t *testing.T) {
	id := "20260102T030405Z-0a1b2c3d"
	for name, doc := range map[string]string{
		"higher":  "schema 2\n2026-01-02T03:04:05Z info something new\n",
		"missing": "2026-01-02T03:04:05Z info an earlier log\n",
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
