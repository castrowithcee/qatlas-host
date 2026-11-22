package datadir

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
)

var restoreChown = chownFd

func self() Owner { return Owner{UID: uint32(os.Getuid()), GID: uint32(os.Getgid())} }

func stackDir(t *testing.T) string {
	t.Helper()
	d, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestParseOwnerAndMode(t *testing.T) {
	if o, err := ParseOwner("1000:1001"); err != nil || o.String() != "1000:1001" {
		t.Fatalf("owner: %v %v", o, err)
	}
	for _, bad := range []string{"", "1000", "a:b", "1:2:3", "-1:0", "0:4294967295", "99999999999:1", " 1:1", "1:1\n"} {
		if _, err := ParseOwner(bad); err == nil {
			t.Errorf("owner %q accepted", bad)
		}
	}
	for s, want := range map[string]fs.FileMode{"0750": 0o750, "750": 0o750, "0777": 0o777, "0700": 0o700} {
		if m, err := ParseMode(s); err != nil || m != want {
			t.Errorf("mode %q = %v, %v", s, m, err)
		}
	}
	for _, bad := range []string{"", "4750", "2750", "1777", "01777", "0o750", "rwx", "9", "-1", "07777"} {
		if _, err := ParseMode(bad); err == nil {
			t.Errorf("mode %q accepted", bad)
		}
	}
}

func TestCreateMakesDirsWithModeAndOwner(t *testing.T) {
	dir := stackDir(t)
	res, err := Create(Request{StackDir: dir, Paths: []string{"data/db", "cache"}, Owner: self(), Mode: 0o750})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(res.Created, []string{"cache", "data/db"}) || !slices.Equal(res.Parents, []string{"data"}) {
		t.Fatalf("result = %+v", res)
	}
	for _, p := range []string{"cache", "data", "data/db"} {
		fi, err := os.Lstat(filepath.Join(dir, p))
		if err != nil || !fi.IsDir() || fi.Mode().Perm() != 0o750 {
			t.Fatalf("%s: %v %v", p, fi, err)
		}
	}
}

func TestCreateDefaultModeIgnoresUmask(t *testing.T) {
	old := syscall.Umask(0o077)
	defer syscall.Umask(old)
	dir := stackDir(t)
	if _, err := Create(Request{StackDir: dir, Paths: []string{"d"}, Owner: self()}); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(filepath.Join(dir, "d")); fi.Mode().Perm() != 0o750 {
		t.Fatalf("mode %v", fi.Mode())
	}
}

func TestCreateNestedRequestedBothReportedAsCreated(t *testing.T) {
	dir := stackDir(t)
	res, err := Create(Request{StackDir: dir, Paths: []string{"data/sub", "data"}, Owner: self()})
	if err != nil || !slices.Equal(res.Created, []string{"data", "data/sub"}) || len(res.Parents) != 0 {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestCreateLeavesExistingUntouched(t *testing.T) {
	dir := stackDir(t)
	if err := os.Mkdir(filepath.Join(dir, "data"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(dir, "data"), 0o700); err != nil {
		t.Fatal(err)
	}
	called := 0
	chownFd = func(int, int, int) error { called++; return nil }
	t.Cleanup(func() { chownFd = restoreChown })
	res, err := Create(Request{StackDir: dir, Paths: []string{"data", "data/new"}, Owner: self(), Mode: 0o755})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(res.Existing, []string{"data"}) || !slices.Equal(res.Created, []string{"data/new"}) || called != 1 {
		t.Fatalf("%+v chown calls %d", res, called)
	}
	if fi, _ := os.Stat(filepath.Join(dir, "data")); fi.Mode().Perm() != 0o700 {
		t.Fatalf("existing dir changed: %v", fi.Mode())
	}
}

func TestCreateRefusesBadPaths(t *testing.T) {
	dir := stackDir(t)
	outside := filepath.Join(filepath.Dir(dir), "outside-"+filepath.Base(dir))
	for _, p := range []string{"", ".", "..", "../x", "a/../../x", "/abs", "/etc", "a//b", "a/./b", "a/", "./a", ".git", ".git/hooks", "a\\b", "a\x00b", "a\nb", "../" + filepath.Base(outside)} {
		res, err := Create(Request{StackDir: dir, Paths: []string{"ok", p}, Owner: self()})
		if err == nil || len(res.Created) != 0 {
			t.Errorf("path %q: err %v, result %+v", p, err, res)
		}
	}
	if _, err := os.Lstat(filepath.Join(dir, "ok")); err == nil {
		t.Error("a valid path was created although another failed validation")
	}
	if _, err := os.Lstat(outside); err == nil {
		t.Error("outside path created")
	}
}

func TestCreateRefusesBadRequest(t *testing.T) {
	dir := stackDir(t)
	for _, r := range []Request{
		{StackDir: "relative/dir", Paths: []string{"a"}},
		{StackDir: "/", Paths: []string{"a"}},
		{StackDir: dir},
		{StackDir: dir, Paths: []string{"a"}, Mode: 0o4755},
		{StackDir: dir, Paths: []string{"a"}, Mode: 0o1777},
		{StackDir: filepath.Join(dir, "missing"), Paths: []string{"a"}},
	} {
		r.Owner = self()
		if _, err := Create(r); err == nil {
			t.Errorf("request %+v accepted", r)
		}
	}
}

func TestCreateRefusesVersioned(t *testing.T) {
	dir := stackDir(t)
	v := func(rel string) (bool, error) { return rel == "tracked" || strings.HasPrefix(rel, "tracked/"), nil }
	if res, err := Create(Request{StackDir: dir, Paths: []string{"free", "tracked"}, Owner: self(), Versioned: v}); err == nil || len(res.Created) != 0 {
		t.Fatalf("%+v %v", res, err)
	}
	if _, err := os.Lstat(filepath.Join(dir, "free")); err == nil {
		t.Fatal("created although a sibling is versioned")
	}
}

func TestCreateRefusesSymlinkedParentAndTarget(t *testing.T) {
	dir := stackDir(t)
	outside := stackDir(t)
	if err := os.Symlink(outside, filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "target")); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"link/sub", "target"} {
		if res, err := Create(Request{StackDir: dir, Paths: []string{p}, Owner: self()}); err == nil {
			t.Errorf("%s: accepted: %+v", p, res)
		}
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Fatalf("outside touched: %v", entries)
	}
}

func TestCreateRefusesSymlinkedStackDir(t *testing.T) {
	real := stackDir(t)
	link := filepath.Join(stackDir(t), "stack")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if _, err := Create(Request{StackDir: link, Paths: []string{"a"}, Owner: self()}); err == nil {
		t.Fatal("symlinked stack folder accepted")
	}
	if _, err := os.Lstat(filepath.Join(real, "a")); err == nil {
		t.Fatal("created through the link")
	}
}

func TestCreateRefusesFileAtPath(t *testing.T) {
	dir := stackDir(t)
	if err := os.WriteFile(filepath.Join(dir, "f"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Create(Request{StackDir: dir, Paths: []string{"f"}, Owner: self()}); err == nil {
		t.Fatal("file at path accepted")
	}
	if _, err := Create(Request{StackDir: dir, Paths: []string{"f/sub"}, Owner: self()}); err == nil {
		t.Fatal("file as parent accepted")
	}
}

func TestCreateOtherOwnerAsRoot(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root")
	}
	dir := stackDir(t)
	if _, err := Create(Request{StackDir: dir, Paths: []string{"a/b"}, Owner: Owner{UID: 4242, GID: 4343}, Mode: 0o700}); err != nil {
		t.Fatal(err)
	}
	var parent syscall.Stat_t
	if err := syscall.Stat(dir, &parent); err != nil {
		t.Fatal(err)
	}
	want := map[string][3]uint32{"a": {parent.Uid, parent.Gid, 0o750}, "a/b": {4242, 4343, 0o700}}
	for p, w := range want {
		fi, _ := os.Lstat(filepath.Join(dir, p))
		st := fi.Sys().(*syscall.Stat_t)
		if st.Uid != w[0] || st.Gid != w[1] || uint32(fi.Mode().Perm()) != w[2] {
			t.Fatalf("%s: %d:%d %v", p, st.Uid, st.Gid, fi.Mode())
		}
	}
}

// recordChown replaces chownFd with a fake that records the owner each directory is given, so no test
// changes the owner of a real file.
func recordChown(t *testing.T) *[][2]int {
	t.Helper()
	var calls [][2]int
	chownFd = func(_ int, uid, gid int) error { calls = append(calls, [2]int{uid, gid}); return nil }
	t.Cleanup(func() { chownFd = restoreChown })
	return &calls
}

func TestCreateParentsGetTheOwnerOfTheNearestExistingParent(t *testing.T) {
	dir := stackDir(t)
	if err := os.Mkdir(filepath.Join(dir, "keep"), 0o750); err != nil {
		t.Fatal(err)
	}
	calls := recordChown(t)
	req := Owner{UID: 101233, GID: 101233}
	res, err := Create(Request{StackDir: dir, Paths: []string{"keep/a/b/db"}, Owner: req, Mode: 0o700})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(res.Parents, []string{"keep/a", "keep/a/b"}) || !slices.Equal(res.Created, []string{"keep/a/b/db"}) {
		t.Fatalf("%+v", res)
	}
	me := self()
	want := [][2]int{{int(me.UID), int(me.GID)}, {int(me.UID), int(me.GID)}, {101233, 101233}}
	if !slices.Equal(*calls, want) {
		t.Fatalf("chown calls %v, want %v", *calls, want)
	}
	for _, p := range []string{"keep/a", "keep/a/b"} {
		if fi, _ := os.Stat(filepath.Join(dir, p)); fi.Mode().Perm() != 0o750 {
			t.Fatalf("%s mode %v, want 0750: the service user must be able to search it", p, fi.Mode().Perm())
		}
	}
	if fi, _ := os.Stat(filepath.Join(dir, "keep/a/b/db")); fi.Mode().Perm() != 0o700 {
		t.Fatalf("requested directory mode %v", fi.Mode().Perm())
	}
}

func TestCreateParentKeepsWiderModeOfTheRequest(t *testing.T) {
	dir := stackDir(t)
	recordChown(t)
	if _, err := Create(Request{StackDir: dir, Paths: []string{"a/b"}, Owner: self(), Mode: 0o777}); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(filepath.Join(dir, "a")); fi.Mode().Perm() != 0o777 {
		t.Fatalf("parent mode %v", fi.Mode().Perm())
	}
}
