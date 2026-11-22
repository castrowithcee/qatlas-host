package window

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/castrowithcee/qatlas-host/internal/formatnum"
)

// The file of the host window. It is owned by root and readable by every service; only the client running
// under sudo writes it, and every deploy service instance reads it afresh, so a change applies without a restart.
const (
	// HostFilePath is the production path of the host window.
	HostFilePath = "/etc/qhost/window.json"
	hostFileMode = 0o644
	hostDirMode  = 0o755
	hostSchema   = 1
)

// ErrHostFile is a host window file that cannot be used: its format number is not read by this version or its
// content is malformed. The file is never changed because of it.
var ErrHostFile = errors.New("the host window file cannot be used")

type hostDoc struct {
	Schema   int      `json:"schema"`
	Days     []string `json:"days"`
	Start    string   `json:"start"`
	Duration string   `json:"duration"`
	Zone     string   `json:"zone"`
}

// ReadHost returns the window in the file at path; ok is false when the file does not exist. A file that cannot
// be used is an error that matches ErrHostFile, or the format number error when its number is unknown.
func ReadHost(path string) (w Window, ok bool, err error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Window{}, false, nil
	}
	if err != nil {
		return Window{}, false, err
	}
	if err := formatnum.CheckJSON(path, "schema", data, hostSchema); err != nil {
		return Window{}, false, err
	}
	bad := fmt.Errorf("%w: %s is malformed", ErrHostFile, path)
	var d hostDoc
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if dec.Decode(&d) != nil {
		return Window{}, false, bad
	}
	days, err1 := ParseDays(d.Days)
	start, err2 := ParseStart(d.Start)
	dur, err3 := ParseDuration(d.Duration)
	if err := errors.Join(err1, err2, err3); err != nil || !validZoneName(d.Zone) {
		return Window{}, false, bad
	}
	w = Window{Days: days, Start: start, Duration: dur, Zone: d.Zone}
	if w.check() != nil {
		return Window{}, false, bad
	}
	return w, true, nil
}

// WriteHost replaces the file at path with w. A file that exists but cannot be used is refused and kept. The
// file is written whole to a temporary file next to it and moved over, mode 0644; a missing folder is created
// with mode 0755.
func WriteHost(path string, w Window) error {
	if err := w.check(); err != nil {
		return err
	}
	if _, _, err := ReadHost(path); err != nil {
		return err
	}
	data, err := json.MarshalIndent(hostDoc{Schema: hostSchema, Days: w.Days.Names(), Start: FormatStart(w.Start),
		Duration: FormatDuration(w.Duration), Zone: w.Zone}, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, hostDirMode); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".window-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	_, err = tmp.Write(append(data, '\n'))
	if err == nil {
		err = tmp.Chmod(hostFileMode)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// Apply returns w with the named parts replaced and checks the result. A zone is checked against the zone database.
func (c Change) Apply(w Window) (Window, error) {
	if c.Days == nil && c.Start == nil && c.Duration == nil && c.Zone == nil {
		return Window{}, invalid("name at least one part of the window to change")
	}
	if c.Zone != nil {
		if _, err := ParseZone(*c.Zone); err != nil {
			return Window{}, err
		}
		w.Zone = *c.Zone
	}
	if c.Days != nil {
		w.Days = *c.Days
	}
	if c.Start != nil {
		w.Start = *c.Start
	}
	if c.Duration != nil {
		w.Duration = *c.Duration
	}
	if err := w.check(); err != nil {
		return Window{}, err
	}
	return w, nil
}

// HostBase is the window a change of the host window starts from: the one in the file at path; without a
// file, the window the service state in stateDir kept before the host window existed (see AdoptState); else the
// default.
func HostBase(path, stateDir string) (Window, error) {
	w, ok, err := ReadHost(path)
	if err != nil || ok {
		return w, err
	}
	if stateDir != "" {
		if adopted, found := stateWindow(stateDir); found {
			return adopted, nil
		}
	}
	return Default(), nil
}

// SetHost applies c to HostBase and writes the result to the file at path. It returns the window written.
func SetHost(path, stateDir string, c Change) (Window, error) {
	w, err := HostBase(path, stateDir)
	if err != nil {
		return Window{}, err
	}
	w, err = c.Apply(w)
	if err != nil {
		return Window{}, err
	}
	return w, WriteHost(path, w)
}

// AdoptState makes the window a service kept before the host window existed the host window: when there is no
// file at path and the service state in stateDir holds a window that is not the default, that window is written
// to the file. It reports whether it wrote the file.
func AdoptState(path, stateDir string) (bool, error) {
	if _, ok, err := ReadHost(path); err != nil || ok {
		return false, err
	}
	w, found := stateWindow(stateDir)
	if !found {
		return false, nil
	}
	return true, WriteHost(path, w)
}

// stateWindow returns the window of a service state that is not an own one and not the default. An unreadable
// state counts as none.
func stateWindow(stateDir string) (Window, bool) {
	path := filepath.Join(stateDir, fileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return Window{}, false
	}
	r, err := decodeFile(path, data)
	if err != nil || r.Own || r.Window == Default() {
		return Window{}, false
	}
	return r.Window, true
}

// ParseChange parses the parts of a request, each as its own parser does. Days present but empty is refused,
// not read as unset; an empty start, duration or zone is unset.
func ParseChange(days []string, start, duration, zone string) (Change, error) {
	var c Change
	if days != nil {
		d, err := ParseDays(days)
		if err != nil {
			return c, err
		}
		c.Days = &d
	}
	if start != "" {
		m, err := ParseStart(start)
		if err != nil {
			return c, err
		}
		c.Start = &m
	}
	if duration != "" {
		d, err := ParseDuration(duration)
		if err != nil {
			return c, err
		}
		c.Duration = &d
	}
	if zone != "" {
		z, err := ParseZone(zone)
		if err != nil {
			return c, err
		}
		c.Zone = &z
	}
	return c, nil
}
