package control

import (
	"maps"
	"slices"
	"strings"
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
