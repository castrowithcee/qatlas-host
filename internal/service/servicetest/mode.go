package servicetest

import (
	"os"
	"path/filepath"
)

// fileMode is the mode of a file the fake writes: a secrets.env is private, as an operator keeps it.
func fileMode(p string) os.FileMode {
	if filepath.Base(p) == "secrets.env" {
		return 0o600
	}
	return 0o644
}
