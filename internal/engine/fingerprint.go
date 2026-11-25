package engine

import (
	"cmp"
	"context"
	"errors"
	"regexp"
	"slices"
	"time"

	"github.com/castrowithcee/qatlas-host/internal/stacks"
)

// fingerprintTimeout bounds reading the fingerprints after a run; a run never fails on them.
const fingerprintTimeout = 2 * time.Minute

// sha256Re is the format of an image ID and of a repository digest.
var sha256Re = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// ImageFingerprint tells which image a service of a stack runs. An empty RepoDigest or ImageID is missing: the
// image has no repository digest (it was built or loaded locally) or Docker could not be read.
type ImageFingerprint struct {
	Service string `json:"service"`
	// Image is the image reference of the service as the Compose project has it.
	Image string `json:"image,omitempty"`
	// RepoDigest is the digest of the image in the repository of Image, "sha256:" and 64 hex digits.
	RepoDigest string `json:"repo_digest,omitempty"`
	// ImageID is the ID of the image the running container was created from, in the same format.
	ImageID string `json:"image_id,omitempty"`
}

// ImageFingerprinter is implemented by a Deployer that can read the fingerprints of the images of the running
// containers of a stack. A Deployer without it leaves the fingerprints missing.
type ImageFingerprinter interface {
	// ImageFingerprints returns one entry per service that runs. It reads only and changes nothing.
	ImageFingerprints(ctx context.Context, stack stacks.Stack, log *Log) ([]ImageFingerprint, error)
}

// tagAndRead tags the commit a stack runs at after a success or a rollback and reads the fingerprints of its
// images into rec.
func (r *run) tagAndRead(rec *RunRecord, at string) (tagErr, pruneErr error) {
	if rec.Result != ResultSucceeded && rec.Result != ResultRolledBack {
		return nil, nil
	}
	r.log.Step("tag")
	rec.Tag, tagErr = r.tagRun(at)
	if tagErr != nil {
		r.log.Printf("not set: %s", tagErr)
	} else if rec.Tag != "" {
		r.log.Printf("%s", rec.Tag)
		if pruneErr = r.pruneTags(); pruneErr != nil {
			r.log.Printf("old tags not deleted: %s", pruneErr)
		}
	}
	rec.Images = r.fingerprints()
	return tagErr, pruneErr
}

// fingerprints reads the fingerprints of the stack after a run that left it running. A failure is logged and
// gives none.
func (r *run) fingerprints() []ImageFingerprint {
	f, ok := r.e.cfg.Deployer.(ImageFingerprinter)
	if !ok {
		return nil
	}
	ctx, cancel := context.WithTimeout(r.e.base, fingerprintTimeout)
	defer cancel()
	r.log.Step("fingerprints")
	list, err := f.ImageFingerprints(ctx, r.st, r.log)
	if err != nil {
		r.log.Printf("not read: %s", r.e.text(err.Error()))
		return nil
	}
	list = cleanFingerprints(list)
	r.log.Printf("%d services", len(list))
	return list
}

// cleanFingerprints keeps what validateState accepts: an entry without a valid service name is dropped, a field
// of a wrong format is missing. The result is ordered by service and image ID.
func cleanFingerprints(in []ImageFingerprint) []ImageFingerprint {
	var out []ImageFingerprint
	for _, f := range in {
		if f.Service == "" || len(f.Service) > maxNameLen || len(out) >= MaxMonitorServices {
			continue
		}
		if len(f.Image) > maxNameLen {
			f.Image = ""
		}
		if !sha256Re.MatchString(f.RepoDigest) {
			f.RepoDigest = ""
		}
		if !sha256Re.MatchString(f.ImageID) {
			f.ImageID = ""
		}
		out = append(out, f)
	}
	slices.SortFunc(out, func(a, b ImageFingerprint) int {
		return cmp.Or(cmp.Compare(a.Service, b.Service), cmp.Compare(a.ImageID, b.ImageID))
	})
	return slices.Compact(out)
}

// validateHistory checks the run records and the fingerprints of the stacks.
func validateHistory(s State) error {
	for _, st := range s.Stacks {
		if err := validateFingerprints(st.Images); err != nil {
			return err
		}
	}
	for _, r := range s.Runs {
		if !runIDRe.MatchString(r.ID) {
			return errors.New("invalid run id")
		}
		if r.RolledBackTo != "" && !commitRe.MatchString(r.RolledBackTo) {
			return errors.New("invalid commit id")
		}
		if err := validateFingerprints(r.Images); err != nil {
			return err
		}
	}
	return nil
}

func validateFingerprints(list []ImageFingerprint) error {
	if len(list) > MaxMonitorServices {
		return errors.New("too many image fingerprints")
	}
	for _, f := range list {
		if f.Service == "" || len(f.Service) > maxNameLen || len(f.Image) > maxNameLen ||
			(f.RepoDigest != "" && !sha256Re.MatchString(f.RepoDigest)) ||
			(f.ImageID != "" && !sha256Re.MatchString(f.ImageID)) {
			return errors.New("invalid image fingerprint")
		}
	}
	return nil
}
