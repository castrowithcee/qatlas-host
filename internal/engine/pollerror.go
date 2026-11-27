package engine

import "github.com/castrowithcee/qatlas-host/internal/gitrepo"

// gitChecker is implemented by a working copy that can tell whether git may run in it.
type gitChecker interface{ CheckGit() error }

// checkGit returns gitrepo.ErrGitUnsafe while the working copy's .git is not safe to run git in.
func checkGit(c WorkingCopy) error {
	if g, ok := c.(gitChecker); ok {
		return g.CheckGit()
	}
	return nil
}

func (e *Engine) recordPollError(perr error) {
	now := e.cfg.Clock.Now().UTC()
	err := e.update(func(s State) (State, error) {
		s = normalize(s)
		s.LastPoll, s.PollError, s.GitUnsafe = now, e.text(perr.Error()), ""
		if reason, ok := gitrepo.UnsafeReason(perr); ok {
			s.GitUnsafe = e.text(reason)
		}
		return s, nil
	})
	if err != nil {
		e.cfg.Logger.Warn("poll error not recorded", "error", err)
	}
}
