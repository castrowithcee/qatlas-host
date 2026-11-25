package compose

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/castrowithcee/qatlas-host/internal/engine"
	"github.com/castrowithcee/qatlas-host/internal/stacks"
)

// sha256Ref is the format of an image ID and of a repository digest.
var sha256Ref = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// ImageFingerprints implements engine.ImageFingerprinter.
func (d *Deployer) ImageFingerprints(ctx context.Context, st stacks.Stack, log *engine.Log) ([]engine.ImageFingerprint, error) {
	return d.Exec.ImageFingerprints(ctx, st, log)
}

// ImageFingerprints returns one fingerprint per running container of the stack: the service, its image
// reference, the ID of the image the container was created from and the digest of that image in the repository
// of the reference. The calls go through the executor's docker environment, so a rootless engine is read like
// any other. A value that cannot be read or has an unexpected format is empty; only the container listing can
// fail the call. Nothing of the docker output is used but the expected fields.
func (x *Executor) ImageFingerprints(ctx context.Context, st stacks.Stack, log Logger) ([]engine.ImageFingerprint, error) {
	cs, err := x.Ps(ctx, st, log)
	if err != nil {
		return nil, err
	}
	digests := map[string][]string{} // image ID -> RepoDigests, nil when unreadable
	var out []engine.ImageFingerprint
	for _, c := range cs {
		if c.State != "running" || c.Service == "" {
			continue
		}
		fp := engine.ImageFingerprint{Service: c.Service}
		if imageNameRe.MatchString(c.Image) {
			fp.Image = c.Image
		}
		if containerID.MatchString(c.ID) {
			if id := x.containerImageID(ctx, log, c.ID); id != "" {
				fp.ImageID = id
				ds, seen := digests[id]
				if !seen {
					ds = x.repoDigests(ctx, log, id)
					digests[id] = ds
				}
				fp.RepoDigest = pickDigest(fp.Image, ds)
			}
		}
		out = append(out, fp)
	}
	return out, nil
}

// containerImageID returns the ID of the image of a container, empty when it cannot be read.
func (x *Executor) containerImageID(ctx context.Context, log Logger, container string) string {
	out, err := x.dockerOut(ctx, log, "container inspect", "container", "inspect", "--format", "{{.Image}}", container)
	if id := strings.TrimSpace(string(out)); err == nil && sha256Ref.MatchString(id) {
		return id
	}
	return ""
}

// repoDigests returns the RepoDigests of an image, nil when they cannot be read.
func (x *Executor) repoDigests(ctx context.Context, log Logger, imageID string) []string {
	out, err := x.dockerOut(ctx, log, "image inspect", "image", "inspect", "--format", "{{json .RepoDigests}}", imageID)
	if err != nil {
		return nil
	}
	var list []string
	if json.Unmarshal(out, &list) != nil {
		return nil
	}
	return list
}

// pickDigest returns the digest of the first RepoDigests entry that belongs to the repository of ref and has a
// valid format, empty when none does: a local image has none, and an entry of another repository says nothing
// about this reference.
func pickDigest(ref string, repoDigests []string) string {
	if ref == "" {
		return ""
	}
	repo := normalizeRepo(ref)
	for _, d := range repoDigests {
		name, digest, ok := strings.Cut(d, "@")
		if ok && sha256Ref.MatchString(digest) && normalizeRepo(name) == repo {
			return digest
		}
	}
	return ""
}

// normalizeRepo drops the digest and the tag of a reference and the implicit Docker Hub prefixes.
func normalizeRepo(ref string) string {
	ref, _, _ = strings.Cut(ref, "@")
	if i := strings.LastIndex(ref, ":"); i > strings.LastIndex(ref, "/") {
		ref = ref[:i]
	}
	for _, p := range []string{"index.docker.io/", "docker.io/", "library/"} {
		ref = strings.TrimPrefix(ref, p)
	}
	return ref
}
