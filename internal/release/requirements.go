package release

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// RequirementsEntry is the archive entry that says from which installed versions a release updates. The
// release build copies packaging/update.json there.
const RequirementsEntry = "share/qhost/update.json"

// maxRequirementsBytes bounds the entry; the file holds two short fields.
const maxRequirementsBytes = 64 << 10

// Requirements are what a release needs of the installed version before an update to it may start.
type Requirements struct {
	// UpdatesFrom is the oldest installed version that updates to this release directly; older ones go through it.
	UpdatesFrom string `json:"updatesFrom"`
	// Reinstall, when set, is the next step for every installed version: the release needs a fresh setup instead
	// of an update.
	Reinstall string `json:"reinstall"`
}

// Refusal says why an update to a release may not start and what to do instead.
type Refusal struct {
	Message, NextStep string
}

func (r *Refusal) Error() string { return r.Message }

var releaseVersion = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?$`)

// ParseRequirements reads the file of RequirementsEntry. Unknown fields are ignored, so a later release may add
// some without stopping the updates of programs that do not know them.
func ParseRequirements(data []byte) (Requirements, error) {
	var r Requirements
	if err := json.Unmarshal(data, &r); err != nil {
		return Requirements{}, fmt.Errorf("read the update requirements: %w", err)
	}
	if m := releaseVersion.FindStringSubmatch(r.UpdatesFrom); m == nil || m[4] != "" {
		return Requirements{}, fmt.Errorf("the update requirements name %q as the oldest version that updates directly, which is no release version such as v1.2.3", r.UpdatesFrom)
	}
	if len(r.Reinstall) > 500 || strings.IndexFunc(r.Reinstall, unicode.IsControl) >= 0 {
		return Requirements{}, errors.New("the next step of the update requirements is longer than 500 bytes or holds a control character")
	}
	return r, nil
}

// ReadRequirements finds and parses RequirementsEntry in a verified .tar.gz release archive. found is false for
// an archive without the entry, which then requires nothing.
func ReadRequirements(archive []byte) (r Requirements, found bool, err error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return Requirements{}, false, fmt.Errorf("open the archive: %w", err)
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return Requirements{}, false, nil
		}
		if err != nil {
			return Requirements{}, false, fmt.Errorf("read the archive: %w", err)
		}
		if strings.TrimPrefix(header.Name, "./") != RequirementsEntry {
			continue
		}
		if header.Typeflag != tar.TypeReg || header.Size > maxRequirementsBytes {
			return Requirements{}, false, fmt.Errorf("the archive entry %s is no regular file of at most %d bytes", RequirementsEntry, maxRequirementsBytes)
		}
		data, err := io.ReadAll(io.LimitReader(reader, maxRequirementsBytes))
		if err != nil {
			return Requirements{}, false, fmt.Errorf("read %s: %w", RequirementsEntry, err)
		}
		r, err := ParseRequirements(data)
		return r, err == nil, err
	}
}

// Check returns a *Refusal when installed may not update to the release target directly.
func (r Requirements) Check(installed, target string) error {
	if r.Reinstall != "" {
		return &Refusal{Message: "release " + target + " needs a fresh setup instead of an update; the installation is unchanged",
			NextStep: r.Reinstall}
	}
	older, err := olderThan(installed, r.UpdatesFrom)
	if err != nil {
		return err
	}
	if older {
		return &Refusal{
			Message: fmt.Sprintf("release %s updates directly only from %s or newer, and %s is installed; the installation is unchanged",
				target, r.UpdatesFrom, installed),
			NextStep: fmt.Sprintf("Update to %s first with 'sudo qhost update %s --confirm', then run this update again.", r.UpdatesFrom, r.UpdatesFrom),
		}
	}
	return nil
}

// olderThan compares two versions of the form vX.Y.Z with an optional pre-release suffix, which sorts before the
// release of the same numbers; pre-releases of the same numbers count as equal.
func olderThan(a, b string) (bool, error) {
	pa, preA, err := versionParts(a)
	if err != nil {
		return false, err
	}
	pb, preB, err := versionParts(b)
	if err != nil {
		return false, err
	}
	for i := range pa {
		if pa[i] != pb[i] {
			return pa[i] < pb[i], nil
		}
	}
	return preA && !preB, nil
}

func versionParts(v string) (parts [3]uint64, pre bool, err error) {
	m := releaseVersion.FindStringSubmatch(v)
	if m == nil {
		return parts, false, fmt.Errorf("%q is no release version such as v1.2.3", v)
	}
	for i := range parts {
		if parts[i], err = strconv.ParseUint(m[i+1], 10, 64); err != nil {
			return parts, false, fmt.Errorf("%q is no release version such as v1.2.3", v)
		}
	}
	return parts, m[4] != "", nil
}
