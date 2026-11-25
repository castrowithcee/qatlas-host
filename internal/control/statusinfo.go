package control

import (
	"maps"
	"slices"
	"strings"
	"time"
)

// The parts of StackStatus that describe one aspect of a stack.

// FailedVersionsText lists locked versions as "VAR=version, ..." in the order of the variables.
func FailedVersionsText(m map[string]string) string {
	keys := slices.Sorted(maps.Keys(m))
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + "=" + m[k]
	}
	return strings.Join(parts, ", ")
}

// ObserveInfo is a running observation time: the update commit and when the time ends.
type ObserveInfo struct {
	Commit string    `json:"commit"`
	Until  time.Time `json:"until"`
}

// RepairInfo is the state of the automatic repair: the attempts made of Max, when the last was made and the
// next is due (zero when none is due), and since when the repair is halted after Max failed attempts.
type RepairInfo struct {
	Attempts int       `json:"attempts"`
	Max      int       `json:"max"`
	Last     time.Time `json:"last,omitzero"`
	Next     time.Time `json:"next,omitzero"`
	Halted   *HoldInfo `json:"halted,omitempty"`
}

// FailureInfo is a failure of a stack's containers: exited, unhealthy, restart-loop or missing, the container
// (the service for a missing one) and since when.
type FailureInfo struct {
	Kind      string    `json:"kind"`
	Container string    `json:"container,omitempty"`
	Since     time.Time `json:"since"`
}

// HoldInfo says since when an operator mark is set.
type HoldInfo struct {
	Since time.Time `json:"since"`
}

// PauseInfo says since when a stack is paused, after which run and why.
type PauseInfo struct {
	Since  time.Time `json:"since"`
	Run    string    `json:"run"`
	Reason string    `json:"reason,omitempty"`
}

// FindingInfo is one finding of the engine about a stack.
type FindingInfo struct {
	Kind   string `json:"kind"`
	Detail string `json:"detail,omitempty"`
	// Dirs lists the missing data directories of a finding of kind "data-dir-missing".
	Dirs []DirInfo `json:"dirs,omitempty"`
}

// DirInfo is a missing data directory: Path relative to the stack folder, Container the "UID:GID" of the
// container's user, Owner the proposed host "UID:GID" (empty when unknown) and Warning why there is none.
type DirInfo struct {
	Path      string `json:"path"`
	Owner     string `json:"owner,omitempty"`
	Container string `json:"container,omitempty"`
	Warning   string `json:"warning,omitempty"`
}
