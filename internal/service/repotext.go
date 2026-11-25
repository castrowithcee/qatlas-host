package service

import (
	"errors"
	"strings"
	"unicode"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/gitrepo"
)

// gitFailure turns an error of the git layer into a fixed-class answer. Text that came from git or the remote
// is reduced to printable characters and a short length.
func gitFailure(err error) error {
	text := printable(err.Error())
	lower := strings.ToLower(text)
	if strings.Contains(lower, "host key verification failed") || strings.Contains(lower, "host identification has changed") {
		return control.NewError(control.CodeHostKey,
			"the server's host key does not match the pinned key; the connection was refused")
	}
	if errors.Is(err, gitrepo.ErrNotEmpty) {
		return control.NewError(control.CodeInvalid, "the directory is not empty")
	}
	return control.NewError(control.CodeRemote, text)
}

func printable(s string) string {
	var b strings.Builder
	n := 0
	for _, r := range s {
		if n == maxErrorChars {
			b.WriteString("...")
			break
		}
		if unicode.IsPrint(r) {
			b.WriteRune(r)
		} else {
			b.WriteByte(' ')
		}
		n++
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
