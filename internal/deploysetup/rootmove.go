package deploysetup

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// stateDir is the state directory of the deploy service.
const stateDir = "/var/lib/qhost-deploy"

// HasAccountSetup reports whether the host runs the deploy service in the setup of earlier releases: the service
// is set up and the system user or group qhost-deploy still exists. The group counts too, so that a run that
// stopped between userdel and groupdel is repeated. A host without the deploy service has none.
func (h Host) HasAccountSetup() (bool, error) {
	if !Installed(h.Root) {
		return false, nil
	}
	accounts, err := h.readAccounts()
	if err != nil {
		return false, err
	}
	groups, err := h.readGroupDB()
	if err != nil {
		return false, err
	}
	_, hasUser := accounts[Service]
	_, hasGroup := groups[Service]
	return hasUser || hasGroup, nil
}

// MoveToRoot brings the setup of earlier releases to the one where the deploy service runs as root: it rewrites
// the channel rule, hands the state to root, gives the marker folder of the system instance to the group of the
// receiver where the receiver is set up, reloads systemd, restarts the service if it runs and removes the user
// and group qhost-deploy. Each step is written to report before it runs. Every step is safe to repeat, so after a
// failure a repeated call continues; the first failure stops the rest, which stays as it is. Files in the stack
// folders are not touched.
func (h Host) MoveToRoot(ctx context.Context, report io.Writer) error {
	return h.moveToRoot(ctx, report, true)
}

// MoveToRootAtStart is MoveToRoot for the start step of the deploy service unit: it neither reloads systemd nor
// restarts the unit, because a restart would wait for the start it runs in.
func (h Host) MoveToRootAtStart(ctx context.Context, report io.Writer) error {
	return h.moveToRoot(ctx, report, false)
}

func (h Host) moveToRoot(ctx context.Context, report io.Writer, restart bool) error {
	unit, err := os.ReadFile(h.path(UnitPath()))
	if err != nil {
		return fmt.Errorf("read %s: %w", UnitPath(), err)
	}
	// The restart below would fail as a user that is about to go.
	if err := ValidateDeployUnit(unit); err != nil {
		return fmt.Errorf("the installed deploy unit %s is not the one that runs as root: %w", UnitPath(), err)
	}
	plan, err := h.rootMovePlan(ctx, restart)
	if err != nil {
		return err
	}
	for _, s := range plan.Steps {
		_, _ = fmt.Fprintf(report, "deploy service to root: %s\n", s.Command)
		if err := s.do(ctx); err != nil {
			return fmt.Errorf("%s: %w", s.Command, err)
		}
	}
	return nil
}

func (h Host) rootMovePlan(ctx context.Context, restart bool) (Plan, error) {
	var plan Plan
	add := func(command string, do func(ctx context.Context) error) {
		plan.Steps = append(plan.Steps, Step{Command: command, do: do})
	}
	command := func(name string, args ...string) {
		add(name+" "+strings.Join(args, " "), func(ctx context.Context) error { return h.run(ctx, name, args...) })
	}
	writeRule := func(conf string, rule []byte) {
		if h.differs(conf, rule) {
			add("write "+conf+" (mode 0644): "+strings.ReplaceAll(strings.TrimSpace(string(rule)), "\n", "; "),
				func(context.Context) error { return h.write(conf, rule) })
		}
		// Always: a repeat after a failed tmpfiles run must still apply the rule that is written already.
		command("systemd-tmpfiles", "--create", conf)
	}

	ep := control.DeployEndpoint()
	writeRule(tmpfilesConf, tmpfilesRule(ep.Dir, ep.ServiceUser, ep.Group))

	if info, err := os.Lstat(h.path(stateDir)); err == nil && info.IsDir() {
		add("chown "+stateDir+" and its content to root, links not followed", func(context.Context) error {
			return h.chownTree(h.path(stateDir))
		})
	}

	if HookInstalled(h.Root) {
		groups, err := h.readGroupDB()
		if err != nil {
			return Plan{}, err
		}
		accounts, err := h.readAccounts()
		if err != nil {
			return Plan{}, err
		}
		rule, _, err := h.hookRule(groups, accounts)
		if err != nil {
			return Plan{}, err
		}
		writeRule(HookTmpfilesConf, rule)
	}

	if restart {
		command("systemctl", "daemon-reload")
		command("systemctl", "try-restart", Service)
	}
	add("userdel "+Service, func(ctx context.Context) error {
		if accounts, err := h.readAccounts(); err != nil || !hasKey(accounts, Service) {
			return err
		}
		return h.run(ctx, "userdel", Service)
	})
	add("groupdel "+Service+" if userdel did not remove it", func(ctx context.Context) error {
		if groups, err := h.readGroupDB(); err != nil || !hasKey(groups, Service) {
			return err
		}
		return h.run(ctx, "groupdel", Service)
	})
	return plan, nil
}

func hasKey[V any](m map[string]V, k string) bool {
	_, ok := m[k]
	return ok
}

// chownTree gives root and everything below it to root and group 0. It changes links themselves and descends
// into no linked folder; modes stay.
func (h Host) chownTree(root string) error {
	return filepath.WalkDir(root, func(p string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return h.chown(p, 0, 0)
	})
}
