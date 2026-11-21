package compose

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/stacks"
)

func deniedStack() stacks.Stack {
	return stacks.Stack{
		Name: "demo-stack",
		Images: []stacks.ImageVar{
			{Image: "host.example:5000/demo/app", Var: "APP_TAG"},
			{Image: "ghcr.io/demo/web", Var: "WEB_TAG"},
			{Image: "nginx", Var: "NGINX_TAG"},
		},
		Findings: []stacks.Finding{{Image: "ghcr.io/demo/web"}},
	}
}

func TestExplainDeniedNamesTheHostOfTheOutput(t *testing.T) {
	for _, tc := range []struct {
		name, output, host, cred string
	}{
		{"head unauthorized", ` ! app Error Head "https://host.example:5000/v2/demo/app/manifests/1.0.0": unauthorized`, "host.example:5000", "registry.host.example-5000"},
		{"get no basic auth", `Error response from daemon: Get "https://ghcr.io/v2/": no basic auth credentials`, "ghcr.io", "registry.ghcr.io"},
		{"upper case", `Head "https://GHCR.IO/v2/demo/web/manifests/1": Unauthorized`, "ghcr.io", "registry.ghcr.io"},
		{"denied for host image", "pull access denied for ghcr.io/demo/web, repository does not exist or may require 'docker login'", "ghcr.io", "registry.ghcr.io"},
		{"hub", `Head "https://registry-1.docker.io/v2/demo/app/manifests/1": denied: requested access to the resource is denied`, "docker.io", "registry.docker.io"},
	} {
		got := explainDenied(deniedStack(), tc.output, &ExitError{Code: 1})
		want := "the registry " + tc.host + " denied access; store a login as credential " + tc.cred + " (see qhost help credentials)"
		if got == nil || got.Error() != want {
			t.Errorf("%s: got %v, want %q", tc.name, got, want)
		}
		var ee *ExitError
		if !errors.As(got, &ee) {
			t.Errorf("%s: the exit error is lost", tc.name)
		}
	}
}

func TestExplainDeniedFallsBackToTheImagesOfTheStack(t *testing.T) {
	for _, output := range []string{
		"Error response from daemon: unauthorized: authentication required",
		"denied: requested access to the resource is denied",
		"pull access denied for nginx, repository does not exist or may require 'docker login'",
		"Error: no basic auth credentials",
		// a host that does not fit the pattern is not taken over
		`Head "https://bad_host$(x)/v2/x": unauthorized`,
		`Head "https://` + strings.Repeat("a", 300) + `/v2/x": unauthorized`,
	} {
		got := explainDenied(deniedStack(), output, &ExitError{Code: 1})
		want := "a registry denied access; store a login as credential registry.host.example-5000 for host.example:5000 or " +
			"registry.ghcr.io for ghcr.io or registry.docker.io for docker.io (see qhost help credentials)"
		if got == nil || got.Error() != want {
			t.Errorf("%q: got %v, want %q", output, got, want)
		}
		if strings.Contains(got.Error(), "bad_host") || strings.Contains(got.Error(), "aaaa") {
			t.Errorf("%q: foreign content in %q", output, got)
		}
	}
}

func TestExplainDeniedLeavesOtherErrorsAlone(t *testing.T) {
	exit := &ExitError{Code: 1}
	for _, output := range []string{
		"boom",
		"Cannot connect to the Docker daemon at unix:///var/run/docker.sock",
		"permission denied while trying to connect to the Docker daemon socket",
		`Head "https://host.example:5000/v2/x": dial tcp: connection refused`,
	} {
		if got := explainDenied(deniedStack(), output, exit); got != error(exit) {
			t.Errorf("%q: got %v", output, got)
		}
	}
	// Only a failed exit counts: a timeout stays a timeout.
	timeout := errors.New("docker compose pull timed out after 10m0s")
	if got := explainDenied(deniedStack(), "unauthorized", timeout); got != timeout {
		t.Errorf("timeout: got %v", got)
	}
	// Without any image to name, the error stays as it is.
	if got := explainDenied(stacks.Stack{Name: "x"}, "unauthorized", exit); got != error(exit) {
		t.Errorf("no images: got %v", got)
	}
}

func TestPullAndUpExplainADenial(t *testing.T) {
	for _, verb := range []string{"pull", "up"} {
		st := newStack(t, false)
		st.Images = []stacks.ImageVar{{Image: "ghcr.io/demo/web", Var: "APP_TAG"}}
		r := &fakeRunner{failOn: verb, ps: []string{runningJSON}}
		x := &Executor{Runner: &outputRunner{fakeRunner: r, out: "Error response from daemon: unauthorized: authentication required"}, Env: []string{"PATH=/bin"}}
		var err error
		if verb == "pull" {
			err = x.Pull(context.Background(), st, &fakeLog{})
		} else {
			err = x.Up(context.Background(), st, &fakeLog{})
		}
		if err == nil || !strings.Contains(err.Error(), "store a login as credential registry.ghcr.io for ghcr.io") {
			t.Errorf("%s: err = %v", verb, err)
		}
	}
}

// outputRunner lets the failing call of a fakeRunner carry a chosen output.
type outputRunner struct {
	*fakeRunner
	out string
}

func (r *outputRunner) Run(ctx context.Context, c Command) (Result, error) {
	res, err := r.fakeRunner.Run(ctx, c)
	if err != nil {
		res.Output = []byte(r.out)
	}
	return res, err
}
