package service

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/registrylogin"
)

// Test hooks for the pure parts of the registry login lookup.
var (
	RegistryCredentialName = registrylogin.Name
	ParseRegistryLogin     = registrylogin.Parse
)

// RegistryCredentialFrom exposes the lookup the update check uses, to show it resolves through registrylogin.
var RegistryCredentialFrom = registryCredentialFrom

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

// RunAsRoot makes the service judge write access as the root service of the system engine for one test.
func RunAsRoot(t interface{ Cleanup(func()) }) {
	old := runsAsRoot
	runsAsRoot = func() bool { return true }
	t.Cleanup(func() { runsAsRoot = old })
}

// GitUnsafeAnswer exposes the answer to an error of an unsafe .git.
func GitUnsafeAnswer(err error) error { return gitUnsafeAnswer(err) }

// ChangesAnswer exposes the answer of the change commands.
func ChangesAnswer(err error) error { return changesAnswer(err) }
