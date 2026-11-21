package service_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/castrowithcee/qatlas-host/internal/service"
	"github.com/castrowithcee/qatlas-host/internal/service/servicetest"
)

// A marker in the folder of the service checks the stack of its name, and one for a stack that waits for the
// window checks nothing. Both markers are gone afterwards, and a service whose folder does not exist runs.
func TestServiceTakesWebhookMarkers(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "hook")
	clock := &svcClock{now: noon}
	reg := newFakeRegistry(t, "0.9.1", "0.9.2", "1.2.3", "1.3.0", "2.0.0", "2.1.0-rc.1")
	dep := &sequentialDeployer{}
	boundWith(t, dep, map[string]string{
		servicetest.Settings("demo-stack"): "version: 1\nupdate:\n  mode: manual\n  trigger: webhook\n",
	}, withRegistry(reg.URL), withClock(clock, &noticeLog{}), func(c *service.Config) {
		c.MonitorInterval, c.PollInterval = 24*time.Hour, 24*time.Hour
		c.HookDir, c.HookInterval = dir, time.Second
	})
	// The folder is not there yet: the service runs on.
	clock.Advance(time.Second)
	base := reg.listCount()

	if err := os.Mkdir(dir, 0o770); err != nil {
		t.Fatal(err)
	}
	marker := func(name string) string { return filepath.Join(dir, name) }
	for _, name := range []string{"plain-stack", "demo-stack"} {
		if err := os.WriteFile(marker(name), nil, 0o660); err != nil {
			t.Fatal(err)
		}
	}
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		clock.Advance(time.Second)
		_, e1 := os.Lstat(marker("plain-stack"))
		_, e2 := os.Lstat(marker("demo-stack"))
		if os.IsNotExist(e1) && os.IsNotExist(e2) && reg.listCount() > base {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("markers not taken: %v %v, tag lists %d", e1, e2, reg.listCount()-base)
		}
	}
	if got := reg.listCount() - base; got != 1 {
		t.Fatalf("tag lists = %d, want the one check of the webhook stack", got)
	}
}
