package uninstall

import (
	"context"
	"errors"
	"io/fs"
	"strings"
)

// planAccounts adds the removal of the accounts: with purge the users and groups of both services and of the
// receiver, otherwise only qhost-deploy, because the deploy service runs as root, so a user and group of an
// earlier setup have no use left.
func (h Host) planAccounts(plan *Plan, purge bool) error {
	accounts := []string{deployService}
	if purge {
		accounts = []string{hookService, deployService, hostService}
	}
	users, groups, err := h.accountNames(purge)
	if err != nil {
		return err
	}
	if purge && (users[hookService] || groups[hookService]) {
		plan.Hook = true
	}
	for _, name := range accounts {
		if users[name] {
			plan.Steps = append(plan.Steps, Step{Command: "userdel " + name,
				do: func(ctx context.Context) error { return h.run(ctx, "userdel", name) }})
		}
	}
	for _, name := range accounts {
		if groups[name] {
			plan.Steps = append(plan.Steps, Step{Command: "groupdel " + name + " if userdel did not remove it",
				do: func(ctx context.Context) error { return h.dropGroup(ctx, name) }})
		}
	}
	return nil
}

// accountNames reads the user and group names. Without purge a missing file means no account to remove.
func (h Host) accountNames(purge bool) (users, groups map[string]bool, err error) {
	for _, file := range []struct {
		path string
		out  *map[string]bool
	}{{"/etc/passwd", &users}, {"/etc/group", &groups}} {
		names, err := h.names(file.path)
		if err != nil && (purge || !errors.Is(err, fs.ErrNotExist)) {
			return nil, nil, err
		}
		*file.out = names
	}
	return users, groups, nil
}

func (h Host) dropGroup(ctx context.Context, name string) error {
	if now, err := h.names("/etc/group"); err != nil || !now[name] {
		return err
	}
	return h.run(ctx, "groupdel", name)
}

// removesDeployAccount reports whether the plan deletes the user or group qhost-deploy.
func (p Plan) removesDeployAccount() bool {
	for _, s := range p.Steps {
		if strings.HasPrefix(s.Command, "userdel "+deployService) || strings.HasPrefix(s.Command, "groupdel "+deployService+" ") {
			return true
		}
	}
	return false
}

// purgedAccounts names the accounts a purge deletes.
func purgedAccounts(deploy bool) string {
	if deploy {
		return "the system users qhost and qhost-deploy and the group qhost"
	}
	return "the system user qhost and the group qhost"
}
