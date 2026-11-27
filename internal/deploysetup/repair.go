package deploysetup

import (
	"context"
	"os"
	"slices"
	"strings"
	"syscall"

	"github.com/castrowithcee/qatlas-kit/localipc"
)

func (h Host) lstat(p string) (os.FileInfo, error) {
	if h.Lstat != nil {
		return h.Lstat(p)
	}
	return os.Lstat(p)
}

// channelDirOK reports whether the channel directory is a folder (not a link to one) with the owner, group and
// mode the rule gives it: the service user, the group of the receiver of the channel and 2750. A rule that is right
// does not repair a folder an earlier setup left to another owner.
func (h Host) channelDirOK(dir, user, group string) bool {
	info, err := h.lstat(h.path(dir))
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0o750 || info.Mode()&os.ModeSetgid == 0 {
		return false
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return false
	}
	accounts, err := h.readAccounts()
	if err != nil {
		return false
	}
	groups, err := h.readGroupDB()
	if err != nil {
		return false
	}
	u, g := accounts[user], groups[group]
	return st.Uid == u.uid && st.Gid == g.gid && hasKey(accounts, user) && hasKey(groups, group)
}

// planAccountRemains adds the steps that remove what the setup of earlier releases left, the user and group
// qhost-deploy with the state they own. They are the steps of MoveToRoot; those the plan has already stay as they
// are. They come after the unit is written and the service started, so that userdel finds no process of the user.
func (h Host) planAccountRemains(ctx context.Context, plan *Plan) error {
	old, err := h.HasAccountSetup()
	if err != nil || !old {
		return err
	}
	move, err := h.rootMovePlan(ctx, true)
	if err != nil {
		return err
	}
	have := plan.Commands()
	for _, s := range move.Steps {
		if !slices.Contains(have, s.Command) {
			plan.Steps = append(plan.Steps, s)
		}
	}
	return nil
}

// planChannelDir adds the steps for the rule of the channel directory and for the directory itself.
func (h Host) planChannelDir(plan *Plan, ep localipc.ServiceEndpoint) {
	rule := tmpfilesRule(ep.Dir, ep.ServiceUser, ep.Group)
	ruleChanged := h.differs(tmpfilesConf, rule)
	if ruleChanged {
		plan.Steps = append(plan.Steps, Step{Command: "write " + tmpfilesConf + " (mode 0644): " + strings.TrimSpace(string(rule)),
			do: func(context.Context) error { return h.write(tmpfilesConf, rule) }})
	}
	if ruleChanged || !h.channelDirOK(ep.Dir, ep.ServiceUser, ep.Group) {
		plan.Steps = append(plan.Steps, Step{Command: "systemd-tmpfiles --create " + tmpfilesConf,
			do: func(ctx context.Context) error { return h.run(ctx, "systemd-tmpfiles", "--create", tmpfilesConf) }})
	}
}
