package hostservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/host/auditlog"
	"github.com/castrowithcee/qatlas-host/internal/logview"
	"github.com/castrowithcee/qatlas-host/internal/proposals"
)

// call is one request as a handler sees it. A handler that records a change of the host sets change, changeUID
// and changeCode; the entry then names that change and that user instead of the peer, and carries changeCode as
// its error code when it is set.
type call struct {
	// uid is the peer of the request.
	uid  uint32
	args json.RawMessage

	change     *auditlog.Host
	changeUID  uint32
	changeCode string
}

type callKey struct{}

// withCall returns ctx carrying c, so a handler of the registry can reach the request it serves.
func withCall(ctx context.Context, c *call) context.Context {
	return context.WithValue(ctx, callKey{}, c)
}

// handler adapts h to a registry handler; the request comes from the context dispatch built.
func handler(h func(*call) (any, error)) control.Handler {
	return func(ctx context.Context, _ json.RawMessage) (any, error) {
		c, _ := ctx.Value(callKey{}).(*call)
		if c == nil {
			return nil, errors.New("no request in the context")
		}
		return h(c)
	}
}

// registry returns the commands of the host service, each with its class and scope host, and the log action
// each one's requests are recorded under.
func (s *server) registry(store *proposals.Store) (*control.Registry, map[string]string, error) {
	reg := control.NewRegistry()
	actions := map[string]string{}
	add := func(name, action string, class control.Class, h control.Handler) error {
		actions[name] = action
		return reg.Register(name, control.Command{Class: class, Scope: control.ScopeHost, Handle: h})
	}
	for _, c := range []struct {
		name, action string
		class        control.Class
		handle       func(*call) (any, error)
	}{
		{control.StatusCommand, auditlog.OperationControlStatus, control.ClassRead, s.status},
		{control.HostLogListCommand, auditlog.OperationLogRead, control.ClassRead, s.logList},
		{control.HostLogVerifyCommand, auditlog.OperationLogRead, control.ClassRead, s.logVerify},
		// The sudo call that made the change is the proof, so only root may report one.
		{control.HostRecordCommand, auditlog.OperationHostChange, control.ClassChange, hostRecord},
		{control.HostCommandsSetCommand, auditlog.OperationProposalCommands, control.ClassChange, s.commandsSet},
		{control.HostCommandsStatusCommand, auditlog.OperationProposalRead, control.ClassRead, s.commandsStatus},
	} {
		if err := add(c.name, c.action, c.class, handler(c.handle)); err != nil {
			return nil, nil, err
		}
	}
	if err := proposalCommands(add, store, s.commandSwitch.Enabled); err != nil {
		return nil, nil, err
	}
	return reg, actions, nil
}

func badRequest() *control.Error {
	return control.NewError(control.CodeBadRequest, "the arguments are not valid for this command")
}

func (s *server) status(*call) (any, error) {
	return StatusResult{Version: s.version, StartedAt: s.started}, nil
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
	f, err := logview.ParseFilter(a)
	if err != nil {
		return nil, err
	}
	q := auditlog.Query{From: f.From, To: f.To, Action: f.Action, Result: f.Result, Limit: f.Limit, After: f.After}
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
			out.Entries[i].Change = &control.HostLogChange{ActionID: h.ActionID, JournalID: h.JournalID, UndoOf: h.UndoOf, ProposalID: h.ProposalID}
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
	return logview.VerifyResult(report), nil
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
	change := auditlog.Host{ActionID: a.Action, JournalID: a.JournalID, UndoOf: a.UndoOf, ProposalID: a.ProposalID}
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
