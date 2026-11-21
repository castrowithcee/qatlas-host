package hostservice

import (
	"fmt"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/proposals"
)

func (s *server) commandsSet(c *call) (any, error) {
	var a control.HostCommandsSetArgs
	if len(c.args) == 0 || proposals.DecodeStrict(c.args, &a) != nil {
		return nil, badRequest()
	}
	if err := s.commandSwitch.Set(a.Enabled); err != nil {
		return nil, fmt.Errorf("store the switch for proposals of commands: %w", err)
	}
	return control.HostCommandsResult{Enabled: a.Enabled}, nil
}

func (s *server) commandsStatus(*call) (any, error) {
	enabled, err := s.commandSwitch.Enabled()
	if err != nil {
		return nil, fmt.Errorf("read the switch for proposals of commands: %w", err)
	}
	return control.HostCommandsResult{Enabled: enabled}, nil
}
