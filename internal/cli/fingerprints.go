package cli

import (
	"strings"

	"github.com/castrowithcee/qatlas-kit/output"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// missingFingerprint stands for a digest or ID the engine could not give, so that it never looks like a value
// that was left out of the output.
const missingFingerprint = "missing"

func orMissing(s string) string {
	if s == "" {
		return missingFingerprint
	}
	return s
}

// imageFields is the image, repository digest and image ID of each service of a stack as flat fields under prefix.
func imageFields(prefix string, images []control.FingerprintInfo) []output.Field {
	var out []output.Field
	for _, f := range images {
		p := prefix + "image." + f.Service + "."
		out = append(out, output.Field{Name: p + "ref", Value: orMissing(f.Image)},
			output.Field{Name: p + "repo_digest", Value: orMissing(f.RepoDigest)},
			output.Field{Name: p + "image_id", Value: orMissing(f.ImageID)})
	}
	return out
}

// imagesText is one "service=repo digest" per service for the runs list; short cuts the digest for a table.
func imagesText(images []control.FingerprintInfo, short bool) string {
	parts := make([]string, 0, len(images))
	for _, f := range images {
		d := orMissing(f.RepoDigest)
		if short && len(d) > len("sha256:")+12 {
			d = d[:len("sha256:")+12]
		}
		parts = append(parts, f.Service+"="+d)
	}
	return strings.Join(parts, ", ")
}

// findingsField is the findings of a stack as one field under prefix, none when there are no findings.
func findingsField(prefix string, findings []control.FindingInfo) []output.Field {
	if len(findings) == 0 {
		return nil
	}
	parts := make([]string, 0, len(findings))
	for _, f := range findings {
		part := f.Kind
		if f.Detail != "" {
			part += ": " + f.Detail
		}
		parts = append(parts, part)
	}
	return []output.Field{{Name: prefix + "findings", Value: strings.Join(parts, "; ")}}
}
