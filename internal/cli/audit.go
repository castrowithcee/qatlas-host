package cli

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-kit/cli"
	"github.com/castrowithcee/qatlas-kit/cobracli"
	"github.com/castrowithcee/qatlas-kit/output"

	"github.com/castrowithcee/qatlas-host/internal/client"
	"github.com/castrowithcee/qatlas-host/internal/control"
)

func newAuditCommand(program *cobracli.Program, state *notRunning) *cobra.Command {
	var since, until, action, result string
	var limit int
	var target auditTarget
	cmd := &cobra.Command{
		Use: "audit [--service host|deploy] [--engine <engine>] [--since <time>] [--until <time>] [--action <action>] " +
			"[--result success|failure] [--limit <n>]",
		Short: "Show the action log of the qhost host service or of a deploy service, newest first",
		Long: "audit shows the action log that the host service keeps: one entry per request on its channel and per change of\n" +
			"the host, with time, the user ID of the client, the action, the result with its error code, and the duration.\n" +
			"An entry never holds an argument or a secret value. A change of the host names the person behind sudo, the\n" +
			"registered action and its journal entry. The log is a hash chain; 'qhost audit verify' checks it. It is\n" +
			"not the log of the service itself ('qhost service log') and not the protocol of a run ('qhost stack log').\n\n" +
			"--service deploy shows the log of a deploy service instead: one entry per request and per refusal, with time,\n" +
			"the user ID of the client, the command, its class and scope, the stack of a command for one stack, the\n" +
			"result with its error code, the duration and the origin (cli, tui, mcp or unknown) the client claimed, which\n" +
			"is for display only. --action then shows one command, such as stack.restart. --engine chooses the deploy\n" +
			"service: " + instanceRule + " Every deploy service keeps its own log, readable only by its service user. The\n" +
			"machine-readable output names the service and the engine.\n\n" +
			"--since and --until take a duration back from now (90s, 15m, 2h) or a time (2026-01-02T15:04:05Z) and are\n" +
			"inclusive. --action shows one action, such as log.read or host.change. --result shows success or failure\n" +
			fmt.Sprintf("only. --limit shows at most n entries (default %d, at most %d). ", control.HostLogDefaultLimit, control.HostLogMaxLimit) +
			"It changes nothing and needs no sudo for an\nadministrator in the group qhost.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if err := target.resolve(c); err != nil {
				return err
			}
			now := time.Now()
			from, err := parseLogTime("--since", since, now)
			if err != nil {
				return err
			}
			to, err := parseLogTime("--until", until, now)
			if err != nil {
				return err
			}
			if result != "" && result != control.HostLogResultSuccess && result != control.HostLogResultFailure {
				return &cli.Failure{Code: output.CodeInvalidRequest, Usage: true, Message: "--result must be success or failure",
					NextStep: "Fix the value and run the command again."}
			}
			if limit < 1 || limit > control.HostLogMaxLimit {
				return &cli.Failure{Code: output.CodeInvalidRequest, Usage: true,
					Message:  fmt.Sprintf("--limit must be between 1 and %d", control.HostLogMaxLimit),
					NextStep: "Use a smaller --limit, or narrow the entries with --since and --until."}
			}
			a := control.HostLogListArgs{Action: action, Result: result, Limit: limit}
			if !from.IsZero() {
				a.From = from.UTC().Format(time.RFC3339)
			}
			if !to.IsZero() {
				a.To = to.UTC().Format(time.RFC3339)
			}
			if target.deploy() {
				return auditDeployList(c, program, state, a)
			}
			var res control.HostLogListResult
			if err := hostCall(c, state, control.HostLogListCommand, a, &res); err != nil {
				return err
			}
			rows := make([]output.Row, 0, len(res.Entries))
			for _, e := range res.Entries {
				row := output.Row{
					"seq": int64(e.Seq), "time": e.Time.UTC().Format(time.RFC3339Nano), "action": e.Action,
					"result": e.Result, "duration_ms": e.DurationMS,
				}
				if e.ErrorCode != "" {
					row["error_code"] = e.ErrorCode
				}
				if e.ClientUID != nil {
					row["client_uid"] = int64(*e.ClientUID)
				}
				if ch := e.Change; ch != nil {
					text := ch.ActionID + " " + ch.JournalID
					if ch.ProposalID != "" {
						text += " proposal " + ch.ProposalID
					}
					if ch.UndoOf != "" {
						text += " undo of " + ch.UndoOf
					}
					row["change"] = text
				}
				rows = append(rows, row)
			}
			return output.Encode(c.OutOrStdout(), program.Format(), output.Collection{
				Columns: []string{"seq", "time", "client_uid", "action", "result", "error_code", "duration_ms", "change"}, Rows: rows})
		},
	}
	cmd.Flags().StringVar(&since, "since", "", "only entries from this time on: a duration back from now or an RFC 3339 time")
	cmd.Flags().StringVar(&until, "until", "", "only entries up to this time: a duration back from now or an RFC 3339 time")
	cmd.Flags().StringVar(&action, "action", "", "only entries of this action, such as log.read or host.change")
	cmd.Flags().StringVar(&result, "result", "", "only entries with this result: success or failure")
	cmd.Flags().IntVar(&limit, "limit", control.HostLogDefaultLimit, "show at most n entries")
	cmd.PersistentFlags().StringVar(&target.service, "service", auditServiceHost, "service whose log is shown: host or deploy")
	cmd.PersistentFlags().StringVar(&target.engine, "engine", "", instanceEngineUsage+"; only with --service deploy")
	cmd.AddCommand(newAuditVerifyCommand(program, state, &target))
	return cmd
}

func newAuditVerifyCommand(program *cobracli.Program, state *notRunning, target *auditTarget) *cobra.Command {
	return &cobra.Command{
		Use:   "verify",
		Short: "Check the hash chain of the action log, day by day",
		Long: "verify asks the host service to walk every day file of the action log and check that its entries chain\n" +
			"together without a change, a swap, or a deletion that no retention cut documents. It shows one row per day.\n" +
			"An unreadable line, such as one torn by a crash, that a marker entry right after it documents leaves the\n" +
			"chain intact and is counted as damaged. A broken chain ends with exit code 1 after the report. The chain\n" +
			"shows damage and gaps; whoever controls root or the service user can still rewrite the log consistently.\n" +
			"--service deploy checks the log of a deploy service; --engine chooses it as 'qhost audit' describes.\n" +
			"It changes nothing and needs no sudo for an administrator in the group qhost.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if err := target.resolve(c); err != nil {
				return err
			}
			var res control.HostLogVerifyResult
			if target.deploy() {
				if err := credentialCall(c, state, control.HostLogVerifyCommand, nil, &res); err != nil {
					return err
				}
			} else if err := hostCall(c, state, control.HostLogVerifyCommand, nil, &res); err != nil {
				return err
			}
			problems := map[string]control.HostLogProblem{}
			for _, p := range res.Problems {
				problems[p.Date] = p
			}
			rows := make([]output.Row, 0, len(res.Days))
			for _, d := range res.Days {
				row := output.Row{"day": d.Date, "entries": int64(d.Entries), "chain": "intact", "damaged": int64(d.Damaged)}
				if !d.Intact {
					row["chain"] = "broken"
					if p, ok := problems[d.Date]; ok {
						row["detail"] = problemDetail(p)
					}
				}
				target.label(c, row)
				rows = append(rows, row)
			}
			if err := output.Encode(c.OutOrStdout(), program.Format(), output.Collection{
				Columns: target.columns("day", "entries", "chain", "damaged", "detail"), Rows: rows}); err != nil {
				return err
			}
			if !res.Intact {
				return &cli.Failure{Code: output.CodeRuntime, Message: "the action log of the " + target.service + " service is broken",
					NextStep:      "Read the report above for the broken day; keep the log files as they are and restore them from a backup if needed.",
					AgentNextStep: "Tell the user which day is broken; do not repair or delete log files."}
			}
			return nil
		},
	}
}

// problemDetail is the sentence for the first problem of a day.
func problemDetail(p control.HostLogProblem) string {
	switch p.Code {
	case "unparsable-line":
		return fmt.Sprintf("line %d does not parse as a log entry", p.Line)
	case "bad-genesis":
		return fmt.Sprintf("the first entry (line %d) does not chain from the genesis hash", p.Line)
	case "missing-entries":
		return fmt.Sprintf("entries before sequence %d (line %d) are missing without a retention cut", p.Seq, p.Line)
	case "chain-broken":
		return fmt.Sprintf("entry with sequence %d (line %d) does not chain from the previous entry", p.Seq, p.Line)
	}
	return fmt.Sprintf("%s at line %d", p.Code, p.Line)
}

// hostCall calls the host service and turns a failure into a diagnostic with a next step.
func hostCall(c *cobra.Command, state *notRunning, command string, args, result any) error {
	err := call(c.Context(), hostEndpoint(), command, args, result)
	if err == nil {
		return nil
	}
	if errors.Is(err, client.ErrNotRunning) {
		state.seen = true
	}
	return hostFailure(err)
}

// hostFailure is failureOf for the host service.
func hostFailure(err error) error {
	var remote *control.Error
	switch {
	case errors.Is(err, client.ErrNotRunning):
		return &cli.Failure{Code: output.CodeUnreachable, Message: "the host service is not running",
			NextStep: "Start it with 'sudo systemctl start qhost', then run the command again.", Cause: err}
	case errors.As(err, &remote) && (remote.Code == control.CodeNotAdmin || remote.Code == control.CodeNoAccess):
		return &cli.Failure{Code: output.CodePermission, Message: "qhost is reserved for administrators (sudo)",
			NextStep: "Ask an administrator for access, or run the command as root.", Cause: err}
	case errors.As(err, &remote) && remote.Code == control.CodeBadRequest:
		return &cli.Failure{Code: output.CodeInvalidRequest, Message: remote.Message, Usage: true,
			NextStep: "Fix the argument and run the command again.", Cause: err}
	}
	return failureOf(err)
}

// The services whose action log qhost audit shows.
const (
	auditServiceHost   = "host"
	auditServiceDeploy = "deploy"
)

// auditTarget is the choice of --service and --engine.
type auditTarget struct{ service, engine string }

func (t *auditTarget) deploy() bool { return t.service == auditServiceDeploy }

// resolve checks the options and, for the deploy service, makes the instance --engine names the target of every
// later call. The host service has one log, so --engine without --service deploy is an error.
func (t *auditTarget) resolve(c *cobra.Command) error {
	switch t.service {
	case auditServiceHost:
		if t.engine != "" {
			return usageFailure("--engine needs --service deploy", "Add --service deploy, or leave --engine out.")
		}
		return nil
	case auditServiceDeploy:
		return routeInstance(c, t.engine)
	}
	return usageFailure("--service must be host or deploy", "Fix the value and run the command again.")
}

// label names the service and, for the deploy service, the engine in a row; the log of the host service has
// neither, so its output stays as it was.
func (t *auditTarget) label(c *cobra.Command, row output.Row) {
	if t.deploy() {
		row["service"], row["engine"] = auditServiceDeploy, targetOf(c.Context()).id()
	}
}

// columns puts the service and engine columns of the deploy service before cols.
func (t *auditTarget) columns(cols ...string) []string {
	if t.deploy() {
		return append([]string{"service", "engine"}, cols...)
	}
	return cols
}

// auditDeployList shows the entries of the action log of the chosen deploy service.
func auditDeployList(c *cobra.Command, program *cobracli.Program, state *notRunning, args control.HostLogListArgs) error {
	var res control.DeployLogListResult
	if err := credentialCall(c, state, control.HostLogListCommand, args, &res); err != nil {
		return err
	}
	target := auditTarget{service: auditServiceDeploy}
	rows := make([]output.Row, 0, len(res.Entries))
	for _, e := range res.Entries {
		row := output.Row{
			"seq": int64(e.Seq), "time": e.Time.UTC().Format(time.RFC3339Nano), "result": e.Result,
			"duration_ms": e.DurationMS, "origin": e.Origin,
		}
		for k, v := range map[string]string{"command": e.Command, "class": e.Class, "scope": e.Scope, "stack": e.Stack, "error_code": e.ErrorCode} {
			if v != "" {
				row[k] = v
			}
		}
		if e.ClientUID != nil {
			row["client_uid"] = int64(*e.ClientUID)
		}
		target.label(c, row)
		rows = append(rows, row)
	}
	return output.Encode(c.OutOrStdout(), program.Format(), output.Collection{
		Columns: target.columns("seq", "time", "client_uid", "command", "class", "scope", "stack", "result", "error_code", "duration_ms", "origin"),
		Rows:    rows})
}
