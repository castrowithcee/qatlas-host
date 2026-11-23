package tui

import (
	"testing"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

// The goldens of moving a foreign project into a source that exists, without root, at the narrow width: the choice,
// the preview with a block, and the last question with the command.

// goldenMove renders the screen after steps in both languages; the fake sudo prints the preview.
func goldenMove(t *testing.T, prefix string, preview control.SourceMoveResult, steps ...string) {
	t.Helper()
	for name, lang := range goldenLangs {
		t.Run(name, func(t *testing.T) {
			m, _ := moveModel(t, lang, moveBackend(), false, ctNarrow, sudoFolderRows, jsonOf(t, preview), "")
			toMoveChoice(m)
			send(m, steps...)
			screen := m.render()
			checkScreen(t, screen, ctNarrow, sudoFolderRows)
			checkGolden(t, prefix+"."+name+".narrow.golden", screen+"\n")
		})
	}
}

func TestGoldenMoveChoice(t *testing.T) {
	goldenMove(t, "containers.move.choice", movePreview(nil), "enter")
}

func TestGoldenMoveBlocked(t *testing.T) {
	goldenMove(t, "containers.move.blocked", movePreview(func(r *control.SourceMoveResult) {
		r.Blocks = []control.SourceMoveIssue{{Code: control.MoveOtherFilesystem,
			Reason: "the folder and the source are on different file systems, so its data would have to be copied",
			Fix:    "Choose a source on the same file system as the project folder."}}
	}), "right", "f2")
}

func TestGoldenMoveConfirm(t *testing.T) {
	goldenMove(t, "containers.move.confirm", movePreview(nil), "right", "f2")
}
