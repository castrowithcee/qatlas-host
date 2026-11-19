package deploysetup

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/castrowithcee/qatlas-host/packaging"
)

// shadowDir holds the units an administrator overrides the installed ones with.
const shadowDir = "/etc/systemd/system"

// Refresh is what RefreshInstances did.
type Refresh struct {
	// Instances are the units of the instances found.
	Instances []string
	// TemplateWritten reports that the template unit differed from the embedded one and was replaced.
	TemplateWritten bool
	// Restarted are the units that were restarted.
	Restarted []string
	// Shadowed are the units an administrator's own unit or template in /etc/systemd/system overrides.
	Shadowed []string
}

// shadows reports whether a regular file or a symbolic link of that name exists in /etc/systemd/system.
func (h Host) shadows(name string) bool {
	info, err := os.Lstat(h.path(shadowDir + "/" + name))
	return err == nil && (info.Mode().IsRegular() || info.Mode()&fs.ModeSymlink != 0)
}

// RefreshInstances brings the instances to the template of this program: when an instance is set up, it replaces
// the template unit if it differs from the embedded one, reloads systemd and restarts every instance. Without an
// instance it changes nothing, not even the template, because only the setup of an instance installs it. A
// failed restart does not stop the others; the errors of all are returned together with the result.
func (h Host) RefreshInstances(ctx context.Context) (Refresh, error) {
	instances, err := Instances(h.Root)
	if err != nil {
		return Refresh{}, err
	}
	var res Refresh
	if len(instances) == 0 {
		return res, nil
	}
	for _, i := range instances {
		res.Instances = append(res.Instances, i.Unit())
	}
	if h.differs(TemplatePath(), packaging.DeployInstanceUnit) {
		if err := h.write(TemplatePath(), packaging.DeployInstanceUnit); err != nil {
			return res, fmt.Errorf("write %s: %w", TemplatePath(), err)
		}
		res.TemplateWritten = true
	}
	if err := h.run(ctx, "systemctl", "daemon-reload"); err != nil {
		return res, fmt.Errorf("systemctl daemon-reload: %w", err)
	}
	templateShadowed := h.shadows(Service + "@.service")
	var errs []error
	for _, i := range instances {
		if templateShadowed || h.shadows(i.Unit()) {
			res.Shadowed = append(res.Shadowed, i.Unit())
		}
		if err := h.run(ctx, "systemctl", "restart", i.Unit()); err != nil {
			errs = append(errs, fmt.Errorf("systemctl restart %s: %w", i.Unit(), err))
			continue
		}
		res.Restarted = append(res.Restarted, i.Unit())
	}
	return res, errors.Join(errs...)
}
