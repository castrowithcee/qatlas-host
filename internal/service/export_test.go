package service

import "github.com/castrowithcee/qatlas-host/internal/control"

// Test hooks for the pure parts of the registry login lookup.
var (
	RegistryCredentialName = registryCredentialName
	ParseRegistryLogin     = parseRegistryLogin
)

// SeeRegistry makes Run hand its registry to f once every command is registered.
func SeeRegistry(cfg *Config, f func(*control.Registry)) { cfg.registryHook = f }
