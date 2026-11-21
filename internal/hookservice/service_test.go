package hookservice

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type fixture struct {
	svc    *Service
	runDir string
	cfg    string
	logs   *bytes.Buffer
	clock  *fakeClock
}

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func newFixture(t *testing.T, hooks ...Hook) *fixture {
	t.Helper()
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.json")
	writeConfig(t, cfg, cfgOf(hooks...), 0o644)
	runDir := filepath.Join(dir, "run")
	for _, inst := range []string{"system", "alice"} {
		if err := os.MkdirAll(filepath.Join(runDir, inst), 0o770); err != nil {
			t.Fatal(err)
		}
	}
	buf := &bytes.Buffer{}
	clock := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	svc, err := New(Options{ConfigPath: cfg, RunDir: runDir, OwnerUID: uint32(os.Getuid()), Now: clock.now,
		Logger: slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))})
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{svc: svc, runDir: runDir, cfg: cfg, logs: buf, clock: clock}
}

func (f *fixture) do(t *testing.T, method, path, body string) int {
	t.Helper()
	rec := httptest.NewRecorder()
	f.svc.Handler().ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	if rec.Body.Len() != 0 {
		t.Errorf("%s %s: body %q, want none", method, path, rec.Body.String())
	}
	return rec.Code
}

func (f *fixture) marker(inst, stack string) string { return filepath.Join(f.runDir, inst, stack) }

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

func TestKnownSecretDropsTheMarkerOfTheRightInstance(t *testing.T) {
	sa, sb := newSecret(t), newSecret(t)
	f := newFixture(t, hookFor("h-00000001", "web", "system", sa), hookFor("h-00000002", "blog", "alice", sb))
	if code := f.do(t, "POST", "/hook/"+sb, "{}"); code != 202 {
		t.Fatalf("code = %d", code)
	}
	if !exists(f.marker("alice", "blog")) {
		t.Error("marker of alice/blog missing")
	}
	if exists(f.marker("system", "web")) || exists(f.marker("system", "blog")) || exists(f.marker("alice", "web")) {
		t.Error("a marker of another hook or instance exists")
	}
	info, _ := os.Stat(f.marker("alice", "blog"))
	if info.Size() != 0 || info.Mode().Perm() != 0o660 {
		t.Errorf("marker size %d mode %v, want empty 0660", info.Size(), info.Mode().Perm())
	}
}

func TestOtherRequestsGet404WithoutMarker(t *testing.T) {
	secret := newSecret(t)
	f := newFixture(t, hookFor("h-00000001", "web", "system", secret))
	cases := []struct{ name, method, path string }{
		{"unknown secret", "POST", "/hook/" + newSecret(t)},
		{"GET", "GET", "/hook/" + secret},
		{"PUT", "PUT", "/hook/" + secret},
		{"other path", "POST", "/other/" + secret},
		{"root", "POST", "/"},
		{"empty secret", "POST", "/hook/"},
		{"no slash", "POST", "/hook"},
		{"short secret", "POST", "/hook/" + secret[:10]},
		{"extra segment", "POST", "/hook/" + secret + "/x"},
		{"padded", "POST", "/hook/" + secret + "="},
		{"trailing slash", "POST", "/hook/" + secret + "/"},
	}
	for _, c := range cases {
		if code := f.do(t, c.method, c.path, ""); code != 404 {
			t.Errorf("%s: code = %d, want 404", c.name, code)
		}
	}
	if exists(f.marker("system", "web")) {
		t.Fatal("a rejected request left a marker")
	}
}

func TestQueryAndHeadersAreIgnored(t *testing.T) {
	secret := newSecret(t)
	f := newFixture(t, hookFor("h-00000001", "web", "system", secret))
	req := httptest.NewRequest("POST", "/hook/"+secret+"?stack=../../x&instance=alice", nil)
	req.Header.Set("X-Stack", "blog")
	rec := httptest.NewRecorder()
	f.svc.Handler().ServeHTTP(rec, req)
	if rec.Code != 202 || !exists(f.marker("system", "web")) || exists(f.marker("alice", "blog")) {
		t.Fatalf("code %d", rec.Code)
	}
}

type countingReader struct{ n int }

func (c *countingReader) Read(p []byte) (int, error) {
	c.n += len(p)
	for i := range p {
		p[i] = 'x'
	}
	return len(p), nil
}

func TestBodyIsReadUpTo64KiBAndDiscarded(t *testing.T) {
	secret := newSecret(t)
	f := newFixture(t, hookFor("h-00000001", "web", "system", secret))
	body := &countingReader{}
	req := httptest.NewRequest("POST", "/hook/"+secret, io.NopCloser(body))
	rec := httptest.NewRecorder()
	f.svc.Handler().ServeHTTP(rec, req)
	if rec.Code != 202 {
		t.Fatalf("code = %d", rec.Code)
	}
	if body.n > 64<<10 {
		t.Fatalf("read %d bytes, want at most %d", body.n, 64<<10)
	}
	if body.n == 0 {
		t.Fatal("body not read")
	}
}

func TestSecondCallWithinFiveSecondsWritesNothing(t *testing.T) {
	secret := newSecret(t)
	f := newFixture(t, hookFor("h-00000001", "web", "system", secret))
	path := "/hook/" + secret
	if f.do(t, "POST", path, "") != 202 {
		t.Fatal("first call")
	}
	if err := os.Remove(f.marker("system", "web")); err != nil {
		t.Fatal(err)
	}
	f.clock.advance(4 * time.Second)
	if f.do(t, "POST", path, "") != 202 {
		t.Fatal("second call must still answer 202")
	}
	if exists(f.marker("system", "web")) {
		t.Fatal("second call within 5 s wrote a marker")
	}
	f.clock.advance(time.Second)
	if f.do(t, "POST", path, "") != 202 || !exists(f.marker("system", "web")) {
		t.Fatal("call after 5 s must write again")
	}
}

func TestTotalLimitAnswers429(t *testing.T) {
	secret := newSecret(t)
	f := newFixture(t, hookFor("h-00000001", "web", "system", secret))
	for i := 0; i < maxRequests; i++ {
		if code := f.do(t, "POST", "/hook/"+secret, ""); code != 202 {
			t.Fatalf("call %d: %d", i, code)
		}
	}
	if code := f.do(t, "POST", "/hook/"+secret, ""); code != 429 {
		t.Fatalf("over the limit: %d, want 429", code)
	}
	if code := f.do(t, "GET", "/", ""); code != 429 {
		t.Fatalf("a rejected request counts too: %d, want 429", code)
	}
	f.clock.advance(time.Minute)
	if code := f.do(t, "POST", "/hook/"+secret, ""); code != 202 {
		t.Fatalf("next window: %d", code)
	}
}

func TestExistingMarkerIsNoError(t *testing.T) {
	secret := newSecret(t)
	f := newFixture(t, hookFor("h-00000001", "web", "system", secret))
	if err := os.WriteFile(f.marker("system", "web"), nil, 0o660); err != nil {
		t.Fatal(err)
	}
	if code := f.do(t, "POST", "/hook/"+secret, ""); code != 202 {
		t.Fatalf("code = %d", code)
	}
}

func TestSymlinkAtTheMarkerIsNotFollowed(t *testing.T) {
	secret := newSecret(t)
	f := newFixture(t, hookFor("h-00000001", "web", "system", secret))
	target := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(target, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, f.marker("system", "web")); err != nil {
		t.Fatal(err)
	}
	if code := f.do(t, "POST", "/hook/"+secret, ""); code == 202 {
		t.Fatal("a symlink at the marker was accepted")
	}
	data, err := os.ReadFile(target)
	info, _ := os.Stat(target)
	if err != nil || string(data) != "keep" || info.Mode().Perm() != 0o600 {
		t.Fatalf("symlink target changed: %q %v %v", data, info.Mode().Perm(), err)
	}
	// A missing target must not be created either.
	gone := filepath.Join(t.TempDir(), "gone")
	_ = os.Remove(f.marker("system", "web"))
	if err := os.Symlink(gone, f.marker("system", "web")); err != nil {
		t.Fatal(err)
	}
	f.clock.advance(time.Minute)
	f.do(t, "POST", "/hook/"+secret, "")
	if exists(gone) {
		t.Fatal("a dangling symlink target was created")
	}
}

// Instance and stack are checked against their patterns at the moment of writing, whatever the table holds.
func TestMarkerPathIsCheckedAgainstThePatterns(t *testing.T) {
	runDir := t.TempDir()
	hash := hashOf(newSecret(t))
	for _, h := range []Hook{
		{ID: "h-00000001", Stack: "../escape", Instance: "system", Hash: hash},
		{ID: "h-00000001", Stack: "a/b", Instance: "system", Hash: hash},
		{ID: "h-00000001", Stack: "web", Instance: "../x", Hash: hash},
		{ID: "h-00000001", Stack: "", Instance: "system", Hash: hash},
		{ID: "h-00000001", Stack: ".hidden", Instance: "system", Hash: hash},
	} {
		if err := writeMarker(runDir, h); err == nil {
			t.Errorf("writeMarker accepted %+v", h)
		}
	}
	entries, _ := os.ReadDir(runDir)
	if len(entries) != 0 {
		t.Fatalf("run dir holds %d entries", len(entries))
	}
}

func TestNewHookTakesEffectWithoutRestart(t *testing.T) {
	first, second := newSecret(t), newSecret(t)
	f := newFixture(t, hookFor("h-00000001", "web", "system", first))
	if f.do(t, "POST", "/hook/"+second, "") != 404 {
		t.Fatal("second secret known too early")
	}
	writeConfig(t, f.cfg, cfgOf(hookFor("h-00000001", "web", "system", first), hookFor("h-00000002", "blog", "alice", second)), 0o644)
	if err := f.svc.Reload(); err != nil {
		t.Fatal(err)
	}
	if f.do(t, "POST", "/hook/"+second, "") != 202 || !exists(f.marker("alice", "blog")) {
		t.Fatal("new hook not effective after reload")
	}
}

func TestRefusedConfigKeepsTheOldTable(t *testing.T) {
	secret := newSecret(t)
	f := newFixture(t, hookFor("h-00000001", "web", "system", secret))
	for name, write := range map[string]func(){
		"group writable": func() { writeConfig(t, f.cfg, cfgOf(), 0o664) },
		"unknown field":  func() { writeRaw(t, f.cfg, `{"version":1,"listen":"a:1","hooks":[],"x":1}`, 0o644) },
		"wrong version":  func() { writeRaw(t, f.cfg, `{"version":9,"listen":"a:1","hooks":[]}`, 0o644) },
		"garbage":        func() { writeRaw(t, f.cfg, `nope`, 0o644) },
		"removed":        func() { _ = os.Remove(f.cfg) },
	} {
		write()
		if err := f.svc.Reload(); err == nil {
			t.Errorf("%s: reload accepted", name)
		}
		f.clock.advance(time.Minute)
		if f.do(t, "POST", "/hook/"+secret, "") != 202 {
			t.Errorf("%s: old table lost", name)
		}
	}
}

func TestReloadLoopPicksUpAChange(t *testing.T) {
	secret := newSecret(t)
	f := newFixture(t, hookFor("h-00000001", "web", "system", newSecret(t)))
	f.svc.opts.ReloadEvery = 5 * time.Millisecond
	go f.svc.reloadLoop(t.Context())
	writeConfig(t, f.cfg, cfgOf(hookFor("h-00000002", "blog", "alice", secret)), 0o644)
	deadline := time.Now().Add(5 * time.Second)
	for {
		rec := httptest.NewRecorder()
		f.svc.Handler().ServeHTTP(rec, httptest.NewRequest("POST", "/hook/"+secret, nil))
		if rec.Code == 202 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("reload loop did not apply the change")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestNewRefusesABadConfigAtStart(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "config.json")
	writeConfig(t, cfg, cfgOf(), 0o666)
	if _, err := New(Options{ConfigPath: cfg, OwnerUID: uint32(os.Getuid())}); err == nil {
		t.Fatal("started with a world-writable configuration")
	}
}

func TestNeitherSecretNorBodyReachTheLog(t *testing.T) {
	secret, wrong := newSecret(t), newSecret(t)
	f := newFixture(t, hookFor("h-00000001", "web", "system", secret))
	const payload = "payload-marker-demo"
	f.do(t, "POST", "/hook/"+secret+"?q="+payload, payload)
	f.clock.advance(time.Minute)
	f.do(t, "POST", "/hook/"+wrong+"?q="+payload, payload)
	f.do(t, "GET", "/hook/"+secret, payload)
	// A failed write logs an error; the path must not carry the secret either.
	if err := os.RemoveAll(filepath.Join(f.runDir, "system")); err != nil {
		t.Fatal(err)
	}
	f.clock.advance(time.Minute)
	f.do(t, "POST", "/hook/"+secret, payload)
	f.svc.flushRejected()
	log := f.logs.String()
	for _, bad := range []string{secret, wrong, payload, "/hook/"} {
		if strings.Contains(log, bad) {
			t.Errorf("log contains %q:\n%s", bad, log)
		}
	}
	if !strings.Contains(log, "h-00000001") || !strings.Contains(log, "web") {
		t.Errorf("log lacks hook id and stack:\n%s", log)
	}
	if !strings.Contains(log, "count=2") {
		t.Errorf("rejected requests not summarized once:\n%s", log)
	}
}

func TestAcceptedCallsLogOneLineEach(t *testing.T) {
	secret := newSecret(t)
	f := newFixture(t, hookFor("h-00000001", "web", "system", secret))
	f.do(t, "POST", "/hook/"+secret, "")
	f.do(t, "POST", "/hook/"+secret, "")
	if n := strings.Count(f.logs.String(), "hook accepted"); n != 2 {
		t.Fatalf("%d accepted lines, want 2", n)
	}
}

func TestHTTPServerLimits(t *testing.T) {
	f := newFixture(t)
	srv := f.svc.newHTTPServer()
	if srv.ReadHeaderTimeout != 5*time.Second || srv.ReadTimeout != 10*time.Second || srv.WriteTimeout != 10*time.Second ||
		srv.IdleTimeout != 30*time.Second || srv.MaxHeaderBytes != 8<<10 {
		t.Fatalf("limits = %v %v %v %v %d", srv.ReadHeaderTimeout, srv.ReadTimeout, srv.WriteTimeout, srv.IdleTimeout, srv.MaxHeaderBytes)
	}
}

func TestWorksOverARealServer(t *testing.T) {
	secret := newSecret(t)
	f := newFixture(t, hookFor("h-00000001", "web", "system", secret))
	ts := httptest.NewServer(f.svc.Handler())
	defer ts.Close()
	resp, err := http.Post(ts.URL+"/hook/"+secret, "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 202 || !exists(f.marker("system", "web")) {
		t.Fatalf("status %d", resp.StatusCode)
	}
}
