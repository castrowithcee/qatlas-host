package service

import (
	"bufio"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// adminGroup is the group whose members are the administrators of the host.
const adminGroup = "sudo"

// The group database the access check reads when Config names none.
const (
	defaultGroupFile  = "/etc/group"
	defaultPasswdFile = "/etc/passwd"
)

// maxDatabaseLine bounds one line of the group database; a longer line fails the check.
const maxDatabaseLine = 1 << 20

// groupDB is the group database the access check reads on every request, so a revoked membership counts from
// the next request on. The files are the whole truth: no NSS, so a user known only to LDAP or SSSD has no access.
type groupDB struct {
	group, passwd string
}

// authorize decides whether the user uid may run a command that needs access. Root may run every command.
// Anyone else needs to be, per the group database, a member of control.ServiceGroup and of adminGroup, each as
// a supplementary or the primary group, and may then run commands that read. Any doubt means no access.
func (db groupDB) authorize(uid uint32, access control.Access) error {
	if uid == 0 {
		return nil
	}
	ok, err := db.admin(uid)
	if err != nil || !ok {
		return control.ErrNoAccess()
	}
	if access != control.AccessRead {
		return control.ErrNotRoot()
	}
	return nil
}

func (db groupDB) admin(uid uint32) (bool, error) {
	group, err := os.Open(db.group)
	if err != nil {
		return false, err
	}
	defer group.Close()
	passwd, err := os.Open(db.passwd)
	if err != nil {
		return false, err
	}
	defer passwd.Close()
	return adminInDatabase(group, passwd, uid)
}

type groupEntry struct {
	gid     string
	members []string
}

// adminInDatabase reports whether uid belongs to both control.ServiceGroup and adminGroup. A missing group, an
// unknown uid or an unreadable database yields false or an error, never access.
func adminInDatabase(group, passwd io.Reader, uid uint32) (bool, error) {
	groups := map[string]groupEntry{}
	sc := databaseScanner(group)
	for sc.Scan() {
		f := strings.Split(sc.Text(), ":")
		if len(f) < 4 || f[0] != control.ServiceGroup && f[0] != adminGroup {
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
	sc = databaseScanner(passwd)
	for sc.Scan() {
		f := strings.Split(sc.Text(), ":")
		if len(f) < 4 || f[2] != want {
			continue
		}
		// The first entry of the uid decides, as getpwuid does.
		member := func(g groupEntry) bool { return g.gid != "" && f[3] == g.gid || slices.Contains(g.members, f[0]) }
		return f[0] != "" && member(groups[control.ServiceGroup]) && member(groups[adminGroup]), nil
	}
	return false, sc.Err()
}

func databaseScanner(r io.Reader) *bufio.Scanner {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 4096), maxDatabaseLine)
	return sc
}
