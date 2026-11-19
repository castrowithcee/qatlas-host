package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-kit/cli"
	"github.com/castrowithcee/qatlas-kit/cobracli"
	"github.com/castrowithcee/qatlas-kit/output"
	"github.com/castrowithcee/qatlas-kit/redact"

	"github.com/castrowithcee/qatlas-host/internal/admins"
	"github.com/castrowithcee/qatlas-host/internal/host/hostadmin"
)

// hostEnv is the caller and the databases the host commands work on. Production code never changes it; tests
// replace it so that no real account, folder or journal of the machine is involved.
var hostEnv = hostEnvironment{euid: os.Geteuid, admin: func(uid int) (bool, error) {
	return admins.Database{}.Admin(uint32(uid))
}}

type hostEnvironment struct {
	// euid is the effective user of the caller; root is 0.
	euid func() int
	// admin reports whether a caller other than root administers the host.
	admin func(uid int) (bool, error)
	// accounts reads the databases; nil reads the real ones, privileged for root.
	accounts func(privileged bool) *hostadmin.Host
}

func (e hostEnvironment) reader() *hostadmin.Host {
	privileged := e.euid() == 0
	if e.accounts != nil {
		return e.accounts(privileged)
	}
	return hostadmin.NewHost(privileged)
}

// requireAdmin lets root and the administrators of the host through. A failure to read the group database is
// treated as no access.
func (e hostEnvironment) requireAdmin() error {
	uid := e.euid()
	if uid == 0 {
		return nil
	}
	if ok, err := e.admin(uid); err == nil && ok {
		return nil
	}
	return &cli.Failure{Code: output.CodePermission, Message: "qhost is reserved for administrators (sudo)",
		NextStep: "Ask an administrator for access, or run the command as root."}
}

func newHostCommand(program *cobracli.Program, secrets *redact.Redactor) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "host",
		Short: "Read and change the users, groups and folder permissions of this host",
		Long: "The commands users, user, groups, group and folder read the local users, groups and folder permissions\n" +
			"and change nothing. Only root and the administrators of the group qhost may use them. Whether an account\n" +
			"is locked or expired and which SSH keys it has are shown only to root; otherwise they are unknown.\n" +
			"The commands apply, undo and journal change the host or read its journal and need root. Every change is\n" +
			"recorded in the action log ('qhost audit'). See 'qhost help host-admin'.",
		// The caller is checked after the command line is, so a mistyped command line shows its usage first.
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error { return hostEnv.requireAdmin() },
		RunE:              func(c *cobra.Command, _ []string) error { return c.Help() },
	}
	cmd.AddCommand(newHostUsersCommand(program), newHostUserCommand(program), newHostGroupsCommand(program),
		newHostGroupCommand(program), newHostFolderCommand(program), newHostApplyCommand(program, secrets),
		newHostUndoCommand(program), newHostJournalCommand(program))
	return cmd
}

func exactlyOne(what string) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) != 1 {
			return fmt.Errorf("exactly one %s is needed, got %d arguments", what, len(args))
		}
		return nil
	}
}

// hostError is a failed read of the accounts or folders of the host.
func hostError(what string, err error) error {
	return &cli.Failure{Code: output.CodeRuntime, Message: fmt.Sprintf("%s: %v", what, err), Cause: err,
		NextStep:      "Check the name or path in the message, then repeat the command.",
		AgentNextStep: "Tell the user what failed; do not retry on your own."}
}

func newHostUsersCommand(program *cobracli.Program) *cobra.Command {
	return &cobra.Command{
		Use:   "users",
		Short: "List the local users",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			users, err := hostEnv.reader().Users()
			if err != nil {
				return hostError("host users", err)
			}
			result := output.Collection{Columns: []string{"name", "uid", "primary_group", "groups", "home", "system",
				"admin", "docker", "enabled", "locked", "expired"}}
			for _, u := range users {
				result.Rows = append(result.Rows, output.Row{
					"name": u.Name, "uid": int64(u.UID), "primary_group": u.PrimaryGroup, "groups": listValue(program, u.Groups), "home": u.Home,
					"system": u.System, "admin": u.Admin, "docker": u.Docker, "enabled": u.Enabled,
					"locked": triState(program, u.Locked), "expired": triState(program, u.Expired),
				})
			}
			return output.Encode(c.OutOrStdout(), program.Format(), result)
		},
	}
}

func newHostUserCommand(program *cobracli.Program) *cobra.Command {
	return &cobra.Command{
		Use:   "user <name>",
		Short: "Show one local user",
		Args:  exactlyOne("user name"),
		RunE: func(c *cobra.Command, args []string) error {
			u, err := hostEnv.reader().User(args[0])
			if err != nil {
				return hostError("host user", err)
			}
			return output.Encode(c.OutOrStdout(), program.Format(), output.Object{Fields: []output.Field{
				{Name: "name", Value: u.Name},
				{Name: "uid", Value: int64(u.UID)},
				{Name: "primary_group", Value: u.PrimaryGroup},
				{Name: "groups", Value: listValue(program, u.Groups)},
				{Name: "home", Value: u.Home},
				{Name: "system", Value: u.System},
				{Name: "admin", Value: u.Admin},
				{Name: "docker", Value: u.Docker},
				{Name: "enabled", Value: u.Enabled},
				{Name: "locked", Value: triState(program, u.Locked)},
				{Name: "expired", Value: triState(program, u.Expired)},
				{Name: "ssh_keys", Value: keysValue(program, u.SSHKeys)},
			}})
		},
	}
}

func newHostGroupsCommand(program *cobracli.Program) *cobra.Command {
	return &cobra.Command{
		Use:   "groups",
		Short: "List the local groups",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			groups, err := hostEnv.reader().Groups()
			if err != nil {
				return hostError("host groups", err)
			}
			result := output.Collection{Columns: []string{"name", "gid", "members", "system"}}
			for _, g := range groups {
				result.Rows = append(result.Rows, output.Row{
					"name": g.Name, "gid": int64(g.GID), "members": listValue(program, g.Members), "system": g.System,
				})
			}
			return output.Encode(c.OutOrStdout(), program.Format(), result)
		},
	}
}

func newHostGroupCommand(program *cobracli.Program) *cobra.Command {
	return &cobra.Command{
		Use:   "group <name>",
		Short: "Show one local group",
		Args:  exactlyOne("group name"),
		RunE: func(c *cobra.Command, args []string) error {
			g, err := hostEnv.reader().Group(args[0])
			if err != nil {
				return hostError("host group", err)
			}
			return output.Encode(c.OutOrStdout(), program.Format(), output.Object{Fields: []output.Field{
				{Name: "name", Value: g.Name},
				{Name: "gid", Value: int64(g.GID)},
				{Name: "members", Value: listValue(program, g.Members)},
				{Name: "system", Value: g.System},
			}})
		},
	}
}

func newHostFolderCommand(program *cobracli.Program) *cobra.Command {
	return &cobra.Command{
		Use:   "folder <path>",
		Short: "Show owner, group and permissions of a folder or file",
		Args:  exactlyOne("path"),
		RunE: func(c *cobra.Command, args []string) error {
			f, err := hostEnv.reader().Folder(args[0])
			if err != nil {
				return hostError("host folder", err)
			}
			return output.Encode(c.OutOrStdout(), program.Format(), output.Object{Fields: []output.Field{
				{Name: "path", Value: f.Path},
				{Name: "resolved", Value: f.Resolved},
				{Name: "kind", Value: f.Kind},
				{Name: "owner", Value: f.Owner},
				{Name: "group", Value: f.Group},
				{Name: "mode", Value: f.Mode},
				{Name: "owner_access", Value: string(f.OwnerAcc)},
				{Name: "group_access", Value: string(f.GroupAcc)},
				{Name: "others_access", Value: string(f.OthersAcc)},
				{Name: "acl", Value: triState(program, f.ACL)},
				{Name: "setuid", Value: f.Setuid},
				{Name: "setgid", Value: f.Setgid},
				{Name: "sticky", Value: f.Sticky},
				{Name: "protected", Value: f.Protected},
			}})
		},
	}
}

// listValue renders a list of words: a list in JSON, one comma-separated text elsewhere, because a table or
// compact row holds no list.
func listValue(program *cobracli.Program, list []string) any { return listValueSep(program, list, ",") }

func listValueSep(program *cobracli.Program, list []string, sep string) any {
	if list == nil {
		list = []string{}
	}
	if program.Format() == output.FormatJSON {
		return list
	}
	return strings.Join(list, sep)
}

// triState renders a value that may be unknown: absent in JSON, "unknown" in the other formats.
func triState(program *cobracli.Program, v *bool) any {
	switch {
	case v != nil:
		return *v
	case program.Format() == output.FormatJSON:
		return nil
	}
	return "unknown"
}

// keysValue renders the keys of a user: a list of objects in JSON, one text elsewhere. Unknown is absent in
// JSON and "unknown" elsewhere. A key itself is never shown.
func keysValue(program *cobracli.Program, keys []hostadmin.SSHKey) any {
	if program.Format() == output.FormatJSON {
		if keys == nil {
			return nil
		}
		return keys
	}
	if keys == nil {
		return "unknown"
	}
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = strings.TrimSpace(k.Fingerprint + " " + k.Type + " " + k.Comment)
	}
	return strings.Join(parts, "; ")
}

var errNoAction = errors.New("an action id is needed")
