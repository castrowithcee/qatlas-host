package proposals

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/castrowithcee/qatlas-kit/configfile"

	"github.com/castrowithcee/qatlas-host/internal/formatnum"
)

const (
	commandSwitchFile   = "proposal-commands.json"
	commandSwitchSchema = 1
)

// CommandSwitch is the switch for proposals of free commands. It lives in the state directory of the service, so
// it survives a restart, and is off until root turns it on. A missing file means off; a file that cannot be read
// is an error, never a silent default.
type CommandSwitch struct {
	path string
	mu   sync.Mutex
}

type commandSwitchRecord struct {
	Schema  int  `json:"schema"`
	Enabled bool `json:"enabled"`
}

// NewCommandSwitch returns the switch kept in stateDir.
func NewCommandSwitch(stateDir string) *CommandSwitch {
	return &CommandSwitch{path: filepath.Join(stateDir, commandSwitchFile)}
}

// Enabled reports whether proposals of the kind control.ProposalKindCommand are accepted.
func (w *CommandSwitch) Enabled() (bool, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.read()
}

func (w *CommandSwitch) read() (bool, error) {
	data, err := os.ReadFile(w.path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := formatnum.CheckJSON(w.path, "schema", data, commandSwitchSchema); err != nil {
		return false, err
	}
	var r commandSwitchRecord
	if err := json.Unmarshal(data, &r); err != nil {
		return false, fmt.Errorf("read %s: %w", w.path, err)
	}
	return r.Enabled, nil
}

// Set turns the switch on or off. Proposals that are open stay as they are.
func (w *CommandSwitch) Set(enabled bool) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, err := w.read(); err != nil {
		return err
	}
	data, err := json.Marshal(commandSwitchRecord{Schema: commandSwitchSchema, Enabled: enabled})
	if err != nil {
		return err
	}
	tmp := w.path + ".tmp"
	_ = os.Remove(tmp)
	if err := configfile.WriteNewPrivateFile(tmp, data); err != nil {
		return err
	}
	if err := os.Rename(tmp, w.path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
