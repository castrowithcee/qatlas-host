package compose

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/castrowithcee/qatlas-host/internal/registry"
	"github.com/castrowithcee/qatlas-host/internal/registrylogin"
	"github.com/castrowithcee/qatlas-host/internal/stacks"
)

// maxDeniedHosts bounds the registries named when the output does not name one.
const maxDeniedHosts = 3

var (
	// registryHostRe is what an adopted registry host must match.
	registryHostRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]*[a-z0-9])?(:[0-9]{1,5})?$`)
	// denialMarkers are the phrases of a refusal by the registry, matched on the lowercase output.
	denialMarkers = []string{"unauthorized", "denied", "authentication required", "no basic auth credentials", "docker login"}
	// requestHostRe finds the registry of a failed request: Head "https://<host>/v2/...".
	requestHostRe = regexp.MustCompile(`https?://([^/\s"']+)/v2/`)
	// deniedForRe finds the image of "pull access denied for <image>,".
	deniedForRe = regexp.MustCompile(`denied for ([^\s,]+)`)
)

// deniedError is the cause of a pull the registry refused. Its text is built from the stack and, at most, a
// host that passed registryHostRe; nothing else of Docker's output is in it.
type deniedError struct {
	text string
	err  error
}

func (e *deniedError) Error() string { return e.text }
func (e *deniedError) Unwrap() error { return e.err }

// explainDenied returns err unchanged unless the failed call ended with a non-zero exit and its output shows a
// refusal by a registry. Docker's output is foreign content: it is only matched, and the one thing taken from
// it is a host that has the shape of a registry host.
func explainDenied(st stacks.Stack, output string, err error) error {
	var exit *ExitError
	if !errors.As(err, &exit) || !deniedOutput(output) {
		return err
	}
	if host := deniedHost(output); host != "" {
		if name, ok := registrylogin.Name(host); ok {
			return &deniedError{fmt.Sprintf("the registry %s denied access; store a login as credential %s (see qhost help credentials)", displayHost(host), name), err}
		}
	}
	var parts []string
	seen := map[string]bool{}
	add := func(image string) {
		repo, perr := registry.ParseRepo(image)
		if perr != nil || len(parts) == maxDeniedHosts {
			return
		}
		if name, ok := registrylogin.Name(repo.Host); ok && !seen[name] {
			seen[name] = true
			parts = append(parts, fmt.Sprintf("%s for %s", name, displayHost(repo.Host)))
		}
	}
	for _, img := range st.Images {
		add(img.Image)
	}
	for _, f := range st.Findings {
		add(f.Image)
	}
	if len(parts) == 0 {
		return err
	}
	return &deniedError{"a registry denied access; store a login as credential " + strings.Join(parts, " or ") + " (see qhost help credentials)", err}
}

func displayHost(host string) string {
	if host == "registry-1.docker.io" {
		return "docker.io"
	}
	return host
}

func deniedOutput(output string) bool {
	// A refused Docker socket is not a refused registry.
	s := strings.ReplaceAll(strings.ToLower(output), "permission denied", "")
	for _, m := range denialMarkers {
		if strings.Contains(s, m) {
			return true
		}
	}
	return false
}

// deniedHost returns the registry host the output names, or "" when it names none that fits.
func deniedHost(output string) string {
	s := strings.ToLower(output)
	host := ""
	if m := requestHostRe.FindStringSubmatch(s); m != nil {
		host = m[1]
	} else if m := deniedForRe.FindStringSubmatch(s); m != nil {
		if first, _, ok := strings.Cut(m[1], "/"); ok && (strings.ContainsAny(first, ".:") || first == "localhost") {
			host = first
		}
	}
	if len(host) > 255 || !registryHostRe.MatchString(host) {
		return ""
	}
	return host
}
