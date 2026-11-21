//go:build e2efakes

package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// runHTPasswd prints an htpasswd line with a bcrypt hash for the user in the first argument and the password on
// standard input. The registry image has no htpasswd program, and the image build has no network.
func runHTPasswd() error {
	if len(os.Args) != 3 {
		return fmt.Errorf("usage: fakes htpasswd <user> (password on standard input)")
	}
	user := os.Args[2]
	if user == "" || strings.ContainsAny(user, ":\n") {
		return fmt.Errorf("invalid user name")
	}
	password, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}
	if len(password) == 0 {
		return fmt.Errorf("empty password")
	}
	hash, err := bcrypt.GenerateFromPassword(password, bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	fmt.Printf("%s:%s\n", user, hash)
	return nil
}
