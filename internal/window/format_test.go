package window

import (
	"path/filepath"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/formatnum/formatnumtest"
)

func TestUnknownFormatNumberIsRefusedAndKept(t *testing.T) {
	for name, doc := range formatnumtest.Cases("schema", fileSchema, `"days":["mon"]`) {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, fileName)
			formatnumtest.Seed(t, path, doc)
			s := newScheduler(t, dir, newClock(monday), newJob())
			_, err := s.Status()
			formatnumtest.Refused(t, err, path)
			_, err = s.Set(Change{Zone: ptr("UTC")})
			formatnumtest.Refused(t, err, path)
			formatnumtest.Unchanged(t, path, doc)
		})
	}
}
