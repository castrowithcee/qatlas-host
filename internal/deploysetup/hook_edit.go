package deploysetup

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/castrowithcee/qatlas-host/internal/hookservice"
)

// ErrHookNotSetUp means the webhook receiver was never set up: there is no unit or no configuration.
var ErrHookNotSetUp = errors.New("the webhook receiver is not set up")

// ReadHooks reads the configuration of the receiver. It does not need root.
func (h Host) ReadHooks() (hookservice.Config, error) {
	if !HookInstalled(h.Root) {
		return hookservice.Config{}, ErrHookNotSetUp
	}
	cfg, err := HookConfig(h.Root)
	if errors.Is(err, fs.ErrNotExist) {
		return hookservice.Config{}, ErrHookNotSetUp
	}
	if err != nil {
		return hookservice.Config{}, fmt.Errorf("read %s: %w", HookConfigPath, err)
	}
	return cfg, nil
}

// UpdateHooks changes the configuration of the receiver through edit and writes it back atomically as root and
// the group of the receiver. A configuration that does not parse stays as it is and the error says so.
func (h Host) UpdateHooks(edit func(*hookservice.Config) error) error {
	if !HookInstalled(h.Root) {
		return ErrHookNotSetUp
	}
	err := hookservice.UpdateConfig(h.path(HookConfigPath), h.chownHook, edit)
	if errors.Is(err, fs.ErrNotExist) {
		return ErrHookNotSetUp
	}
	return err
}
