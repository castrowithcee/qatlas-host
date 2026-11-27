package usage

import (
	"errors"
	"sync"
	"time"
)

// statfsTimeout is how long one request waits for a mount that does not answer.
const statfsTimeout = time.Second

var errNoAnswer = errors.New("the file system does not answer")

// flight is one running statfs call. abandoned is set once a request gave up on it.
type flight struct {
	done      chan struct{}
	st        FSStat
	err       error
	abandoned bool
}

// bounded wraps fn so that a call never holds a request longer than timeout. A call that outlives its
// deadline keeps running in its goroutine, and until it returns no new call starts for that path, so a
// hanging mount costs at most one thread.
func bounded(fn func(string) (FSStat, error), timeout time.Duration) func(string) (FSStat, error) {
	var mu sync.Mutex
	flights := map[string]*flight{}
	return func(path string) (FSStat, error) {
		mu.Lock()
		f := flights[path]
		if f != nil && f.abandoned {
			mu.Unlock()
			return FSStat{}, errNoAnswer
		}
		if f == nil {
			f = &flight{done: make(chan struct{})}
			flights[path] = f
			go func() {
				st, err := fn(path)
				mu.Lock()
				f.st, f.err = st, err
				delete(flights, path)
				mu.Unlock()
				close(f.done)
			}()
		}
		mu.Unlock()
		t := time.NewTimer(timeout)
		defer t.Stop()
		select {
		case <-f.done:
			return f.st, f.err
		case <-t.C:
			mu.Lock()
			f.abandoned = true
			mu.Unlock()
			return FSStat{}, errNoAnswer
		}
	}
}
