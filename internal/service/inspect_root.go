package service

import (
	"os"
	"syscall"
)

// runsAsRoot says the service is the deploy service of the system engine. Root writes a stack folder whatever
// its owner, group and mode; a folder its sandbox does not list yet is read-only only until source add lists it.
var runsAsRoot = func() bool { return os.Geteuid() == 0 }

// serviceWrites says the service may create files in the folder at path once it is a stack folder.
func serviceWrites(path string) bool {
	return runsAsRoot() || syscall.Access(path, accessWriteSearch) == nil
}
