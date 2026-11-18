// Package userfile creates files for the person who operates qhost, in the folder qhost of their home, even
// when the process runs as root through sudo.
package userfile

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/sys/unix"
)

// DirName is the folder in the home that receives the files.
const DirName = "qhost"

// maxAttempts bounds the search for a free name when the wanted one exists.
const maxAttempts = 1000

// Person is the human the files belong to.
type Person struct {
	UID, GID int
	// Home is the absolute path of the home from the account database.
	Home string
}

// Account is an entry of the account database.
type Account struct {
	UID, GID int
	Home     string
}

// Env is what the identity depends on; the zero value is replaced by the real process and account database.
type Env struct {
	Getenv func(string) string
	Euid   func() int
	Egid   func() int
	// Lookup reads an account by user ID.
	Lookup func(uid int) (Account, error)
}

func (e Env) filled() Env {
	if e.Getenv == nil {
		e.Getenv = os.Getenv
	}
	if e.Euid == nil {
		e.Euid = os.Geteuid
	}
	if e.Egid == nil {
		e.Egid = os.Getegid
	}
	if e.Lookup == nil {
		e.Lookup = lookupAccount
	}
	return e
}

func lookupAccount(uid int) (Account, error) {
	u, err := user.LookupId(strconv.Itoa(uid))
	if err != nil {
		return Account{}, err
	}
	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		return Account{}, fmt.Errorf("the group of user %d is not numeric", uid)
	}
	return Account{UID: uid, GID: gid, Home: u.HomeDir}, nil
}

// Operator finds the person who operates the process. The user named by SUDO_UID counts only when the process
// is effectively root; otherwise it is the user of the process. The home comes from the account database, as
// sudo may leave HOME at that of the caller.
func Operator(env Env) (Person, error) {
	env = env.filled()
	uid, gid := env.Euid(), env.Egid()
	if uid == 0 {
		if v, err := strconv.Atoi(env.Getenv("SUDO_UID")); err == nil && v >= 0 {
			uid, gid = v, -1
			if g, err := strconv.Atoi(env.Getenv("SUDO_GID")); err == nil && g >= 0 {
				gid = g
			}
		}
	}
	acc, err := env.Lookup(uid)
	if err != nil {
		return Person{}, fmt.Errorf("look up user %d: %w", uid, err)
	}
	if gid < 0 {
		gid = acc.GID
	}
	if !filepath.IsAbs(acc.Home) {
		return Person{}, fmt.Errorf("the home %q of user %d is not an absolute path", acc.Home, uid)
	}
	return Person{UID: uid, GID: gid, Home: filepath.Clean(acc.Home)}, nil
}

// Dir is the folder the files go to.
func (p Person) Dir() string { return filepath.Join(p.Home, DirName) }

// Create makes a new file called name in the folder of the person and returns it open for writing with its
// full path. The file never replaces one that exists: when the name is taken, a counter is put before the
// extension. The caller closes the file.
func (p Person) Create(name string) (*os.File, string, error) {
	dir, err := p.openDir()
	if err != nil {
		return nil, "", err
	}
	defer unix.Close(dir)
	fd, used, err := p.createIn(dir, name)
	if err != nil {
		return nil, "", err
	}
	path := filepath.Join(p.Dir(), used)
	return os.NewFile(uintptr(fd), path), path, nil
}

// Save writes data to a new file called name, as Create does, and returns the full path. A file that could
// not be written completely is removed again.
func (p Person) Save(name string, data []byte) (string, error) {
	dir, err := p.openDir()
	if err != nil {
		return "", err
	}
	defer unix.Close(dir)
	fd, used, err := p.createIn(dir, name)
	if err != nil {
		return "", err
	}
	path := filepath.Join(p.Dir(), used)
	f := os.NewFile(uintptr(fd), path)
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = unix.Unlinkat(dir, used, 0)
		return "", fmt.Errorf("write %s: %w", path, err)
	}
	return path, nil
}

// openDir opens the folder without following a link from the home on, and creates it when it is missing.
func (p Person) openDir() (int, error) {
	home, err := unix.Open(p.Home, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, fmt.Errorf("open the home %s: %w", p.Home, err)
	}
	defer unix.Close(home)
	var st unix.Stat_t
	if err := unix.Fstat(home, &st); err != nil {
		return -1, err
	}
	if int(st.Uid) != p.UID {
		return -1, fmt.Errorf("the home %s does not belong to the user", p.Home)
	}
	created := false
	switch err := unix.Mkdirat(home, DirName, 0o700); {
	case err == nil:
		created = true
	case !errors.Is(err, unix.EEXIST):
		return -1, fmt.Errorf("create %s: %w", p.Dir(), err)
	}
	dir, err := unix.Openat(home, DirName, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, fmt.Errorf("open %s (a link or a file is refused): %w", p.Dir(), err)
	}
	if created {
		err = unix.Fchown(dir, p.UID, p.GID)
		if err == nil {
			err = unix.Fchmod(dir, 0o700)
		}
		if err != nil {
			unix.Close(dir)
			return -1, fmt.Errorf("set the owner of %s: %w", p.Dir(), err)
		}
		return dir, nil
	}
	if err := unix.Fstat(dir, &st); err != nil {
		unix.Close(dir)
		return -1, err
	}
	if int(st.Uid) != p.UID {
		unix.Close(dir)
		return -1, fmt.Errorf("%s does not belong to the user", p.Dir())
	}
	return dir, nil
}

// createIn creates the file exclusively in dir and gives it to the person with mode 0600.
func (p Person) createIn(dir int, name string) (fd int, used string, err error) {
	if err := checkName(name); err != nil {
		return -1, "", err
	}
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for i := 1; i <= maxAttempts; i++ {
		used = name
		if i > 1 {
			used = stem + "-" + strconv.Itoa(i) + ext
		}
		fd, err = unix.Openat(dir, used, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
		if errors.Is(err, unix.EEXIST) {
			continue
		}
		if err != nil {
			return -1, "", fmt.Errorf("create %s: %w", filepath.Join(p.Dir(), used), err)
		}
		err = unix.Fchown(fd, p.UID, p.GID)
		if err == nil {
			err = unix.Fchmod(fd, 0o600) // the umask may have taken bits of the creation mode
		}
		if err != nil {
			unix.Close(fd)
			_ = unix.Unlinkat(dir, used, 0)
			return -1, "", fmt.Errorf("set the owner of %s: %w", filepath.Join(p.Dir(), used), err)
		}
		return fd, used, nil
	}
	return -1, "", fmt.Errorf("no free name for %s in %s", name, p.Dir())
}

// checkName refuses anything but one plain name.
func checkName(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\x00") || !utf8.ValidString(name) {
		return fmt.Errorf("%q is not a file name", name)
	}
	return nil
}

// maxSubjectRunes limits the subject part of a name.
const maxSubjectRunes = 48

// Slug reduces text to characters that are safe in a file name: letters, digits, dot, dash and underscore.
// Anything else becomes a dash, a leading dot or dash is dropped and the length is limited; text without
// such characters becomes "unnamed".
func Slug(text string) string {
	var b strings.Builder
	for _, r := range text {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	s := strings.TrimLeft(b.String(), ".-")
	if len(s) > maxSubjectRunes {
		s = s[:maxSubjectRunes]
	}
	s = strings.TrimRight(s, ".-")
	if s == "" {
		return "unnamed"
	}
	return s
}

// Name builds "qhost-<kind>-<subject>-<YYYYMMDD-HHMMSS><ext>" with kind and subject reduced by Slug; ext
// includes the dot.
func Name(kind, subject string, at time.Time, ext string) string {
	return "qhost-" + Slug(kind) + "-" + Slug(subject) + "-" + at.Format("20060102-150405") + ext
}
