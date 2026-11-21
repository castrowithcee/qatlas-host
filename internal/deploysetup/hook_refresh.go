package deploysetup

import (
	"context"
	"fmt"

	"github.com/castrowithcee/qatlas-host/packaging"
)

// HookRefresh is what RefreshHook did.
type HookRefresh struct {
	// UnitWritten reports that the unit differed from the embedded one and was replaced.
	UnitWritten bool
	// Restarted reports that the receiver was restarted.
	Restarted bool
	// Shadowed reports that a unit of the administrator in /etc/systemd/system overrides the installed one.
	Shadowed bool
}

// RefreshHook brings the webhook receiver to this program: when it is set up, it replaces the unit if it differs
// from the embedded one, reloads systemd in that case and restarts the receiver. Without the setup it changes
// nothing, because an update never installs the unit by itself.
func (h Host) RefreshHook(ctx context.Context) (HookRefresh, error) {
	var res HookRefresh
	if !HookInstalled(h.Root) {
		return res, nil
	}
	if h.differs(HookUnitPath(), packaging.HookUnit) {
		if err := h.write(HookUnitPath(), packaging.HookUnit); err != nil {
			return res, fmt.Errorf("write %s: %w", HookUnitPath(), err)
		}
		res.UnitWritten = true
		if err := h.run(ctx, "systemctl", "daemon-reload"); err != nil {
			return res, fmt.Errorf("systemctl daemon-reload: %w", err)
		}
	}
	res.Shadowed = h.shadows(HookService + ".service")
	if err := h.run(ctx, "systemctl", "restart", HookService+".service"); err != nil {
		return res, fmt.Errorf("systemctl restart %s.service: %w", HookService, err)
	}
	res.Restarted = true
	return res, nil
}
