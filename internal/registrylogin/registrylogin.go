// Package registrylogin maps a registry host to the name of the credential that holds its login and parses
// that credential's value. The update check and the Docker credential helper share it.
package registrylogin

import (
	"errors"
	"strings"

	"github.com/castrowithcee/qatlas-host/internal/credentials"
	"github.com/castrowithcee/qatlas-host/internal/registry"
)

// dockerHubAPIHost is the host registry.ParseRepo gives Docker Hub images.
const dockerHubAPIHost = "registry-1.docker.io"

// ErrMalformed means a stored login is not "<user>:<token>". Its text never contains the value.
var ErrMalformed = errors.New("the stored registry login is malformed")

// Name derives the credential name for a registry host: "registry." plus the lowercase host with the port
// separator written as '-'. Docker Hub uses "registry.docker.io". ok is false when the result is no valid
// credential name.
func Name(host string) (string, bool) {
	host = strings.ToLower(host)
	if host == dockerHubAPIHost {
		host = "docker.io"
	}
	name := "registry." + strings.ReplaceAll(host, ":", "-")
	return name, credentials.ValidName(name)
}

// Host reduces a server address as Docker hands it over to a bare host: scheme and everything from the first
// '/' are dropped and the rest is lowercased. Docker Hub's index host is the Hub API host.
func Host(server string) string {
	server = strings.TrimSpace(server)
	server = strings.TrimPrefix(strings.TrimPrefix(server, "https://"), "http://")
	if i := strings.IndexByte(server, '/'); i >= 0 {
		server = server[:i]
	}
	server = strings.ToLower(server)
	if server == "index.docker.io" {
		return dockerHubAPIHost
	}
	return server
}

// Parse splits "<user>:<token>" at the first ':'; both parts must be non-empty. The error never contains the
// value.
func Parse(value string) (registry.Credential, error) {
	user, token, ok := strings.Cut(value, ":")
	if !ok || user == "" || token == "" {
		return registry.Credential{}, ErrMalformed
	}
	return registry.Credential{User: user, Token: token}, nil
}
