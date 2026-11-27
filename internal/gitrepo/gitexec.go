package gitrepo

import (
	"bytes"
	"errors"
	"time"
)

// gitBinary is the git the call runs: the test's replacement or the one in the fixed path.
func (r *Repo) gitBinary() (string, error) {
	if r.cfg.test.gitPath != "" {
		return r.cfg.test.gitPath, nil
	}
	p, err := lookIn(fixedPath, "git")
	if err != nil {
		return "", errors.New("git is not installed")
	}
	return p, nil
}

func (r *Repo) timeout() time.Duration {
	if r.cfg.Timeout <= 0 {
		return DefaultTimeout
	}
	return r.cfg.Timeout
}

// capped keeps the head of what is written; once more arrives it calls onOver, which ends the process.
type capped struct {
	buf    bytes.Buffer
	limit  int
	over   bool
	onOver func()
}

func (c *capped) Write(p []byte) (int, error) {
	n := len(p)
	if room := c.limit - c.buf.Len(); n > room {
		if !c.over && c.onOver != nil {
			c.onOver()
		}
		c.over = true
		p = p[:max(room, 0)]
	}
	c.buf.Write(p)
	return n, nil
}

func (c *capped) String() string { return c.buf.String() }
