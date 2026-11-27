package cli

import "context"

// SecureGit is `source secure` as root without its question: the interface asked before.
func (h tuiFolders) SecureGit(ctx context.Context, dir string) (int, error) {
	if err := folders.requireRoot(); err != nil {
		return 0, err
	}
	ctx, err := h.engineCtx(ctx)
	if err != nil {
		return 0, err
	}
	s, err := planGitSecure(ctx, dir)
	if err != nil || s.count() == 0 {
		return 0, err
	}
	return s.count(), folders.applyPermissions(ctx, &permPlan{dir: dir, secure: s})
}
