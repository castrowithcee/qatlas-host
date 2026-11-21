// Package packaging carries the unit files of the services, so the program can install the deploy units
// without a release archive at hand.
package packaging

import _ "embed"

// DeployUnit is the systemd unit of the deploy service, as shipped in the release archive.
//
//go:embed qhost-deploy.service
var DeployUnit []byte

// DeployInstanceUnit is the template unit of a deploy service instance for the rootless engine of a user. It is
// not in the release archive, because the archive allows no '@' in its paths; the setup writes it.
//
//go:embed qhost-deploy@.service
var DeployInstanceUnit []byte

// HookUnit is the systemd unit of the webhook receiver. The setup installs it; it is not in the release archive.
//
//go:embed qhost-hook.service
var HookUnit []byte
