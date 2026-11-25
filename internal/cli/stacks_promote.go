package cli

import (
	"context"
	"sync"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// The texts that stand for the versions of a promote source that cannot be shown.
const (
	promoteMissing    = "(source missing)"
	promoteUnreadable = "(source unreadable)"
)

var unprovenTexts = map[string]string{
	control.UnprovenNotDeployed:  "(source not proven: never deployed)",
	control.UnprovenRun:          "(source not proven: last run did not succeed)",
	control.UnprovenFailedCommit: "(source not proven: has a failed commit)",
	control.UnprovenNotHealthy:   "(source not proven: not healthy)",
}

// promoteVersions says, for each stack of the replies that has a promote source, what the list shows beside
// its own versions: the versions the source runs, or why none are shown. It reads the source at the deploy
// service that has it; a stack without a source has no entry.
func promoteVersions(ctx context.Context, replies []engineReply[control.StacksResult]) map[engineStack]string {
	byName := map[string]engineTarget{}
	for _, r := range replies {
		for _, s := range r.value.Stacks {
			if _, ok := byName[s.Name]; !ok {
				byName[s.Name] = r.target
			}
		}
	}
	out := map[engineStack]string{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, r := range replies {
		for _, s := range r.value.Stacks {
			if s.PromoteSource == "" {
				continue
			}
			key := engineStack{r.target.id(), s.Name}
			at, ok := byName[s.PromoteSource]
			if !ok {
				out[key] = promoteMissing
				continue
			}
			wg.Go(func() {
				var src control.PromoteSource
				err := callFor(ctx, peerLimit, at.endpoint(), control.PromoteSourceCommand, control.PromoteSourceArgs{Stack: s.PromoteSource}, &src)
				text := promoteUnreadable
				if err == nil {
					text = promoteText(src, s.Images)
				}
				mu.Lock()
				out[key] = text
				mu.Unlock()
			})
		}
	}
	wg.Wait()
	return out
}

// promoteText is the versions of a proven source beside the images of a target, or the reason it has none.
func promoteText(src control.PromoteSource, images []control.ImageInfo) string {
	if why := src.Unproven(); why != "" {
		return unprovenTexts[why]
	}
	return joinVersions(src.VersionsFor(images))
}

func joinVersions(vs []control.PromoteVersion) string {
	text := ""
	for i, v := range vs {
		if i > 0 {
			text += ", "
		}
		text += v.Variable + "=" + v.Version
	}
	return text
}

// engineStack names a stack by its engine and name.
type engineStack struct{ engine, name string }
