package control

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"
)

// Handler serves one command. It receives the raw arguments of the request, which it must treat as untrusted,
// and returns a value that is encoded as the result. An *Error becomes the answer's error as it is; any other
// error is answered as an internal error without its text.
type Handler func(ctx context.Context, args json.RawMessage) (any, error)

// Class says what a command does. The zero value marks nothing and is refused by Register.
type Class uint8

const (
	// ClassRead marks a command that changes nothing: no file, no state, no container, no remote, and nothing
	// leaves the host. Its answer holds no secret value. Root and the administrators of the service group may
	// run it.
	ClassRead Class = iota + 1
	// ClassPropose marks a command that only submits or withdraws a proposal of the caller. Its one effect is
	// the stored proposal of the caller's own, which changes nothing on the host. Root and the administrators
	// of the service group may run it.
	ClassPropose
	// ClassChange marks every other command. Only root may run it.
	ClassChange
)

func (c Class) String() string {
	switch c {
	case ClassRead:
		return "read"
	case ClassPropose:
		return "propose"
	case ClassChange:
		return "change"
	}
	return "unmarked"
}

// Scope says what a request reaches. The zero value marks nothing and is refused by Register.
type Scope uint8

const (
	// ScopeDeploy marks a request that concerns the whole instance.
	ScopeDeploy Scope = iota + 1
	// ScopeStack marks a request that concerns one stack, whose name is known before the command runs.
	ScopeStack
	// ScopeHost marks a request that concerns the host itself, which the host service serves.
	ScopeHost
)

func (s Scope) String() string {
	switch s {
	case ScopeDeploy:
		return "deploy"
	case ScopeStack:
		return "stack"
	case ScopeHost:
		return "host"
	}
	return "unmarked"
}

// StackFunc reads the names of the stacks a request names from its raw arguments, which are untrusted. It must
// not panic, and answers nothing for arguments it cannot read.
type StackFunc func(args json.RawMessage) []string

// Command is a registered command. Class and Scope are required. A command of scope ScopeStack needs Stack.
// Writes, allowed on a read command only, tells whether the arguments of one request ask for a side effect,
// such as a refresh from a remote; that request then needs ClassChange. It must answer true for arguments it
// cannot read.
type Command struct {
	Class  Class
	Scope  Scope
	Stack  StackFunc
	Handle Handler
	Writes func(args json.RawMessage) bool
}

// Access is what one request needs: its class, and its scope with the stack when the scope is ScopeStack.
type Access struct {
	Class Class
	Scope Scope
	Stack string
}

// AccessFor returns the access one request with these arguments needs. Anything but a plain read or a proposal
// is a change. A stack command whose arguments do not name exactly one stack is a request of scope ScopeDeploy.
func (c Command) AccessFor(args json.RawMessage) Access {
	class := ClassChange
	switch {
	case c.Class == ClassPropose:
		class = ClassPropose
	case c.Class == ClassRead && (c.Writes == nil || !c.Writes(args)):
		class = ClassRead
	}
	if c.Scope == ScopeHost {
		return Access{Class: class, Scope: ScopeHost}
	}
	if c.Scope == ScopeStack && c.Stack != nil {
		if names := c.Stack(args); len(names) == 1 && names[0] != "" {
			return Access{Class: class, Scope: ScopeStack, Stack: names[0]}
		}
	}
	return Access{Class: class, Scope: ScopeDeploy}
}

// Entry is one row of the table of a registry.
type Entry struct {
	Name  string
	Class Class
	Scope Scope
}

var commandName = regexp.MustCompile(`^[a-z][a-z0-9]*(\.[a-z][a-z0-9-]*)*$`)

// Registry maps command names to commands. Features register their commands before the service serves.
type Registry struct {
	mu       sync.RWMutex
	commands map[string]Command
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry { return &Registry{commands: map[string]Command{}} }

// Register adds a command. A name registers once, and must be lower-case words joined by dots. The command
// needs a handler, its class and its scope.
func (r *Registry) Register(name string, c Command) error {
	if !commandName.MatchString(name) || c.Handle == nil {
		return fmt.Errorf("invalid command registration %q", name)
	}
	if c.Class != ClassRead && c.Class != ClassPropose && c.Class != ClassChange {
		return fmt.Errorf("command %q is not marked as read, propose or change", name)
	}
	if c.Scope != ScopeDeploy && c.Scope != ScopeStack && c.Scope != ScopeHost {
		return fmt.Errorf("command %q is not marked with a scope", name)
	}
	if (c.Scope == ScopeStack) != (c.Stack != nil) {
		return fmt.Errorf("command %q needs a stack function exactly when its scope is stack", name)
	}
	if c.Writes != nil && c.Class != ClassRead {
		return fmt.Errorf("command %q is a change and cannot raise its class", name)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.commands[name]; ok {
		return fmt.Errorf("command %q is already registered", name)
	}
	r.commands[name] = c
	return nil
}

// Lookup returns a command.
func (r *Registry) Lookup(name string) (Command, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.commands[name]
	return c, ok
}

// Names returns the names of every registered command in order.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.commands))
	for name := range r.commands {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// Entries returns the table of the registry, one row per command, in the order of the names.
func (r *Registry) Entries() []Entry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entries := make([]Entry, 0, len(r.commands))
	for name, c := range r.commands {
		entries = append(entries, Entry{Name: name, Class: c.Class, Scope: c.Scope})
	}
	slices.SortFunc(entries, func(a, b Entry) int { return strings.Compare(a.Name, b.Name) })
	return entries
}

type peerKey struct{}

// WithPeer returns ctx carrying the user ID the kernel reported for the connection of the request, so a handler
// can tell whose request it serves.
func WithPeer(ctx context.Context, uid uint32) context.Context {
	return context.WithValue(ctx, peerKey{}, uid)
}

// PeerUID returns the user ID of the peer of the request a handler serves; false when the context carries none.
func PeerUID(ctx context.Context) (uint32, bool) {
	uid, ok := ctx.Value(peerKey{}).(uint32)
	return uid, ok
}
