package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-host/internal/tools"
)

// notFunctions are the command lines that are no function of qhost for an agent to call, each with its reason.
var notFunctions = map[string]string{
	"tui":                "opens the terminal interface, which calls the same functions itself",
	"manpage":            "writes the manual pages of the program",
	"version":            "prints the version of the program",
	"run":                "runs the host service process in the foreground",
	"deploy-service run": "runs the deploy service process in the foreground",
	"hook-service run":   "runs the webhook receiver process in the foreground",
}

// actingGroups are the commands that have subcommands and act on their own as well.
var actingGroups = map[string]bool{"audit": true, "update": true}

// commandLines returns every command line of the tree that does something: each command without
// subcommands and each acting group. Help and completion are the framework's own.
func commandLines(t *testing.T) map[string]bool {
	t.Helper()
	root, _, _, err := newRoot()
	if err != nil {
		t.Fatal(err)
	}
	lines := map[string]bool{}
	groups := map[string]bool{}
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		line := strings.TrimPrefix(c.CommandPath(), root.Name()+" ")
		switch {
		case c == root:
		case c.Name() == "help" || c.Name() == "completion":
			return
		case !c.Runnable() && !c.HasSubCommands():
			// A help topic the kit registered; it has no action.
			return
		case !c.HasSubCommands():
			lines[line] = true
		case actingGroups[line]:
			lines[line] = true
			groups[line] = true
		}
		for _, s := range c.Commands() {
			walk(s)
		}
	}
	walk(root)
	for g := range actingGroups {
		if !groups[g] {
			t.Errorf("%q is listed as an acting group but is none", g)
		}
	}
	return lines
}

func TestEveryCommandLineHasAToolEntry(t *testing.T) {
	lines := commandLines(t)
	mapped := map[string]bool{}
	for _, e := range tools.All() {
		for _, line := range e.CLI {
			if !lines[line] {
				t.Errorf("entry %s names the command line %q, which does not exist", e.Name, line)
			}
			mapped[line] = true
		}
	}
	for line := range lines {
		_, excepted := notFunctions[line]
		switch {
		case excepted && mapped[line]:
			t.Errorf("%q is excepted but also has an entry", line)
		case !excepted && !mapped[line]:
			t.Errorf("the command line %q has no entry in the tool table", line)
		}
	}
	for line := range notFunctions {
		if !lines[line] {
			t.Errorf("the exception %q is not a command line any more", line)
		}
	}
}

func TestEveryChangingCommandLineTakesYes(t *testing.T) {
	root, _, _, err := newRoot()
	if err != nil {
		t.Fatal(err)
	}
	changing := changingLines()
	found := map[string]bool{}
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		line := strings.TrimPrefix(c.CommandPath(), root.Name()+" ")
		if changing[line] {
			found[line] = true
			if has := c.Flags().Lookup("yes") != nil; has == humanOnly[line] {
				t.Errorf("%q: has --yes = %v, human-only = %v", line, has, humanOnly[line])
			}
		}
		for _, s := range c.Commands() {
			walk(s)
		}
	}
	walk(root)
	for line := range changing {
		if !found[line] {
			t.Errorf("changing line %q is not in the tree", line)
		}
	}
}

func TestYesChangesNothingOnACommandWithoutAQuestion(t *testing.T) {
	startBound(t, okDeployer{})
	settle(t)
	code, out, stderr := run(t, "stack", "unpin", "plain-stack", "--agent", "--yes", "--output", "json")
	if code != 0 || !strings.Contains(out, "plain-stack") {
		t.Fatalf("unpin --yes: %d %q %q", code, out, stderr)
	}
}

func TestYesDoesNotConfirmAChange(t *testing.T) {
	startGrantService(t)
	code, _, stderr := run(t, "grant", "add", "demo-admin", "stack.restart", "plain-stack", "--agent", "--yes")
	if code == 0 || !strings.Contains(stderr, "--confirm") {
		t.Fatalf("grant add --yes: %d %q", code, stderr)
	}
}
