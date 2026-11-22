package main

import (
	"os"
	"testing"

	"github.com/castrowithcee/qatlas-kit/archtest"
)

func TestArchitecture(t *testing.T) {
	archtest.Check(t, os.DirFS("."), []archtest.Rule{
		{
			Name:  "service-stays-below-the-command-surface",
			Files: archtest.Files{Under: []string{"internal/service"}, Tests: archtest.WithTests},
			Check: archtest.ForbidImports("internal/cli"),
			Hint:  "the command surface calls the service, never the other way round",
		},
		{
			Name:  "service-does-not-call-its-own-client",
			Files: archtest.Files{Under: []string{"internal/service"}, Tests: archtest.SkipTests},
			Check: archtest.ForbidImports("internal/client"),
			Hint:  "the service answers over the channel; the client is how others call it",
		},
		{
			Name:  "control-holds-the-protocol-only",
			Files: archtest.Files{Under: []string{"internal/control"}, Tests: archtest.WithTests},
			Check: archtest.ForbidImports("internal/service", "internal/client", "internal/cli"),
			Hint:  "the protocol sits below the service and the client and knows neither",
		},
		{
			Name:  "client-talks-protocol-only",
			Files: archtest.Files{Under: []string{"internal/client"}, Tests: archtest.WithTests},
			Check: archtest.ForbidImports("internal/service", "internal/cli"),
			Hint:  "the client reaches the service over the channel, never by import",
		},
		{
			Name:  "tui-reaches-the-service-over-the-channel-only",
			Files: archtest.Files{Under: []string{"internal/tui"}, Tests: archtest.SkipTests},
			Check: archtest.ForbidImports("internal/service", "internal/cli", "internal/engine", "internal/stacks",
				"internal/stackcheck", "internal/gitrepo", "internal/compose", "internal/backup", "internal/credentials",
				"internal/registry", "internal/changelog", "internal/envfile"),
			Hint: "the terminal interface reads the stacks, Docker and the service state through the service channel, never directly",
		},
		{
			Name:  "cli-and-tui-read-containers-over-the-channel-only",
			Files: archtest.Files{Under: []string{"internal/cli", "internal/tui"}, Tests: archtest.SkipTests},
			Check: archtest.ForbidImportPaths(archtest.Exact("github.com/castrowithcee/qatlas-kit/docker")),
			Hint:  "only the service talks to the container engine; the command surface and the terminal interface ask the service",
		},
		{
			Name:  "diagnose-reaches-the-service-through-a-call-only",
			Files: archtest.Files{Under: []string{"internal/diagnose"}, Tests: archtest.SkipTests},
			Check: archtest.ForbidImports("internal/service", "internal/client", "internal/cli", "internal/tui", "internal/engine", "internal/stacks"),
			Hint:  "the package is built from channel answers handed in by the caller, so the command line and the terminal interface share it",
		},
		{
			Name:  "helptopics-import-nothing-of-the-module",
			Files: archtest.Files{Under: []string{"internal/helptopics"}, Tests: archtest.WithTests},
			Check: archtest.AllowImports(),
			Hint:  "help topics are plain text shared by every surface",
		},
		{
			Name:  "credentials-import-nothing-of-the-module",
			Files: archtest.Files{Under: []string{"internal/credentials"}, Tests: archtest.WithTests},
			Check: archtest.ForbidImports("internal/service", "internal/client", "internal/cli", "internal/control"),
			Hint:  "the store sits below the service and the command surface and knows neither",
		},
		{
			Name:  "stacks-read-the-working-copy-only",
			Files: archtest.Files{Under: []string{"internal/stacks"}, Tests: archtest.WithTests},
			Check: archtest.ForbidImports("internal/service", "internal/client", "internal/cli", "internal/control"),
			Hint:  "reading stacks is pure and sits below the service and the command surface",
		},
		{
			Name:  "stackcheck-sees-stacks-and-an-interface-only",
			Files: archtest.Files{Under: []string{"internal/stackcheck"}, Tests: archtest.WithTests},
			Check: archtest.ForbidImports("internal/service", "internal/client", "internal/cli", "internal/control", "internal/gitrepo"),
			Hint:  "the check sits below the service and the command surface and reaches git only through its own interface",
		},
		{
			Name:  "hostkey-knows-no-service-layer",
			Files: archtest.Files{Under: []string{"internal/hostkey"}, Tests: archtest.WithTests},
			Check: archtest.ForbidImports("internal/service", "internal/client", "internal/cli", "internal/control"),
			Hint:  "host key handling sits below the service and the command surface and knows neither",
		},
		{
			Name:  "gitrepo-runs-git-only",
			Files: archtest.Files{Under: []string{"internal/gitrepo"}, Tests: archtest.WithTests},
			Check: archtest.ForbidImports("internal/service", "internal/client", "internal/cli", "internal/control"),
			Hint:  "the git layer sits below the service and the command surface and knows neither",
		},
		{
			Name:  "compose-sees-stacks-and-the-engine-log-only",
			Files: archtest.Files{Under: []string{"internal/compose"}, Tests: archtest.WithTests},
			Check: archtest.ForbidImports("internal/service", "internal/client", "internal/cli", "internal/control", "internal/gitrepo"),
			Hint:  "the compose layer sits below the service and the command surface and knows neither",
		},
		{
			Name:  "backup-sees-compose-and-stacks-only",
			Files: archtest.Files{Under: []string{"internal/backup"}, Tests: archtest.WithTests},
			Check: archtest.ForbidImports("internal/service", "internal/client", "internal/cli", "internal/control", "internal/gitrepo", "internal/engine"),
			Hint:  "the backup layer sits below the service and the command surface and gets the deployed commit as a value",
		},
		{
			Name:  "engine-knows-no-service-layer",
			Files: archtest.Files{Under: []string{"internal/engine"}, Tests: archtest.WithTests},
			Check: archtest.ForbidImports("internal/service", "internal/client", "internal/cli", "internal/control"),
			Hint:  "the engine sits below the service and the command surface and knows neither",
		},
		{
			Name:  "host-part-imports-only-its-own-packages",
			Files: archtest.Files{Under: []string{"internal/host"}, Tests: archtest.WithTests},
			Check: archtest.AllowImports("internal/host/...", "internal/formatnum", "internal/formatnum/formatnumtest"),
			Hint:  "the host administration sits below the service and the command surface and knows no package of the deploy part",
		},
		{
			Name:  "host-part-does-not-touch-the-container-engine",
			Files: archtest.Files{Under: []string{"internal/host"}, Tests: archtest.WithTests},
			Check: archtest.ForbidImportPaths(archtest.Exact("github.com/castrowithcee/qatlas-kit/docker")),
			Hint:  "containers belong to the deploy part; the host administration never talks to the container engine",
		},
		{
			Name:  "host-service-imports-only-the-protocol-the-admin-check-the-action-log-and-the-proposals",
			Files: archtest.Files{Under: []string{"internal/hostservice"}, Tests: archtest.WithTests},
			Check: archtest.AllowImports("internal/control", "internal/client", "internal/channeltest", "internal/admins", "internal/host/auditlog",
				"internal/logview", "internal/proposals", "internal/host/hostproposal", "internal/tools"),
			Hint: "the host service knows the wire protocol, the administrator check, the action log with its shared log views, the shared proposal store and the check of host proposals, " +
				"and nothing of the deploy part or the command surface; it reaches the actions of the host only through that check, never the runner",
		},
		{
			Name:  "host-proposal-check-sees-the-actions-only",
			Files: archtest.Files{Under: []string{"internal/host/hostproposal"}, Tests: archtest.WithTests},
			Check: archtest.AllowImports("internal/host/hostchange", "internal/host/hostadmin"),
			Hint:  "the check of host proposals builds plans of the actions and imports no other package of the module, so the host service can use it without the runner",
		},
		{
			Name:  "formatnum-imports-nothing-of-the-module",
			Files: archtest.Files{Under: []string{"internal/formatnum"}, Tests: archtest.WithTests},
			Check: archtest.AllowImports("internal/formatnum", "internal/formatnum/formatnumtest"),
			Hint:  "the refusal of an unknown format number sits below every store and knows none of them",
		},
		{
			Name:  "proposals-know-the-protocol-only",
			Files: archtest.Files{Under: []string{"internal/proposals"}, Tests: archtest.WithTests},
			Check: archtest.AllowImports("internal/control", "internal/formatnum", "internal/formatnum/formatnumtest"),
			Hint:  "the proposal store is shared by both services and knows neither, nor any kind of proposal",
		},
		{
			Name:  "logview-knows-the-protocol-only",
			Files: archtest.Files{Under: []string{"internal/logview"}, Tests: archtest.WithTests},
			Check: archtest.AllowImports("internal/control"),
			Hint:  "the log views are shared by both services and know neither",
		},
		{
			Name:  "admins-know-the-protocol-only",
			Files: archtest.Files{Under: []string{"internal/admins"}, Tests: archtest.WithTests},
			Check: archtest.AllowImports("internal/control"),
			Hint:  "the administrator check is shared by both services and knows neither",
		},
		{
			Name:  "host-service-does-not-touch-the-container-engine",
			Files: archtest.Files{Under: []string{"internal/hostservice"}, Tests: archtest.WithTests},
			Check: archtest.ForbidImportPaths(archtest.Exact("github.com/castrowithcee/qatlas-kit/docker")),
			Hint:  "containers belong to the deploy service; the host service runs without access to the container engine",
		},
		{
			Name:  "hostadmin-imports-no-other-package-of-the-module",
			Files: archtest.Files{Under: []string{"internal/host/hostadmin"}, Tests: archtest.WithTests},
			Check: archtest.AllowImports(),
			Hint:  "the read model of the host sits below the changes and imports nothing of the module",
		},
		{
			Name:  "hostchange-sees-hostadmin-only",
			Files: archtest.Files{Under: []string{"internal/host/hostchange"}, Tests: archtest.WithTests},
			Check: archtest.AllowImports("internal/host/hostadmin", "internal/formatnum", "internal/formatnum/formatnumtest"),
			Hint:  "the host changes read the host through hostadmin and import no other package of the module",
		},
		{
			Name:  "host-service-reads-the-tool-table-in-tests-only",
			Files: archtest.Files{Under: []string{"internal/hostservice"}, Tests: archtest.SkipTests},
			Check: archtest.ForbidImports("internal/tools"),
			Hint:  "only the test that holds the table to the registry may import it",
		},
		{
			Name:  "tools-import-nothing-of-the-module",
			Files: archtest.Files{Under: []string{"internal/tools"}, Tests: archtest.WithTests},
			Check: archtest.AllowImports(),
			Hint:  "the tool table is plain data below the services and the command surface and knows none of them",
		},
		{
			Name:  "tool-table-is-read-by-the-command-surface-and-the-service-tests-only",
			Files: archtest.Files{Under: []string{"internal"}, Except: []string{"internal/tools", "internal/cli", "internal/service/tools_test.go", "internal/hostservice/tools_test.go"}, Tests: archtest.WithTests},
			Check: archtest.ForbidImports("internal/tools"),
			Hint:  "the table describes what the services serve; a service never reads it, and only the tests that hold it to the registries do",
		},
	}, nil)
}
