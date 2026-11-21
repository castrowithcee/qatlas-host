package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/castrowithcee/qatlas-kit/cli"
	"github.com/castrowithcee/qatlas-kit/localipc"
	"github.com/castrowithcee/qatlas-kit/output"

	"github.com/castrowithcee/qatlas-host/internal/client"
	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/deploysetup"
)

// rootlessPrefix starts the engine id of the rootless engine of a user.
const rootlessPrefix = "rootless:"

// engineTarget is the Docker engine a stack folder is taken over into, and with it the deploy service that
// serves the engine. The zero value is the system engine.
type engineTarget struct {
	inst *deploysetup.Instance
}

type engineKey struct{}

// withEngine returns ctx carrying the target, so that every step of a command that takes a folder over talks
// to the same instance.
func withEngine(ctx context.Context, t engineTarget) context.Context {
	return context.WithValue(ctx, engineKey{}, t)
}

func targetOf(ctx context.Context) engineTarget {
	t, _ := ctx.Value(engineKey{}).(engineTarget)
	return t
}

// id is the engine id as in SourceConflict.Engine.
func (t engineTarget) id() string {
	if t.inst == nil {
		return control.EngineSystem
	}
	return rootlessPrefix + t.inst.User()
}

// endpoint is the channel of the deploy service of the engine.
func (t engineTarget) endpoint() localipc.ServiceEndpoint {
	if t.inst == nil {
		return serviceEndpoint()
	}
	return instanceEndpoint(*t.inst)
}

// unit is the systemd unit of the deploy service of the engine.
func (t engineTarget) unit() string {
	if t.inst == nil {
		return serviceAccount
	}
	return t.inst.Unit()
}

// account is the user the deploy service runs as; it owns the stack folders the client creates.
func (t engineTarget) account() string {
	if t.inst == nil {
		return serviceAccount
	}
	return t.inst.User()
}

// dropIn is the path of the drop-in that lists the stack folders the service may write, below root.
func (t engineTarget) dropIn(root string) string {
	if t.inst == nil {
		return filepath.Join(root, stackFoldersDropIn)
	}
	return filepath.Join(root, t.inst.StackFoldersDropIn())
}

// resolveEngine turns an engine id from the command line into a target. A user name enters paths, unit names
// and drop-in text only as the name of an instance that is set up on the host.
func (h folderHost) resolveEngine(engine string) (engineTarget, error) {
	if engine == "" || engine == control.EngineSystem {
		return engineTarget{}, nil
	}
	name, ok := strings.CutPrefix(engine, rootlessPrefix)
	if !ok {
		return engineTarget{}, usageFailure(fmt.Sprintf("the engine %q is not known: use %s or %s<user>", engine, control.EngineSystem, rootlessPrefix),
			"Pass --engine "+control.EngineSystem+" or --engine "+rootlessPrefix+"<user>.")
	}
	list, err := h.instances()
	if err != nil {
		return engineTarget{}, folderFailure("cannot list the deploy service instances", err)
	}
	for _, inst := range list {
		if inst.User() == name {
			return engineTarget{inst: &inst}, nil
		}
	}
	return engineTarget{}, usageFailure(fmt.Sprintf("no deploy service is set up for the rootless engine of %q", name),
		"Set it up with 'sudo qhost deploy-service setup --rootless <user>', or choose another engine.")
}

// selectEngine resolves the engine id and makes it the target of every later call of the command.
func (h folderHost) selectEngine(c *cobra.Command, engine string) (engineTarget, error) {
	t, err := h.resolveEngine(engine)
	if err != nil {
		return t, err
	}
	c.SetContext(withEngine(c.Context(), t))
	return t, nil
}

// showUnchecked prints the engines that could not be asked as warnings and returns the text that opens the
// question: the folder is then taken over only after the operator confirmed it.
func showUnchecked(w io.Writer, unchecked []string) string {
	for _, u := range unchecked {
		_, _ = fmt.Fprintln(w, "warning: "+u)
	}
	return uncheckedQuestion
}

const uncheckedQuestion = "Uniqueness across the engines could not be checked completely. "

// lookupOwner returns the ids of the account that owns the folders of the target.
func lookupOwner(name string) (uid, gid int, err error) {
	u, err := user.Lookup(name)
	if err != nil {
		return 0, 0, err
	}
	if uid, err = strconv.Atoi(u.Uid); err != nil {
		return 0, 0, err
	}
	gid, err = strconv.Atoi(u.Gid)
	return uid, gid, err
}

// targetFailure names the instance when the service of a rootless engine does not run; the generic text would
// point at the unit of the system engine.
func targetFailure(ctx context.Context, err error) error {
	t := targetOf(ctx)
	if t.inst != nil && errors.Is(err, client.ErrNotRunning) {
		return &cli.Failure{Code: output.CodeUnreachable, Cause: err,
			Message:  "the deploy service for the rootless engine of " + t.inst.User() + " is not running",
			NextStep: "Start it with 'sudo systemctl start " + t.inst.Unit() + "', then run the command again."}
	}
	return failureOf(err)
}

// overlapping reports whether one folder is or contains the other.
func overlapping(a, b string) bool {
	return a == b || strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/")
}

// peerLimit bounds the question to one other deploy service.
const peerLimit = 10 * time.Second

// askPeers asks the deploy services of every other engine once. Only root may ask all of them, and the client
// runs as root here. A service that cannot be asked is kept with its error: the operator decides.
func (h folderHost) askPeers(ctx context.Context, t engineTarget) ([]engineAnswer, error) {
	all, err := h.engineTargets()
	if err != nil {
		return nil, err
	}
	var peers []engineTarget
	for _, p := range all {
		if p.id() != t.id() {
			peers = append(peers, p)
		}
	}
	sort.SliceStable(peers, func(a, b int) bool { return peers[a].id() < peers[b].id() })
	return askEngines(ctx, peers), nil
}

// checkPath refuses a folder that is or contains a stack folder of another engine, or lies inside one.
func checkPath(peers []engineAnswer, path string) error {
	for _, p := range peers {
		for _, src := range p.status.Engine.Sources {
			if p.err == nil && overlapping(path, src.Path) {
				return usageFailure(fmt.Sprintf("the folder %s overlaps the stack folder %s of the %s engine", path, src.Path, p.engine()),
					"Choose a folder that does not overlap a stack folder that is in use.")
			}
		}
	}
	return nil
}

// checkStacks refuses a stack name that another engine already has.
func checkStacks(peers []engineAnswer, stacks []control.SourceStack) error {
	for _, p := range peers {
		for _, mine := range stacks {
			for _, theirs := range p.status.Engine.Stacks {
				if p.err == nil && mine.Name == theirs.Name {
					return usageFailure(fmt.Sprintf("the %s engine already has a stack named %s", p.engine(), mine.Name),
						"Rename the stack in the folder, or take the folder over into that engine.")
				}
			}
		}
	}
	return nil
}

// uncheckedPeers describes the services that could not be asked, one line each.
func uncheckedPeers(peers []engineAnswer) []string {
	var out []string
	for _, p := range peers {
		if p.err != nil {
			out = append(out, uncheckedText(p))
		}
	}
	return out
}

func briefReason(err error) string {
	var remote *control.Error
	switch {
	case errors.Is(err, client.ErrNotRunning):
		return "not running"
	case errors.Is(err, client.ErrNoAccess):
		return "no access to its channel"
	case errors.As(err, &remote):
		return "refused: " + remote.Message
	}
	return err.Error()
}

// suggestedEngines lists the engines other than the system engine in which containers of the folder run, so
// that the operator can take the folder over there.
func suggestedEngines(res control.SourceInspectResult) []string {
	seen := map[string]bool{}
	var out []string
	for _, st := range res.Stacks {
		for _, cf := range st.Conflicts {
			if e := engineLabel(cf.Engine); e != control.EngineSystem && !seen[e] {
				seen[e] = true
				out = append(out, e)
			}
		}
	}
	sort.Strings(out)
	return out
}

func suggestionText(path string, engines []string) string {
	return fmt.Sprintf("containers of this folder run in the engine %s; to deploy it there, take it over with 'qhost source add %s --engine %s'",
		strings.Join(engines, ", "), path, engines[0])
}
