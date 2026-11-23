package compose

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/engine"
	"github.com/castrowithcee/qatlas-host/internal/idmap"
)

const demoID = "0123456789abcdef0123456789abcdef"

// imageRunner answers the docker calls of the data directory check.
type imageRunner struct {
	config  string
	user    string // answer of image inspect
	noImage bool   // image inspect fails, also after a pull
	passwd  string
	group   string
	calls   []string
}

func (r *imageRunner) Run(_ context.Context, c Command) (Result, error) {
	r.calls = append(r.calls, strings.Join(c.Args, " "))
	write := func(b []byte) { _, _ = c.Stdout.Write(b) }
	switch {
	case len(c.Args) > 1 && c.Args[0] == "compose" && commandVerb(c.Args) == "config":
		write([]byte(r.config))
	case len(c.Args) > 1 && c.Args[0] == "compose":
		return Result{}, nil
	case c.Args[0] == "image":
		if r.noImage {
			return Result{}, &ExitError{Code: 1}
		}
		b, _ := json.Marshal(r.user)
		write(b)
	case c.Args[0] == "create":
		write([]byte(demoID + "\n"))
	case c.Args[0] == "cp":
		content := r.passwd
		if strings.HasSuffix(c.Args[len(c.Args)-2], "/etc/group") {
			content = r.group
		}
		if content == "" {
			return Result{}, &ExitError{Code: 1}
		}
		var b bytes.Buffer
		tw := tar.NewWriter(&b)
		_ = tw.WriteHeader(&tar.Header{Name: "passwd", Mode: 0o644, Size: int64(len(content)), Typeflag: tar.TypeReg})
		_, _ = tw.Write([]byte(content))
		_ = tw.Close()
		write(b.Bytes())
	}
	return Result{}, nil
}

func (r *imageRunner) ran(prefix string) bool {
	for _, c := range r.calls {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

func configJSON(t *testing.T, services map[string]map[string]any) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{"services": services})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func bindsOf(sources ...string) []map[string]any {
	var out []map[string]any
	for _, s := range sources {
		out = append(out, map[string]any{"type": "bind", "source": s, "target": "/data"})
	}
	return out
}

func check(t *testing.T, r *imageRunner, dir string, versioned func(context.Context, string) (bool, error)) ([]engine.MissingDir, error) {
	t.Helper()
	return checkMapped(t, r, dir, versioned, idmap.Default(), idmap.Default(), nil)
}

func checkMapped(t *testing.T, r *imageRunner, dir string, versioned func(context.Context, string) (bool, error),
	uid, gid idmap.Map, mapErr error) ([]engine.MissingDir, error) {
	t.Helper()
	st := newStack(t, true)
	st.Dir = dir
	if err := os.WriteFile(filepath.Join(dir, ".env"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "secrets.env"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	x := &Executor{Runner: r, Env: []string{"PATH=/bin"},
		IDMaps: func(context.Context, Logger) (idmap.Map, idmap.Map, error) { return uid, gid, mapErr }}
	return x.MissingDataDirs(context.Background(), st, versioned, &fakeLog{})
}

func TestMissingDataDirsImageUser(t *testing.T) {
	const users = "root:x:0:0:root:/root:/bin/sh\nnode:x:1000:1000::/home/node:/bin/sh\napp:x:2000:3000::/:/bin/sh\n"
	const group = "root:x:0:\nstaff:x:50:\n"
	cases := []struct {
		name    string
		user    string // image user
		svcUser string
		passwd  string
		group   string
		want    []engine.MissingDir
		files   bool // the image files were read
	}{
		{name: "empty user is root", user: ""},
		{name: "root", user: "root"},
		{name: "uid 0", user: "0:0"},
		{name: "numeric uid and gid", user: "1234:5678", want: []engine.MissingDir{{Path: "data", Owner: "1234:5678"}}},
		{name: "numeric uid, gid from passwd", user: "2000", passwd: users, want: []engine.MissingDir{{Path: "data", Owner: "2000:3000"}}, files: true},
		{name: "numeric uid, gid defaults to uid", user: "4242", want: []engine.MissingDir{{Path: "data", Owner: "4242:4242"}}, files: true},
		{name: "name from passwd", user: "node", passwd: users, want: []engine.MissingDir{{Path: "data", Owner: "1000:1000"}}, files: true},
		{name: "name with group name", user: "node:staff", passwd: users, group: group, want: []engine.MissingDir{{Path: "data", Owner: "1000:50"}}, files: true},
		{name: "name not resolvable", user: "node", want: []engine.MissingDir{{Path: "data"}}, files: true},
		{name: "name unknown in passwd", user: "ghost", passwd: users, want: []engine.MissingDir{{Path: "data"}}, files: true},
		{name: "service user overrides the image", user: "node", svcUser: "0", passwd: users},
		{name: "service user wins", user: "", svcUser: "7:8", want: []engine.MissingDir{{Path: "data", Owner: "7:8"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			svc := map[string]any{"image": "demo/app:1.0", "volumes": bindsOf(filepath.Join(dir, "data"))}
			if tc.svcUser != "" {
				svc["user"] = tc.svcUser
			}
			r := &imageRunner{config: configJSON(t, map[string]map[string]any{"app": svc}),
				user: tc.user, passwd: tc.passwd, group: tc.group}
			got, err := check(t, r, dir, nil)
			if err != nil {
				t.Fatal(err)
			}
			for i := range got { // the default mapping keeps the owner, which equals the container user
				if got[i].Container != got[i].Owner {
					t.Fatalf("container user %q, owner %q", got[i].Container, got[i].Owner)
				}
				got[i].Container = ""
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
			if r.ran("create") != tc.files {
				t.Fatalf("image files read = %v, want %v (%v)", r.ran("create"), tc.files, r.calls)
			}
			if tc.files && !r.ran("rm -f") {
				t.Fatalf("the temporary container was not removed: %v", r.calls)
			}
			if r.ran("start") || r.ran("run") || r.ran("compose --project-name demo-stack --project-directory "+dir+" up") {
				t.Fatalf("something was started: %v", r.calls)
			}
		})
	}
}

func TestMissingDataDirsUnknownImageUserIsAsBefore(t *testing.T) {
	dir := t.TempDir()
	svc := map[string]any{"image": "demo/app:1.0", "volumes": bindsOf(filepath.Join(dir, "data"))}
	r := &imageRunner{config: configJSON(t, map[string]map[string]any{"app": svc}), noImage: true}
	got, err := check(t, r, dir, nil)
	if err != nil || len(got) != 0 {
		t.Fatalf("got %+v, %v", got, err)
	}
	if !r.ran("compose") || !strings.Contains(strings.Join(r.calls, "\n"), "pull --quiet app") {
		t.Fatalf("the missing image was not pulled: %v", r.calls)
	}
}

func TestMissingDataDirsCandidates(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	mustMkdir := func(p string) {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	mustMkdir(filepath.Join(dir, "exists"))
	mustMkdir(filepath.Join(dir, "sub"))
	if err := os.Symlink(outside, filepath.Join(dir, "escape")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "gone"), filepath.Join(dir, "dangling")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "file"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	sources := []string{
		filepath.Join(dir, "exists"),                    // exists
		filepath.Join(dir, "sub", "new"),                // candidate
		filepath.Join(dir, "versioned"),                 // tracked
		dir,                                             // the stack folder itself
		filepath.Join(dir, "..", "elsewhere"),           // traversal
		filepath.Join(outside, "new"),                   // absolute, outside
		filepath.Join(dir, "escape", "new"),             // symlinked parent leaves the folder
		filepath.Join(dir, "dangling", "new"),           // dangling symlink parent
		filepath.Join(dir, "file", "new"),               // parent is a file
		filepath.Join(dir, "dangling"),                  // an existing (dangling) link is not missing
		"relative/path",                                 // not absolute
		filepath.Join(dir, "sub", "..", "..", "x"),      // cleans to outside
		filepath.Join(dir, "sub", "..", "cleaned"),      // cleans to a candidate
		filepath.Join(dir, "new") + "/",                 // trailing slash
		filepath.Join(dir, "a") + "\x00" + "/b",         // NUL
		filepath.Join(dir, "nofollow", "..", "..", "y"), // outside after cleaning
	}
	svc := map[string]any{"image": "demo/app:1.0", "user": "1000:1000", "volumes": bindsOf(sources...)}
	r := &imageRunner{config: configJSON(t, map[string]map[string]any{"app": svc})}
	versioned := func(_ context.Context, rel string) (bool, error) { return rel == "versioned", nil }
	got, err := check(t, r, dir, versioned)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, d := range got {
		paths = append(paths, d.Path)
		if d.Owner != "1000:1000" {
			t.Fatalf("owner %q", d.Owner)
		}
	}
	want := []string{"cleaned", "new", "sub/new"}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("got %v, want %v", paths, want)
	}
}

func TestMissingDataDirsSkipsCreateHostPathFalseAndNamedVolumes(t *testing.T) {
	dir := t.TempDir()
	svc := map[string]any{"image": "demo/app:1.0", "user": "5:5", "volumes": []map[string]any{
		{"type": "bind", "source": filepath.Join(dir, "a"), "bind": map[string]any{"create_host_path": false}},
		{"type": "volume", "source": "named"},
		{"type": "bind", "source": filepath.Join(dir, "b"), "bind": map[string]any{"create_host_path": true}},
	}}
	got, err := check(t, &imageRunner{config: configJSON(t, map[string]map[string]any{"app": svc})}, dir, nil)
	if err != nil || len(got) != 1 || got[0].Path != "b" {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestMissingDataDirsVersionedErrorFails(t *testing.T) {
	dir := t.TempDir()
	svc := map[string]any{"image": "demo/app:1.0", "user": "5:5", "volumes": bindsOf(filepath.Join(dir, "a"))}
	_, err := check(t, &imageRunner{config: configJSON(t, map[string]map[string]any{"app": svc})}, dir,
		func(context.Context, string) (bool, error) { return false, errors.New("git broke") })
	if err == nil {
		t.Fatal("want an error")
	}
}

func TestMissingDataDirsNeverLogsTheConfig(t *testing.T) {
	dir := t.TempDir()
	svc := map[string]any{"image": "demo/app:1.0", "user": "5:5", "environment": map[string]string{"DEMO_VALUE": "rendered-demo-value"},
		"volumes": bindsOf(filepath.Join(dir, "a"))}
	st := newStack(t, true)
	st.Dir = dir
	_ = os.WriteFile(filepath.Join(dir, ".env"), nil, 0o600)
	_ = os.WriteFile(filepath.Join(dir, "secrets.env"), nil, 0o600)
	log := &fakeLog{}
	x := &Executor{Runner: &imageRunner{config: configJSON(t, map[string]map[string]any{"app": svc})}, Env: []string{"PATH=/bin"}}
	if _, err := x.MissingDataDirs(context.Background(), st, nil, log); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(log.text(), "rendered-demo-value") {
		t.Fatalf("the configuration was logged: %s", log.text())
	}
}

func TestMissingDataDirsOwnerIsMappedToTheHost(t *testing.T) {
	// Instance user 1000 is container root, 100000..165535 are container users 1..65536.
	uid := idmap.Map{{Inside: 0, Outside: 1000, Count: 1}, {Inside: 1, Outside: 100000, Count: 65536}}
	gid := idmap.Map{{Inside: 0, Outside: 1000, Count: 1}, {Inside: 1, Outside: 200000, Count: 100}, {Inside: 101, Outside: 300000, Count: 100}}
	cases := []struct {
		name, user, owner, container, warn string
	}{
		{"one range", "1234:50", "101233:200049", "1234:50", ""},
		{"second gid range", "7:150", "100006:300049", "7:150", ""},
		{"last uid of the range", "65536:1", "165535:200000", "65536:1", ""},
		{"uid past the range", "65537:1", "", "65537:1", "container user 65537 is outside the user ID range of the engine"},
		{"gid past the ranges", "5:201", "", "5:201", "container group 201 is outside the group ID range of the engine"},
		{"gid 0 is the instance group", "5:0", "100004:1000", "5:0", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			svc := map[string]any{"image": "demo/app:1.0", "user": tc.user, "volumes": bindsOf(filepath.Join(dir, "data"))}
			r := &imageRunner{config: configJSON(t, map[string]map[string]any{"app": svc})}
			got, err := checkMapped(t, r, dir, nil, uid, gid, nil)
			if err != nil {
				t.Fatal(err)
			}
			want := []engine.MissingDir{{Path: "data", Owner: tc.owner, Container: tc.container, Warning: tc.warn}}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("got %+v, want %+v", got, want)
			}
		})
	}
}

func TestHostViewOfTheEngineMapping(t *testing.T) {
	// The host is itself a container: its root is user 1000 above it, its users 1.. are 524288.. there.
	host := idmap.Map{{Inside: 0, Outside: 1000, Count: 1}, {Inside: 1, Outside: 524288, Count: 65536}}
	rootless := idmap.Map{{Inside: 0, Outside: 1001, Count: 1}, {Inside: 1, Outside: 100000, Count: 65536}}
	cases := []struct {
		name                      string
		engineUID, engineGID, own idmap.Map
		wantUID, wantGID          idmap.Map
	}{
		{"system engine on a plain host", idmap.Default(), idmap.Default(), idmap.Default(), idmap.Default(), idmap.Default()},
		{"system engine on a host in a container", host, host, host, idmap.Default(), idmap.Default()},
		{"rootless engine on a host in a container", rootless, rootless, host, rootless, rootless},
		{"rootless engine on a plain host", rootless, rootless, idmap.Default(), rootless, rootless},
		{"only the uid map is shared", host, rootless, host, host, rootless},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			uid, gid := hostView(tc.engineUID, tc.engineGID, tc.own, tc.own)
			if !uid.Equal(tc.wantUID) || !gid.Equal(tc.wantGID) {
				t.Fatalf("got %v / %v, want %v / %v", uid, gid, tc.wantUID, tc.wantGID)
			}
		})
	}
}

func TestReadMaps(t *testing.T) {
	dir := t.TempDir()
	files := [2]string{filepath.Join(dir, "uid_map"), filepath.Join(dir, "gid_map")}
	if err := os.WriteFile(files[0], []byte("         0       1000          1\n         1     524288      65536\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files[1], []byte("0 0 4294967295\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	uid, gid, err := readMaps(files)
	if err != nil {
		t.Fatal(err)
	}
	if want := (idmap.Map{{Inside: 0, Outside: 1000, Count: 1}, {Inside: 1, Outside: 524288, Count: 65536}}); !uid.Equal(want) {
		t.Fatalf("uid map %v, want %v", uid, want)
	}
	if !gid.Equal(idmap.Default()) {
		t.Fatalf("gid map %v, want the default", gid)
	}
	if err := os.WriteFile(files[1], nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readMaps(files); err == nil {
		t.Fatal("an empty gid map was read")
	}
	if _, _, err := readMaps([2]string{filepath.Join(dir, "missing"), files[1]}); err == nil {
		t.Fatal("a missing map file was read")
	}
}

func TestMissingDataDirsUnreadableNamespaceProposesNothing(t *testing.T) {
	dir := t.TempDir()
	svc := map[string]any{"image": "demo/app:1.0", "user": "1234:1234", "volumes": bindsOf(filepath.Join(dir, "data"))}
	r := &imageRunner{config: configJSON(t, map[string]map[string]any{"app": svc})}
	got, err := checkMapped(t, r, dir, nil, nil, nil, errors.New("boom"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Owner != "" || !strings.Contains(got[0].Warning, "cannot be read: boom") {
		t.Fatalf("got %+v", got)
	}
}

func TestMissingDataDirsReportsAnAccessError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory modes")
	}
	dir := t.TempDir()
	locked := filepath.Join(dir, "locked")
	if err := os.Mkdir(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })
	svc := map[string]any{"image": "demo/app:1.0", "user": "1234:1234", "volumes": bindsOf(filepath.Join(locked, "data"))}
	r := &imageRunner{config: configJSON(t, map[string]map[string]any{"app": svc})}
	_, err := check(t, r, dir, nil)
	if err == nil || !strings.Contains(err.Error(), "locked/data") || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("err = %v", err)
	}
}
