package tui

import "time"

// HookServiceInfo is the state of the webhook receiver. Without SetUp nothing else is known; Problem says why
// the configuration of a receiver that is set up could not be read.
type HookServiceInfo struct {
	SetUp   bool
	Running bool
	Listen  string
	Hooks   int
	Problem string
}

// HookInfo is one hook of a stack. Created is zero for a hook added before the time was kept.
type HookInfo struct {
	ID      string
	Created time.Time
}

// HookListResult is the hooks of a stack. Without SetUp there is no receiver and so no hook.
type HookListResult struct {
	SetUp bool
	Hooks []HookInfo
}

// HookAddResult is a new hook. Path holds its secret and must not be kept beyond the one dialog that shows it.
type HookAddResult struct {
	ID     string
	Listen string
	Path   string
}

// HookSetupResult is what a setup of the webhook receiver reports.
type HookSetupResult struct {
	Listen string
}
