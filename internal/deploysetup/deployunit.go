package deploysetup

import (
	"fmt"
	"slices"
	"strings"

	"github.com/castrowithcee/qatlas-kit/sysservice"
)

// deployUnitLines are the lines the deploy unit of the system engine must have. The service runs as root, which
// the kit's unit check refuses, so the sandbox below is what keeps a mistake or foreign content from a commit
// inside the stack folders.
func deployUnitLines() []string {
	return []string{
		// The only start step. A '+' step runs without the sandbox and the systemd lock, so it may run nothing else.
		"ExecStartPre=+" + sysservice.ServicePaths(Program, "").Program + " deploy-service prepare",
		"ExecStart=" + sysservice.ServicePaths(Program, "").Program + " deploy-service run",
		"ReadWritePaths=/run/qhost-deploy",
		"ReadWritePaths=-/run/qhost-hook/system",
		"InaccessiblePaths=-/var/lib/qhost",
		"InaccessiblePaths=-/etc/qhost",
		"InaccessiblePaths=-/var/lib/qhost-host",
		"InaccessiblePaths=-/run/qhost",
		"StateDirectory=" + Service,
		"StateDirectoryMode=0700",
		"NoNewPrivileges=yes",
		"ProtectSystem=strict",
		"ProtectHome=read-only",
		"PrivateTmp=yes",
		"PrivateDevices=yes",
		"ProtectKernelTunables=yes",
		"ProtectKernelModules=yes",
		"ProtectControlGroups=yes",
		"RestrictSUIDSGID=yes",
		"LockPersonality=yes",
		"RestrictNamespaces=yes",
		"RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6",
		"SystemCallFilter=@system-service",
		"CapabilityBoundingSet=CAP_DAC_OVERRIDE CAP_DAC_READ_SEARCH CAP_FOWNER CAP_CHOWN",
	}
}

// ValidateDeployUnit checks the unit of the deploy service of the system engine: no user, group or additional
// capability, and every line of the sandbox. A line may only be missing or wrong, never replaced by another that
// widens the sandbox: a second CapabilityBoundingSet= or ReadWritePaths= is refused, and so is any ExecStartPre=
// line but the one that prepares the service.
func ValidateDeployUnit(unit []byte) error {
	want := deployUnitLines()
	var got []string
	for _, l := range strings.Split(string(unit), "\n") {
		got = append(got, strings.TrimSpace(l))
	}
	for _, w := range want {
		if !slices.Contains(got, w) {
			return fmt.Errorf("the line %q is missing", w)
		}
	}
	for _, l := range got {
		for _, bad := range []string{"User=", "Group=", "SupplementaryGroups=", "AmbientCapabilities=", "DynamicUser="} {
			if strings.HasPrefix(l, bad) {
				return fmt.Errorf("the line %q is not allowed", l)
			}
		}
		for _, only := range []string{"ExecStartPre=", "CapabilityBoundingSet=", "ReadWritePaths=", "ProtectHome=", "ProtectSystem="} {
			if strings.HasPrefix(l, only) && !slices.Contains(want, l) {
				return fmt.Errorf("the line %q is not allowed", l)
			}
		}
	}
	return nil
}
