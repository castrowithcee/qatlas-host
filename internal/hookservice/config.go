// Package hookservice is the webhook receiver: it accepts POST /hook/<secret>, checks the secret against the
// stored hashes, discards the body and drops an empty marker for the stack in the folder of the deploy
// instance that owns it. Everything in a request is untrusted: instance and stack of a marker come from the
// loaded configuration only.
package hookservice

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"syscall"
	"time"

	"github.com/castrowithcee/qatlas-host/internal/formatnum"
)

const (
	// DefaultConfigPath is the configuration file; it holds hashes only, never a secret.
	DefaultConfigPath = "/etc/qhost-hook/config.json"
	// DefaultRunDir holds one folder of markers per deploy instance.
	DefaultRunDir = "/run/qhost-hook"

	// ConfigVersion is the format number written to the file; 1 (hooks without a creation time) is still read.
	ConfigVersion  = 2
	maxConfigBytes = 1 << 20
	// SystemInstance names the deploy instance of the system engine.
	SystemInstance = "system"
)

var (
	idPattern       = regexp.MustCompile(`^h-[0-9a-f]{8}$`)
	hashPattern     = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	stackPattern    = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
	instancePattern = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)
)

// Hook is one stored webhook: the hash of its secret and the stack it marks.
type Hook struct {
	ID       string `json:"id"`
	Stack    string `json:"stack"`
	Instance string `json:"instance"`
	Hash     string `json:"hash"`
	// Created is when the hook was added, RFC 3339 in UTC. Hooks from before format 2 have none and keep none.
	Created string `json:"created,omitempty"`
}

// Config is the content of the configuration file.
type Config struct {
	Version int    `json:"version"`
	Listen  string `json:"listen"`
	Hooks   []Hook `json:"hooks"`
}

// LoadConfig reads and validates the file at path. The file must belong to ownerUID (root in production) and
// be writable by nobody else, or it is refused: whoever may write it decides which secrets open a stack.
func LoadConfig(path string, ownerUID uint32) (Config, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return Config{}, fmt.Errorf("hook configuration: %w", err)
	}
	defer f.Close()
	if err := checkOwnerOnlyWritable(f, ownerUID); err != nil {
		return Config{}, err
	}
	data, err := io.ReadAll(io.LimitReader(f, maxConfigBytes+1))
	if err != nil {
		return Config{}, fmt.Errorf("hook configuration: %w", err)
	}
	if len(data) > maxConfigBytes {
		return Config{}, errors.New("hook configuration: file too large")
	}
	return parseConfig(path, data)
}

func checkOwnerOnlyWritable(f *os.File, ownerUID uint32) error {
	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("hook configuration: %w", err)
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return errors.New("hook configuration: cannot read the owner of the file")
	}
	if !info.Mode().IsRegular() {
		return errors.New("hook configuration: not a regular file")
	}
	if st.Uid != ownerUID {
		return fmt.Errorf("hook configuration: owned by uid %d, want uid %d", st.Uid, ownerUID)
	}
	if info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("hook configuration: mode %04o is writable by group or others", info.Mode().Perm())
	}
	return nil
}

// ParseConfig validates the content of the configuration file at path, which the refusal of an unknown format
// number names; the setup uses it to keep the stored hooks.
func ParseConfig(path string, data []byte) (Config, error) { return parseConfig(path, data) }

func parseConfig(path string, data []byte) (Config, error) {
	if err := formatnum.CheckJSON(path, "version", data, ConfigVersion, 1); err != nil {
		return Config{}, fmt.Errorf("hook configuration: %w", err)
	}
	var c Config
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return Config{}, fmt.Errorf("hook configuration: %w", err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return Config{}, errors.New("hook configuration: data after the JSON object")
	}
	if c.Listen == "" {
		return Config{}, errors.New("hook configuration: listen is empty")
	}
	ids, hashes := map[string]bool{}, map[string]bool{}
	for i, h := range c.Hooks {
		if err := h.validate(); err != nil {
			return Config{}, fmt.Errorf("hook configuration: hook %d: %w", i, err)
		}
		if ids[h.ID] || hashes[h.Hash] {
			return Config{}, fmt.Errorf("hook configuration: hook %d repeats an id or a hash", i)
		}
		ids[h.ID], hashes[h.Hash] = true, true
	}
	return c, nil
}

func (h Hook) validate() error {
	switch {
	case !idPattern.MatchString(h.ID):
		return errors.New("id does not match h-<8 hex>")
	case !stackPattern.MatchString(h.Stack):
		return errors.New("stack does not match " + stackPattern.String())
	case h.Instance != SystemInstance && !instancePattern.MatchString(h.Instance):
		return errors.New("instance is neither system nor a user name")
	case !hashPattern.MatchString(h.Hash):
		return errors.New("hash does not match sha256:<64 hex>")
	}
	if h.Created != "" {
		if _, err := time.Parse(time.RFC3339, h.Created); err != nil {
			return errors.New("created is not an RFC 3339 time")
		}
	}
	return nil
}

// UpdateConfig reads the file at path, lets edit change the configuration and writes the result back
// atomically: a temporary file in the same folder, synced, then renamed over the file. prepare gives the
// temporary file its owner before the rename. Nothing is written when the file is missing, does not
// validate (a damaged or unknown file is never overwritten), edit fails or the result does not validate.
// A lock on the folder serializes concurrent updates. The error for a missing file wraps fs.ErrNotExist.
func UpdateConfig(path string, prepare func(tmp string) error, edit func(*Config) error) error {
	dir := filepath.Dir(path)
	d, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("hook configuration: %w", err)
	}
	defer d.Close()
	if err := syscall.Flock(int(d.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("hook configuration: lock: %w", err)
	}
	defer func() { _ = syscall.Flock(int(d.Fd()), syscall.LOCK_UN) }()

	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return fmt.Errorf("hook configuration: %w", err)
	}
	data, err := io.ReadAll(io.LimitReader(f, maxConfigBytes+1))
	_ = f.Close()
	if err != nil {
		return fmt.Errorf("hook configuration: %w", err)
	}
	if len(data) > maxConfigBytes {
		return errors.New("hook configuration: file too large")
	}
	cfg, err := parseConfig(path, data)
	if err != nil {
		return err
	}
	if err := edit(&cfg); err != nil {
		return err
	}
	cfg.Version = ConfigVersion
	if cfg.Hooks == nil {
		cfg.Hooks = []Hook{}
	}
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("hook configuration: %w", err)
	}
	out = append(out, '\n')
	if _, err := parseConfig(path, out); err != nil {
		return err
	}
	return replaceFile(dir, path, out, prepare)
}

func replaceFile(dir, path string, content []byte, prepare func(tmp string) error) error {
	tmp, err := os.CreateTemp(dir, ".config-*")
	if err != nil {
		return fmt.Errorf("hook configuration: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	steps := []func() error{
		func() error { _, err := tmp.Write(content); return err },
		func() error { return tmp.Chmod(0o644) },
		func() error {
			if prepare == nil {
				return nil
			}
			return prepare(tmp.Name())
		},
		tmp.Sync,
		tmp.Close,
		func() error { return os.Rename(tmp.Name(), path) },
	}
	for _, step := range steps {
		if err := step(); err != nil {
			_ = tmp.Close()
			return fmt.Errorf("hook configuration: %w", err)
		}
	}
	// Make the rename durable; a failure here leaves the new file in place.
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}
