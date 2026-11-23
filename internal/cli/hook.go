package cli

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-kit/cli"
	"github.com/castrowithcee/qatlas-kit/cobracli"
	"github.com/castrowithcee/qatlas-kit/output"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/deploysetup"
	"github.com/castrowithcee/qatlas-host/internal/hookservice"
)

func newHookCommand(program *cobracli.Program, state *notRunning) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hook",
		Short: "Add, list and remove the webhooks of stacks",
		Long: "A webhook is a secret address of the webhook receiver that lets a call make qhost check one stack at once and,\n" +
			"in automatic mode, update it. add and remove need root. list needs no sudo for administrators. The receiver\n" +
			"comes first: 'sudo qhost hook-service setup'. See 'qhost help hook-setup'.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error { return c.Help() },
	}
	cmd.AddCommand(newHookAddCommand(program, state), newHookListCommand(program), newHookRemoveCommand(program))
	return cmd
}

// hookFailure words a failed read or write of the hook configuration. The receiver that is not set up gets
// its next step; a configuration that does not parse stays as it is.
func hookFailure(err error) error {
	switch {
	case errors.Is(err, deploysetup.ErrHookNotSetUp):
		return &cli.Failure{Code: output.CodeConfigMissing, Message: err.Error(), Cause: err,
			NextStep: "Set up the receiver first with 'sudo qhost hook-service setup'."}
	default:
		return &cli.Failure{Code: output.CodeConfigInvalid, Message: err.Error(), Cause: err,
			NextStep: "Nothing was changed. Fix or remove " + deploysetup.HookConfigPath + ", then run the command again."}
	}
}

func requireHookRoot(what string) error {
	if deploySetup.euid() != 0 {
		return &cli.Failure{Code: output.CodePermission, Message: what + " needs root; run with sudo",
			NextStep: "Run the same command with sudo."}
	}
	return nil
}

func newHookAddCommand(program *cobracli.Program, state *notRunning) *cobra.Command {
	var engine string
	cmd := &cobra.Command{
		Use:   "add <stack> [--engine <engine>]",
		Short: "Give a stack a secret webhook address",
		Long: "add creates a secret for the stack, stores only its hash in " + deploysetup.HookConfigPath + " and prints the path\n" +
			"/hook/<secret> with the address the receiver listens on. The path is shown once and cannot be read again;\n" +
			"a lost secret is replaced by adding another hook and removing the old one. A stack may have several hooks.\n" +
			"A call of the path as POST without content makes qhost check the stack within seconds. The stack must\n" +
			"have the update trigger webhook or interval to act on it; with window the hook is stored and add warns.\n" +
			"Needs root and the receiver set up. A configuration that does not parse is never overwritten.",
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if err := requireHookRoot("adding a webhook"); err != nil {
				return err
			}
			stack := args[0]
			// Refuse before anything else when the receiver is missing or its file is damaged.
			if _, err := deploySetup.host.ReadHooks(); err != nil {
				return hookFailure(err)
			}
			if err := routeStacks(c, state, engine, stack); err != nil {
				return err
			}
			target := targetOf(c.Context())
			instance := hookservice.SystemInstance
			if target.inst != nil {
				instance = target.inst.User()
			}
			var stacks control.StacksResult
			if err := credentialCall(c, state, control.StacksListCommand, nil, &stacks); err != nil {
				return err
			}
			info, ok := stackInfo(stacks, stack)
			if !ok {
				return &cli.Failure{Code: output.CodeNotFound,
					Message:  fmt.Sprintf("no such stack: %s is not in the engine %s", stack, target.id()),
					NextStep: "List the stacks with 'qhost stack list'."}
			}
			id, secret, listen, err := appendHook(stack, instance)
			if err != nil {
				return err
			}
			if info.Trigger == "window" || info.Trigger == "" {
				_, _ = fmt.Fprintf(c.ErrOrStderr(), "warning: the stack %s has the update trigger window; the hook acts only with the "+
					"trigger interval or webhook (see 'qhost stack settings set')\n", stack)
			}
			_, _ = fmt.Fprintln(c.ErrOrStderr(), "The path is shown only once and cannot be read again.")
			return output.Encode(c.OutOrStdout(), program.Format(), output.Object{Fields: []output.Field{
				{Name: "id", Value: id}, {Name: "stack", Value: stack}, {Name: "instance", Value: instance},
				{Name: "path", Value: "/hook/" + secret}, {Name: "listen", Value: listen},
			}})
		},
	}
	addEngineFlag(cmd, &engine, stackEngineUsage, stackEngineNote)
	return cmd
}

// appendHook stores a new hook of the stack with the hash of a fresh secret and returns the secret with the id and the
// address the receiver listens on. The secret is not kept anywhere else.
func appendHook(stack, instance string) (id, secret, listen string, err error) {
	secret, err = hookservice.NewSecret()
	if err != nil {
		return "", "", "", &cli.Failure{Code: output.CodeRuntime, Message: err.Error(), Cause: err,
			NextStep: "Run the command again."}
	}
	err = deploySetup.host.UpdateHooks(func(cfg *hookservice.Config) error {
		var err error
		if id, err = cfg.NewID(); err != nil {
			return err
		}
		listen = cfg.Listen
		cfg.Hooks = append(cfg.Hooks, hookservice.Hook{ID: id, Stack: stack, Instance: instance,
			Hash: hookservice.HashSecret(secret), Created: time.Now().UTC().Format(time.RFC3339)})
		return nil
	})
	if err != nil {
		return "", "", "", hookFailure(err)
	}
	return id, secret, listen, nil
}

func stackInfo(res control.StacksResult, name string) (control.StackInfo, bool) {
	for _, s := range res.Stacks {
		if s.Name == name {
			return s, true
		}
	}
	return control.StackInfo{}, false
}

func newHookListCommand(program *cobracli.Program) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the webhooks of the stacks",
		Long: "list shows each hook with its id, stack, instance (system or the user of a rootless engine) and created, the\n" +
			"time the hook was added in UTC, a dash (empty in JSON) for a hook added before the time was kept. It never\n" +
			"shows a hash or a secret. Root and the administrators of the group qhost may use it.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if err := hostEnv.requireAdmin(); err != nil {
				return err
			}
			r, err := hookListResult(program.Format() == output.FormatTable)
			if err != nil {
				return err
			}
			return output.Encode(c.OutOrStdout(), program.Format(), r)
		},
	}
}

// hookListResult builds the list; a table shows a dash for a hook without a time, machine formats keep it empty.
func hookListResult(table bool) (output.Collection, error) {
	cfg, err := deploySetup.host.ReadHooks()
	if err != nil {
		return output.Collection{}, hookFailure(err)
	}
	rows := make([]output.Row, 0, len(cfg.Hooks))
	for _, h := range cfg.Hooks {
		created := h.Created
		if created == "" && table {
			created = "-"
		}
		rows = append(rows, output.Row{"id": h.ID, "stack": h.Stack, "instance": h.Instance, "created": created})
	}
	return output.Collection{Columns: []string{"id", "stack", "instance", "created"}, Rows: rows}, nil
}

func newHookRemoveCommand(program *cobracli.Program) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <id>",
		Short: "Remove a webhook",
		Long: "remove deletes the hook with the id from the configuration; its secret then gets 404 at the receiver within ten\n" +
			"seconds. Needs root and the receiver set up. A configuration that does not parse is never overwritten.",
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if err := requireHookRoot("removing a webhook"); err != nil {
				return err
			}
			id := args[0]
			if err := removeHook(id); err != nil {
				return err
			}
			return output.Encode(c.OutOrStdout(), program.Format(), output.Object{Fields: []output.Field{
				{Name: "id", Value: id}, {Name: "removed", Value: true},
			}})
		},
	}
}

// removeHook deletes the hook with the id from the configuration.
func removeHook(id string) error {
	err := deploySetup.host.UpdateHooks(func(cfg *hookservice.Config) error {
		if !cfg.Remove(id) {
			return errHookUnknown
		}
		return nil
	})
	if errors.Is(err, errHookUnknown) {
		return &cli.Failure{Code: output.CodeNotFound, Message: "no such webhook: " + id,
			NextStep: "List the hooks with 'qhost hook list'."}
	}
	if err != nil {
		return hookFailure(err)
	}
	return nil
}

var errHookUnknown = errors.New("unknown hook")
