package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/deploylog"
	"github.com/castrowithcee/qatlas-host/internal/logview"
)

// registerAuditCommands registers log.list and log.verify, which read the action log in dir. Both are plain
// reads of the whole instance; the log holds no argument and no secret, and no raw line leaves the service.
func registerAuditCommands(r *control.Registry, dir string) error {
	if err := r.Register(control.HostLogListCommand, reading(func(_ context.Context, args json.RawMessage) (any, error) {
		return auditList(dir, args)
	})); err != nil {
		return err
	}
	return r.Register(control.HostLogVerifyCommand, reading(func(context.Context, json.RawMessage) (any, error) {
		report, err := deploylog.Verify(dir)
		if err != nil {
			return nil, fmt.Errorf("verify the action log: %w", err)
		}
		return logview.VerifyResult(report), nil
	}))
}

func auditList(dir string, args json.RawMessage) (any, error) {
	var a control.HostLogListArgs
	if len(args) > 0 {
		if err := json.Unmarshal(args, &a); err != nil {
			return nil, control.NewError(control.CodeBadRequest, "the arguments are not valid for this command")
		}
	}
	f, err := logview.ParseFilter(a)
	if err != nil {
		return nil, err
	}
	page, err := deploylog.Read(dir, deploylog.Query{From: f.From, To: f.To, Command: f.Action, Result: f.Result, Limit: f.Limit, After: f.After})
	if err != nil {
		if errors.Is(err, deploylog.ErrBadCursor) {
			return nil, control.NewError(control.CodeBadRequest, "the arguments are not valid for this command")
		}
		return nil, fmt.Errorf("read the action log: %w", err)
	}
	out := control.DeployLogListResult{Entries: make([]control.DeployLogEntry, len(page.Records)), Next: page.Next}
	for i, r := range page.Records {
		e := r.Entry
		out.Entries[i] = control.DeployLogEntry{
			Seq: r.Seq, Time: r.Time, ClientUID: e.ClientUID, Command: e.Command, Class: e.Class, Scope: e.Scope,
			Stack: e.Stack, Result: e.Result, ErrorCode: e.ErrorCode, DurationMS: e.DurationMS, Origin: e.Origin, Grant: e.Grant,
		}
	}
	return out, nil
}
