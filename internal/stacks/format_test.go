package stacks

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/formatnum/formatnumtest"
)

func TestUnknownSettingsFormatNumberIsRefusedAndKept(t *testing.T) {
	for name, doc := range map[string]string{
		"higher":  "version: 2\nupdate:\n  mode: manual\n  added_later: true\n",
		"missing": "update:\n  mode: manual\n",
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, filepath.FromSlash(SettingsPath("web")))
			formatnumtest.Seed(t, path, doc)
			_, err := readSettings(root, "web")
			formatnumtest.Refused(t, err, SettingsPath("web"))
			formatnumtest.Unchanged(t, path, doc)
			if _, err := os.Stat(path); err != nil {
				t.Fatal(err)
			}
		})
	}
}
