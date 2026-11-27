package tui

import "testing"

// The goldens of the history of a stack folder: the list of commits, the files of the first and its diff, as root.

func goldenStackHistory(t *testing.T, prefix string, keys ...string) {
	t.Helper()
	for name, lang := range goldenLangs {
		for size, width := range goldenSizes {
			t.Run(name+"."+size, func(t *testing.T) {
				m := newTestModel(t, lang, historyBackend(), width, settingsRows)
				m.euid, m.human = func() int { return 0 }, func() int { return 1000 }
				openHistoryOfRow(m)
				send(m, keys...)
				screen := m.render()
				checkScreen(t, screen, width, settingsRows)
				checkGolden(t, prefix+"."+name+"."+size+".golden", screen+"\n")
			})
		}
	}
}

func TestGoldenStackHistoryList(t *testing.T) { goldenStackHistory(t, "stackhistory.list") }
func TestGoldenStackHistoryFiles(t *testing.T) {
	goldenStackHistory(t, "stackhistory.files", "down", "enter")
}
func TestGoldenStackHistoryDiff(t *testing.T) { goldenStackHistory(t, "stackhistory.diff", "<") }
