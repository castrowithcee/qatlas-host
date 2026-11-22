package control

import "time"

// DeployLogEntry is one entry of the action log of a deploy service as its parsed fields carry it, newest first
// in DeployLogListResult. Command and Class are empty for a peer refused before it sent a command or for a
// command the service does not know; Stack is set only for a command of scope ScopeStack that names one stack.
// The entry never holds an argument. ClientUID is a pointer because 0 is root.
type DeployLogEntry struct {
	Seq        uint64    `json:"seq"`
	Time       time.Time `json:"time"`
	ClientUID  *uint32   `json:"client_uid,omitempty"`
	Command    string    `json:"command,omitempty"`
	Class      string    `json:"class,omitempty"`
	Scope      string    `json:"scope,omitempty"`
	Stack      string    `json:"stack,omitempty"`
	Result     string    `json:"result"`
	ErrorCode  string    `json:"error_code,omitempty"`
	DurationMS int64     `json:"duration_ms,omitempty"`
	// Origin is what the client claimed: cli, tui, mcp or unknown. It is display only.
	Origin string `json:"origin"`
}

// DeployLogListResult answers LogListCommand of a deploy service; the filters are those of HostLogListArgs, with
// Action matching the command name. Next is set when more entries match.
type DeployLogListResult struct {
	Entries []DeployLogEntry `json:"entries"`
	Next    string           `json:"next,omitempty"`
}
