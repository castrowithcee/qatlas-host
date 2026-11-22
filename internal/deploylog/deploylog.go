// Package deploylog is the action log of a deploy service: one entry per request on its channel, with the user,
// the command, its class and scope, the stack, the result and the duration. It is the kit's auditlog with an
// entry type of its own, because a deploy request names a command with a class and a stack where the host
// service logs a fixed action. An entry never holds an argument, a result payload or a secret.
package deploylog

import (
	"errors"
	"fmt"
	"regexp"
	"time"

	kit "github.com/castrowithcee/qatlas-kit/auditlog"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/formatnum"
)

const (
	// RetentionDays is how long a day file is kept; the host service keeps its log as long.
	RetentionDays = 90
	// EntryFormat is the format number every entry carries as "format". Append refuses an entry with
	// another number.
	EntryFormat = 1
	// DirName is the folder of the log inside the state directory of the service.
	DirName = "audit"

	lockName = ".lock"
)

// The two results an entry records.
const (
	ResultSuccess = "success"
	ResultFailure = "failure"
)

// The kit types callers work with.
type (
	// Record is one parsed line: its sequence number, time and Entry.
	Record = kit.Record[Entry]
	// Report is the result of Verify.
	Report = kit.Report
)

// ErrBadCursor reports a Query.After that is not a value Read handed out.
var ErrBadCursor = kit.ErrBadCursor

// Entry is the part of a log line the deploy service owns, in the order it is written.
type Entry struct {
	Format int `json:"format"`
	// ClientUID is the Unix user ID of the peer; a pointer because 0 is root.
	ClientUID *uint32 `json:"client_uid,omitempty"`
	// Command is a registered command; empty for a peer refused before its first request and for a command
	// that is not registered, whose name is untrusted text.
	Command    string `json:"command,omitempty"`
	Class      string `json:"class,omitempty"`
	Scope      string `json:"scope,omitempty"`
	Stack      string `json:"stack,omitempty"`
	Result     string `json:"result,omitempty"`
	ErrorCode  string `json:"error_code,omitempty"`
	DurationMS int64  `json:"duration_ms,omitempty"`
	// Origin is what the client claimed and is display only: one of the control.Origin values.
	Origin string `json:"origin,omitempty"`
}

// Fields is what the server supplies for one finished request; the log adds the format number, the sequence
// number, the time and the chain.
type Fields struct {
	ClientUID uint32
	Command   string
	Class     control.Class
	Scope     control.Scope
	Stack     string
	// ErrorCode is empty for success.
	ErrorCode string
	Duration  time.Duration
	// Origin is normalized by the log.
	Origin string
}

func (f Fields) entry() Entry {
	uid := f.ClientUID
	e := Entry{
		Format: EntryFormat, ClientUID: &uid, Command: f.Command, Stack: f.Stack, ErrorCode: f.ErrorCode,
		Result: ResultSuccess, DurationMS: f.Duration.Milliseconds(), Origin: control.NormalizeOrigin(f.Origin),
	}
	if f.Class != 0 {
		e.Class = f.Class.String()
	}
	if f.Scope != 0 {
		e.Scope = f.Scope.String()
	}
	if f.ErrorCode != "" {
		e.Result = ResultFailure
	}
	return e
}

// MaxDuration is the longest duration Validate accepts for one request.
const MaxDuration = 30 * 24 * time.Hour

var (
	commandPattern   = regexp.MustCompile(`^[a-z][a-z0-9]*(\.[a-z][a-z0-9-]*)*$`)
	stackPattern     = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
	errorCodePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)
)

// maxNameLength bounds a command name and a stack name.
const maxNameLength = 64

// ValidStack reports whether name has the form of a stack name; the entry of any other is written without one,
// so a request cannot place free text in the log through the stack field.
func ValidStack(name string) bool {
	return len(name) <= maxNameLength && stackPattern.MatchString(name)
}

// Validate reports whether e is something the deploy service could log. The error names the field, never its
// value.
func (e Entry) Validate() error {
	switch {
	case e.Format != EntryFormat:
		return errors.New("format number is not supported")
	case e.Command != "" && (len(e.Command) > maxNameLength || !commandPattern.MatchString(e.Command)):
		return errors.New("command has an invalid form")
	case e.Class != "" && e.Class != control.ClassRead.String() && e.Class != control.ClassPropose.String() && e.Class != control.ClassChange.String():
		return errors.New("class is unknown")
	case e.Scope != "" && e.Scope != control.ScopeDeploy.String() && e.Scope != control.ScopeStack.String():
		return errors.New("scope is unknown")
	case e.Stack != "" && !ValidStack(e.Stack):
		return errors.New("stack has an invalid form")
	case e.Result == ResultSuccess && e.ErrorCode != "":
		return errors.New("a successful result has no error code")
	case e.Result == ResultFailure && !errorCodePattern.MatchString(e.ErrorCode):
		return errors.New("a failed result needs an error code of lowercase words with dashes")
	case e.Result != ResultSuccess && e.Result != ResultFailure:
		return errors.New("result must be success or failure")
	case e.Origin != control.NormalizeOrigin(e.Origin):
		return errors.New("origin is not a known value")
	case e.DurationMS < 0 || e.DurationMS > MaxDuration.Milliseconds():
		return errors.New("duration is out of range")
	}
	return nil
}

// Logger appends entries to the day files of one directory.
type Logger struct{ log *kit.Log[Entry] }

// Open returns a logger for dir. It touches no file: the directory and its files are created on the first
// Append. A line that does not parse is documented by a marker instead of refusing the next entry.
func Open(dir string) (*Logger, error) {
	log, err := newLog(dir, nil)
	if err != nil {
		return nil, err
	}
	return &Logger{log: log}, nil
}

func newLog(dir string, now func() time.Time) (*kit.Log[Entry], error) {
	log, err := kit.New(kit.Options[Entry]{
		Dir: dir, LockName: lockName, Validate: Entry.Validate,
		RetentionDays: RetentionDays, TornLine: kit.TornLineDocument, Now: now,
	})
	if err != nil {
		return nil, fmt.Errorf("open the deploy action log: %w", err)
	}
	return log, nil
}

// Append writes one entry for a finished request. A returned error means the entry might not be durable; the
// caller reports it and never changes the result of the request.
func (l *Logger) Append(f Fields) error { return l.log.Append(f.entry()) }

// Query selects the entries Read returns. Zero values leave a filter open; Limit must be positive. Command
// matches the command of an entry.
type Query struct {
	From, To time.Time
	Command  string
	Result   string
	Limit    int
	// After is the Next value of the previous Page; empty starts at the newest entry.
	After string
}

// Page is one slice of the matching entries, newest first. Next is non-empty when more match.
type Page struct {
	Records []Record
	Next    string
}

// Read returns the entries of dir that match q, newest first.
func Read(dir string, q Query) (Page, error) {
	log, err := newLog(dir, nil)
	if err != nil {
		return Page{}, err
	}
	page, err := log.Read(kit.Query[Entry]{
		From: q.From, To: q.To, Limit: q.Limit, After: q.After,
		Match: func(r Record) bool {
			return (q.Command == "" || r.Entry.Command == q.Command) && (q.Result == "" || r.Entry.Result == q.Result)
		},
	})
	if err != nil {
		return Page{}, err
	}
	for _, r := range page.Records {
		if err := checkFormat(r.Entry); err != nil {
			return Page{}, err
		}
	}
	return Page{Records: page.Records, Next: page.Next}, nil
}

// checkFormat refuses a line whose number this version does not read, a missing one included.
func checkFormat(e Entry) error {
	return formatnum.Check("deploy action log", "format", e.Format, EntryFormat)
}

// Verify checks the hash chain of the day files of dir. No key is held, so the day counts as unchecked for
// signatures.
func Verify(dir string) (Report, error) {
	log, err := newLog(dir, nil)
	if err != nil {
		return Report{}, err
	}
	report, err := log.Verify(nil)
	if err != nil {
		return Report{}, err
	}
	// The kit validates only on append, so the numbers of the lines on disk are checked here.
	days, err := log.VerifyLines(nil, "", "")
	if err != nil {
		return Report{}, err
	}
	for _, d := range days {
		for _, l := range d.Lines {
			if l.Parsed && l.Record.Kind == "" {
				if err := checkFormat(l.Record.Entry); err != nil {
					return Report{}, err
				}
			}
		}
	}
	return report, nil
}
