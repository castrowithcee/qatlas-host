package tui

import (
	"fmt"
	"strings"

	kittui "github.com/castrowithcee/qatlas-kit/tui"

	"github.com/castrowithcee/qatlas-host/internal/host/hostadmin"
)

// userDetailLines describe the marked user below the list. A key is never shown, only its fingerprint.
func (m *Model) userDetailLines(u hostadmin.User, width int) []string {
	t := m.text
	line := func(id kittui.TextID, args ...any) string {
		return m.line(detailIndent, kittui.StyleText, t.Lookup(id, args...), width)
	}
	access := t.Lookup(textAccessNone)
	switch {
	case u.Admin && u.Enabled:
		access = t.Lookup(textAccessAdmin)
	case u.Admin:
		access = t.Lookup(textAccessAdminNoQhost)
	}
	groups := strings.Join(furtherGroupsOf(u), ", ")
	if groups == "" {
		groups = t.Lookup(textNoValue)
	}
	lines := []string{
		line(textDetailHome, u.Home),
		line(textDetailPrimary, u.PrimaryGroup),
		line(textDetailGroups, groups),
		line(textDetailAccess, access),
		line(textDetailLogin, m.loginText(u)),
	}
	switch {
	case u.SSHKeys == nil:
		lines = append(lines, m.line(detailIndent, kittui.StyleHint, t.Lookup(textDetailKeysUnknown), width))
	case len(u.SSHKeys) == 0:
		lines = append(lines, line(textDetailKeys, t.Lookup(textNoValue)))
	default:
		var fps []string
		for _, k := range u.SSHKeys {
			fps = append(fps, k.Fingerprint)
		}
		lines = append(lines, line(textDetailKeys, fmt.Sprintf("%d (%s)", len(u.SSHKeys), strings.Join(fps, ", "))))
	}
	return lines
}
