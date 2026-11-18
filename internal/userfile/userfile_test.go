package userfile

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// fakeEnv runs as root with SUDO_UID set to the test's own user, whose account has a home under a temporary base.
func fakeEnv(t *testing.T, home string, vars map[string]string) Env {
	t.Helper()
	return Env{
		Getenv: func(k string) string { return vars[k] },
		Euid:   func() int { return 0 },
		Egid:   func() int { return 0 },
		Lookup: func(uid int) (Account, error) {
			return Account{UID: uid, GID: os.Getgid(), Home: home}, nil
		},
	}
}

func person(t *testing.T) (Person, string) {
	t.Helper()
	home := t.TempDir()
	p, err := Operator(fakeEnv(t, home, map[string]string{
		"SUDO_UID": strconv.Itoa(os.Getuid()), "SUDO_GID": strconv.Itoa(os.Getgid())}))
	if err != nil {
		t.Fatal(err)
	}
	return p, home
}

func TestOperatorUsesSudoUIDOnlyAsRoot(t *testing.T) {
	var asked int
	env := Env{
		Getenv: func(k string) string { return map[string]string{"SUDO_UID": "1234", "SUDO_GID": "4321"}[k] },
		Euid:   func() int { return 1000 }, Egid: func() int { return 1001 },
		Lookup: func(uid int) (Account, error) {
			asked = uid
			return Account{UID: uid, GID: 77, Home: "/home/demo"}, nil
		},
	}
	p, err := Operator(env)
	if err != nil || asked != 1000 || p.UID != 1000 || p.GID != 1001 {
		t.Fatalf("not root: %+v, %v, asked %d", p, err, asked)
	}
	env.Euid = func() int { return 0 }
	p, err = Operator(env)
	if err != nil || asked != 1234 || p.UID != 1234 || p.GID != 4321 || p.Home != "/home/demo" {
		t.Fatalf("root: %+v, %v, asked %d", p, err, asked)
	}
	env.Getenv = func(k string) string { return map[string]string{"SUDO_UID": "1234"}[k] }
	if p, err = Operator(env); err != nil || p.GID != 77 {
		t.Fatalf("without SUDO_GID: %+v, %v", p, err)
	}
	env.Getenv = func(string) string { return "" }
	if p, err = Operator(env); err != nil || p.UID != 0 {
		t.Fatalf("without SUDO_UID: %+v, %v", p, err)
	}
}

func TestSaveGivesTheFileToTheOperatorWithMode0600(t *testing.T) {
	p, home := person(t)
	path, err := p.Save("a.log", []byte("one\n"))
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(home, "qhost", "a.log") {
		t.Fatalf("path %s", path)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 || int(st.Sys().(*syscall.Stat_t).Uid) != os.Getuid() {
		t.Fatalf("mode %v, owner %d", st.Mode(), st.Sys().(*syscall.Stat_t).Uid)
	}
	if d, _ := os.Stat(filepath.Join(home, "qhost")); d.Mode().Perm() != 0o700 {
		t.Fatalf("folder mode %v", d.Mode())
	}
	if b, _ := os.ReadFile(path); string(b) != "one\n" {
		t.Fatalf("content %q", b)
	}
}

func TestSaveNeverReplacesAFile(t *testing.T) {
	p, _ := person(t)
	first, err := p.Save("a.log", []byte("first"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := p.Save("a.log", []byte("second"))
	if err != nil {
		t.Fatal(err)
	}
	if second == first || filepath.Base(second) != "a-2.log" {
		t.Fatalf("second path %s", second)
	}
	if b, _ := os.ReadFile(first); string(b) != "first" {
		t.Fatalf("first file now %q", b)
	}
}

func TestSaveDoesNotFollowALinkAtTheName(t *testing.T) {
	p, home := person(t)
	target := filepath.Join(t.TempDir(), "victim")
	if err := os.WriteFile(target, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(home, "qhost"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(home, "qhost", "a.log")); err != nil {
		t.Fatal(err)
	}
	path, err := p.Save("a.log", []byte("new"))
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "a-2.log" {
		t.Fatalf("path %s", path)
	}
	if b, _ := os.ReadFile(target); string(b) != "keep" {
		t.Fatalf("target now %q", b)
	}
	if st, _ := os.Stat(target); st.Mode().Perm() != 0o644 {
		t.Fatalf("target mode %v", st.Mode())
	}
}

func TestSaveDoesNotFollowALinkInsteadOfTheFolder(t *testing.T) {
	p, home := person(t)
	elsewhere := t.TempDir()
	if err := os.Symlink(elsewhere, filepath.Join(home, "qhost")); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Save("a.log", []byte("x")); err == nil {
		t.Fatal("saved through a link")
	}
	if entries, _ := os.ReadDir(elsewhere); len(entries) != 0 {
		t.Fatalf("%d entries behind the link", len(entries))
	}
}

func TestSaveRefusesAFileInsteadOfTheFolder(t *testing.T) {
	p, home := person(t)
	if err := os.WriteFile(filepath.Join(home, "qhost"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Save("a.log", []byte("x")); err == nil {
		t.Fatal("saved although qhost is a file")
	}
}

func TestSaveRefusesAHomeOfAnotherUser(t *testing.T) {
	p, _ := person(t)
	p.UID++
	if _, err := p.Save("a.log", nil); err == nil {
		t.Fatal("saved into the home of another user")
	}
}

func TestCreateRefusesPathNames(t *testing.T) {
	p, _ := person(t)
	for _, name := range []string{"", ".", "..", "../x", "a/b"} {
		if f, _, err := p.Create(name); err == nil {
			f.Close()
			t.Fatalf("created %q", name)
		}
	}
}

func TestCreateReturnsAWritableFile(t *testing.T) {
	p, home := person(t)
	f, path, err := p.Create("b.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("hi"); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if path != filepath.Join(home, "qhost", "b.txt") {
		t.Fatalf("path %s", path)
	}
	if b, _ := os.ReadFile(path); string(b) != "hi" {
		t.Fatalf("content %q", b)
	}
}

func TestNameUsesSafeParts(t *testing.T) {
	at := time.Date(2026, time.March, 4, 5, 6, 7, 0, time.UTC)
	got := Name("run", "../../.etc/pa ss", at, ".log")
	if got != "qhost-run-etc-pa-ss-20260304-050607.log" {
		t.Fatalf("name %q", got)
	}
	if Slug("") != "unnamed" || Slug("...") != "unnamed" || len(Slug(strings.Repeat("a", 500))) != maxSubjectRunes {
		t.Fatal("slug limits")
	}
}
