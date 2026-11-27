package control

import "time"

// The kinds SourceStatus.Kind takes: a clone of a repository, or a repository without a remote deployed as it is.
const (
	SourceKindRemote = "remote"
	SourceKindLocal  = "local"
)

// SourceStatus is the state of one stack folder as of its last poll. PollError tells why the poll could not
// complete, Note why the working copy was not updated, Unreadable which engine state cannot be read (no stacks then).
type SourceStatus struct {
	ID   string `json:"id"`
	Path string `json:"path"`
	// Engine is the engine of the deploy service that holds the stack folder; empty as in StackStatus.Engine.
	Engine string `json:"engine,omitempty"`
	// Kind is SourceKindRemote or SourceKindLocal; URL is the repository address and empty for a local one.
	Kind       string    `json:"kind"`
	URL        string    `json:"url,omitempty"`
	LastPoll   time.Time `json:"last_poll,omitzero"`
	PollError  string    `json:"poll_error,omitempty"`
	Note       string    `json:"note,omitempty"`
	Head       string    `json:"head,omitempty"`
	RemoteHead string    `json:"remote_head,omitempty"`
	Unreadable string    `json:"unreadable,omitempty"`
	// GitUnsafe is why the service does not run git in the folder: its .git could be changed by someone other
	// than root. Nothing is fetched or run until it is made safe.
	GitUnsafe string `json:"git_unsafe,omitempty"`
}
