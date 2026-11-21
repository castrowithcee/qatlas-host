package proposals

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/formatnum/formatnumtest"
)

func TestSwitchSurvivesARestart(t *testing.T) {
	dir := t.TempDir()
	w := NewCommandSwitch(dir)
	if on, err := w.Enabled(); err != nil || on {
		t.Fatalf("fresh switch = %v, %v", on, err)
	}
	if err := w.Set(true); err != nil {
		t.Fatal(err)
	}
	if on, err := NewCommandSwitch(dir).Enabled(); err != nil || !on {
		t.Fatalf("after a restart = %v, %v", on, err)
	}
	if err := w.Set(false); err != nil {
		t.Fatal(err)
	}
	if on, err := NewCommandSwitch(dir).Enabled(); err != nil || on {
		t.Fatalf("after disabling = %v, %v", on, err)
	}
}

func TestSwitchRefusesAnUnknownFormatNumberAndKeepsTheFile(t *testing.T) {
	for name, doc := range map[string]string{
		"higher":  `{"schema":2,"enabled":true,"added_later":1}`,
		"missing": `{"enabled":true}`,
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, commandSwitchFile)
			formatnumtest.Seed(t, path, doc)
			w := NewCommandSwitch(dir)
			on, err := w.Enabled()
			if on {
				t.Fatal("an unreadable switch counts as on")
			}
			formatnumtest.Refused(t, err, path)
			formatnumtest.Refused(t, w.Set(false), path)
			if got, _ := os.ReadFile(path); string(got) != doc {
				t.Fatalf("the file changed: %s", got)
			}
		})
	}
}
