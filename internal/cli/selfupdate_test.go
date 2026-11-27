package cli

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/castrowithcee/qatlas-kit/release/releasetest"
	"github.com/castrowithcee/qatlas-kit/sysservice"

	"github.com/castrowithcee/qatlas-host/internal/release"
)

const apiRepo = "/repos/castrowithcee/qatlas-host"

type fakeAsset struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type fakeRelease struct {
	TagName string      `json:"tag_name"`
	Assets  []fakeAsset `json:"assets"`
}

// fakeReleases serves the GitHub API subset that selfupdate reads, with assets signed by signer.
type fakeReleases struct {
	server   *httptest.Server
	releases []fakeRelease
	blobs    map[string][]byte
}

// defaultRequirements is the update requirements file of every fake release that requirements leaves out.
const defaultRequirements = `{"updatesFrom": "v0.1.0", "reinstall": ""}`

func newFakeReleases(t *testing.T, signer ssh.Signer, tamper bool, tags ...string) *fakeReleases {
	t.Helper()
	return newFakeReleasesWith(t, signer, tamper, nil, tags...)
}

// newFakeReleasesWith is newFakeReleases with the update requirements file of some releases, by tag.
func newFakeReleasesWith(t *testing.T, signer ssh.Signer, tamper bool, requirements map[string]string, tags ...string) *fakeReleases {
	t.Helper()
	f := &fakeReleases{blobs: map[string][]byte{}}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.server.Close)
	id := 0
	for _, tag := range tags {
		name := fmt.Sprintf("qhost_%s_linux_%s.tar.gz", tag, runtime.GOARCH)
		archive := releasetest.Archive(t, name,
			releasetest.File{Name: "bin/qhost", Body: []byte("new program " + tag)},
			releasetest.File{Name: sysservice.UnitEntry("qhost"), Body: []byte("[Service]\n# unit " + tag + "\n")},
			releasetest.File{Name: sysservice.UnitEntry("qhost-deploy"), Body: []byte("[Service]\n# deploy unit " + tag + "\n")},
			releasetest.File{Name: manEntry, Body: []byte(".TH QHOST 1 " + tag + "\n")},
			releasetest.File{Name: release.RequirementsEntry, Body: []byte(cmp.Or(requirements[tag], defaultRequirements))})
		assets := releasetest.Release(t, signer, release.Namespace, release.SignatureAsset, map[string][]byte{name: archive})
		if tamper {
			assets["checksums.txt"] = append(assets["checksums.txt"], '\n')
		}
		rel := fakeRelease{TagName: tag}
		for _, n := range []string{name, "checksums.txt", release.SignatureAsset} {
			id++
			rel.Assets = append(rel.Assets, fakeAsset{Name: n, URL: f.server.URL + apiRepo + "/releases/assets/" + strconv.Itoa(id)})
			f.blobs[strconv.Itoa(id)] = assets[n]
		}
		f.releases = append(f.releases, rel)
	}
	return f
}

func (f *fakeReleases) serve(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == apiRepo+"/releases":
		if r.URL.Query().Get("page") != "1" {
			_ = json.NewEncoder(w).Encode([]fakeRelease{})
			return
		}
		_ = json.NewEncoder(w).Encode(f.releases)
	case strings.HasPrefix(r.URL.Path, apiRepo+"/releases/tags/"):
		for _, rel := range f.releases {
			if rel.TagName == strings.TrimPrefix(r.URL.Path, apiRepo+"/releases/tags/") {
				_ = json.NewEncoder(w).Encode(rel)
				return
			}
		}
		http.NotFound(w, r)
	case strings.HasPrefix(r.URL.Path, apiRepo+"/releases/assets/"):
		if blob, ok := f.blobs[strings.TrimPrefix(r.URL.Path, apiRepo+"/releases/assets/")]; ok {
			_, _ = w.Write(blob)
			return
		}
		http.NotFound(w, r)
	default:
		http.NotFound(w, r)
	}
}

type fakeHost struct {
	dir  string
	exe  string
	unit string
	// deployUnit is where the unit of the deploy service is installed, if it is.
	deployUnit string
	// calls are the systemctl invocations, one string each.
	calls []string
	// migrated holds, for each run of the transitions of the new program, the program and the number of service
	// restarts before it.
	migrated []migrateCall
}

type migrateCall struct {
	exe      string
	restarts int
}

// restarted lists the services restarted, in order.
func (h *fakeHost) restarted() []string {
	var out []string
	for _, c := range h.calls {
		if name, ok := strings.CutPrefix(c, "restart "); ok {
			out = append(out, name)
		}
	}
	return out
}

// useFakeHost points the self-update at the fake release server, trusts signer and records systemctl calls. The
// units live below the fake host's root; the deploy unit is installed only if withDeploy is set.
func useFakeHost(t *testing.T, f *fakeReleases, signer ssh.Signer, running string, withDeploy ...bool) *fakeHost {
	t.Helper()
	h := &fakeHost{dir: t.TempDir()}
	h.exe = filepath.Join(h.dir, "qhost")
	unitDir := filepath.Join(h.dir, "usr/local/lib/systemd/system")
	h.unit = filepath.Join(unitDir, "qhost.service")
	h.deployUnit = filepath.Join(unitDir, "qhost-deploy.service")
	if err := os.MkdirAll(unitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(h.dir, "usr/local/share/man/man1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(h.exe, []byte("old program"), 0o755); err != nil {
		t.Fatal(err)
	}
	if len(withDeploy) > 0 && withDeploy[0] {
		if err := os.WriteFile(h.deployUnit, []byte("old deploy unit"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	oldHost, oldVersion := selfUpdate, version
	t.Cleanup(func() { selfUpdate, version = oldHost, oldVersion })
	version = running
	selfUpdate = selfUpdateHost{
		apiURL:     f.server.URL + apiRepo,
		keys:       func() []ssh.PublicKey { return []ssh.PublicKey{signer.PublicKey()} },
		euid:       func() int { return 0 },
		executable: func() (string, error) { return h.exe, nil },
		root:       h.dir,
		systemctl: func(_ context.Context, args ...string) error {
			h.calls = append(h.calls, strings.Join(args, " "))
			return nil
		},
		migrateRun: func(_ context.Context, exe string, _ io.Writer) error {
			h.migrated = append(h.migrated, migrateCall{exe: exe, restarts: len(h.restarted())})
			return nil
		},
	}
	return h
}

func (h *fakeHost) program(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile(h.exe)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestSelfUpdateStatusReportsNewerRelease(t *testing.T) {
	signer := releasetest.NewSigner(t)
	h := useFakeHost(t, newFakeReleases(t, signer, false, "v0.1.0", "v0.2.0"), signer, "v0.1.0")
	selfUpdate.euid = func() int { return 1000 }
	code, stdout, stderr := run(t, "update", "status", "--output", "json")
	if code != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatal(err)
	}
	if got["installed"] != "v0.1.0" || got["latest"] != "v0.2.0" || got["update_available"] != true {
		t.Fatalf("status = %v", got)
	}
	if h.program(t) != "old program" || len(h.calls) != 0 {
		t.Fatal("status changed the installation")
	}
}

func TestSelfUpdateInstallsLatestAndRestarts(t *testing.T) {
	signer := releasetest.NewSigner(t)
	h := useFakeHost(t, newFakeReleases(t, signer, false, "v0.1.0", "v0.2.0"), signer, "v0.1.0")
	code, stdout, stderr := run(t, "update", "--yes", "--confirm", "--output", "json")
	if code != 0 {
		t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if h.program(t) != "new program v0.2.0" {
		t.Fatalf("program = %q", h.program(t))
	}
	if got := h.restarted(); !slices.Equal(got, []string{"qhost"}) || !slices.Contains(h.calls, "daemon-reload") {
		t.Fatalf("systemctl calls = %q", h.calls)
	}
	if _, err := os.Lstat(h.deployUnit); err == nil {
		t.Fatal("the deploy unit was installed on a host that has not set it up")
	}
	if body, err := os.ReadFile(h.unit); err != nil || !strings.Contains(string(body), "# unit v0.2.0") {
		t.Fatalf("unit = %q, %v", body, err)
	}
	if !strings.Contains(stdout, `"updated":true`) && !strings.Contains(stdout, `"updated": true`) {
		t.Fatalf("stdout %q", stdout)
	}
}

func TestSelfUpdateReplacesAndRestartsBothUnitsWhenDeployIsSetUp(t *testing.T) {
	signer := releasetest.NewSigner(t)
	h := useFakeHost(t, newFakeReleases(t, signer, false, "v0.1.0", "v0.2.0"), signer, "v0.1.0", true)
	if code, stdout, stderr := run(t, "update", "--yes", "--confirm"); code != 0 {
		t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if got := h.restarted(); !slices.Equal(got, []string{"qhost", "qhost-deploy"}) {
		t.Fatalf("restarted = %q, calls = %q", got, h.calls)
	}
	for path, want := range map[string]string{h.unit: "# unit v0.2.0", h.deployUnit: "# deploy unit v0.2.0"} {
		if body, err := os.ReadFile(path); err != nil || !strings.Contains(string(body), want) {
			t.Fatalf("%s = %q, %v", path, body, err)
		}
	}
}

func TestSelfUpdateWarnsAboutAShadowingUnitOfEachService(t *testing.T) {
	signer := releasetest.NewSigner(t)
	h := useFakeHost(t, newFakeReleases(t, signer, false, "v0.1.0", "v0.2.0"), signer, "v0.1.0", true)
	shadow := filepath.Join(h.dir, "etc/systemd/system")
	if err := os.MkdirAll(shadow, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"qhost.service", "qhost-deploy.service"} {
		if err := os.WriteFile(filepath.Join(shadow, name), []byte("[Service]\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	code, _, stderr := run(t, "update", "--yes", "--confirm")
	if code != 0 || !strings.Contains(stderr, "installed unit of qhost\n") || !strings.Contains(stderr, "installed unit of qhost-deploy\n") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
}

func TestSelfUpdateToNamedVersion(t *testing.T) {
	signer := releasetest.NewSigner(t)
	h := useFakeHost(t, newFakeReleases(t, signer, false, "v0.1.0", "v0.2.0", "v0.3.0"), signer, "v0.1.0")
	if code, stdout, stderr := run(t, "update", "v0.2.0", "--yes", "--confirm"); code != 0 {
		t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if h.program(t) != "new program v0.2.0" || len(h.restarted()) != 1 {
		t.Fatalf("program %q, restarts %v", h.program(t), h.restarted())
	}
}

func TestSelfUpdateRefusesTamperedSignature(t *testing.T) {
	signer := releasetest.NewSigner(t)
	h := useFakeHost(t, newFakeReleases(t, signer, true, "v0.1.0", "v0.2.0"), signer, "v0.1.0")
	code, _, stderr := run(t, "update", "--yes", "--confirm")
	if code == 0 {
		t.Fatal("a release with a manipulated checksum file was installed")
	}
	if !strings.Contains(stderr, "update:") {
		t.Fatalf("stderr %q", stderr)
	}
	if h.program(t) != "old program" || len(h.calls) != 0 {
		t.Fatal("the installation changed or the service restarted")
	}
}

func TestSelfUpdateRefusesUntrustedKey(t *testing.T) {
	signer := releasetest.NewSigner(t)
	h := useFakeHost(t, newFakeReleases(t, signer, false, "v0.1.0", "v0.2.0"), signer, "v0.1.0")
	other := releasetest.NewSigner(t)
	selfUpdate.keys = func() []ssh.PublicKey { return []ssh.PublicKey{other.PublicKey()} }
	if code, _, _ := run(t, "update", "--yes", "--confirm"); code == 0 {
		t.Fatal("a release signed by an untrusted key was installed")
	}
	if h.program(t) != "old program" || len(h.calls) != 0 {
		t.Fatal("the installation changed or the service restarted")
	}
}

func TestSelfUpdateNeedsRootAndConfirm(t *testing.T) {
	signer := releasetest.NewSigner(t)
	h := useFakeHost(t, newFakeReleases(t, signer, false, "v0.1.0", "v0.2.0"), signer, "v0.1.0")
	selfUpdate.euid = func() int { return 1000 }
	if code, _, stderr := run(t, "update", "--yes", "--confirm"); code == 0 || !strings.Contains(stderr, "root") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	selfUpdate.euid = func() int { return 0 }
	if code, _, _ := run(t, "update", "--yes"); code == 0 {
		t.Fatal("update ran without --confirm")
	}
	if h.program(t) != "old program" || len(h.calls) != 0 {
		t.Fatal("the installation changed")
	}
}

func TestSelfUpdateWithoutConfirmShowsThePlanWithoutTheNetwork(t *testing.T) {
	signer := releasetest.NewSigner(t)
	releases := newFakeReleases(t, signer, false, "v0.1.0", "v0.2.0")
	h := useFakeHost(t, releases, signer, "v0.1.0", true)
	// Any request to the release server fails the test.
	releases.server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("the release server was asked: %s", r.URL.Path)
		http.NotFound(w, r)
	})
	for _, args := range [][]string{{"update"}, {"update", "v0.2.0", "--yes"}} {
		code, stdout, stderr := run(t, args...)
		if code != 2 || stdout != "" || !strings.Contains(stderr, "--confirm") {
			t.Fatalf("%v: code %d, stdout %q, stderr %q", args, code, stdout, stderr)
		}
		for _, want := range []string{"from v0.1.0 to", h.exe, h.unit, h.deployUnit, "qhost.1", "qhost, qhost-deploy"} {
			if strings.Count(stderr, want) != 1 {
				t.Errorf("%v: %q must appear once in %q", args, want, stderr)
			}
		}
	}
	if code, _, stderr := run(t, "update", "v0.2.0"); code != 2 || !strings.Contains(stderr, "release v0.2.0") {
		t.Errorf("a named version: code %d, stderr %q", code, stderr)
	}
	if h.program(t) != "old program" || len(h.calls) != 0 {
		t.Fatal("the installation changed or a service was touched")
	}
	selfUpdate.euid = func() int { return 1000 }
	if code, _, stderr := run(t, "update"); code == 0 || !strings.Contains(stderr, "root") || strings.Contains(stderr, "These files") {
		t.Errorf("not root: code %d, stderr %q", code, stderr)
	}
}

func TestSelfUpdateNothingToDo(t *testing.T) {
	signer := releasetest.NewSigner(t)
	h := useFakeHost(t, newFakeReleases(t, signer, false, "v0.1.0", "v0.2.0"), signer, "v0.2.0")
	if code, stdout, stderr := run(t, "update", "--yes", "--confirm"); code != 0 || !strings.Contains(stdout, "false") {
		t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if code, _, _ := run(t, "update", "v0.1.0", "--yes", "--confirm"); code != 2 {
		t.Fatalf("an older version gave code %d, want 2", code)
	}
	if h.program(t) != "old program" || len(h.calls) != 0 {
		t.Fatal("the installation changed")
	}
}

func TestSelfUpdateDevelopmentBuild(t *testing.T) {
	signer := releasetest.NewSigner(t)
	useFakeHost(t, newFakeReleases(t, signer, false, "v0.1.0"), signer, "dev")
	for _, args := range [][]string{{"update", "status"}, {"update", "--yes", "--confirm"}} {
		if code, _, stderr := run(t, args...); code == 0 || !strings.Contains(stderr, "development build") {
			t.Fatalf("%v: code %d, stderr %q", args, code, stderr)
		}
	}
}

func TestStackUpdateStaysSeparate(t *testing.T) {
	code, stdout, _ := run(t, "stack", "update", "--help")
	if code != 0 || !strings.Contains(stdout, "<stack>") {
		t.Fatalf("code %d, stdout %q", code, stdout)
	}
}

// installHookUnit puts a unit of the webhook receiver on the fake host and records the renewals the update asks for.
func (h *fakeHost) installHookUnit(t *testing.T) *[]string {
	t.Helper()
	unit := filepath.Join(h.dir, "usr/local/lib/systemd/system/qhost-hook.service")
	if err := os.WriteFile(unit, []byte("old hook unit"), 0o644); err != nil {
		t.Fatal(err)
	}
	var renewed []string
	selfUpdate.refreshHook = func(_ context.Context, exe string, _ io.Writer) error {
		renewed = append(renewed, exe)
		return nil
	}
	return &renewed
}

func TestSelfUpdateRenewsTheWebhookReceiverOnlyWhereItIsSetUp(t *testing.T) {
	signer := releasetest.NewSigner(t)
	h := useFakeHost(t, newFakeReleases(t, signer, false, "v0.1.0", "v0.2.0"), signer, "v0.1.0")
	renewed := h.installHookUnit(t)
	os.Remove(filepath.Join(h.dir, "usr/local/lib/systemd/system/qhost-hook.service"))
	if code, _, stderr := run(t, "update", "--yes", "--confirm"); code != 0 || len(*renewed) != 0 {
		t.Fatalf("code %d, renewed %v, stderr %q", code, *renewed, stderr)
	}

	h = useFakeHost(t, newFakeReleases(t, signer, false, "v0.1.0", "v0.2.0"), signer, "v0.1.0")
	renewed = h.installHookUnit(t)
	if code, _, stderr := run(t, "update", "--yes", "--confirm"); code != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if len(*renewed) != 1 || (*renewed)[0] != h.exe {
		t.Fatalf("renewed = %v, want one call of the installed program", *renewed)
	}
}

func TestSelfUpdateWarnsWhenTheWebhookReceiverCannotBeRenewed(t *testing.T) {
	signer := releasetest.NewSigner(t)
	h := useFakeHost(t, newFakeReleases(t, signer, false, "v0.1.0", "v0.2.0"), signer, "v0.1.0")
	h.installHookUnit(t)
	selfUpdate.refreshHook = func(context.Context, string, io.Writer) error { return errors.New("boom") }
	code, _, stderr := run(t, "update", "--yes", "--confirm")
	if code != 0 || !strings.Contains(stderr, "webhook receiver was not renewed") || !strings.Contains(stderr, "hook-service refresh") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
}

func TestSelfUpdatePlanNamesTheWebhookReceiverOnlyWhereItIsSetUp(t *testing.T) {
	signer := releasetest.NewSigner(t)
	h := useFakeHost(t, newFakeReleases(t, signer, false, "v0.1.0", "v0.2.0"), signer, "v0.1.0")
	if _, _, stderr := run(t, "update"); strings.Contains(stderr, "qhost-hook") {
		t.Fatalf("stderr %q", stderr)
	}
	h.installHookUnit(t)
	if _, _, stderr := run(t, "update"); !strings.Contains(stderr, "qhost-hook.service is renewed") {
		t.Fatalf("stderr %q", stderr)
	}
}
