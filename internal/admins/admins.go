// Package admins decides who administers the host for both services: per the group database, a member of
// control.ServiceGroup and of AdminGroup. It reads the files on every call, so a revoked membership counts from
// the next call on. It knows no service.
package admins

import (
	"bufio"
	"cmp"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// AdminGroup is the group whose members are the administrators of the host.
const AdminGroup = "sudo"

// The group database a Database reads when it names none.
const (
	DefaultGroupFile  = "/etc/group"
	DefaultPasswdFile = "/etc/passwd"
)

// maxLine bounds one line of the group database; a longer line fails the check.
const maxLine = 1 << 20

// Database is the group database. The files are the whole truth: no NSS, so a user known only to LDAP or SSSD
// is no administrator. An empty path means the default file.
type Database struct {
	Group, Passwd string
}

// Admin reports whether the user uid belongs, per the database as it is now, to both control.ServiceGroup and
// AdminGroup, each as a supplementary or the primary group. Root is no exception here; callers decide about
// root. An unreadable database is an error, never access.
func (db Database) Admin(uid uint32) (bool, error) {
	group, err := os.Open(cmp.Or(db.Group, DefaultGroupFile))
	if err != nil {
		return false, err
	}
	defer group.Close()
	passwd, err := os.Open(cmp.Or(db.Passwd, DefaultPasswdFile))
	if err != nil {
		return false, err
	}
	defer passwd.Close()
	return InDatabase(group, passwd, uid)
}

type groupEntry struct {
	gid     string
	members []string
}

// InDatabase reports whether uid belongs to both control.ServiceGroup and AdminGroup in the group and passwd
// files read from group and passwd. A missing group, an unknown uid or an unreadable database yields false or
// an error, never access.
func InDatabase(group, passwd io.Reader, uid uint32) (bool, error) {
	groups := map[string]groupEntry{}
	sc := scanner(group)
	for sc.Scan() {
		f := strings.Split(sc.Text(), ":")
		if len(f) < 4 || f[0] != control.ServiceGroup && f[0] != AdminGroup {
			continue
		}
		if _, seen := groups[f[0]]; !seen { // the first entry of a name decides, as getgrnam does
			groups[f[0]] = groupEntry{gid: f[2], members: strings.Split(f[3], ",")}
		}
	}
	if err := sc.Err(); err != nil {
		return false, err
	}
	if len(groups) != 2 {
		return false, nil
	}
	want := strconv.FormatUint(uint64(uid), 10)
	sc = scanner(passwd)
	for sc.Scan() {
		f := strings.Split(sc.Text(), ":")
		if len(f) < 4 || f[2] != want {
			continue
		}
		// The first entry of the uid decides, as getpwuid does.
		member := func(g groupEntry) bool { return g.gid != "" && f[3] == g.gid || slices.Contains(g.members, f[0]) }
		return f[0] != "" && member(groups[control.ServiceGroup]) && member(groups[AdminGroup]), nil
	}
	return false, sc.Err()
}

func scanner(r io.Reader) *bufio.Scanner {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 4096), maxLine)
	return sc
}
