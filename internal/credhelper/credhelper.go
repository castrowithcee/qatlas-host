// Package credhelper implements the Docker credential helper protocol on top of the service's credential
// store, read-only. Docker calls the binary as docker-credential-qhost and gets the stored registry login.
package credhelper

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/castrowithcee/qatlas-host/internal/credentials"
	"github.com/castrowithcee/qatlas-host/internal/registrylogin"
)

const (
	// Name is the base name the binary is called by to act as the helper.
	Name = "docker-credential-qhost"
	// StateDirEnv carries the absolute state directory of the deploy service that owns the store.
	StateDirEnv = "QHOST_STATE_DIR"

	// notFound is the text Docker's helper protocol defines for "no login stored".
	notFound = "credentials not found in native keychain"
	// maxServerBytes bounds what is read from stdin; a registry address is far shorter.
	maxServerBytes = 2048
)

type response struct {
	ServerURL string
	Username  string
	Secret    string
}

// Run executes one helper command. It writes a secret to stdout only in the answer to get; every message on
// stderr is a fixed text that carries neither a value nor the address read from stdin.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer, getenv func(string) string) int {
	if len(args) != 1 {
		return fail(stderr, "usage: "+Name+" get|list")
	}
	switch args[0] {
	case "list":
		fmt.Fprintln(stdout, "{}")
		return 0
	case "get":
		return get(stdin, stdout, stderr, getenv)
	default:
		return fail(stderr, "this credential helper is read-only: only get and list are supported")
	}
}

func fail(stderr io.Writer, msg string) int {
	fmt.Fprintln(stderr, msg)
	return 1
}

func get(stdin io.Reader, stdout, stderr io.Writer, getenv func(string) string) int {
	raw, err := io.ReadAll(io.LimitReader(stdin, maxServerBytes+1))
	server := strings.TrimSpace(string(raw))
	if err != nil || len(raw) > maxServerBytes || server == "" {
		return fail(stderr, "the server address is missing or invalid")
	}
	name, ok := registrylogin.Name(registrylogin.Host(server))
	if !ok {
		return notFoundAnswer(stdout)
	}
	value, err := credentials.ReadValue(getenv(StateDirEnv), name)
	switch {
	case errors.Is(err, credentials.ErrNotFound):
		return notFoundAnswer(stdout)
	case err != nil:
		return fail(stderr, "the credential store cannot be read; check "+StateDirEnv+" and the store's permissions")
	}
	cred, err := registrylogin.Parse(value)
	if err != nil {
		return fail(stderr, registrylogin.ErrMalformed.Error())
	}
	out, err := json.Marshal(response{ServerURL: server, Username: cred.User, Secret: cred.Token})
	if err != nil {
		return fail(stderr, "the answer cannot be encoded")
	}
	fmt.Fprintln(stdout, string(out))
	return 0
}

func notFoundAnswer(stdout io.Writer) int {
	fmt.Fprintln(stdout, notFound)
	return 1
}
