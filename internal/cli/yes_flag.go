package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-kit/cobracli"

	"github.com/castrowithcee/qatlas-host/internal/tools"
)

// agentHelp is the help of --agent. It states the --yes contract that addYesFlags keeps.
const agentHelp = "no questions; --yes answers the questions of a command and every changing command accepts it, " +
	"but a change is confirmed only by --confirm"

// humanOnly are the changing command lines that a person runs at a terminal and that therefore take no --yes.
var humanOnly = map[string]bool{"proposal approve": true, "proposal reject": true, "proposal run": true}

// changingLines returns the command lines that have a tool entry of a class other than read.
func changingLines() map[string]bool {
	lines := map[string]bool{}
	for _, e := range tools.All() {
		if e.Class == tools.ClassRead {
			continue
		}
		for _, line := range e.CLI {
			lines[line] = true
		}
	}
	return lines
}

// addYesFlags gives every changing command line without a --yes flag one, so that a caller can set --yes on
// all of them. Where a command asks no question the flag does nothing. It must run after the commands are
// added to root.
func addYesFlags(program *cobracli.Program, root *cobra.Command) {
	changing := changingLines()
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		line := strings.TrimPrefix(c.CommandPath(), root.Name()+" ")
		if changing[line] && !humanOnly[line] && c.Flags().Lookup("yes") == nil {
			program.AddGate(c, cobracli.Gates{Question: true})
		}
		for _, s := range c.Commands() {
			walk(s)
		}
	}
	walk(root)
}
