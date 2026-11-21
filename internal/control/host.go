package control

import "time"

// CodeNotAdmin answers a peer of the host service that may reach its channel but is not an administrator per the
// group database, or no longer is.
const CodeNotAdmin = "not-admin"

// ErrNotAdmin is the answer of the host service to a peer that is neither root nor an administrator in the
// service group.
func ErrNotAdmin() *Error {
	return NewError(CodeNotAdmin, "qhost is reserved for administrators (sudo)")
}

// The commands of the host service besides StatusCommand. The log commands read the action log and never return
// a file path or a raw line.
const (
	// HostLogListCommand lists entries of the action log, newest first, in pages.
	HostLogListCommand = "log.list"
	// HostLogVerifyCommand checks the hash chain of the action log.
	HostLogVerifyCommand = "log.verify"
	// HostRecordCommand records a finished change of the host in the action log. Only root may call it: the
	// sudo call that made the change is the proof.
	HostRecordCommand = "host.record"
	// HostCommandsSetCommand turns the switch for proposals of free commands on or off; only root may call it.
	HostCommandsSetCommand = "proposal.commands.set"
	// HostCommandsStatusCommand reads that switch.
	HostCommandsStatusCommand = "proposal.commands.status"
)

// HostCommandsSetArgs are the arguments of HostCommandsSetCommand.
type HostCommandsSetArgs struct {
	Enabled bool `json:"enabled"`
}

// HostCommandsResult answers HostCommandsSetCommand and HostCommandsStatusCommand: whether proposals of the kind
// ProposalKindCommand are accepted. The switch is off until root turns it on.
type HostCommandsResult struct {
	Enabled bool `json:"enabled"`
}

// Limits of HostLogListCommand: a request without a limit gets HostLogDefaultLimit entries.
const (
	HostLogDefaultLimit = 200
	HostLogMaxLimit     = 500
)

// The results an entry records, as HostLogListArgs.Result filters them.
const (
	HostLogResultSuccess = "success"
	HostLogResultFailure = "failure"
)

// HostLogListArgs are the arguments of HostLogListCommand. Every field narrows the result; From and To are
// RFC 3339 times, both inclusive. Action is an action word such as "log.read". Limit is 1 to HostLogMaxLimit, 0
// meaning HostLogDefaultLimit. After is the Next of the previous answer.
type HostLogListArgs struct {
	From   string `json:"from,omitempty"`
	To     string `json:"to,omitempty"`
	Action string `json:"action,omitempty"`
	Result string `json:"result,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	After  string `json:"after,omitempty"`
}

// HostLogEntry is one entry of the action log as its parsed fields carry it. ClientUID is a pointer because 0
// is root.
type HostLogEntry struct {
	Seq        uint64         `json:"seq"`
	Time       time.Time      `json:"time"`
	ClientUID  *uint32        `json:"client_uid,omitempty"`
	Action     string         `json:"action"`
	Result     string         `json:"result"`
	ErrorCode  string         `json:"error_code,omitempty"`
	DurationMS int64          `json:"duration_ms,omitempty"`
	Change     *HostLogChange `json:"change,omitempty"`
}

// HostLogChange names the change of the host an entry records: the registered action and the journal entries.
type HostLogChange struct {
	ActionID  string `json:"action_id"`
	JournalID string `json:"journal_id"`
	UndoOf    string `json:"undo_of,omitempty"`
}

// HostLogListResult answers HostLogListCommand. Next is set when more entries match; passing it as After
// continues right behind the last entry returned.
type HostLogListResult struct {
	Entries []HostLogEntry `json:"entries"`
	Next    string         `json:"next,omitempty"`
}

// HostLogVerifyMaxProblems is how many problems an answer of HostLogVerifyCommand lists at most.
const HostLogVerifyMaxProblems = 20

// HostLogDay is the state of one day file. Date is YYYY-MM-DD; Entries counts action entries. Damaged counts the
// unreadable lines of the day, such as one torn by a crash, that a marker entry right after them documents;
// such a line leaves the day intact.
type HostLogDay struct {
	Date    string `json:"date"`
	Entries int    `json:"entries"`
	Intact  bool   `json:"intact"`
	Damaged int    `json:"damaged,omitempty"`
}

// HostLogProblem is the first problem of one day: Seq is the sequence number of the entry concerned (0 for a
// line that does not parse) and Line the 1-based line number in the day's file. Code is a fixed word of the
// log, such as "chain-broken" or "unparsable-line".
type HostLogProblem struct {
	Date string `json:"date"`
	Seq  uint64 `json:"seq,omitempty"`
	Line int    `json:"line"`
	Code string `json:"code"`
}

// HostLogVerifyResult answers HostLogVerifyCommand. Intact is false when the chain is broken anywhere. Problems
// lists the first HostLogVerifyMaxProblems days with a problem, oldest first; MoreProblems says there were more.
type HostLogVerifyResult struct {
	Intact       bool             `json:"intact"`
	Days         []HostLogDay     `json:"days"`
	Problems     []HostLogProblem `json:"problems,omitempty"`
	MoreProblems bool             `json:"more_problems,omitempty"`
}

// The results of a change of the host that HostRecordCommand takes: the change stands, the change failed, and
// the change failed and the previous state is back.
const (
	HostResultApplied  = "applied"
	HostResultFailed   = "failed"
	HostResultRestored = "restored"
)

// HostRecordArgs are the arguments of HostRecordCommand. SudoUID is the human behind the sudo call, 0 when
// unknown. The arguments carry no sentence, command, argument or secret.
type HostRecordArgs struct {
	Action    string `json:"action"`
	Result    string `json:"result"`
	SudoUID   uint32 `json:"sudo_uid"`
	JournalID string `json:"journal_id"`
	UndoOf    string `json:"undo_of,omitempty"`
}
