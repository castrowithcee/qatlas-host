package service

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/castrowithcee/qatlas-host/internal/engine"
	"github.com/castrowithcee/qatlas-host/internal/formatnum"
	"github.com/castrowithcee/qatlas-host/internal/notifier"
	"github.com/castrowithcee/qatlas-host/internal/window"
)

// CheckState reads the files that hold the core state of a deploy service in stateDir and returns the first
// format number refusal, which names the file, or the first read error, such as a missing permission. It uses
// the readers of the service and changes nothing, so it can run while the service does not. A new file of the
// core state belongs here too, or 'qhost status' cannot name it.
func CheckState(stateDir string) error {
	sources := filepath.Join(stateDir, sourcesFile)
	if err := formatnum.CheckFile(sources, "version", sourcesVersion); err != nil {
		return err
	}
	if err := formatnum.CheckFile(filepath.Join(stateDir, legacyFile), "version", legacyVersion); err != nil {
		return err
	}
	if err := notifier.CheckFormat(stateDir); err != nil {
		return err
	}
	if err := window.CheckFormat(stateDir); err != nil {
		return err
	}
	data, err := os.ReadFile(sources)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var conf sourcesConfig
	if json.Unmarshal(data, &conf) != nil {
		return nil
	}
	for _, src := range conf.Sources {
		if src.ID != sourceID(src.Path) {
			continue
		}
		if err := engine.CheckFormat(filepath.Join(stateDir, sourcesDir, src.ID)); err != nil {
			return err
		}
	}
	return nil
}
