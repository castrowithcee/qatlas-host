package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"

	tea "charm.land/bubbletea/v2"
	"golang.org/x/sys/unix"

	"github.com/castrowithcee/qatlas-kit/exteditor"
)

// editHooks reach the editor and the file system for an edit; a zero field takes the real one, and tests replace
// them so that no editor starts and nothing outside their own folder is written.
type editHooks struct {
	start  func(exteditor.Request) (tea.Cmd, error)
	getenv func(string) string
	// runUser and shm are the parents of the private folders, /run/user and /dev/shm.
	runUser, shm string
	tmpfs        func(dir string) bool
	// pick replaces choosePrivateDir, for an edit as root whose folders a test cannot hand to another user.
	pick func(uid int, target *exteditor.Target) (privateDir, error)
}

func (h editHooks) withDefaults() editHooks {
	if h.start == nil {
		h.start = exteditor.Edit
	}
	if h.getenv == nil {
		h.getenv = os.Getenv
	}
	if h.runUser == "" {
		h.runUser = "/run/user"
	}
	if h.shm == "" {
		h.shm = "/dev/shm"
	}
	if h.tmpfs == nil {
		h.tmpfs = onTmpfs
	}
	if h.pick == nil {
		h.pick = h.choosePrivateDir
	}
	return h
}

func onTmpfs(dir string) bool {
	var st unix.Statfs_t
	return unix.Statfs(dir, &st) == nil && st.Type == unix.TMPFS_MAGIC
}

// privateDir is the folder the copy of an edit lies in. ram says it is in memory; made marks a folder the edit
// created under /dev/shm, which it removes again.
type privateDir struct {
	path      string
	ram, made bool
}

func (d privateDir) remove() {
	if d.made {
		_ = os.Remove(d.path)
	}
}

var errNoPrivateDir = errors.New("neither the runtime folder of the user nor /dev/shm can hold it")

// choosePrivateDir picks the folder for the copy of the user uid: the runtime folder of the user, else a folder of
// its own under /dev/shm, the first of them that is in memory. target is the user the editor runs as under root;
// the runtime folder is then /run/user/<uid>, since XDG_RUNTIME_DIR belongs to root.
func (h editHooks) choosePrivateDir(uid int, target *exteditor.Target) (privateDir, error) {
	runtime := h.getenv("XDG_RUNTIME_DIR")
	if target != nil || !filepath.IsAbs(runtime) {
		runtime = filepath.Join(h.runUser, strconv.Itoa(uid))
	}
	own := ownedPrivate(runtime, uid)
	if own && h.tmpfs(runtime) {
		return privateDir{path: runtime, ram: true}, nil
	}
	shmRAM := h.tmpfs(h.shm)
	if own && !shmRAM {
		return privateDir{path: runtime}, nil
	}
	dir, err := makeShmDir(h.shm, target)
	switch {
	case err == nil:
		return privateDir{path: dir, ram: shmRAM, made: true}, nil
	case own:
		return privateDir{path: runtime}, nil
	}
	return privateDir{}, fmt.Errorf("%w: %w", errNoPrivateDir, err)
}

// ownedPrivate reports a folder, not a link, of mode 0700 that belongs to uid; the kit checks it again on the
// open folder.
func ownedPrivate(dir string, uid int) bool {
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
		return false
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(st.Uid) == uid
}

// makeShmDir creates a folder with a random name and mode 0700 under shm, owned by target where it is set.
func makeShmDir(shm string, target *exteditor.Target) (string, error) {
	dir, err := os.MkdirTemp(shm, "qhost-edit-")
	if err != nil {
		return "", err
	}
	err = os.Chmod(dir, 0o700)
	if err == nil && target != nil {
		err = os.Chown(dir, int(target.UID), int(target.GID))
	}
	if err != nil {
		_ = os.Remove(dir)
		return "", err
	}
	return dir, nil
}

// editTarget is the user the editor runs as: nil without root, else the person behind sudo. The text is the reason
// there is none.
func (m *Model) editTarget() (*exteditor.Target, string) {
	if m.euid() != 0 {
		return nil, ""
	}
	uid := m.humanUID()
	if uid == 0 {
		return nil, m.text.Lookup(textEditAsRoot)
	}
	for _, u := range m.sys.users {
		if u.UID != uid || !filepath.IsAbs(u.Home) {
			continue
		}
		for _, g := range m.sys.groups {
			if g.Name == u.PrimaryGroup && g.GID > 0 {
				return &exteditor.Target{UID: uint32(uid), GID: uint32(g.GID), Home: u.Home}, ""
			}
		}
	}
	return nil, m.text.Lookup(textEditNoUser, uid)
}

// readableWithoutSudo reports a file the user uid may read by its owner or the bits for everyone; a group is not
// counted, so a file the user reads only through a group counts as unreadable, the narrow reading.
func readableWithoutSudo(mode uint32, owner, uid int) bool {
	return mode&0o004 != 0 || (owner == uid && mode&0o400 != 0)
}
