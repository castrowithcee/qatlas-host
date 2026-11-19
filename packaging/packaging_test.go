package packaging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/release"
	"github.com/castrowithcee/qatlas-kit/release/installer"
	"github.com/castrowithcee/qatlas-kit/release/releasebuild"
	"github.com/castrowithcee/qatlas-kit/sysservice"
	"golang.org/x/crypto/ssh"
)

const (
	program   = "qhost"
	namespace = "qhost-release"
	repoRoot  = ".."
)

func readFile(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(name)))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func loadConfig(t *testing.T) releasebuild.Config {
	t.Helper()
	c, err := releasebuild.LoadConfig(filepath.Join(repoRoot, "release.json"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Service == nil {
		t.Fatal("release.json has no service section")
	}
	return c
}

const deployService = "qhost-deploy"

func TestHostUnitPassesKitValidation(t *testing.T) {
	c := loadConfig(t)
	unit := readFile(t, c.Service.Unit)
	rules := sysservice.UnitRules{Program: program, Groups: c.Service.Groups, WritableConfig: c.Service.WritableConfig}
	if err := sysservice.ValidateUnit(unit, rules); err != nil {
		t.Fatal(err)
	}
}

func deployRules() sysservice.UnitRules {
	return sysservice.UnitRules{Program: program, Service: deployService, Groups: []string{"docker"}}
}

func TestDeployUnitPassesKitValidation(t *testing.T) {
	if err := sysservice.ValidateUnit(readFile(t, "packaging/qhost-deploy.service"), deployRules()); err != nil {
		t.Fatal(err)
	}
}

func TestDeployUnitWithRootFailsKitValidation(t *testing.T) {
	unit := strings.NewReplacer("User=qhost-deploy", "User=root", "Group=qhost-deploy", "Group=root").
		Replace(string(readFile(t, "packaging/qhost-deploy.service")))
	if err := sysservice.ValidateUnit([]byte(unit), deployRules()); err == nil {
		t.Fatal("a deploy unit that runs as root passed the validation")
	}
}

func TestEmbeddedDeployUnitIsTheShippedFile(t *testing.T) {
	if string(DeployUnit) != string(readFile(t, "packaging/qhost-deploy.service")) {
		t.Fatal("the embedded deploy unit differs from packaging/qhost-deploy.service")
	}
}

func unitLines(unit []byte) map[string]bool {
	lines := map[string]bool{}
	for _, l := range strings.Split(string(unit), "\n") {
		lines[strings.TrimSpace(l)] = true
	}
	return lines
}

// The host service refuses to start unless /run/qhost belongs to qhost:qhost without access for others, so the
// unit must ask systemd for exactly that. It never has the container engine's group.
func TestHostUnitMeetsRuntimeDirectoryContract(t *testing.T) {
	lines := unitLines(readFile(t, loadConfig(t).Service.Unit))
	for _, want := range []string{
		"ExecStart=/usr/local/bin/qhost run",
		"User=qhost",
		"Group=qhost",
		"RuntimeDirectory=qhost",
		"RuntimeDirectoryMode=0750",
		"StateDirectory=qhost",
		"StateDirectoryMode=0700",
		"NoNewPrivileges=yes",
		"ProtectSystem=strict",
	} {
		if !lines[want] {
			t.Errorf("host unit lacks the line %q", want)
		}
	}
	for l := range lines {
		if strings.HasPrefix(l, "SupplementaryGroups=") || strings.HasPrefix(l, "ReadWritePaths=") || strings.Contains(l, "docker.service") {
			t.Errorf("host unit has the line %q", l)
		}
	}
}

// The deploy service's channel directory is made by the setup, not by systemd, so the unit only releases it
// for writing. The stack folders are the host's choice and reach the unit only through a drop-in.
func TestDeployUnitMeetsChannelContract(t *testing.T) {
	lines := unitLines(readFile(t, "packaging/qhost-deploy.service"))
	for _, want := range []string{
		"ExecStart=/usr/local/bin/qhost deploy-service run",
		"User=qhost-deploy",
		"Group=qhost-deploy",
		"SupplementaryGroups=docker",
		"ReadWritePaths=/run/qhost-deploy",
		"StateDirectory=qhost-deploy",
		"StateDirectoryMode=0700",
		"NoNewPrivileges=yes",
		"ProtectSystem=strict",
	} {
		if !lines[want] {
			t.Errorf("deploy unit lacks the line %q", want)
		}
	}
	for l := range lines {
		if strings.HasPrefix(l, "RuntimeDirectory") {
			t.Errorf("deploy unit lets systemd create its runtime directory: %q", l)
		}
		if strings.HasPrefix(l, "ReadWritePaths=") && l != "ReadWritePaths=/run/qhost-deploy" {
			t.Errorf("deploy unit names a writable path: %q", l)
		}
	}
}

func TestReleaseConfigSelectsTheSystemVariant(t *testing.T) {
	c := loadConfig(t)
	if got, want := strings.Join(c.Targets, " "), "linux/amd64 linux/arm64"; got != want {
		t.Errorf("targets = %q, want %q", got, want)
	}
	if c.VersionVariable != "github.com/castrowithcee/qatlas-host/internal/cli.version" {
		t.Errorf("version variable = %q", c.VersionVariable)
	}
	if len(c.Service.Groups) != 0 {
		t.Errorf("service groups = %v, want none", c.Service.Groups)
	}
	if len(c.Files) != 1 || c.Files[0].Source != "packaging/qhost-deploy.service" || c.Files[0].Path != sysservice.UnitEntry(deployService) {
		t.Errorf("release files = %+v, want the deploy unit at %s", c.Files, sysservice.UnitEntry(deployService))
	}
	if !c.Service.AddInstallingUser {
		t.Error("an administrator in the service group reads without sudo, so the installing user joins the group")
	}
	if c.Service.WritableConfig {
		t.Error("the configuration stays read-only")
	}
}

func TestAllowedSignersListsTheReleaseKey(t *testing.T) {
	keys, err := releasebuild.ParseAllowedSigners(readFile(t, ".github/release-allowed-signers"), namespace)
	if err != nil {
		t.Fatal(err)
	}
	const fingerprint = "SHA256:mF2Jh6YNweMA/fy5TM/VjNiLRD512PReMHVoWP52iek"
	if len(keys) != 1 || ssh.FingerprintSHA256(keys[0]) != fingerprint {
		t.Fatalf("allowed signers hold %d keys, want exactly the key %s", len(keys), fingerprint)
	}
}

func TestInstallerRendersTheSystemVariant(t *testing.T) {
	c := loadConfig(t)
	keys, err := releasebuild.ParseAllowedSigners(readFile(t, ".github/release-allowed-signers"), namespace)
	if err != nil {
		t.Fatal(err)
	}
	script, err := installer.Shell(installer.Params{
		Program:        program,
		DisplayName:    c.Installer.DisplayName,
		ReleasesURL:    "https://github.com/castrowithcee/qatlas-host/releases",
		Namespace:      namespace,
		SignatureAsset: release.SignatureAsset,
		TrustedKeys:    keys,
		Targets:        c.Targets,
		NextStep:       c.Installer.NextStep,
		Service: &installer.Service{
			Groups:            c.Service.Groups,
			AddInstallingUser: c.Service.AddInstallingUser,
			WritableConfig:    c.Service.WritableConfig,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	text := string(script)
	for _, want := range []string{
		"unit_entry='" + sysservice.UnitEntry(program) + "'",
		"groups=''",
		"namespace='" + namespace + "'",
		"--uninstall",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("install.sh lacks %q", want)
		}
	}
}

const instanceTemplate = "packaging/qhost-deploy@.service"

func TestEmbeddedInstanceTemplateIsTheFile(t *testing.T) {
	if string(DeployInstanceUnit) != string(readFile(t, instanceTemplate)) {
		t.Fatal("the embedded instance template differs from " + instanceTemplate)
	}
}

// An instance runs as the owner of a rootless engine, who is in neither docker nor qhost. The kit's unit check
// does not apply to a service under an existing user, so this test holds the hardening of the template. %i keeps
// a '-' of the user name, which %I would turn into '/'.
func TestInstanceTemplateMeetsItsContract(t *testing.T) {
	lines := unitLines(DeployInstanceUnit)
	for _, want := range []string{
		"ExecStart=/usr/local/bin/qhost deploy-service run --instance %i",
		"User=%i",
		"Environment=DOCKER_CONFIG=/var/lib/qhost-deploy-%i/docker",
		"ReadWritePaths=/run/qhost-deploy-%i",
		"StateDirectory=qhost-deploy-%i",
		"StateDirectoryMode=0700",
		"ConfigurationDirectory=qhost-deploy-%i",
		"NoNewPrivileges=yes",
		"PrivateTmp=yes",
		"ProtectSystem=strict",
	} {
		if !lines[want] {
			t.Errorf("instance template lacks the line %q", want)
		}
	}
	forbidden := []string{"Group=", "SupplementaryGroups=", "DynamicUser=", "AmbientCapabilities=", "RuntimeDirectory",
		"PermissionsStartOnly=", "ProtectHome=", "User=root", "RootDirectory="}
	execs := 0
	for l := range lines {
		for _, f := range forbidden {
			if strings.HasPrefix(l, f) {
				t.Errorf("instance template has the line %q", l)
			}
		}
		if strings.Contains(l, "docker.service") || strings.Contains(l, "%I") {
			t.Errorf("instance template has the line %q", l)
		}
		if strings.HasPrefix(l, "ReadWritePaths=") && l != "ReadWritePaths=/run/qhost-deploy-%i" {
			t.Errorf("instance template names a writable path: %q", l)
		}
		if key, value, ok := strings.Cut(l, "="); ok && strings.HasPrefix(key, "Exec") {
			execs++
			if prefix := value[:len(value)-len(strings.TrimLeft(value, "@-:+!|"))]; strings.ContainsAny(prefix, "+!") {
				t.Errorf("instance template runs a command with elevated rights: %q", l)
			}
		}
	}
	if execs != 1 {
		t.Errorf("instance template has %d Exec lines, want only ExecStart", execs)
	}
}

// The template keeps every hardening line of the deploy unit of the system engine.
func TestInstanceTemplateKeepsTheHardeningOfTheDeployUnit(t *testing.T) {
	lines := unitLines(DeployInstanceUnit)
	for l := range unitLines(DeployUnit) {
		for _, key := range []string{"NoNewPrivileges=", "PrivateTmp=", "ProtectSystem=", "StateDirectoryMode=", "Restart="} {
			if strings.HasPrefix(l, key) && !lines[l] {
				t.Errorf("instance template lacks %q of the deploy unit", l)
			}
		}
	}
}

func TestInstanceTemplateStaysOutOfTheReleaseArchive(t *testing.T) {
	for _, f := range loadConfig(t).Files {
		if strings.Contains(f.Source, "@") || strings.Contains(f.Path, "@") {
			t.Errorf("release file %+v; the setup writes the instance template", f)
		}
	}
}
