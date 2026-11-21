package service

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"syscall"

	"github.com/castrowithcee/qatlas-host/internal/credhelper"
)

const (
	dockerConfigDir  = "docker"
	dockerConfigFile = "config.json"
	// dockerConfigContent makes docker ask the credential helper for every registry login.
	dockerConfigContent = `{"credsStore":"qhost"}`
)

// prepareDockerConfig sets up the Docker configuration of the service under stateDir: the folder, a config.json
// that hands every login to the credential helper, and the helper's symlink to exe. Existing, correct parts
// stay untouched. Nothing found in the folder is followed: a link where a folder or file belongs is never
// written through.
func prepareDockerConfig(stateDir, exe string, log *slog.Logger) error {
	dir := filepath.Join(stateDir, dockerConfigDir)
	if err := ensureOwnDir(dir); err != nil {
		return err
	}
	bin := filepath.Join(dir, "bin")
	if err := ensureOwnDir(bin); err != nil {
		return err
	}
	if err := ensureConfigFile(filepath.Join(dir, dockerConfigFile), log); err != nil {
		return err
	}
	return ensureSymlink(filepath.Join(bin, credhelper.Name), exe)
}

// ensureOwnDir makes path a 0700 folder of the current user. A symlink or file in its place is an error.
func ensureOwnDir(path string) error {
	fi, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		if err := os.Mkdir(path, 0o700); err != nil {
			return fmt.Errorf("create %s: %w", path, err)
		}
		fi, err = os.Lstat(path)
	}
	if err != nil {
		return fmt.Errorf("check %s: %w", path, err)
	}
	if !fi.IsDir() {
		return fmt.Errorf("%s is not a folder; remove it", path)
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok && int(st.Uid) != os.Getuid() {
		return fmt.Errorf("%s belongs to another user", path)
	}
	if fi.Mode().Perm() != 0o700 {
		if err := os.Chmod(path, 0o700); err != nil {
			return fmt.Errorf("restrict %s: %w", path, err)
		}
	}
	return nil
}

// ensureConfigFile makes path a 0600 file with exactly dockerConfigContent. Anything else there is renamed to
// path+".bak" and only its existence is logged.
func ensureConfigFile(path string, log *slog.Logger) error {
	fi, err := os.Lstat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return fmt.Errorf("check %s: %w", path, err)
	case fi.IsDir():
		return fmt.Errorf("%s is a folder; remove it", path)
	default:
		if fi.Mode().IsRegular() {
			same, err := hasContent(path, dockerConfigContent)
			if err != nil {
				return err
			}
			if same {
				if fi.Mode().Perm() != 0o600 {
					if err := os.Chmod(path, 0o600); err != nil {
						return fmt.Errorf("restrict %s: %w", path, err)
					}
				}
				return nil
			}
		}
		if err := os.Rename(path, path+".bak"); err != nil {
			return fmt.Errorf("keep the old %s: %w", path, err)
		}
		log.Warn("docker configuration of the service differed and was kept as backup", "file", path+".bak")
	}
	tmp := path + ".tmp"
	_ = os.Remove(tmp)
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	_, werr := f.WriteString(dockerConfigContent)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Rename(tmp, path)
	}
	if werr != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("write %s: %w", path, werr)
	}
	return nil
}

// hasContent reports whether the regular file at path holds exactly want; it never follows a link.
func hasContent(path, want string) (bool, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return false, nil // swapped for a link meanwhile: treat as different
	}
	defer func() { _ = f.Close() }()
	got, err := io.ReadAll(io.LimitReader(f, int64(len(want))+1))
	if err != nil {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	return bytes.Equal(got, []byte(want)), nil
}

// ensureSymlink makes path a symlink to target and replaces whatever else is there, except a folder.
func ensureSymlink(path, target string) error {
	fi, err := os.Lstat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return fmt.Errorf("check %s: %w", path, err)
	case fi.IsDir():
		return fmt.Errorf("%s is a folder; remove it", path)
	case fi.Mode()&fs.ModeSymlink != 0:
		if cur, err := os.Readlink(path); err == nil && cur == target {
			return nil
		}
	}
	tmp := path + ".tmp"
	_ = os.Remove(tmp)
	if err := os.Symlink(target, tmp); err != nil {
		return fmt.Errorf("link %s: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("link %s: %w", path, err)
	}
	return nil
}

// resolvedExecutable is the running program with symlinks resolved, so a later replacement at that path
// keeps the helper link valid.
func resolvedExecutable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}
