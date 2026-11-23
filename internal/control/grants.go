package control

import (
	"regexp"
	"time"
)

// The commands that manage the standing grants of one deploy instance: a person with root allows a user one
// action for one stack. They only record, change and list grants; what a grant allows at run time is decided
// elsewhere. A grant never covers these commands.
const (
	GrantAddCommand    = "grant.add"
	GrantRemoveCommand = "grant.remove"
	GrantListCommand   = "grant.list"
)

// The actions a grant can name, and no others.
const (
	GrantActionStackRestart     = "stack.restart"
	GrantActionContainerRestart = "container.restart"
	GrantActionUpdateCheck      = "update.check"
	GrantActionUpdateRun        = "update.run"
)

// GrantActions lists every action a grant can name.
var GrantActions = []string{GrantActionStackRestart, GrantActionContainerRestart, GrantActionUpdateCheck, GrantActionUpdateRun}

const (
	// GrantIDPrefix starts the ID of every grant; eight lower-case hex digits follow.
	GrantIDPrefix = "g-"
	// GrantMax is the most grants one instance stores, valid or not.
	GrantMax = 200
	// GrantExpiresLayout is the form of an expiry date. A grant is valid until the end of that day in UTC.
	GrantExpiresLayout = "2006-01-02"
)

var grantIDPattern = regexp.MustCompile(`^` + regexp.QuoteMeta(GrantIDPrefix) + `[0-9a-f]{8}$`)

// ValidGrantID reports whether id has the form of a grant ID.
func ValidGrantID(id string) bool { return grantIDPattern.MatchString(id) }

// The codes of the answers to the grant commands, besides CodeInvalid, CodeNoSuchStack and CodeNotRoot.
const (
	CodeGrantAction      = "unknown-grant-action"
	CodeNoSuchUser       = "no-such-user"
	CodeUserNoAccess     = "user-without-access"
	CodeNoSuchGrant      = "no-such-grant"
	CodeGrantLimit       = "grant-limit"
	CodeGrantsUnreadable = "grants-unreadable"
)

// The reasons a stored grant is invalid. A grant with one of them is listed, never applied.
const (
	GrantExpired      = "expired"
	GrantStackMissing = "stack-missing"
	// GrantUserChanged means the user ID belongs to another name today, or to nobody.
	GrantUserChanged = "user-changed"
)

// Grant is one standing grant. User is the name the user ID had when the grant was made.
type Grant struct {
	ID     string `json:"id"`
	User   string `json:"user"`
	UID    uint32 `json:"uid"`
	Action string `json:"action"`
	Stack  string `json:"stack"`
	// Expires is a date in GrantExpiresLayout; empty means the grant does not expire.
	Expires   string    `json:"expires,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	// CreatedBy is the user ID of the person behind the sudo call, 0 when unknown.
	CreatedBy uint32 `json:"created_by"`
}

// GrantAddArgs are the arguments of GrantAddCommand. Without Confirm nothing is stored: the answer is the plan.
// SudoUID is the person behind the sudo call; only root reaches the command, so the field is as trustworthy as
// the caller.
type GrantAddArgs struct {
	User    string `json:"user"`
	Action  string `json:"action"`
	Stack   string `json:"stack"`
	Expires string `json:"expires,omitempty"`
	SudoUID uint32 `json:"sudo_uid,omitempty"`
	Confirm bool   `json:"confirm,omitempty"`
}

// GrantAddResult answers GrantAddCommand. Grant is the grant as it is or would be stored. Replaced is true when an
// equal grant existed and only its expiry changes; Applied is false for a plan. Backup and Check tell whether
// the stack has a backup and a health check set up, Settings whether it has settings at all.
type GrantAddResult struct {
	Grant    Grant `json:"grant"`
	Applied  bool  `json:"applied"`
	Replaced bool  `json:"replaced,omitempty"`
	Settings bool  `json:"settings"`
	Backup   bool  `json:"backup"`
	Check    bool  `json:"check"`
}

// GrantRemoveArgs are the arguments of GrantRemoveCommand. The ID alone names the grant, and a request with only
// the ID is one of scope deploy. A caller that knows the stack of the grant may name it: the request is then one
// of scope stack, and the command refuses an ID whose grant belongs to another stack.
type GrantRemoveArgs struct {
	ID    string `json:"id"`
	Stack string `json:"stack,omitempty"`
}

// GrantRemoveResult answers GrantRemoveCommand with the grant that was removed.
type GrantRemoveResult struct {
	Grant Grant `json:"grant"`
}

// GrantListArgs are the arguments of GrantListCommand; an empty field does not filter. User is a name.
type GrantListArgs struct {
	Stack string `json:"stack,omitempty"`
	User  string `json:"user,omitempty"`
}

// GrantInfo is a stored grant with its state today. Invalid holds one of the Grant reasons, empty for a valid
// grant.
type GrantInfo struct {
	Grant
	Invalid string `json:"invalid,omitempty"`
}

// GrantListResult answers GrantListCommand. Problem is set when the file of the grants cannot be read: no grant
// is listed then, and every grant counts as invalid until a person has dealt with the file.
type GrantListResult struct {
	Grants  []GrantInfo `json:"grants"`
	Problem string      `json:"problem,omitempty"`
}

// ProposalKindGrant proposes a standing grant for the user who submits it; its content is a
// ProposalGrantRequest, and the stored form is a ProposalGrant. The deploy service keeps these proposals.
const ProposalKindGrant = "grant"

// ProposalGrantReasonMaxChars is the most characters the reason of a grant proposal may have.
const ProposalGrantReasonMaxChars = 500

// ProposalGrantRequest is the content a user submits for a proposal of kind ProposalKindGrant. It has no
// member for a user: the grant is always for the submitter.
type ProposalGrantRequest struct {
	Action  string `json:"action"`
	Stack   string `json:"stack"`
	Expires string `json:"expires,omitempty"`
	Reason  string `json:"reason"`
}

// ProposalGrant is the normalized content of a proposal of kind ProposalKindGrant, the one the fingerprint
// covers. User and UID are those of the submitter, set by the service. Reason is text of the submitter: it is
// stored and shown, never interpreted.
type ProposalGrant struct {
	User    string `json:"user"`
	UID     uint32 `json:"uid"`
	Action  string `json:"action"`
	Stack   string `json:"stack"`
	Expires string `json:"expires,omitempty"`
	Reason  string `json:"reason"`
}
