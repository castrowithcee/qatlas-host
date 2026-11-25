package control

// The commands that take over the image versions of one stack (the source) for another (the target). A deploy
// service sees only its own stacks, so the source is read at the service that has it with PromoteSourceCommand
// and its answer is handed to the service of the target, which applies every rule of an update by hand to the
// target itself: a handed-over source never gives more power than 'update.run' with a version. The target's
// service is the one the commands below reach.
//
// PromoteCheckCommand changes nothing: it refuses as PromoteRunCommand would and tells which versions the
// promotion sets. PromoteRunCommand runs the promotion as a secured run; it is bound to the source commit of
// the check and refuses with CodeNotUpdatable when the source has deployed another commit since.
const (
	PromoteSourceCommand = "promote.source"
	PromoteCheckCommand  = "promote.check"
	PromoteRunCommand    = "promote.run"
)

// Bounds of a handed-over PromoteSource.
const (
	MaxPromoteVersions = 256
	MaxPromoteText     = 512
)

// PromoteSourceArgs are the arguments of PromoteSourceCommand.
type PromoteSourceArgs struct {
	Stack string `json:"stack"`
}

// PromoteVersion is one image of the source and the version it has at the source's deployed commit.
type PromoteVersion struct {
	Variable string `json:"variable"`
	Image    string `json:"image"`
	Version  string `json:"version"`
}

// PromoteSource is what the target's service needs to know of the source stack; PromoteSourceCommand answers
// it. Commit is the commit the source deployed last and the versions were read at. LastResult is the result
// of its last run. Unhealthy is set when the source has a failure, a halted repair, or is paused or stopped.
// Observe is the observation time of the source, set while it runs.
type PromoteSource struct {
	Stack        string           `json:"stack"`
	Commit       string           `json:"commit"`
	LastResult   string           `json:"last_result,omitempty"`
	FailedCommit string           `json:"failed_commit,omitempty"`
	Unhealthy    bool             `json:"unhealthy,omitempty"`
	Observe      *ObserveInfo     `json:"observe,omitempty"`
	Versions     []PromoteVersion `json:"versions"`
}

// PromoteArgs are the arguments of PromoteCheckCommand and PromoteRunCommand. Stack is the target. Source
// is the answer of PromoteSourceCommand. Commit is for PromoteRunCommand only: the source commit of the check
// the run follows.
type PromoteArgs struct {
	Stack  string        `json:"stack"`
	Source PromoteSource `json:"source"`
	Commit string        `json:"commit,omitempty"`
}

// PromoteStepInfo is one variable of the target the promotion sets, from the value it has to the source's.
type PromoteStepInfo struct {
	Variable string `json:"variable"`
	From     string `json:"from"`
	To       string `json:"to"`
}

// PromoteCheckResult answers PromoteCheckCommand. Steps is empty when nothing is to do. Unchanged are the
// variables of the target that already have the source's version, Kept those whose image the source does
// not have. Observe is the source's running observation time, nil when none runs.
type PromoteCheckResult struct {
	Stack  string `json:"stack"`
	Source string `json:"source"`
	// SourceCommit is the commit of the source the versions were read at.
	SourceCommit string            `json:"source_commit"`
	Steps        []PromoteStepInfo `json:"steps"`
	Unchanged    []string          `json:"unchanged"`
	Kept         []string          `json:"kept"`
	Observe      *ObserveInfo      `json:"observe,omitempty"`
}

// PromoteRunResult answers PromoteRunCommand. Without steps nothing was to do and no run was made. Run is the
// id of the run, Result a run result and Commit the pushed commit, or the deployed one when the run failed
// before its push.
type PromoteRunResult struct {
	PromoteCheckResult
	Run    string `json:"run,omitempty"`
	Commit string `json:"commit,omitempty"`
	Result string `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}
