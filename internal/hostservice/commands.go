package hostservice

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/host/auditlog"
)

// command is one command of the host service. action is the log action its requests are recorded under;
// rootOnly refuses every peer but root with control.CodeNotRoot before handle runs.
type command struct {
	action   string
	rootOnly bool
	handle   func(*call) (any, error)
}

// call is one request as a handler sees it. A handler that records a change of the host sets change, changeUID
// and changeCode; the entry then names that change and that user instead of the peer, and carries changeCode as
// its error code when it is set.
type call struct {
	args json.RawMessage

	change     *auditlog.Host
	changeUID  uint32
	changeCode string
}

func (s *server) registry() map[string]command {
	return map[string]command{
		control.StatusCommand:        {action: auditlog.OperationControlStatus, handle: s.status},
		control.HostLogListCommand:   {action: auditlog.OperationLogRead, handle: s.logList},
		control.HostLogVerifyCommand: {action: auditlog.OperationLogRead, handle: s.logVerify},
		// The sudo call that made the change is the proof, so only root may report one.
		control.HostRecordCommand: {action: auditlog.OperationHostChange, rootOnly: true, handle: hostRecord},
	}
}

func badRequest() *control.Error {
	return control.NewError(control.CodeBadRequest, "the arguments are not valid for this command")
}

func (s *server) status(*call) (any, error) {
	return StatusResult{Version: s.version, StartedAt: s.started}, nil
}

// maxLogActionFilter is the longest action filter. The filter is only checked for its shape, not against the
// known actions, so an action that older entries record and qhost no longer writes finds them.
const maxLogActionFilter = 64

// logQuery validates the arguments of log.list; anything unknown or malformed is a bad request.
func logQuery(a control.HostLogListArgs) (auditlog.Query, error) {
	q := auditlog.Query{Action: a.Action, Result: a.Result, After: a.After, Limit: a.Limit}
	for _, t := range []struct {
		text string
		dst  *time.Time
	}{{a.From, &q.From}, {a.To, &q.To}} {
		if t.text == "" {
			continue
		}
		v, err := time.Parse(time.RFC3339, t.text)
		if err != nil {
			return q, badRequest()
		}
		*t.dst = v
	}
	if !q.From.IsZero() && !q.To.IsZero() && q.From.After(q.To) {
		return q, badRequest()
	}
	switch {
	case q.Limit == 0:
		q.Limit = control.HostLogDefaultLimit
	case q.Limit < 0 || q.Limit > control.HostLogMaxLimit:
		return q, badRequest()
	}
	if len(q.Action) > maxLogActionFilter || !utf8.ValidString(q.Action) || strings.IndexFunc(q.Action, unicode.IsControl) >= 0 {
		return q, badRequest()
	}
	if q.Result != "" && q.Result != control.HostLogResultSuccess && q.Result != control.HostLogResultFailure {
		return q, badRequest()
	}
	return q, nil
}

// logList returns the parsed fields of entries only, never a raw line, and at most control.HostLogMaxLimit of
// them, which stays far below the size limit of an answer.
func (s *server) logList(c *call) (any, error) {
	var a control.HostLogListArgs
	if len(c.args) > 0 {
		if err := json.Unmarshal(c.args, &a); err != nil {
			return nil, badRequest()
		}
	}
	q, err := logQuery(a)
	if err != nil {
		return nil, err
	}
	page, err := auditlog.Read(s.logDir, q)
	if err != nil {
		if errors.Is(err, auditlog.ErrBadCursor) {
			return nil, badRequest()
		}
		return nil, fmt.Errorf("read the action log: %w", err)
	}
	out := control.HostLogListResult{Entries: make([]control.HostLogEntry, len(page.Records)), Next: page.Next}
	for i, r := range page.Records {
		e := r.Entry
		out.Entries[i] = control.HostLogEntry{
			Seq: r.Seq, Time: r.Time, ClientUID: e.ClientUID, Action: e.Action,
			Result: e.Result, ErrorCode: e.ErrorCode, DurationMS: e.DurationMS,
		}
		if h := e.Host; h != nil {
			out.Entries[i].Change = &control.HostLogChange{ActionID: h.ActionID, JournalID: h.JournalID, UndoOf: h.UndoOf}
		}
	}
	return out, nil
}

// logVerify checks the hash chain of every day file.
func (s *server) logVerify(*call) (any, error) {
	report, err := auditlog.Verify(s.logDir)
	if err != nil {
		return nil, fmt.Errorf("verify the action log: %w", err)
	}
	out := control.HostLogVerifyResult{Intact: !report.Broken, Days: make([]control.HostLogDay, 0, len(report.Days))}
	for _, d := range report.Days {
		out.Days = append(out.Days, control.HostLogDay{Date: d.Date, Entries: d.Entries, Intact: d.Problem == "", Damaged: d.Damaged})
		if d.Problem == "" {
			continue
		}
		if len(out.Problems) == control.HostLogVerifyMaxProblems {
			out.MoreProblems = true
			continue
		}
		out.Problems = append(out.Problems, control.HostLogProblem{Date: d.Date, Seq: d.ProblemSeq, Line: d.ProblemLine, Code: string(d.Problem)})
	}
	return out, nil
}

// The error codes the entry of a change of the host that did not stand carries.
const (
	codeChangeFailed   = "change-failed"
	codeChangeRestored = "change-restored"
)

// hostRecord turns a host.record request into the entry it leaves: the human behind sudo as its client, the
// change, and the error code of a change that did not stand. Everything it takes is a fixed word or an
// identifier checked against its form.
func hostRecord(c *call) (any, error) {
	var a control.HostRecordArgs
	if err := json.Unmarshal(c.args, &a); err != nil {
		return nil, badRequest()
	}
	change := auditlog.Host{ActionID: a.Action, JournalID: a.JournalID, UndoOf: a.UndoOf}
	if change.Validate() != nil {
		return nil, badRequest()
	}
	var code string
	switch a.Result {
	case control.HostResultApplied:
	case control.HostResultFailed:
		code = codeChangeFailed
	case control.HostResultRestored:
		code = codeChangeRestored
	default:
		return nil, badRequest()
	}
	// The request is served either way; the entry of a change that did not stand carries the code.
	c.change, c.changeUID, c.changeCode = &change, a.SudoUID, code
	return struct{}{}, nil
}
