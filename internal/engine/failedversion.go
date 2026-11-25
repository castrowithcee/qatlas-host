package engine

import (
	"errors"
	"time"
)

// Observation is an update by qhost and its observation time: a failure of the stack from Since until before
// Until rolls back exactly this update. Run made it, From is the commit the stack ran at before it, Commit its
// commit and Backup the backup it made, empty when the stack had no backup rules. Variable and Version are the
// step the update took.
type Observation struct {
	Run    string `json:"run"`
	From   string `json:"from"`
	Commit string `json:"commit"`
	Backup string `json:"backup,omitempty"`
	// Variable and Version are the image version variable the update set and the version it set.
	Variable string    `json:"variable,omitempty"`
	Version  string    `json:"version,omitempty"`
	Since    time.Time `json:"since"`
	Until    time.Time `json:"until"`
}

func validateObservation(o *Observation) error {
	if o != nil && (!runIDRe.MatchString(o.Run) || !commitRe.MatchString(o.From) ||
		!commitRe.MatchString(o.Commit) || len(o.Backup) > maxNameLen || len(o.Variable) > MaxUpdateText ||
		len(o.Version) > MaxUpdateText || o.Since.IsZero() || !o.Until.After(o.Since)) {
		return errors.New("invalid observation")
	}
	return nil
}

func validateFailedVersions(m map[string]string) error {
	if len(m) > MaxUpdateItems {
		return errors.New("too many failed versions")
	}
	for variable, v := range m {
		if variable == "" || v == "" || len(variable) > MaxUpdateText || len(v) > MaxUpdateText {
			return errors.New("invalid failed version")
		}
	}
	return nil
}

// trackVersions updates what the run's result means for the versions of an update: a rolled-back step or
// observed update locks its version, a successful update of a variable lifts its lock, and a successful step
// starts the observation of the stack when its settings ask for one.
func (r *run) trackVersions(cur *StackState, rec RunRecord, now time.Time) {
	cur.Observe = nil
	switch {
	case rec.Result == ResultSucceeded && r.change != nil && r.back == nil:
		delete(cur.FailedVersions, r.change.Variable)
	case rec.Result == ResultRolledBack && r.undo != nil:
		cur.lockVersion(r.undo.Variable, r.undo.Version)
	case rec.Result == ResultRolledBack && r.change != nil && r.back == nil:
		cur.lockVersion(r.change.Variable, r.change.To)
	}
	if d := r.st.ObserveTime(); rec.Result == ResultSucceeded && rec.Trigger == TriggerUpdate && d > 0 {
		cur.Observe = &Observation{Run: rec.ID, From: r.deployed, Commit: rec.Commit, Backup: rec.Backup,
			Variable: r.change.Variable, Version: r.change.To, Since: now, Until: now.Add(d)}
	}
}

// lockVersion records version as failed for variable; an empty variable, from an observation made before
// the field existed, records nothing.
func (s *StackState) lockVersion(variable, version string) {
	if variable == "" {
		return
	}
	if s.FailedVersions == nil {
		s.FailedVersions = map[string]string{}
	}
	s.FailedVersions[variable] = version
}

// clonePtr copies the value p points to, nil stays nil.
func clonePtr[T any](p *T) *T {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}
