package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	kittui "github.com/castrowithcee/qatlas-kit/tui"
)

// Securing the .git of a stack folder that carries the finding git-unsafe: the preview names what changes in the
// words of the command, and the confirmation runs `source secure --yes` through sudo, or the host side as root.
// The preview is the same text in both cases, because only root can read every entry of a .git that others own.

// actSecure makes the .git of a stack folder with the finding git-unsafe root-only.
const actSecure kittui.ActionID = "secure"

const opGitSecure = opReturn

// FolderSecurer is the host side of securing a .git as root, like `source secure`, for a folder of the system
// engine. It returns how many entries changed. The host side of the folders offers it besides Folders.
type FolderSecurer interface {
	SecureGit(ctx context.Context, dir string) (int, error)
}

func secureKeys() []kittui.Action {
	return []kittui.Action{act(actSecure, kittui.LevelLetter, textKeySecure, "&")}
}

// securedGit is the answer of securing a folder: the folder and how many entries changed.
type securedGit struct {
	path    string
	changed int
}

func (s securedGit) finish(m *Model, done func(string, ...string)) tea.Cmd {
	done(m.text.Lookup(textDoneSecured, s.path))
	return m.reloadNow()
}

// stackFolderAction is folderAction with the key to secure the marked folder.
func (m *Model) stackFolderAction(id kittui.ActionID) (tea.Cmd, bool) {
	if id != actSecure || m.set.keyOpen {
		return m.folderAction(id)
	}
	if _, ok := m.selectedFolder(); !ok {
		return nil, false
	}
	return m.askFolderSecure(), true
}

func (m *Model) askFolderSecure() tea.Cmd {
	f, _ := m.selectedFolder()
	t := m.text
	switch {
	case f.GitUnsafe == "":
		m.warn(t.Lookup(textFolderSecureNone))
		return nil
	}
	host, ok := m.folders.(FolderSecurer)
	if m.euid() == 0 && !ok {
		m.warn(t.Lookup(textFoldersNone))
		return nil
	}
	ctx, path := m.engineCtx(f.Engine), f.Path
	m.askSetting(settingChange{title: textTitleFolders, question: t.Lookup(textAskSecure, path), consequence: t.Lookup(textWhatSecure),
		details: []string{t.Lookup(textDetailGitUnsafe, f.GitUnsafe)}, op: opGitSecure, name: path,
		direct: func(b Backend) (any, error) {
			n, err := host.SecureGit(ctx, path)
			if err != nil {
				return nil, hostError{err}
			}
			// The fetch shows that git runs in the folder again; the finding goes at the next poll.
			_, _ = b.RepoFetch(ctx, path)
			return securedGit{path: path, changed: n}, nil
		},
		args:   m.engineArgs(f.Engine, []string{"source", "secure", "--yes", "--output", "json", "--", path}),
		decode: decodeSecured})
	return nil
}

// decodeSecured reads what `source secure` prints.
func decodeSecured(r sudoRun) (any, bool) {
	var v struct {
		Path    string `json:"path"`
		Changed int    `json:"changed"`
	}
	if !r.decode(&v) || v.Path == "" {
		return nil, false
	}
	return securedGit{path: v.Path, changed: v.Changed}, true
}
