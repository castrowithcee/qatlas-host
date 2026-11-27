package usage

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestBoundedSkipsAHangingMountAndRecovers(t *testing.T) {
	release := make(chan struct{})
	var hangCalls atomic.Int32
	fn := func(path string) (FSStat, error) {
		if path == "/hang" {
			hangCalls.Add(1)
			<-release
		}
		return FSStat{Blocks: 1}, nil
	}
	s := bounded(fn, 20*time.Millisecond)
	if _, err := s("/hang"); err == nil {
		t.Fatal("the hanging mount was reported")
	}
	if st, err := s("/ok"); err != nil || st.Blocks != 1 {
		t.Fatalf("healthy mount: %+v, %v", st, err)
	}
	start := time.Now()
	if _, err := s("/hang"); err == nil || time.Since(start) > 15*time.Millisecond {
		t.Fatalf("second read: err %v after %v", err, time.Since(start))
	}
	if n := hangCalls.Load(); n != 1 {
		t.Fatalf("%d calls for the hanging path", n)
	}
	close(release)
	deadline := time.Now().Add(2 * time.Second)
	for {
		if st, err := s("/hang"); err == nil && st.Blocks == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the mount was not measured again after it answered")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
