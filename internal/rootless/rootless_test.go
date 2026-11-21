package rootless

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeEngine struct {
	list  []Container
	err   error
	block bool
}

func (f *fakeEngine) List(ctx context.Context) ([]Container, error) {
	if f.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return f.list, f.err
}
func (f *fakeEngine) Close() {}

type env struct {
	t      *testing.T
	run    string
	home   string
	dialed int
	eng    *fakeEngine
}

func newEnv(t *testing.T) *env {
	t.Helper()
	d, err := os.MkdirTemp("", "rl")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(d) })
	return &env{t: t, run: filepath.Join(d, "run"), home: filepath.Join(d, "home"), eng: &fakeEngine{}}
}

func (e *env) uid() uint32 { return uint32(os.Getuid()) }

func (e *env) runDir(uid uint32) string {
	dir := filepath.Join(e.run, strconv.Itoa(int(uid)))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		e.t.Fatal(err)
	}
	return dir
}

func (e *env) listen(path string) {
	l, err := net.Listen("unix", path)
	if err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(func() { l.Close() })
}

func (e *env) unit() {
	dir := filepath.Join(e.home, ".config", "systemd", "user")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docker.service"), []byte("[Service]\n"), 0o600); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) scan(users ...User) Scan {
	if len(users) == 0 {
		users = []User{{Name: "alice", UID: e.uid(), Home: e.home}}
	}
	d := Detector{RunDir: e.run, Users: users, Timeout: 200 * time.Millisecond,
		Dial: func(string, time.Duration) (Client, error) { e.dialed++; return e.eng, nil }}
	return d.Scan(context.Background())
}

func ctr(name, project, dir string) Container {
	return Container{Name: name, Project: project, WorkingDir: dir}
}

func TestSocketMissing(t *testing.T) {
	e := newEnv(t)
	s := e.scan()
	if len(s.NotChecked) != 0 || e.dialed != 0 {
		t.Fatalf("no unit and no socket must be silent: %+v", s)
	}
	e.unit()
	s = e.scan()
	if len(s.NotChecked) != 1 || s.NotChecked[0].Engine != "rootless:alice" || s.NotChecked[0].Reason == "" || e.dialed != 0 {
		t.Fatalf("unit without socket must be not checked: %+v", s)
	}
}

func TestSocketOfAnotherUserIsNotContacted(t *testing.T) {
	e := newEnv(t)
	e.unit()
	other := e.uid() + 1
	e.listen(filepath.Join(e.runDir(other), "docker.sock")) // owned by the test user, not by `other`
	s := e.scan(User{Name: "bob", UID: other, Home: e.home})
	if e.dialed != 0 || len(s.NotChecked) != 1 || !strings.Contains(s.NotChecked[0].Reason, "another user") {
		t.Fatalf("dialed=%d %+v", e.dialed, s)
	}
}

func TestLinkIsNotContacted(t *testing.T) {
	e := newEnv(t)
	dir := e.runDir(e.uid())
	target := filepath.Join(filepath.Dir(dir), "real.sock")
	e.listen(target)
	if err := os.Symlink(target, filepath.Join(dir, "docker.sock")); err != nil {
		t.Fatal(err)
	}
	if s := e.scan(); e.dialed != 0 || len(s.NotChecked) != 0 {
		t.Fatalf("link without unit: dialed=%d %+v", e.dialed, s)
	}
	e.unit()
	s := e.scan()
	if e.dialed != 0 || len(s.NotChecked) != 1 || !strings.Contains(s.NotChecked[0].Reason, "link") {
		t.Fatalf("dialed=%d %+v", e.dialed, s)
	}
}

func TestNotASocketIsNotContacted(t *testing.T) {
	e := newEnv(t)
	e.unit()
	if err := os.WriteFile(filepath.Join(e.runDir(e.uid()), "docker.sock"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if s := e.scan(); e.dialed != 0 || len(s.NotChecked) != 1 {
		t.Fatalf("dialed=%d %+v", e.dialed, s)
	}
}

func TestEngineTimeoutAndError(t *testing.T) {
	e := newEnv(t)
	e.listen(filepath.Join(e.runDir(e.uid()), "docker.sock"))
	e.eng.block = true
	start := time.Now()
	s := e.scan()
	if len(s.NotChecked) != 1 || !strings.Contains(s.NotChecked[0].Reason, "in time") || time.Since(start) > 2*time.Second {
		t.Fatalf("%+v", s)
	}
	e.eng.block = false
	e.eng.err = errors.New("secret body")
	s = e.scan()
	if len(s.NotChecked) != 1 || strings.Contains(s.NotChecked[0].Reason, "secret") {
		t.Fatalf("%+v", s)
	}
}

func TestOneFailingEngineKeepsTheOthers(t *testing.T) {
	e := newEnv(t)
	e.listen(filepath.Join(e.runDir(e.uid()), "docker.sock"))
	e.eng.list = []Container{ctr("web-1", "shop", "/srv/shop")}
	s := e.scan(User{Name: "bad", UID: e.uid() + 1, Home: e.home}, User{Name: "alice", UID: e.uid(), Home: e.home})
	if got := s.Conflicts("/srv/shop"); len(got) != 1 || got[0].Engine != "rootless:alice" {
		t.Fatalf("%+v", got)
	}
}

func TestConflicts(t *testing.T) {
	e := newEnv(t)
	e.listen(filepath.Join(e.runDir(e.uid()), "docker.sock"))
	e.eng.list = []Container{
		ctr("shop-web-1", "shop", "/srv/shop"),
		ctr("old-web-1", "oldname", "/srv/shop/"),
		ctr("blog-1", "blog", "/srv/blog"),
		{Name: "loose", WorkingDir: "/srv/shop"},
	}
	s := e.scan()
	if len(s.NotChecked) != 0 || e.dialed != 1 {
		t.Fatalf("%+v dialed=%d", s, e.dialed)
	}
	got := s.Conflicts("/srv/shop")
	want := []Conflict{
		{Container: "old-web-1", Project: "oldname", Engine: "rootless:alice"},
		{Container: "shop-web-1", Project: "shop", Engine: "rootless:alice"},
	}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %+v", got)
	}
	if got := s.Conflicts("/srv/other"); len(got) != 0 {
		t.Fatalf("other folder: %+v", got)
	}
	if got := s.Conflicts(""); got != nil {
		t.Fatalf("empty dir: %+v", got)
	}
}

func TestControlCharactersAreMasked(t *testing.T) {
	e := newEnv(t)
	e.listen(filepath.Join(e.runDir(e.uid()), "docker.sock"))
	e.eng.list = []Container{ctr("a\x1b[31mb\n", "p\tq", "/srv/shop")}
	got := e.scan().Conflicts("/srv/shop")
	if len(got) != 1 || got[0].Container != "a\uFFFD[31mb\uFFFD" || got[0].Project != "p\uFFFDq" {
		t.Fatalf("%q", got)
	}
	s := e.scan(User{Name: "x\ny", UID: e.uid() + 5, Home: e.home})
	e.unit()
	s = e.scan(User{Name: "x\ny", UID: e.uid() + 5, Home: e.home})
	if len(s.NotChecked) != 1 || s.NotChecked[0].Engine != "rootless:x\uFFFDy" {
		t.Fatalf("%q", s.NotChecked)
	}
}

func TestReadUsers(t *testing.T) {
	u, err := parseUsers(strings.NewReader("root:x:0:0:r:/root:/bin/sh\nbad\nalice:x:1001:1001::/home/alice:/bin/bash\nx:x:nan:1::/h:/s\n"))
	if err != nil || len(u) != 2 || u[1] != (User{Name: "alice", UID: 1001, Home: "/home/alice"}) {
		t.Fatalf("%+v %v", u, err)
	}
}

func serve(t *testing.T, e *env, h http.HandlerFunc) {
	t.Helper()
	l, err := net.Listen("unix", filepath.Join(e.runDir(e.uid()), "docker.sock"))
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: h}
	go srv.Serve(l)
	t.Cleanup(func() { srv.Close() })
}

func realScan(e *env) Scan {
	return Detector{RunDir: e.run, Users: []User{{Name: "alice", UID: e.uid()}}, Timeout: 2 * time.Second}.Scan(context.Background())
}

// The default client sends exactly one request, through the descriptor path the detector hands to the dialer.
func TestHTTPClientSendsOneListRequest(t *testing.T) {
	e := newEnv(t)
	var reqs []string
	var mu sync.Mutex
	serve(t, e, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		reqs = append(reqs, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
		mu.Unlock()
		_, _ = w.Write([]byte(`[{"Id":"aaaaaaaaaaaa","Names":["/other/alias","/shop-web-1"],"Labels":{"com.docker.compose.project":"shop","com.docker.compose.project.working_dir":"/srv/shop"}},{"Names":["/loose"],"Labels":null}]`))
	})
	sc := realScan(e)
	if len(sc.NotChecked) != 0 {
		t.Fatalf("%+v", sc.NotChecked)
	}
	got := sc.Conflicts("/srv/shop")
	if len(got) != 1 || got[0].Container != "shop-web-1" || got[0].Project != "shop" {
		t.Fatalf("%+v", got)
	}
	if len(reqs) != 1 || reqs[0] != "GET /containers/json?all=1" {
		t.Fatalf("requests: %q", reqs)
	}
}

// The list also carries the state and the image, still from the same single request, and they are masked.
func TestHTTPClientReadsStateAndImageOfEveryContainer(t *testing.T) {
	e := newEnv(t)
	serve(t, e, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"Names":["/shop-web-1"],"State":"exited","Image":"registry.example.test/demo/web:1","Labels":{"com.docker.compose.project":"shop"}},` +
			`{"Names":["/loose"],"State":"run\u0007ning","Image":"demo/loose:2","Labels":null}]`))
	})
	engines := realScan(e).Engines()
	if len(engines) != 1 || engines[0].ID != "rootless:alice" || len(engines[0].Containers) != 2 {
		t.Fatalf("engines = %+v", engines)
	}
	web, loose := engines[0].Containers[0], engines[0].Containers[1]
	if web.State != "exited" || web.Image != "registry.example.test/demo/web:1" || web.Project != "shop" {
		t.Fatalf("web = %+v", web)
	}
	if loose.State != "run\uFFFDning" {
		t.Fatalf("state is not masked: %q", loose.State)
	}
}

func TestHTTPClientOversizedAnswer(t *testing.T) {
	e := newEnv(t)
	serve(t, e, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("["))
		_, _ = w.Write(bytes.Repeat([]byte(" "), maxAnswer+1))
		_, _ = w.Write([]byte("]"))
	})
	sc := realScan(e)
	if len(sc.NotChecked) != 1 || !strings.Contains(sc.NotChecked[0].Reason, "too large") {
		t.Fatalf("%+v", sc.NotChecked)
	}
}

func TestHTTPClientNon200(t *testing.T) {
	e := newEnv(t)
	serve(t, e, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`[{"Names":["/x"],"Labels":{"com.docker.compose.project":"p","com.docker.compose.project.working_dir":"/srv/shop"}}]`))
	})
	sc := realScan(e)
	if len(sc.NotChecked) != 1 || !strings.Contains(sc.NotChecked[0].Reason, "error status") || len(sc.Conflicts("/srv/shop")) != 0 {
		t.Fatalf("%+v", sc.NotChecked)
	}
}
