package stacks

import "context"

type deployedTagKey struct{}

type deployedTag struct{ commit, tag string }

// WithDeployedTag returns ctx carrying tag as the Git tag of commit, the commit a stack runs at. It is how the
// engine hands the tag to the backup made of that state without the backup layer knowing the engine. An empty
// tag or commit leaves ctx as it is.
func WithDeployedTag(ctx context.Context, commit, tag string) context.Context {
	if commit == "" || tag == "" {
		return ctx
	}
	return context.WithValue(ctx, deployedTagKey{}, deployedTag{commit, tag})
}

// DeployedTag returns the tag WithDeployedTag attached to commit, empty when ctx has none for that commit.
func DeployedTag(ctx context.Context, commit string) string {
	if v, ok := ctx.Value(deployedTagKey{}).(deployedTag); ok && v.commit == commit {
		return v.tag
	}
	return ""
}
