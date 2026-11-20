package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/castrowithcee/qatlas-host/internal/control"
	"github.com/castrowithcee/qatlas-host/internal/rootless"
)

// rootlessEngines is the name of the engines that were not asked at all.
const rootlessEngines = "rootless"

const needsRootReason = "reading the rootless Docker engines of other users needs root; run the command with sudo"

// scanEngines asks the rootless engines of the host's users once. Without root it asks none: access to the
// socket of an engine is full control over it, so a client that is not root must not reach for it.
func (h folderHost) scanEngines(ctx context.Context) rootless.Scan {
	if h.euid() != 0 {
		return rootless.Scan{NotChecked: []rootless.NotChecked{{Engine: rootlessEngines, Reason: needsRootReason}}}
	}
	return h.detector.Scan(ctx)
}

// addEngines adds to the answer of the service the containers that run in the stack folders in a rootless
// engine, and the engines that could not be asked. It asks once per call and only for an answer that has
// stacks. It changes nothing on the host.
func addEngines(ctx context.Context, res control.SourceInspectResult) control.SourceInspectResult {
	if res.Case == control.SourceCaseReject || len(res.Stacks) == 0 {
		return res
	}
	scan := folders.scanEngines(ctx)
	own := targetOf(ctx).id()
	stacks := make([]control.SourceStack, len(res.Stacks))
	copy(stacks, res.Stacks)
	for i, st := range stacks {
		var list []control.SourceConflict
		list = append(list, st.Conflicts...)
		for _, cf := range scan.Conflicts(filepath.Join(res.Path, st.Folder)) {
			// The engine the folder is taken over into is where its containers belong.
			if cf.Engine == own {
				continue
			}
			list = append(list, control.SourceConflict{Container: cf.Container, Project: cf.Project, Engine: cf.Engine})
		}
		stacks[i].Conflicts = list
	}
	res.Stacks = stacks
	res.EnginesNotChecked = nil
	for _, nc := range scan.NotChecked {
		res.EnginesNotChecked = append(res.EnginesNotChecked, control.EngineNotChecked{Engine: nc.Engine, Reason: nc.Reason})
	}
	return res
}

// conflictCount is the number of containers of other engines or projects that run in the stack folders.
func conflictCount(res control.SourceInspectResult) int {
	n := 0
	for _, st := range res.Stacks {
		n += len(st.Conflicts)
	}
	return n
}

// conflictConsequence is the warning shown with the question when containers already run in the folders,
// and empty when none do.
func conflictConsequence(res control.SourceInspectResult) string {
	if conflictCount(res) == 0 {
		return ""
	}
	return "Containers of these stack folders already run; deploying would create duplicate containers and take their ports twice."
}

// conflictText is the line of the inspection for one conflict.
func conflictText(cf control.SourceConflict) string {
	return fmt.Sprintf("container %s of project %s runs in this folder in the %s engine; deploying would create duplicate containers and take their ports twice",
		cf.Container, cf.Project, engineLabel(cf.Engine))
}

func engineLabel(engine string) string {
	if engine == "" {
		return control.EngineSystem
	}
	return engine
}

func notCheckedText(nc control.EngineNotChecked) string {
	return fmt.Sprintf("engine %s was not checked: %s; containers there are unknown", nc.Engine, nc.Reason)
}

// engineSignature is the part of the effect of a source-add that comes from the engines: every conflict with
// its engine and every engine that was not checked, so that a changed situation changes the signature.
func engineSignature(res control.SourceInspectResult) []string {
	var out []string
	for _, st := range res.Stacks {
		for _, cf := range st.Conflicts {
			out = append(out, strings.Join([]string{"conflict", st.Name, engineLabel(cf.Engine), cf.Project, cf.Container}, "\x00"))
		}
	}
	for _, nc := range res.EnginesNotChecked {
		out = append(out, strings.Join([]string{"not-checked", nc.Engine, nc.Reason}, "\x00"))
	}
	return out
}
