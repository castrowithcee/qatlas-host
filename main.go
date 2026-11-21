package main

import (
	"os"
	"path/filepath"

	"github.com/castrowithcee/qatlas-host/internal/cli"
	"github.com/castrowithcee/qatlas-host/internal/credhelper"
)

func main() {
	if filepath.Base(os.Args[0]) == credhelper.Name {
		os.Exit(credhelper.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, os.Getenv))
	}
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
