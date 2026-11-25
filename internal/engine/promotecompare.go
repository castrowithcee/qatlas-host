package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/castrowithcee/qatlas-host/internal/stacks"
	"github.com/castrowithcee/qatlas-host/internal/version"
)

// Refusals of a promotion into a stack that compares the fingerprints of the images, see
// stacks.Rules.PromoteCompare.
var (
	ErrNoFingerprint = errors.New("the source has no fingerprint for the image; deploy the source stack again so that it records one")
	ErrImageChanged  = errors.New("the tag no longer names the image the source runs")
	ErrImageDiffers  = errors.New("the image loaded for the target is not the one the source runs")
	ErrNoImageCheck  = errors.New("the deployer cannot read loaded images, so the fingerprints cannot be compared")
)

// ManifestDigest returns the manifest digest the registry reports for the tag of an image variable's image.
// The caller asks the registry with the target's credentials. The engine calls it without holding any lock.
type ManifestDigest func(ctx context.Context, img stacks.ImageVar, tag string) (string, error)

// LoadedImageReader is implemented by a Deployer that can read the image of a reference as it is loaded on the
// host, which is what a Pull leaves. It reads only. RepoDigest is the digest in the repository of ref; both
// fields are empty when unknown.
type LoadedImageReader interface {
	LoadedImage(ctx context.Context, ref string, log *Log) (ImageFingerprint, error)
}

// imageExpect is what a promoted image must be once it is loaded: the source's fingerprint for Image:Tag. With a
// RepoDigest the digest is compared; a local source image has none and its ImageID is compared.
type imageExpect struct {
	Image, Tag, RepoDigest, ImageID string
}

// fingerprintRepo is the repository of an image reference without tag, digest and the implicit Docker Hub prefix.
func fingerprintRepo(ref string) string {
	ref, _, _ = strings.Cut(ref, "@")
	if i := strings.LastIndex(ref, ":"); i > strings.LastIndex(ref, "/") {
		ref = ref[:i]
	}
	for _, p := range []string{"index.docker.io/", "docker.io/", "library/"} {
		ref = strings.TrimPrefix(ref, p)
	}
	return ref
}

// sourceFingerprint returns the source's fingerprint of the images of repo: ok is false when there is none or
// the services that run the repository disagree. The fields are checked for their format again, since the
// source may have been handed over.
func sourceFingerprint(list []ImageFingerprint, repo string) (digest, id string, ok bool) {
	want := fingerprintRepo(repo)
	for _, f := range list {
		if fingerprintRepo(f.Image) != want {
			continue
		}
		d, i := f.RepoDigest, f.ImageID
		if !sha256Re.MatchString(d) {
			d = ""
		}
		if !sha256Re.MatchString(i) {
			i = ""
		}
		if d == "" && i == "" {
			continue
		}
		if ok && (d != digest || i != id) {
			return "", "", false
		}
		digest, id, ok = d, i, true
	}
	return digest, id, ok
}

// expectImage works out what the target's image must be after the promotion moved img to tag, and refuses before
// anything is done when the registry's tag is not the source's image. A source image without repository digest
// is a local one: only the loaded image can be compared, by its ID.
func expectImage(ctx context.Context, img stacks.ImageVar, tag string, src PromoteSource, digest ManifestDigest) (imageExpect, error) {
	d, id, ok := sourceFingerprint(src.State.Images, img.Image)
	if !ok {
		return imageExpect{}, fmt.Errorf("%s: %w", img.Image, ErrNoFingerprint)
	}
	if d != "" {
		if digest == nil {
			return imageExpect{}, errors.New("the registry cannot be asked for the digest")
		}
		got, err := digest(ctx, img, tag)
		if err != nil {
			return imageExpect{}, err
		}
		if got != d {
			return imageExpect{}, fmt.Errorf("%w: %s:%s is %s in the registry, the source runs %s", ErrImageChanged, img.Image, tag, got, d)
		}
	}
	return imageExpect{Image: img.Image, Tag: tag, RepoDigest: d, ImageID: id}, nil
}

// pullAndVerify pulls the images of the stack and, for a promotion that compares fingerprints, checks that the
// loaded images are the source's before anything starts. It returns the step that failed.
func (r *run) pullAndVerify(ctx context.Context, st stacks.Stack) (string, error) {
	if err := r.step("pull", func() error { return r.e.cfg.Deployer.Pull(ctx, st, r.log) }); err != nil {
		return "pull", err
	}
	if len(r.expect) == 0 {
		return "", nil
	}
	if err := r.step("verify", func() error { return r.verifyLoaded(ctx) }); err != nil {
		return "verify", err
	}
	return "", nil
}

// judge decides one variable of the target the promotion would move to to: the version check, the .env
// requirement and, when the target compares fingerprints, the source's image against the registry's tag.
func (p *PromotePreview) judge(ctx context.Context, img stacks.ImageVar, to string, src PromoteSource, check VersionCheck, digest ManifestDigest) error {
	if err := check(ctx, img, to); err != nil {
		if errors.Is(err, version.ErrNotHigher) {
			err = fmt.Errorf("%w (%s)", ErrLowerVersion, err)
		}
		return err
	}
	if img.Source != stacks.SourceEnv {
		return ErrNotInEnv
	}
	if p.Compare {
		x, err := expectImage(ctx, img, to, src, digest)
		if err != nil {
			return err
		}
		p.expect = append(p.expect, x)
	}
	return nil
}

// verifyLoaded compares the images the Pull loaded with the source's fingerprints.
func (r *run) verifyLoaded(ctx context.Context) error {
	reader, ok := r.e.cfg.Deployer.(LoadedImageReader)
	if !ok {
		return ErrNoImageCheck
	}
	for _, x := range r.expect {
		ref := x.Image + ":" + x.Tag
		got, err := reader.LoadedImage(ctx, ref, r.log)
		if err != nil {
			return fmt.Errorf("read the loaded image %s: %w", ref, err)
		}
		switch {
		case x.RepoDigest != "" && got.RepoDigest != x.RepoDigest:
			return fmt.Errorf("%w: %s is %s, the source runs %s", ErrImageDiffers, ref, orNone(got.RepoDigest), x.RepoDigest)
		case x.RepoDigest == "" && got.ImageID != x.ImageID:
			return fmt.Errorf("%w: %s has the image ID %s, the source runs %s", ErrImageDiffers, ref, orNone(got.ImageID), x.ImageID)
		}
		r.log.Printf("%s is the source's image", ref)
	}
	return nil
}

func orNone(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}
