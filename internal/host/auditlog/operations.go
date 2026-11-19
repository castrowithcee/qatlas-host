package auditlog

// The actions an entry can record, grouped by the area that takes them. An entry names the action, never an
// argument, a value, or a secret.
const (
	// OperationLogRead is a read of the action log.
	OperationLogRead = "log.read"

	// The control socket: a request that names no known operation, is malformed, too large or too slow
	// is logged as OperationControlRequest; a peer turned away as OperationControlRefuse.
	OperationControlRequest = "control.request"
	OperationControlRefuse  = "control.refuse"
	OperationControlStatus  = "control.status"

	// OperationHostChange is a change of the host that root made with a registered action. Its entry names
	// the human behind sudo as the client and the action in Host, never an argument.
	OperationHostChange = "host.change"

	// The proposals of changes of the host: a submission, a withdrawal, a read of the list or of one proposal,
	// and root's approval, rejection and report of the result. The entry names the action, never the proposal,
	// its content or a reason.
	OperationProposalSubmit   = "proposal.submit"
	OperationProposalWithdraw = "proposal.withdraw"
	OperationProposalRead     = "proposal.read"
	OperationProposalApprove  = "proposal.approve"
	OperationProposalReject   = "proposal.reject"
	OperationProposalResult   = "proposal.result"
)

// knownOperations is every action Validate accepts, so a new one is added here.
var knownOperations = map[string]bool{
	OperationLogRead: true, OperationControlRequest: true, OperationControlRefuse: true, OperationControlStatus: true,
	OperationHostChange: true, OperationProposalSubmit: true, OperationProposalWithdraw: true, OperationProposalRead: true,
	OperationProposalApprove: true, OperationProposalReject: true, OperationProposalResult: true,
}

// KnownOperation reports whether action is one of the Operation constants.
func KnownOperation(action string) bool { return knownOperations[action] }
