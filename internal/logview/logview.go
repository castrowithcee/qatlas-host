// Package logview is what the host service and the deploy services share of their log.list and log.verify
// commands: the checks of the filters and the shape of the verify answer.
package logview

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	kit "github.com/castrowithcee/qatlas-kit/auditlog"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// maxActionFilter is the longest action filter. The filter is only checked for its shape, not against the
// known actions, so an action that older entries record and qhost no longer writes finds them.
const maxActionFilter = 64

// Filter is the checked form of control.HostLogListArgs. Zero times leave a bound open; Limit is positive.
type Filter struct {
	From, To time.Time
	Action   string
	Result   string
	Limit    int
	After    string
}

func badRequest() *control.Error {
	return control.NewError(control.CodeBadRequest, "the arguments are not valid for this command")
}

// ParseFilter validates the arguments of log.list; anything unknown or malformed is a bad request.
func ParseFilter(a control.HostLogListArgs) (Filter, error) {
	q := Filter{Action: a.Action, Result: a.Result, After: a.After, Limit: a.Limit}
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
	if len(q.Action) > maxActionFilter || !utf8.ValidString(q.Action) || strings.IndexFunc(q.Action, unicode.IsControl) >= 0 {
		return q, badRequest()
	}
	if q.Result != "" && q.Result != control.HostLogResultSuccess && q.Result != control.HostLogResultFailure {
		return q, badRequest()
	}
	return q, nil
}

// VerifyResult turns the report of a chain check into the answer of log.verify.
func VerifyResult(report kit.Report) control.HostLogVerifyResult {
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
	return out
}
