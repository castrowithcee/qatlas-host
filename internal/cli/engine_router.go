package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-kit/cli"
	"github.com/castrowithcee/qatlas-kit/output"

	"github.com/castrowithcee/qatlas-host/internal/client"
	"github.com/castrowithcee/qatlas-host/internal/control"
)

// The usage of --engine and the note in the help of the commands that take it.
const (
	engineIDs        = control.EngineSystem + " or " + rootlessPrefix + "<user>"
	engineChoice     = "--engine " + control.EngineSystem + " or --engine " + rootlessPrefix + "<user>"
	instanceDefault  = "(default: " + control.EngineSystem + ", or the only instance)"
	stackEngineUsage = "Docker engine of the stack: " + engineIDs + "; needed only when several engines have a stack of that name"
	// optionalStackEngineUsage is for a command whose stack may be left out.
	optionalStackEngineUsage = "Docker engine: " + engineIDs + "; with a stack needed only when several engines have it, " +
		"without one it names the deploy service " + instanceDefault
	folderEngineUsage = "Docker engine of the stack folder: " + engineIDs + "; otherwise the engine that has the folder bound"
	// optionalFolderEngineUsage is for a command whose --dir may be left out.
	optionalFolderEngineUsage = "Docker engine: " + engineIDs + "; with --dir needed only for a folder no engine has bound, " +
		"without --dir it names the deploy service " + instanceDefault
	// compareEngineUsage is for a command whose folder may not be bound yet.
	compareEngineUsage = "Docker engine: " + engineIDs + "; otherwise the engine that has the folder bound, for a folder " +
		"no engine has bound " + instanceDefault
	runEngineUsage      = "Docker engine whose deploy service recorded the run: " + engineIDs + "; needed only when several engines know the run"
	instanceEngineUsage = "Docker engine whose deploy service is meant: " + engineIDs + " " + instanceDefault

	stackEngineNote = "The command reaches the deploy service of the engine that has the stack: the system engine or the " +
		"instance of a rootless engine. When several engines have a stack of that name it stops and names them; " +
		engineChoice + " chooses one. A deploy service that cannot be asked is named in a warning."
	instanceRule = "the deploy service of the engine --engine names, else the one of the system engine, else the only " +
		"instance; with several instances and no deploy service of the system engine the command needs --engine."
	wholeEngineNote  = "Without a stack the command reaches " + instanceRule
	folderEngineNote = "The command reaches the deploy service of the engine that has the stack folder bound: the system " +
		"engine or the instance of a rootless engine. " + engineChoice + " names the engine instead. A deploy " +
		"service that cannot be asked is named in a warning."
	keyEngineNote = "Every deploy service keeps its own deploy keys. With --dir the command reaches the deploy service of the " +
		"engine that has the folder bound; for a folder no engine has bound yet, and without --dir, it reaches " +
		instanceRule + " A key never reaches another engine."
	compareEngineNote = "Every deploy service pins its own host keys. The command reaches the deploy service of the engine that " +
		"has the folder bound; for a folder no engine has bound yet it reaches " + instanceRule + " Name the engine that " +
		"'source add' will use, so that the key confirmed here is pinned there."
	probeEngineNote = "Every deploy service pins its own host keys; pinned tells whether the deploy service the command reaches " +
		"has this key pinned: " + instanceRule
	fetchEngineNote = "With --dir the command reaches the deploy service of the engine that has the folder bound. Without " +
		"--dir it fetches the folders of one deploy service: " + instanceRule
	credentialEngineNote = "Every deploy service keeps its own credentials: the one of the system engine and the instance of each " +
		"rootless engine. The command reaches " + instanceRule + " A credential never reaches another engine."
	notifyEngineNote = "Every deploy service keeps its own notification channels: the one of the system engine and the instance " +
		"of each rootless engine. The command reaches " + instanceRule + " A channel and its secret parts never reach " +
		"another engine."
	runEngineNote = "The command reaches the deploy service that recorded the run: on a host with several deploy services it asks " +
		"all of them and takes the one that knows the run. When several know it, it stops and names them; " + engineChoice +
		" chooses one. A deploy service that cannot be asked is named in a warning, or in the error when none knows the run."
	logsEngineNote = "With --stack the command reaches the deploy service of the engine that has the stack: the system engine " +
		"or the instance of a rootless engine. When several engines have a stack of that name it stops and names " +
		"them; " + engineChoice + " chooses one."
)

// addEngineFlag gives a command --engine with its usage and adds the note to its help.
func addEngineFlag(c *cobra.Command, engine *string, usage, note string) {
	c.Flags().StringVar(engine, "engine", "", usage)
	if c.Long != "" {
		c.Long += "\n\n"
	}
	c.Long += wrapHelp(note)
}

// helpWidth is the line length of the help texts.
const helpWidth = 110

// wrapHelp breaks text into lines of at most helpWidth characters at spaces.
func wrapHelp(text string) string {
	var b strings.Builder
	line := 0
	for i, w := range strings.Fields(text) {
		if i > 0 && line+1+len(w) > helpWidth {
			b.WriteByte('\n')
			line = 0
		} else if i > 0 {
			b.WriteByte(' ')
			line++
		}
		b.WriteString(w)
		line += len(w)
	}
	return b.String()
}

// engineTargets lists the deploy services of the host: the one of the system engine when it is set up, then
// every instance by user. Listing needs no root.
func (h folderHost) engineTargets() ([]engineTarget, error) {
	var out []engineTarget
	if deployInstalled() {
		out = append(out, engineTarget{})
	}
	list, err := h.instances()
	if err != nil {
		return nil, folderFailure("cannot list the deploy service instances", err)
	}
	for _, inst := range list {
		out = append(out, engineTarget{inst: &inst})
	}
	return out, nil
}

// engineAnswer is what the deploy service of one engine said about its stack folders and stacks; err is set
// when it could not be asked.
type engineAnswer struct {
	target engineTarget
	status control.StatusResult
	err    error
}

func (a engineAnswer) engine() string { return a.target.id() }

// askEngines asks the deploy service of every target once for its status, all at the same time, and answers in
// the order of targets. A service that cannot be asked keeps its error; the caller decides what it means.
func askEngines(ctx context.Context, targets []engineTarget) []engineAnswer {
	out := make([]engineAnswer, len(targets))
	var wg sync.WaitGroup
	for i, t := range targets {
		out[i].target = t
		wg.Go(func() {
			out[i].err = callFor(ctx, peerLimit, t.endpoint(), control.StatusCommand, nil, &out[i].status)
		})
	}
	wg.Wait()
	return out
}

func uncheckedText(a engineAnswer) string {
	return fmt.Sprintf("the deploy service of the %s engine could not be asked (%s); its stack folders and stacks are unknown",
		a.engine(), briefReason(a.err))
}

// namedEngine turns --engine into a target. Unlike resolveEngine it refuses the system engine when its deploy
// service is not set up: a named engine is never replaced by another one.
func (h folderHost) namedEngine(engine string) (engineTarget, error) {
	t, err := h.resolveEngine(engine)
	if err != nil {
		return t, err
	}
	if t.inst == nil && !deployInstalled() {
		return t, usageFailure("the deploy service of the "+control.EngineSystem+" engine is not set up",
			"Choose an engine whose deploy service is set up, or run 'sudo qhost deploy-service setup'.")
	}
	return t, nil
}

// instanceEngine chooses the deploy service of a setting of a whole engine: the named engine, else the system
// engine, else the only instance. With several instances and no deploy service of the system engine it
// refuses to choose. Without any deploy service it is the system engine, whose call then says it is not set up.
func (h folderHost) instanceEngine(engine string) (engineTarget, error) {
	if engine != "" {
		return h.namedEngine(engine)
	}
	targets, err := h.engineTargets()
	if err != nil {
		return engineTarget{}, err
	}
	if len(targets) <= 1 || targets[0].inst == nil {
		return firstTarget(targets), nil
	}
	return engineTarget{}, usageFailure("no deploy service of the "+control.EngineSystem+" engine is set up, and several instances are: "+
		"name the engine", "Pass --engine with one of "+targetIDs(targets)+".")
}

// engineItem is what a command names: a stack or a stack folder, with the test whether a service holds it.
// missing starts the message when every service answered and none holds it, with the next step of that error,
// as the deploy service words it for a single engine. byDefault lets such a command follow instanceEngine
// instead of failing.
type engineItem struct {
	what      string
	name      string
	missing   string
	next      string
	byDefault bool
	match     func(control.StatusResult) bool
}

func stackItem(name string) engineItem {
	return engineItem{what: "the stack " + name, name: name, missing: "no such stack",
		next: "List the stacks with 'qhost stack list'.", match: func(st control.StatusResult) bool {
			for _, s := range st.Engine.Stacks {
				if s.Name == name {
					return true
				}
			}
			return false
		}}
}

func folderItem(path string) engineItem {
	return engineItem{what: "the stack folder " + path, name: path, missing: "no source is bound to this folder",
		next: "List the bound folders with 'qhost source list'.", match: func(st control.StatusResult) bool {
			for _, src := range st.Engine.Sources {
				if src.Path == path {
					return true
				}
			}
			return false
		}}
}

// locate finds the one deploy service that holds every item. A named engine is taken as it is. On a host with
// one deploy service nothing is asked. Otherwise every service is asked: an item that several engines hold,
// that none of the answering ones holds, or items of different engines stop the command, because guessing
// could send a stack's request or secret to another engine. A service that could not be asked is a warning
// on warn once the items are found.
func (h folderHost) locate(ctx context.Context, warn io.Writer, engine string, items ...engineItem) (engineTarget, error) {
	if engine != "" {
		return h.namedEngine(engine)
	}
	targets, err := h.engineTargets()
	if err != nil {
		return engineTarget{}, err
	}
	if len(targets) <= 1 {
		return firstTarget(targets), nil
	}
	answers := askEngines(ctx, targets)
	var unchecked []engineAnswer
	for _, a := range answers {
		if a.err != nil {
			unchecked = append(unchecked, a)
		}
	}
	if len(unchecked) == len(answers) {
		// Nobody answered: the first service tells why, as it would on a host with only that service.
		return engineTarget{}, targetFailure(withEngine(ctx, unchecked[0].target), unchecked[0].err)
	}
	var chosen *engineAnswer
	var chosenItem engineItem
	byDefault := false
	for _, item := range items {
		var found []engineAnswer
		for _, a := range answers {
			if a.err == nil && item.match(a.status) {
				found = append(found, a)
			}
		}
		switch {
		case len(found) > 1:
			return engineTarget{}, usageFailure(fmt.Sprintf("%s exists in the engines %s", item.what, answerIDs(found)),
				"Choose one with --engine, for example --engine "+found[0].engine()+".")
		case len(found) == 0 && len(unchecked) > 0:
			return engineTarget{}, &cli.Failure{Code: output.CodeUnreachable,
				Message: fmt.Sprintf("%s is in none of the engines that answered; %s", item.what, uncheckedList(unchecked)),
				NextStep: "Start that deploy service and run the command again, or name its engine with --engine " +
					unchecked[0].engine() + ".", Cause: unchecked[0].err}
		case len(found) == 0 && item.byDefault:
			byDefault = true
			continue
		case len(found) == 0:
			return engineTarget{}, &cli.Failure{Code: output.CodeNotFound,
				Message:  fmt.Sprintf("%s: %s is in none of the engines %s", item.missing, item.name, answerIDs(answers)),
				NextStep: item.next}
		case chosen != nil && chosen.engine() != found[0].engine():
			return engineTarget{}, usageFailure(fmt.Sprintf("%s is in the %s engine and %s in the %s engine; one call reaches one engine",
				chosenItem.what, chosen.engine(), item.what, found[0].engine()), "Run the command once per engine.")
		}
		chosen, chosenItem = &found[0], item
	}
	for _, u := range unchecked {
		_, _ = fmt.Fprintln(warn, "warning: "+uncheckedText(u))
	}
	if chosen == nil && byDefault {
		return h.instanceEngine("")
	}
	if chosen == nil {
		return firstTarget(targets), nil
	}
	return chosen.target, nil
}

func firstTarget(targets []engineTarget) engineTarget {
	if len(targets) == 0 {
		return engineTarget{}
	}
	return targets[0]
}

func targetIDs(targets []engineTarget) string {
	ids := make([]string, len(targets))
	for i, t := range targets {
		ids[i] = t.id()
	}
	return strings.Join(ids, ", ")
}

func answerIDs(answers []engineAnswer) string {
	ids := make([]string, len(answers))
	for i, a := range answers {
		ids[i] = a.engine()
	}
	return strings.Join(ids, ", ")
}

func uncheckedList(unchecked []engineAnswer) string {
	parts := make([]string, len(unchecked))
	for i, u := range unchecked {
		parts[i] = u.engine() + " (" + briefReason(u.err) + ")"
	}
	return "the deploy service of the engine " + strings.Join(parts, ", ") + " could not be asked"
}

// routeStacks makes the deploy service that holds the stacks the target of every later call of the command.
func routeStacks(c *cobra.Command, state *notRunning, engine string, stacks ...string) error {
	items := make([]engineItem, len(stacks))
	for i, s := range stacks {
		items[i] = stackItem(s)
	}
	return route(c, state, engine, items...)
}

// routeFolder makes the deploy service that has the stack folder bound the target of every later call.
func routeFolder(c *cobra.Command, state *notRunning, engine, path string) error {
	return route(c, state, engine, folderItem(path))
}

// routeFolderKey is routeFolder for a deploy key, which exists before its folder is bound: when every service
// answered and none has the folder bound, the key follows the rule of the whole engine.
func routeFolderKey(c *cobra.Command, state *notRunning, engine, path string) error {
	item := folderItem(path)
	item.byDefault = true
	return route(c, state, engine, item)
}

func route(c *cobra.Command, state *notRunning, engine string, items ...engineItem) error {
	t, err := folders.locate(c.Context(), c.ErrOrStderr(), engine, items...)
	if err != nil {
		if errors.Is(err, client.ErrNotRunning) {
			state.seen = true
		}
		return err
	}
	c.SetContext(withEngine(c.Context(), t))
	return nil
}

// routeInstance makes the deploy service of a setting of a whole engine the target of every later call.
func routeInstance(c *cobra.Command, engine string) error {
	t, err := folders.instanceEngine(engine)
	if err != nil {
		return err
	}
	c.SetContext(withEngine(c.Context(), t))
	return nil
}
