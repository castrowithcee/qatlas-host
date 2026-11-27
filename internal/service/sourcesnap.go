package service

import (
	"errors"

	"github.com/castrowithcee/qatlas-host/internal/engine"
	"github.com/castrowithcee/qatlas-host/internal/formatnum"
)

// sourceSnapshot is the state of one source.
type sourceSnapshot struct {
	conf sourceConfig
	snap engine.Snapshot
	err  error // set when the engine state of the source cannot be read
}

// snapshots returns the state of every source in source order, or the first error.
func (s *sourceSet) snapshots() ([]sourceSnapshot, error) {
	out := s.snapshotsEach()
	for _, ss := range out {
		if ss.err != nil {
			return nil, ss.err
		}
	}
	return out, nil
}

// snapshotsEach returns the state of every source in source order; a source whose state cannot be read carries
// the error and an empty state, so the others stay usable.
func (s *sourceSet) snapshotsEach() []sourceSnapshot {
	var out []sourceSnapshot
	for _, src := range s.sources() {
		snap, err := src.eng.Snapshot()
		out = append(out, sourceSnapshot{conf: src.conf, snap: snap, err: err})
	}
	return out
}

// Snapshot merges the state of the sources by stack name: each name has the state of the source that owns it.
func (s *sourceSet) Snapshot() (engine.Snapshot, error) {
	snaps, err := s.snapshots()
	if err != nil {
		return engine.Snapshot{}, err
	}
	owner := s.resolve().owner
	out := engine.Snapshot{State: engine.State{Stacks: map[string]engine.StackState{}}, Active: map[string]string{}}
	if len(snaps) > 0 {
		first := snaps[0].snap.State
		out.State.LastPoll, out.State.PollError, out.State.Head, out.State.RemoteHead, out.State.Note =
			first.LastPoll, first.PollError, first.Head, first.RemoteHead, first.Note
	}
	ownedBy := func(name string, c sourceConfig) bool { o := owner[name]; return o == nil || o.conf.ID == c.ID }
	for _, ss := range snaps {
		for name, st := range ss.snap.State.Stacks {
			if _, seen := out.State.Stacks[name]; !seen && ownedBy(name, ss.conf) {
				out.State.Stacks[name] = st
			}
		}
		for name, id := range ss.snap.Active {
			if _, seen := out.Active[name]; !seen && ownedBy(name, ss.conf) {
				out.Active[name] = id
			}
		}
	}
	return out, nil
}

// unreadableText is the answer's text for a state that cannot be read: a refusal by format number names its
// file once, without the wrapping that repeats the path.
func unreadableText(err error) string {
	var fe *formatnum.Error
	if errors.As(err, &fe) {
		return printable(fe.Error())
	}
	return printable(err.Error())
}
