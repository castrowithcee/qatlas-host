// Package packaging carries the unit files of the services, so the program can install the deploy unit
// without a release archive at hand.
package packaging

import _ "embed"

// DeployUnit is the systemd unit of the deploy service, as shipped in the release archive.
//
//go:embed qhost-deploy.service
var DeployUnit []byte
