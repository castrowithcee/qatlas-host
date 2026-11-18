package control

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"sync"
)

// Handler serves one command. It receives the raw arguments of the request, which it must treat as untrusted,
// and returns a value that is encoded as the result. An *Error becomes the answer's error as it is; any other
// error is answered as an internal error without its text.
type Handler func(ctx context.Context, args json.RawMessage) (any, error)

// Access says what a command does. The zero value marks nothing and is refused by Register.
type Access uint8

const (
	// AccessRead marks a command that changes nothing: no file, no state, no container, no remote, and nothing
	// leaves the host. Its answer holds no secret value. Root and the administrators of the service group may
	// run it.
	AccessRead Access = iota + 1
	// AccessWrite marks every other command. Only root may run it.
	AccessWrite
	// AccessPropose marks a command that only submits or withdraws a proposal of the caller. Its one effect is
	// the stored proposal of the caller's own, which changes nothing on the host. Root and the administrators
	// of the service group may run it.
	AccessPropose
)

func (a Access) String() string {
	switch a {
	case AccessRead:
		return "read"
	case AccessWrite:
		return "write"
	case AccessPropose:
		return "propose"
	}
	return "unmarked"
}

// Command is a registered command. Access is required. Writes, allowed on a read command only, tells whether
// the arguments of one request ask for a side effect, such as a refresh from a remote; that request then needs
// AccessWrite. It must answer true for arguments it cannot read.
type Command struct {
	Access Access
	Handle Handler
	Writes func(args json.RawMessage) bool
}

// AccessFor returns the access one request with these arguments needs; anything but a plain read or a proposal
// is a write.
func (c Command) AccessFor(args json.RawMessage) Access {
	if c.Access == AccessPropose {
		return AccessPropose
	}
	if c.Access == AccessRead && (c.Writes == nil || !c.Writes(args)) {
		return AccessRead
	}
	return AccessWrite
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
// needs a handler and its access.
func (r *Registry) Register(name string, c Command) error {
	if !commandName.MatchString(name) || c.Handle == nil {
		return fmt.Errorf("invalid command registration %q", name)
	}
	if c.Access != AccessRead && c.Access != AccessWrite && c.Access != AccessPropose {
		return fmt.Errorf("command %q is not marked as read, write or propose", name)
	}
	if c.Writes != nil && c.Access != AccessRead {
		return fmt.Errorf("command %q is a write and cannot raise its access", name)
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
