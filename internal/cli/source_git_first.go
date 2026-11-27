package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-kit/cobracli"

	"github.com/castrowithcee/qatlas-host/internal/gitrepo"
)

// gitUnsafeFor reports whether the folder is one the system engine would take over and its .git is one the
// deploy service refuses to run git in. It is the service's own rule, checked here without git and without
// following links, because the service cannot inspect such a folder at all.
func gitUnsafeFor(c *cobra.Command, path string) bool {
	if targetOf(c.Context()).inst != nil || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return false
	}
	if _, err := os.Lstat(filepath.Join(path, ".git")); errors.Is(err, fs.ErrNotExist) {
		return false
	}
	return gitrepo.CheckGitDir(path, uint32(gitOwner.uid)) != nil
}

// secureCommandHint is the advice for a .git that is not root-only.
func secureCommandHint(path string) string {
	return "Run 'sudo qhost source secure " + path + "' to make .git root-only; 'source add' does it first, after you confirm."
}

// refuseUnsafeGit refuses, changing nothing, a folder whose .git the service would refuse.
func refuseUnsafeGit(c *cobra.Command, path string) error {
	if !gitUnsafeFor(c, path) {
		return nil
	}
	return usageFailure(path+" has a .git that is not root-only, so the service cannot inspect the folder; nothing was changed",
		secureCommandHint(path))
}

// secureGitFirst makes an existing .git of the folder root-only before the service inspects it, after the
// preview and the confirmation. Nothing changes without it, and with --expect-effect nothing changes at all: that
// effect was worked out before and does not cover this change.
func secureGitFirst(c *cobra.Command, gate *cobracli.Gate, path, expectEffect string) error {
	if !gitUnsafeFor(c, path) {
		return nil
	}
	if expectEffect != "" {
		return refuseUnsafeGit(c, path)
	}
	peers, err := folders.askPeers(c.Context(), targetOf(c.Context()))
	if err != nil {
		return err
	}
	if err := checkPath(peers, path); err != nil {
		return err
	}
	s, err := planGitSecure(c.Context(), path)
	if err != nil || s.count() == 0 {
		return err
	}
	plan := &permPlan{dir: path, secure: s}
	w := c.ErrOrStderr()
	_, _ = fmt.Fprintf(w, "The .git of %s is not root-only; the service runs git only in such a .git. Before the folder is inspected:\n", path)
	for _, l := range plan.summary() {
		_, _ = fmt.Fprintln(w, l)
	}
	return gate.Run(cobracli.Request{Question: fmt.Sprintf("Make .git of %s root-only first?", path)}, func() error {
		return folders.applyPermissions(c.Context(), plan)
	})
}
