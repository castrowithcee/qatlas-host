package hookservice

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/formatnum/formatnumtest"
)

func TestUnknownFormatNumberIsRefusedAndKept(t *testing.T) {
	for name, doc := range formatnumtest.Cases("version", configVersion, `"listen":"127.0.0.1:8080","hooks":[]`) {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			writeRaw(t, path, doc, 0o644)
			_, err := LoadConfig(path, uint32(os.Getuid()))
			formatnumtest.Refused(t, err, path)
			_, err = ParseConfig(path, []byte(doc))
			formatnumtest.Refused(t, err, path)
			formatnumtest.Unchanged(t, path, doc)
		})
	}
}

func TestUpdateConfigRefusesMissingAndHigherFormatNumberAndKeepsTheFile(t *testing.T) {
	for name, doc := range formatnumtest.Cases("version", configVersion, `"listen":"127.0.0.1:8080","hooks":[]`) {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			writeRaw(t, path, doc, 0o644)
			called := false
			err := UpdateConfig(path, nil, func(*Config) error { called = true; return nil })
			formatnumtest.Refused(t, err, path)
			formatnumtest.Unchanged(t, path, doc)
			if called {
				t.Fatal("the edit ran on a refused file")
			}
		})
	}
}
