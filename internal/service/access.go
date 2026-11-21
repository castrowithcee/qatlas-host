package service

import (
	"github.com/castrowithcee/qatlas-host/internal/admins"
	"github.com/castrowithcee/qatlas-host/internal/control"
)

// The group database the access check reads when Config names none.
const (
	defaultGroupFile  = admins.DefaultGroupFile
	defaultPasswdFile = admins.DefaultPasswdFile
)

// groupDB is the group database the access check reads on every request, so a revoked membership counts from
// the next request on. The files are the whole truth: no NSS, so a user known only to LDAP or SSSD has no access.
type groupDB struct {
	group, passwd string
}

// authorize decides whether the user uid may run a request that needs access, by its class only. Root may run every
// command. Anyone else needs to be, per the group database, a member of control.ServiceGroup and of
// admins.AdminGroup, each as a supplementary or the primary group, and may then run commands that read or propose.
// Any doubt means no access.
func (db groupDB) authorize(uid uint32, access control.Access) error {
	if uid == 0 {
		return nil
	}
	ok, err := db.admin(uid)
	if err != nil || !ok {
		return control.ErrNoAccess()
	}
	if access.Class != control.ClassRead && access.Class != control.ClassPropose {
		return control.ErrNotRoot()
	}
	return nil
}

func (db groupDB) admin(uid uint32) (bool, error) {
	return admins.Database{Group: db.group, Passwd: db.passwd}.Admin(uid)
}
