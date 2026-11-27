package deploysetup

import (
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/packaging"
)

func TestShippedDeployUnitPassesItsValidation(t *testing.T) {
	if err := ValidateDeployUnit(packaging.DeployUnit); err != nil {
		t.Fatal(err)
	}
}

// Dropping any line of the sandbox is refused, one line at a time.
func TestDeployUnitWithoutALineOfTheSandboxIsRefused(t *testing.T) {
	for _, line := range deployUnitLines() {
		unit := strings.Replace(string(packaging.DeployUnit), line+"\n", "", 1)
		if unit == string(packaging.DeployUnit) {
			t.Fatalf("the shipped unit lacks %q", line)
		}
		if err := ValidateDeployUnit([]byte(unit)); err == nil || !strings.Contains(err.Error(), line) {
			t.Errorf("without %q: err = %v", line, err)
		}
	}
}

func TestDeployUnitWithAnAccountOrGroupIsRefused(t *testing.T) {
	for _, extra := range []string{"User=root", "User=qhost-deploy", "Group=root", "Group=qhost-deploy",
		"SupplementaryGroups=docker", "AmbientCapabilities=CAP_CHOWN", "DynamicUser=yes"} {
		unit := strings.Replace(string(packaging.DeployUnit), "[Service]\n", "[Service]\n"+extra+"\n", 1)
		if err := ValidateDeployUnit([]byte(unit)); err == nil {
			t.Errorf("a unit with %q passed", extra)
		}
	}
}

func TestDeployUnitWithMoreCapabilitiesIsRefused(t *testing.T) {
	const caps = "CapabilityBoundingSet=CAP_DAC_OVERRIDE CAP_DAC_READ_SEARCH CAP_FOWNER CAP_CHOWN"
	for name, replacement := range map[string]string{
		"widened": caps + " CAP_SYS_ADMIN",
		"second":  caps + "\nCapabilityBoundingSet=CAP_SYS_ADMIN",
		"empty":   caps + "\nCapabilityBoundingSet=",
	} {
		unit := strings.Replace(string(packaging.DeployUnit), caps, replacement, 1)
		if err := ValidateDeployUnit([]byte(unit)); err == nil {
			t.Errorf("%s capability set passed", name)
		}
	}
}

func TestDeployUnitWithAnotherWritablePathOrHomeIsRefused(t *testing.T) {
	for _, extra := range []string{"ReadWritePaths=/etc", "ProtectHome=no", "ProtectSystem=full"} {
		unit := strings.Replace(string(packaging.DeployUnit), "[Service]\n", "[Service]\n"+extra+"\n", 1)
		if err := ValidateDeployUnit([]byte(unit)); err == nil {
			t.Errorf("a unit with %q passed", extra)
		}
	}
}
