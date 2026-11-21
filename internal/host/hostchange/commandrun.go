package hostchange

import (
	"context"
	"fmt"
	"time"
)

// ActionProposalRun is the action of the journal entry of a proposal of free commands that root ran.
const ActionProposalRun = "proposal.run"

// notReversibleCommands is the reason the journal gives for the entry of free commands.
const notReversibleCommands = "free commands are not checked and nothing records what they changed"

// CommandRun is a proposal of free commands that root runs.
type CommandRun struct {
	ProposalID string
	Commands   []string
	Timeout    time.Duration
	// SudoUID is the human behind the sudo call, 0 when unknown.
	SudoUID int
	// SubmitterUID and SubmitterName name the user who submitted the proposal; the journal keeps them.
	SubmitterUID  int
	SubmitterName string
}

// RunCommands writes the journal entry first, runs the commands in order through sh and stops at the first one
// that does not exit with 0, then completes the entry with the exit codes and hands the run to the audit log
// without the commands. The commands that did not run come back with Ran false. scrub removes known secrets from
// everything that is kept. An error means nothing ran.
func (r *Runner) RunCommands(ctx context.Context, sh ShellExecutor, run CommandRun, scrub func(string) string) (Result, []ShellResult, error) {
	unlock, err := r.Journal.Lock()
	if err != nil {
		return Result{}, nil, err
	}
	defer unlock()
	now := r.Now().UTC()
	id, err := newID(now)
	if err != nil {
		return Result{}, nil, err
	}
	kept := make([]string, len(run.Commands))
	for i, c := range run.Commands {
		kept[i] = scrub(c)
	}
	e := Entry{
		ID: id, Time: now, Action: ActionProposalRun, Args: []string{run.ProposalID},
		Sentence: fmt.Sprintf("Run %d unchecked commands of a proposal.", len(run.Commands)),
		Commands: kept, Result: ResultPending, SudoUID: run.SudoUID, Reversible: false,
		NotReversible: notReversibleCommands, Check: CheckNone,
		SubmitterUID: run.SubmitterUID, SubmitterName: run.SubmitterName,
	}
	if err := r.Journal.Write(e); err != nil {
		return Result{}, nil, fmt.Errorf("write the journal: %w", err)
	}
	results := make([]ShellResult, len(run.Commands))
	failed := -1
	for i, c := range run.Commands {
		if failed >= 0 {
			results[i] = ShellResult{Command: c, ExitCode: -1}
			continue
		}
		results[i] = sh.RunShell(ctx, c, run.Timeout)
		results[i].Output = scrub(results[i].Output)
		e.Exits = append(e.Exits, results[i].ExitCode)
		if !results[i].Succeeded() {
			failed = i
		}
	}
	e.Result = ResultApplied
	if failed >= 0 {
		e.Result = ResultFailed
		e.Error = fmt.Sprintf("command %d ended with exit code %d", failed+1, results[failed].ExitCode)
		if results[failed].TimedOut {
			e.Error = fmt.Sprintf("command %d ran out of time", failed+1)
		}
	}
	res := Result{Entry: e}
	if err := r.Journal.Write(e); err != nil {
		res.Warning = joinWarning(res.Warning, "the journal entry could not be completed: "+err.Error())
	}
	if err := r.Journal.Prune(KeepEntries); err != nil {
		res.Warning = joinWarning(res.Warning, "the journal could not be pruned: "+err.Error())
	}
	if r.Record != nil {
		audit := AuditEntry{ID: e.ID, Time: e.Time, Action: e.Action, Sentence: e.Sentence, Result: e.Result,
			SudoUID: e.SudoUID, ProposalID: run.ProposalID}
		if err := r.Record(audit); err != nil {
			res.Warning = joinWarning(res.Warning, "the audit entry is missing: "+scrub(err.Error()))
		}
	}
	return res, results, nil
}
