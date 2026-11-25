package control

import "time"

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

// FingerprintInfo is the fingerprint of the image a service of a stack runs. RepoDigest and ImageID are empty when the
// engine has none: the image has no repository digest, or it could not be read. Clients only show the values.
type FingerprintInfo struct {
	Service    string `json:"service"`
	Image      string `json:"image,omitempty"`
	RepoDigest string `json:"repo_digest,omitempty"`
	ImageID    string `json:"image_id,omitempty"`
}
