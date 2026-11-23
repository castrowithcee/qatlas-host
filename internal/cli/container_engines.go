package cli

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-kit/cli"
	"github.com/castrowithcee/qatlas-kit/output"

	"github.com/castrowithcee/qatlas-host/internal/client"
	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/rootless"
)

// The notes of the container commands about engines that have no deploy service.
const (
	// noSudoEngineNote is said without root: a client that is not root cannot tell whether such engines exist.
	noSudoEngineNote = "Docker engines without a deploy service are shown only with sudo; whether there are any cannot be told without it."
	// containerEngineNote is the note in the help of the container commands that take --engine.
	containerEngineNote = "On a host with several deploy services the command reaches the one of the engine that has the container: " +
		"it asks all of them for the name or ID. When several engines have a container of that name it stops and names them; " +
		engineChoice + " chooses one. An action never falls to another engine: when a deploy service cannot be asked, " +
		"start, stop and restart stop and name it, and show and logs warn. A Docker engine without a deploy service is " +
		"shown by list for root only, read-only, and no command acts on its containers."
	containerListEngineNote = overviewEngineNote + " As root, list also shows the containers of Docker engines that have no " +
		"deploy service, marked read-only; without root it says that such engines are shown only with sudo."
)

// engineOrder sorts the system engine first, then the rootless engines by id.
func engineOrder(a, b string) int {
	switch {
	case a == b:
		return 0
	case a == control.EngineSystem:
		return -1
	case b == control.EngineSystem:
		return 1
	}
	return strings.Compare(a, b)
}

var kindRank = map[string]int{control.ContainerKindOwn: 0, control.ContainerKindForeign: 1, control.ContainerKindSingle: 2}

// sortGroups orders groups of several engines as the service orders those of one: own, foreign, single, each
// by name, and the same name by engine.
func sortGroups(groups []control.ContainerGroup) {
	sort.SliceStable(groups, func(i, j int) bool {
		a, b := groups[i], groups[j]
		if kindRank[a.Kind] != kindRank[b.Kind] {
			return kindRank[a.Kind] < kindRank[b.Kind]
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return engineOrder(a.Engine, b.Engine) < 0
	})
}

// unattended is what a client running as root reads from the Docker engines that have no deploy service.
type unattended struct {
	engines []rootless.Engine
	// notes say what the reader could not do or must know.
	notes []string
	// failed are the engines without a deploy service that could not be read, by engine.
	failed []control.EngineNotChecked
	// unread is set without root: such engines were not looked for.
	unread bool
}

// readUnattended reads the rootless engines that have no deploy service, as root only. Without root it reads
// nothing and returns the general note: whether such engines exist cannot be told. covered are the engines of
// the deploy services; an engine one of them serves is never read here.
func readUnattended(ctx context.Context, covered []engineTarget) unattended {
	if folders.euid() != 0 {
		return unattended{notes: []string{noSudoEngineNote}, unread: true}
	}
	scan := folders.detector.Scan(ctx)
	has := map[string]bool{}
	for _, t := range covered {
		has[t.id()] = true
	}
	var out unattended
	for _, e := range scan.Engines() {
		if has[e.ID] {
			continue
		}
		out.engines = append(out.engines, e)
		out.notes = append(out.notes, fmt.Sprintf("the Docker engine %s has no deploy service: its containers are shown read-only, "+
			"and no command acts on them", e.ID))
	}
	for _, nc := range scan.NotChecked {
		if !has[nc.Engine] {
			out.failed = append(out.failed, control.EngineNotChecked{Engine: nc.Engine, Reason: nc.Reason})
			out.notes = append(out.notes, fmt.Sprintf("the Docker engine %s has no deploy service and could not be read: %s", nc.Engine, nc.Reason))
		}
	}
	return out
}

// listedState maps the state word of an engine's list to the states of a group; a list has no health.
func listedState(engineState string) string {
	switch engineState {
	case "running":
		return control.ContainerOK
	case "restarting":
		return control.ContainerRestarting
	}
	return control.ContainerStopped
}

var stateRank = map[string]int{
	control.ContainerUnhealthy: 4, control.ContainerRestarting: 3, control.ContainerStopped: 2,
	control.ContainerStarting: 1, control.ContainerOK: 0,
}

// engineGroups turns the containers of an engine without a deploy service into read-only groups: one per
// Compose project, one per container without a project. None is own, because no stack of this host is bound there.
func engineGroups(e rootless.Engine) []control.ContainerGroup {
	byProject := map[string]*control.ContainerGroup{}
	var out []*control.ContainerGroup
	for _, c := range e.Containers {
		sum := control.ContainerSummary{Name: c.Name, Image: c.Image, State: listedState(c.State), EngineState: c.State}
		if c.Project == "" {
			out = append(out, &control.ContainerGroup{Kind: control.ContainerKindSingle, Engine: e.ID, ReadOnly: true,
				Name: c.Name, State: sum.State, Containers: []control.ContainerSummary{sum}})
			continue
		}
		g := byProject[c.Project]
		if g == nil {
			g = &control.ContainerGroup{Kind: control.ContainerKindForeign, Engine: e.ID, ReadOnly: true, Name: c.Project,
				State: control.ContainerOK, WorkingDir: c.WorkingDir}
			byProject[c.Project] = g
			out = append(out, g)
		}
		if stateRank[sum.State] > stateRank[g.State] {
			g.State = sum.State
		}
		g.Containers = append(g.Containers, sum)
	}
	groups := make([]control.ContainerGroup, len(out))
	for i, g := range out {
		groups[i] = *g
	}
	return groups
}

// containerListing is the merged answer of every engine.
type containerListing struct {
	control.ContainerListResult
	notes []string
}

// listContainers asks the deploy service of every engine for its containers and adds, as root, those of the
// engines without a deploy service. A deploy service that cannot be asked is a warning; the command fails
// only when none answers.
func listContainers(c *cobra.Command, state *notRunning, metrics bool) (containerListing, error) {
	limit := client.CallTimeout
	if metrics {
		limit = control.ContainerListTimeout
	}
	replies, err := askOverviewWith[control.ContainerListResult](c, state, control.ContainerListCommand,
		control.ContainerListArgs{Metrics: metrics}, limit)
	if err != nil {
		return containerListing{}, err
	}
	var out containerListing
	out.Groups = []control.ContainerGroup{}
	for _, r := range replies {
		out.Groups = append(out.Groups, r.value.Groups...)
		out.MetricsIncomplete = out.MetricsIncomplete || r.value.MetricsIncomplete
	}
	// A deploy service that failed still covers its engine: its containers are not read behind its back.
	all, err := folders.engineTargets()
	if err != nil {
		return containerListing{}, err
	}
	ro := readUnattended(c.Context(), all)
	for _, e := range ro.engines {
		out.Groups = append(out.Groups, engineGroups(e)...)
	}
	out.notes = ro.notes
	sortGroups(out.Groups)
	return out, nil
}

// containerAnswer is the answer of the deploy service of one engine for a container.
type containerAnswer = engineReply[control.ContainerDetail]

// locateContainer finds the deploy service of the engine that has the container and makes it the target of
// every later call. engine, when given, is taken as it is. Otherwise every deploy service is asked for the
// name or ID, and a name that several engines have, or that an engine without a deploy service has too, stops
// the command: guessing could act on another engine's container. For an action every deploy service must
// answer, because the one that cannot be asked might have a container of that name; for reading it is a
// warning. A container only an engine without a deploy service has is refused: nothing acts on it.
func locateContainer(c *cobra.Command, state *notRunning, engine, name string, action bool) (control.ContainerDetail, error) {
	args := control.ContainerGetArgs{Name: name}
	var d control.ContainerDetail
	if engine != "" {
		t, err := folders.namedEngine(engine)
		if err != nil {
			return d, err
		}
		c.SetContext(withEngine(c.Context(), t))
		return d, containerCall(c, state, client.CallTimeout, control.ContainerGetCommand, args, &d)
	}
	targets, err := folders.engineTargets()
	if err != nil {
		return d, err
	}
	var found, unchecked []containerAnswer
	if len(targets) <= 1 {
		t := firstTarget(targets)
		r := containerAnswer{target: t}
		r.err = callFor(c.Context(), client.CallTimeout, t.endpoint(), control.ContainerGetCommand, args, &r.value)
		switch {
		case r.err == nil:
			found = append(found, r)
		case isNoContainer(r.err):
		default:
			if errors.Is(r.err, client.ErrNotRunning) {
				state.seen = true
			}
			return d, containerFailure(withEngine(c.Context(), t), r.err)
		}
	} else {
		for _, r := range askEach[control.ContainerDetail](c.Context(), peerLimit, targets, control.ContainerGetCommand, args) {
			switch {
			case r.err == nil:
				found = append(found, r)
			case isNoContainer(r.err):
			default:
				unchecked = append(unchecked, r)
			}
		}
		if len(unchecked) == len(targets) {
			// Nobody answered: the first service tells why, as it would on a host with only that service.
			if errors.Is(unchecked[0].err, client.ErrNotRunning) {
				state.seen = true
			}
			return d, containerFailure(withEngine(c.Context(), unchecked[0].target), unchecked[0].err)
		}
	}
	var noService []string
	for _, e := range readUnattended(c.Context(), targets).engines {
		for _, ct := range e.Containers {
			if ct.Name == name {
				noService = append(noService, e.ID)
				break
			}
		}
	}
	switch {
	case len(found)+len(noService) > 1:
		ids := make([]string, 0, len(found)+len(noService))
		for _, f := range found {
			ids = append(ids, f.target.id())
		}
		text := fmt.Sprintf("the container %s exists in the engines %s", name, strings.Join(append(ids, noService...), ", "))
		if len(noService) > 0 {
			text += "; " + strings.Join(noService, ", ") + " has no deploy service, so no command acts on it"
		}
		next := "Choose one with --engine"
		if len(ids) > 0 {
			next += ", for example --engine " + ids[0]
		}
		return d, usageFailure(text, next+".")
	case len(found) == 0 && len(noService) == 1:
		return d, usageFailure(fmt.Sprintf("the container %s runs in the engine %s, which has no deploy service; "+
			"its containers are shown read-only and no command acts on them", name, noService[0]),
			"Set up its deploy service with 'sudo qhost deploy-service setup --rootless <user>'.")
	case len(found) == 0 && len(unchecked) > 0:
		return d, &cli.Failure{Code: output.CodeUnreachable,
			Message: fmt.Sprintf("the container %s is in none of the engines that answered; %s", name, uncheckedList(answersOf(unchecked))),
			NextStep: "Start that deploy service and run the command again, or name its engine with --engine " +
				unchecked[0].target.id() + ".", Cause: unchecked[0].err}
	case len(found) == 0:
		return d, &cli.Failure{Code: output.CodeNotFound, Message: "no such container",
			NextStep: "List the containers with 'qhost container list'."}
	case action && len(unchecked) > 0:
		return d, &cli.Failure{Code: output.CodeUnreachable,
			Message: fmt.Sprintf("the container %s is in the engine %s, but %s, and it might have a container of that name too",
				name, found[0].target.id(), uncheckedList(answersOf(unchecked))),
			NextStep: "Start that deploy service and run the command again, or name the engine with --engine " +
				found[0].target.id() + ".", Cause: unchecked[0].err}
	}
	for _, u := range unchecked {
		_, _ = fmt.Fprintf(c.ErrOrStderr(), "warning: the deploy service of the %s engine could not be asked (%s); its containers are not considered\n",
			u.target.id(), briefReason(u.err))
	}
	c.SetContext(withEngine(c.Context(), found[0].target))
	return found[0].value, nil
}

// isNoContainer reports whether the service answered that it has no such container.
func isNoContainer(err error) bool {
	var remote *control.Error
	return errors.As(err, &remote) && remote.Code == control.CodeNotFound
}

func answersOf(rs []containerAnswer) []engineAnswer {
	out := make([]engineAnswer, len(rs))
	for i, r := range rs {
		out[i] = engineAnswer{target: r.target, err: r.err}
	}
	return out
}
