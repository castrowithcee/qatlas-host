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

// authorize decides whether the user uid may run a request that needs access, by admins.Authorize. The right is
// read from the group database; any doubt means no access.
func (db groupDB) authorize(uid uint32, access control.Access) error {
	admin := false
	if uid != 0 {
		ok, err := db.admin(uid)
		admin = err == nil && ok
	}
	return admins.Authorize(uid, admin, access)
}

func (db groupDB) admin(uid uint32) (bool, error) {
	return admins.Database{Group: db.group, Passwd: db.passwd}.Admin(uid)
}
