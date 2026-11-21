package service

import (
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
