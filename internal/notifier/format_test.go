package notifier

import (
	"path/filepath"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/formatnum/formatnumtest"
)

func TestUnknownFormatNumberIsRefusedAndKept(t *testing.T) {
	for name, doc := range formatnumtest.Cases("schema", fileSchema, `"channels":[]`) {
		t.Run("settings/"+name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, settingsFile)
			formatnumtest.Seed(t, path, doc)
			s, err := OpenSettings(dir)
			if err != nil {
				t.Fatal(err)
			}
			_, err = s.List()
			formatnumtest.Refused(t, err, path)
			formatnumtest.Refused(t, s.Remove("ops"), path)
			formatnumtest.Unchanged(t, path, doc)
		})
	}
	for name, doc := range formatnumtest.Cases("schema", fileSchema, `"versions":{}`) {
		t.Run("seen/"+name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, seenFile)
			formatnumtest.Seed(t, path, doc)
			s, err := OpenSeen(dir)
			if err != nil {
				t.Fatal(err)
			}
			_, err = s.Fresh("web", []Item{{Variable: "WEB_TAG", Available: "2.0.0"}})
			formatnumtest.Refused(t, err, path)
			formatnumtest.Refused(t, s.Mark("web", []Item{{Variable: "WEB_TAG", Available: "2.0.0"}}), path)
			formatnumtest.Unchanged(t, path, doc)
		})
	}
}
