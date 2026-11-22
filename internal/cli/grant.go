package cli

import (
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-kit/cli"
	"github.com/castrowithcee/qatlas-kit/cobracli"
	"github.com/castrowithcee/qatlas-kit/output"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

const grantEngineNote = "Every deploy service keeps its own grants: the one of the system engine and the instance of each " +
	"rootless engine. add reaches the deploy service of the engine that has the stack; list with --stack does the same, " +
	"and list without it and remove reach " + instanceRule

func newGrantCommand(program *cobracli.Program, state *notRunning) *cobra.Command {
	group := &cobra.Command{
		Use:   "grant",
		Short: "Allow a user one action for a stack without sudo, list the grants, or remove one",
		Long: "A standing grant lets a person with root allow a user one action for one stack of a deploy service. The\n" +
			"grantable actions are " + strings.Join(control.GrantActions, ", ") + ". list needs no sudo for an\n" +
			"administrator in the group qhost; add and remove need sudo. See 'qhost help grants'.",
	}
	group.AddCommand(newGrantAdd(program, state), newGrantList(program, state), newGrantRemove(program, state))
	return group
}

func newGrantAdd(program *cobracli.Program, state *notRunning) *cobra.Command {
	var gate *cobracli.Gate
	var engine, expires string
	cmd := &cobra.Command{
		Use:   "add <user> <action> <stack>",
		Short: "Allow a user one action for a stack, or replace the expiry of that grant",
		Long: "add records that the user may take the action for the stack without sudo. The user must exist and be an\n" +
			"administrator in the group qhost, and the stack must exist in the deploy service. An equal grant is not\n" +
			"stored twice: a new --expires replaces the old one. --expires is a date YYYY-MM-DD; the grant holds until\n" +
			"the end of that day in UTC, and without it the grant does not expire.\n" +
			"It first shows the plan on standard error: the user, the action, the stack, the engine, the expiry and\n" +
			"whether the stack has a backup and a check set up. Without --confirm it stops there with exit code 2 and\n" +
			"changes nothing. Run it with sudo.",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) != 3 {
				return usageFailure("grant add takes exactly three arguments: a user, an action and a stack",
					"Run 'sudo qhost grant add <user> <action> <stack>'; the actions are "+strings.Join(control.GrantActions, ", ")+".")
			}
			return nil
		},
		RunE: func(c *cobra.Command, args []string) error {
			if err := requireRoot("grant add"); err != nil {
				return err
			}
			if err := routeStacks(c, state, engine, args[2]); err != nil {
				return err
			}
			req := control.GrantAddArgs{User: args[0], Action: args[1], Stack: args[2], Expires: expires,
				SudoUID: uint32(changeEnv.sudoUID())}
			// Without --confirm the service answers with the plan; this error then ends the command.
			unconfirmed := gate.Run(cobracli.Request{Changes: true}, func() error { return nil })
			req.Confirm = unconfirmed == nil
			var res control.GrantAddResult
			if err := credentialCall(c, state, control.GrantAddCommand, req, &res); err != nil {
				return err
			}
			if !res.Applied {
				showGrantPlan(c.ErrOrStderr(), targetOf(c.Context()).id(), res)
				return unconfirmed
			}
			fields := []output.Field{
				{Name: "id", Value: res.Grant.ID}, {Name: "user", Value: plainText(res.Grant.User)}, {Name: "action", Value: res.Grant.Action},
				{Name: "stack", Value: plainText(res.Grant.Stack)}, {Name: "engine", Value: targetOf(c.Context()).id()},
				{Name: "expires", Value: res.Grant.Expires}, {Name: "replaced", Value: res.Replaced},
				{Name: "backup", Value: res.Backup}, {Name: "check", Value: res.Check},
			}
			return output.Encode(c.OutOrStdout(), program.Format(), output.Object{Fields: fields})
		},
	}
	cmd.Flags().StringVar(&expires, "expires", "", "last day the grant holds, as YYYY-MM-DD (UTC); without it the grant does not expire")
	addEngineFlag(cmd, &engine, stackEngineUsage, grantEngineNote)
	gate = program.AddGate(cmd, cobracli.Gates{Change: true})
	return cmd
}

// showGrantPlan prints what a grant would do and what the stack has set up, on standard error.
func showGrantPlan(w io.Writer, engine string, res control.GrantAddResult) {
	g := res.Grant
	_, _ = fmt.Fprintf(w, "Allow user %s (uid %d) the action %s for the stack %s on the engine %s, without sudo.\n",
		plainText(g.User), g.UID, g.Action, plainText(g.Stack), engine)
	if g.Expires == "" {
		_, _ = fmt.Fprintln(w, "  expires: never")
	} else {
		_, _ = fmt.Fprintf(w, "  expires: after the end of %s (UTC)\n", plainText(g.Expires))
	}
	if res.Replaced {
		_, _ = fmt.Fprintf(w, "  replaces the expiry of the equal grant %s\n", g.ID)
	}
	_, _ = fmt.Fprintf(w, "  backup: %s\n", setUpText(res.Backup))
	_, _ = fmt.Fprintf(w, "  check: %s\n", setUpText(res.Check))
	if !res.Settings {
		_, _ = fmt.Fprintln(w, "  the stack has no settings that can be read, so it has neither backup nor check")
	}
	if g.Action == control.GrantActionUpdateRun {
		_, _ = fmt.Fprintln(w, "  an update started with this grant commits and pushes to the source of the stack")
	}
}

func setUpText(on bool) string {
	if on {
		return "set up"
	}
	return "not set up"
}

func newGrantList(program *cobracli.Program, state *notRunning) *cobra.Command {
	var engine, stack, user string
	cmd := &cobra.Command{
		Use:   "list [--stack <stack>] [--user <user>]",
		Short: "List the grants with their state",
		Long: "list shows the standing grants of one deploy service: id, user, action, stack, expiry, who made it and\n" +
			"when, and the state. The state is valid, or the reason the grant is not applied: expired, stack-missing\n" +
			"(the stack no longer exists) or user-changed (the user ID belongs to another name today). A grant that is\n" +
			"not valid stays listed until it is removed. If the grants file of the service cannot be read, no grant is\n" +
			"listed, every grant counts as invalid and the command ends with exit code 1.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if stack != "" {
				if err := routeStacks(c, state, engine, stack); err != nil {
					return err
				}
			} else if err := routeInstance(c, engine); err != nil {
				return err
			}
			var res control.GrantListResult
			if err := credentialCall(c, state, control.GrantListCommand, control.GrantListArgs{Stack: stack, User: user}, &res); err != nil {
				return err
			}
			rows := make([]output.Row, 0, len(res.Grants))
			for _, g := range res.Grants {
				rows = append(rows, grantRow(g))
			}
			columns := []string{"id", "user", "uid", "action", "stack", "expires", "state", "created_at", "created_by"}
			if err := output.Encode(c.OutOrStdout(), program.Format(), output.Collection{Columns: columns, Rows: rows}); err != nil {
				return err
			}
			if res.Problem != "" {
				return &cli.Failure{Code: output.CodeRuntime, Message: "the grants file cannot be read, so no grant counts: " + plainText(res.Problem),
					NextStep: "Have a person with root check the grants file in the state folder of the deploy service; qhost never rewrites it."}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&stack, "stack", "", "only the grants for this stack")
	cmd.Flags().StringVar(&user, "user", "", "only the grants of this user name")
	addEngineFlag(cmd, &engine, optionalStackEngineUsage, grantEngineNote)
	return cmd
}

func grantRow(g control.GrantInfo) output.Row {
	state := g.Invalid
	if state == "" {
		state = "valid"
	}
	return output.Row{"id": g.ID, "user": plainText(g.User), "uid": g.UID, "action": g.Action, "stack": plainText(g.Stack),
		"expires": g.Expires, "state": state, "created_at": g.CreatedAt.UTC().Format(time.RFC3339), "created_by": g.CreatedBy}
}

func newGrantRemove(program *cobracli.Program, state *notRunning) *cobra.Command {
	var gate *cobracli.Gate
	var engine string
	cmd := &cobra.Command{
		Use:   "remove <id>",
		Short: "Remove a grant by its ID",
		Long: "remove withdraws the grant with that ID, also one that is no longer valid. 'qhost grant list' shows the\n" +
			"IDs. It first shows the grant on standard error; without --confirm it stops there with exit code 2 and\n" +
			"changes nothing. Run it with sudo.",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) != 1 {
				return usageFailure("grant remove takes exactly one argument, the ID of the grant", "Run 'sudo qhost grant remove <id>'; 'qhost grant list' shows the IDs.")
			}
			return nil
		},
		RunE: func(c *cobra.Command, args []string) error {
			if err := requireRoot("grant remove"); err != nil {
				return err
			}
			if err := routeInstance(c, engine); err != nil {
				return err
			}
			if unconfirmed := gate.Run(cobracli.Request{Changes: true}, func() error { return nil }); unconfirmed != nil {
				var list control.GrantListResult
				if err := credentialCall(c, state, control.GrantListCommand, control.GrantListArgs{}, &list); err != nil {
					return err
				}
				i := slices.IndexFunc(list.Grants, func(g control.GrantInfo) bool { return g.ID == args[0] })
				if i < 0 {
					return &cli.Failure{Code: output.CodeNotFound, Message: "no grant has this ID",
						NextStep: "Check the IDs with 'qhost grant list' and the engine with --engine."}
				}
				g := list.Grants[i]
				_, _ = fmt.Fprintf(c.ErrOrStderr(), "Remove the grant %s: user %s may no longer take the action %s for the stack %s on the engine %s without sudo.\n",
					g.ID, plainText(g.User), g.Action, plainText(g.Stack), targetOf(c.Context()).id())
				return unconfirmed
			}
			var res control.GrantRemoveResult
			if err := credentialCall(c, state, control.GrantRemoveCommand, control.GrantRemoveArgs{ID: args[0]}, &res); err != nil {
				return err
			}
			return output.Encode(c.OutOrStdout(), program.Format(), output.Object{Fields: []output.Field{
				{Name: "id", Value: res.Grant.ID}, {Name: "user", Value: plainText(res.Grant.User)}, {Name: "action", Value: res.Grant.Action},
				{Name: "stack", Value: plainText(res.Grant.Stack)}, {Name: "engine", Value: targetOf(c.Context()).id()}, {Name: "removed", Value: true},
			}})
		},
	}
	addEngineFlag(cmd, &engine, instanceEngineUsage, grantEngineNote)
	gate = program.AddGate(cmd, cobracli.Gates{Change: true})
	return cmd
}
