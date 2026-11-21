package service

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// Test hooks for the pure parts of the registry login lookup.
var (
	RegistryCredentialName = registryCredentialName
	ParseRegistryLogin     = parseRegistryLogin
)

// SeeRegistry makes Run hand its registry to f once every command is registered.
func SeeRegistry(cfg *Config, f func(*control.Registry)) { cfg.registryHook = f }

// SeeStackChecks makes Run tell passed the clock time each pass of the regular registry check began at, once
// the check waits for its next pass, and returns its CheckStack for use once Run has started.
func SeeStackChecks(cfg *Config, passed func(time.Time)) func(ctx context.Context, name string) {
	var p atomic.Pointer[pollService]
	cfg.pollHook = func(s *pollService) {
		s.passed = passed
		p.Store(s)
	}
	return func(ctx context.Context, name string) { p.Load().CheckStack(ctx, name) }
}
