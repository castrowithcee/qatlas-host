// Package tools is the table of every function of qhost: its name, what it does to the host, what it reaches,
// who runs it and the schema of its arguments. A service command and a function the calling process runs
// itself stand in the same table, so a reader of the table needs no other source.
//
// The table describes; it does not decide. The service registries enforce class and scope on every request,
// and tests hold the table to them.
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/castrowithcee/qatlas-kit/jsonschema"
)

var (
	// ErrNotCallable refuses a function the table only describes: every function of class root and every entry
	// a service runs.
	ErrNotCallable = errors.New("the function cannot be called through the tool table")
	// ErrUnknownTool refuses a name that is not in the table.
	ErrUnknownTool = errors.New("unknown function")
	// ErrInvalidArguments refuses arguments that do not fit the schema of the entry.
	ErrInvalidArguments = errors.New("invalid arguments")
)

// Class says what a function does to the host. The values of read, propose and change are those of the
// protocol; root is a class of the table only: a function that needs root in the calling process.
type Class string

const (
	// ClassRead changes nothing and answers no secret value.
	ClassRead Class = "read"
	// ClassPropose only submits or withdraws a proposal of the caller's own.
	ClassPropose Class = "propose"
	// ClassChange is every other function a service runs.
	ClassChange Class = "change"
	// ClassRoot needs root in the process that runs it.
	ClassRoot Class = "root"
)

// Scope says what a function reaches.
type Scope string

const (
	ScopeHost   Scope = "host"
	ScopeDeploy Scope = "deploy"
	ScopeStack  Scope = "stack"
)

// Execution says who runs a function.
type Execution string

const (
	// ExecHostService marks a command of the host service.
	ExecHostService Execution = "host-service"
	// ExecDeployService marks a command of the deploy service.
	ExecDeployService Execution = "deploy-service"
	// ExecLocal marks a function the calling process runs itself.
	ExecLocal Execution = "local"
)

// Caller runs a local function with JSON arguments and answers its result as JSON.
type Caller func(ctx context.Context, args json.RawMessage) (json.RawMessage, error)

// Entry is one function of qhost.
type Entry struct {
	// Name is unique in the table: lower-case words joined by dots. The commands of the host service carry the
	// prefix "host." where their command name does not, because both services know commands of one name.
	Name  string
	Class Class
	Scope Scope
	Exec  Execution
	// Command is the name of the service command that runs the entry; empty for a local function.
	Command     string
	Description string
	// Schema checks the arguments; it is in the form the jsonschema package of the kit checks.
	Schema json.RawMessage
	// CLI lists the command lines, without the program name, whose work this entry describes.
	CLI []string
	// Call runs a local function of class read. Nil for every other entry.
	Call Caller
}

// All returns every entry, sorted by name. The slice is the caller's own.
func All() []Entry {
	out := slices.Clone(entries)
	slices.SortFunc(out, func(a, b Entry) int {
		switch {
		case a.Name < b.Name:
			return -1
		case a.Name > b.Name:
			return 1
		}
		return 0
	})
	return out
}

// Lookup returns the entry of a name.
func Lookup(name string) (Entry, bool) {
	for _, e := range entries {
		if e.Name == name {
			return e, true
		}
	}
	return Entry{}, false
}

// Register sets the call of a local function of class read. The packages that own the logic register at start
// (this package imports none of them); a wrong name or a second call is an error of the program and panics.
func Register(name string, call Caller) {
	for i := range entries {
		if entries[i].Name != name {
			continue
		}
		if entries[i].Exec != ExecLocal || entries[i].Class != ClassRead || entries[i].Call != nil || call == nil {
			panic("tools: cannot register a call for " + name)
		}
		entries[i].Call = call
		return
	}
	panic("tools: no entry " + name)
}

// Invoke runs the local read function of a name with JSON arguments, after the arguments passed the schema of
// the entry. The function itself keeps the preconditions of its command line, such as the administrator check.
func Invoke(ctx context.Context, name string, args json.RawMessage) (json.RawMessage, error) {
	e, ok := Lookup(name)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownTool, name)
	}
	if e.Class != ClassRead || e.Exec != ExecLocal || e.Call == nil {
		return nil, fmt.Errorf("%w: %s", ErrNotCallable, name)
	}
	if len(args) == 0 {
		args = json.RawMessage(`{}`)
	}
	if err := jsonschema.ValidateJSON(e.Schema, args); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidArguments, err)
	}
	return e.Call(ctx, args)
}

// schema builders: the table stays short and every schema is built the same way.

type props map[string]any

func object(required []string, p props) json.RawMessage {
	if p == nil {
		p = props{}
	}
	s := map[string]any{"type": "object", "properties": p, "additionalProperties": false}
	if len(required) > 0 {
		s["required"] = required
	}
	raw, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return raw
}

func str() map[string]any              { return map[string]any{"type": "string"} }
func integer() map[string]any          { return map[string]any{"type": "integer"} }
func boolean() map[string]any          { return map[string]any{"type": "boolean"} }
func anyObject() map[string]any        { return map[string]any{"type": "object"} }
func strList() map[string]any          { return map[string]any{"type": "array", "items": str()} }
func timestamp() map[string]any        { return map[string]any{"type": "string", "format": "date-time"} }
func oneOf(v ...string) map[string]any { return map[string]any{"type": "string", "enum": v} }

var none = object(nil, nil)
