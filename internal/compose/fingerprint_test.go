package compose

import (
	"context"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/engine"
)

var (
	idA   = "sha256:" + strings.Repeat("a", 64)
	idB   = "sha256:" + strings.Repeat("b", 64)
	digC  = "sha256:" + strings.Repeat("c", 64)
	digD  = "sha256:" + strings.Repeat("d", 64)
	cWeb  = strings.Repeat("1", 12)
	cDB   = strings.Repeat("2", 12)
	psTwo = `{"ID":"` + cWeb + `","Service":"web","Image":"registry.example.test/demo/web:1.0","State":"running"}
{"ID":"` + cDB + `","Service":"db","Image":"demo-db:local","State":"running"}
{"ID":"` + strings.Repeat("3", 12) + `","Service":"job","Image":"demo-job","State":"exited"}`
)

// dockerRunner answers ps and the docker inspect calls of the fingerprint read.
type dockerRunner struct {
	ps       string
	imageOf  map[string]string // container -> answer of container inspect
	digests  map[string]string // image ID -> answer of image inspect
	failCont bool
	failPs   bool
}

func (r *dockerRunner) Run(_ context.Context, c Command) (Result, error) {
	write := func(s string) { _, _ = c.Stdout.Write([]byte(s)) }
	switch {
	case c.Args[0] == "compose" && r.failPs:
		return Result{}, &ExitError{Code: 1}
	case c.Args[0] == "compose":
		write(r.ps)
	case r.failCont && c.Args[0] == "container":
		return Result{}, &ExitError{Code: 1}
	case c.Args[0] == "container":
		write(r.imageOf[c.Args[len(c.Args)-1]] + "\n")
	case c.Args[0] == "image":
		a, ok := r.digests[c.Args[len(c.Args)-1]]
		if !ok {
			return Result{}, &ExitError{Code: 1}
		}
		write(a)
	}
	return Result{}, nil
}

func TestImageFingerprints(t *testing.T) {
	st := newStack(t, false)
	web := `["other.example.test/x@` + digD + `","registry.example.test/demo/web@` + digC + `"]`
	cases := map[string]struct {
		r    dockerRunner
		want []engine.ImageFingerprint
	}{
		"registry and local image": {
			r: dockerRunner{ps: psTwo, imageOf: map[string]string{cWeb: idA, cDB: idB},
				digests: map[string]string{idA: web, idB: `[]`}},
			want: []engine.ImageFingerprint{
				{Service: "web", Image: "registry.example.test/demo/web:1.0", RepoDigest: digC, ImageID: idA},
				{Service: "db", Image: "demo-db:local", ImageID: idB},
			},
		},
		"invalid formats are missing": {
			r: dockerRunner{ps: psTwo, imageOf: map[string]string{cWeb: "sha256:xyz", cDB: idB},
				digests: map[string]string{idB: `["demo-db@sha256:short", "not json"]`}},
			want: []engine.ImageFingerprint{
				{Service: "web", Image: "registry.example.test/demo/web:1.0"},
				{Service: "db", Image: "demo-db:local", ImageID: idB},
			},
		},
		"unparsable digest list": {
			r: dockerRunner{ps: psTwo, imageOf: map[string]string{cWeb: idA, cDB: idB},
				digests: map[string]string{idA: `{"x":1}`}},
			want: []engine.ImageFingerprint{
				{Service: "web", Image: "registry.example.test/demo/web:1.0", ImageID: idA},
				{Service: "db", Image: "demo-db:local", ImageID: idB},
			},
		},
		"read failure": {
			r: dockerRunner{ps: psTwo, failCont: true},
			want: []engine.ImageFingerprint{
				{Service: "web", Image: "registry.example.test/demo/web:1.0"},
				{Service: "db", Image: "demo-db:local"},
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			x := &Executor{Runner: &tc.r, Env: []string{"PATH=/bin"}}
			got, err := x.ImageFingerprints(context.Background(), st, &fakeLog{})
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("entry %d: got %+v, want %+v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestImageFingerprintsListingFails(t *testing.T) {
	x := &Executor{Runner: &dockerRunner{failPs: true}, Env: []string{"PATH=/bin"}}
	if _, err := x.ImageFingerprints(context.Background(), newStack(t, false), &fakeLog{}); err == nil {
		t.Fatal("a failing listing must be an error")
	}
}

func TestPickDigest(t *testing.T) {
	for ref, want := range map[string]string{
		"nginx:1.27":              digC,
		"docker.io/library/nginx": digC,
		"nginx@" + digD:           digC,
		"localhost:5000/nginx:1":  "",
		"":                        "",
	} {
		if got := pickDigest(ref, []string{"localhost:5000/other@" + digD, "nginx@" + digC}); got != want {
			t.Errorf("%q: got %q, want %q", ref, got, want)
		}
	}
}
