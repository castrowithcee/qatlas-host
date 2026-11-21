package hostchange

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"golang.org/x/sys/unix"
)

// The file actions write one text file in place or create one folder; the tree actions of filemove.go rename,
// copy and remove. Their steps resolve the path one entry at a time from the root and never follow a link
// (filestep.go, filetree.go).
const (
	// ActionFileWrite writes the content on standard input to a file: an existing file in place, a missing one
	// new in an existing folder. Its arguments are the path and optionally optionSystemPath.
	ActionFileWrite = "file.write"
	// ActionFileCreateFolder creates one folder in an existing folder. Its arguments are the path and
	// optionally optionSystemPath.
	ActionFileCreateFolder = "file.create-folder"
)

// MaxFileContent bounds the content file.write takes and the previous content it keeps in the journal. It equals
// the limit of the stack files, which this package may not import.
const MaxFileContent = 1 << 20

const (
	contentLabel         = "<content>"
	previousContentLabel = "<previous content>"
	recordedStateLabel   = "<recorded state>"
)

// RegisterFileActions adds the file actions.
func RegisterFileActions(r *Registry) error {
	// The content comes in like a secret: only on standard input, never as an argument or in the journal. That
	// also keeps the runner from repeating the write without its content when an undo has to be taken back.
	if err := r.Register(Definition{ID: ActionFileWrite, WantsSecret: true, Plan: planFileWrite, Inverse: inverseFileWrite}); err != nil {
		return err
	}
	if err := r.Register(Definition{ID: ActionFileCreateFolder, Plan: planCreateFolder, Inverse: inverseCreateFolder}); err != nil {
		return err
	}
	return registerTreeActions(r)
}

// isText accepts content without a NUL byte that is valid UTF-8.
func isText(data []byte) bool { return bytes.IndexByte(data, 0) < 0 && utf8.Valid(data) }

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// journalStateDir holds the journal and its lock file. It is a variable so tests can point it at a fake root.
var journalStateDir = filepath.Dir(DefaultJournalDir)

// lockedReason names why nothing is written at path, or is empty.
func lockedReason(path string) string {
	if lexicallyUnder(journalStateDir, path) {
		return "it holds the journal of the host changes, which is the way back (undo)"
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for i, p := range parts {
		switch {
		case p == ".git":
			return "nothing in a .git folder is written; it belongs to Git"
		case p == ".qatlas" && i+1 < len(parts) && parts[i+1] == "qhost":
			return "the settings of a stack in .qatlas/qhost are changed through the stack settings only"
		}
	}
	return ""
}

// checkFilePath is the lexical check of a path the file actions and their steps write at.
func checkFilePath(path string) error {
	if err := checkCleanPath(path); err != nil {
		return err
	}
	if reason := lockedReason(path); reason != "" {
		return Refusef("%s is locked: %s", path, reason)
	}
	return nil
}

// checkCleanPath is the lexical check of every path of a file action, also one it only reads.
func checkCleanPath(path string) error {
	switch {
	case !filepath.IsAbs(path) || strings.ContainsAny(path, "\x00\n\r"):
		return Argf("the path must be absolute and without control characters")
	case filepath.Clean(path) != path:
		return Argf("the path must be given clean, without \"..\", \"//\" or a trailing \"/\"")
	case path == "/":
		return Argf("the path must name an entry below /")
	}
	return nil
}

// fileTarget checks the arguments of a file action: the path, then the acknowledgement of a system path, whose
// refusal names consequence.
func fileTarget(action string, args []string, consequence string) (string, bool, error) {
	if len(args) < 1 || len(args) > 2 || (len(args) == 2 && args[1] != optionSystemPath) {
		return "", false, Argf("%s needs an absolute path and optionally %s", action, optionSystemPath)
	}
	path := args[0]
	if err := checkFilePath(path); err != nil {
		return "", false, err
	}
	if isPseudoFS(path) {
		return "", false, Refusef("%s lies in a virtual file system; it is not written", path)
	}
	system := IsSystemPath(path)
	if system && len(args) < 2 {
		return "", false, Refusef("%s is a system path: %s; it needs the acknowledgement of the system path", path, consequence)
	}
	return path, system, nil
}

const systemSentence = " This is a system path."

// writeInput is the standard input of stepFileWrite.
type writeInput struct {
	Content []byte `json:"content"`
}

// fileMeta is the identity, owner and permissions of a file at one moment.
type fileMeta struct {
	Dev uint64 `json:"dev"`
	Ino uint64 `json:"ino"`
	UID int    `json:"uid"`
	GID int    `json:"gid"`
	// Mode holds the permission and the special bits.
	Mode uint32 `json:"mode"`
}

func metaOf(st *unix.Stat_t) fileMeta {
	return fileMeta{Dev: uint64(st.Dev), Ino: st.Ino, UID: int(st.Uid), GID: int(st.Gid), Mode: st.Mode & 0o7777}
}

// fileResult is a file right after a change.
type fileResult struct {
	Exists bool      `json:"exists"`
	Meta   *fileMeta `json:"meta,omitempty"`
	SHA256 string    `json:"sha256,omitempty"`
}

// fileState is the journaled state of file.write and the standard input of the step that restores it.
type fileState struct {
	Path    string    `json:"path"`
	Existed bool      `json:"existed"`
	Content []byte    `json:"content,omitempty"`
	Meta    *fileMeta `json:"meta,omitempty"`
	// After is set once the change ran. An undo overwrites or removes the file only while it is still the
	// same file with the same content, so that it never destroys what was written after the change.
	After *fileResult `json:"after,omitempty"`
}

func planFileWrite(args []string, content []byte) (Plan, error) {
	path, system, err := fileTarget(ActionFileWrite, args, "a wrong file there can stop services or keep the host from starting")
	if err != nil {
		return Plan{}, err
	}
	if len(content) > MaxFileContent {
		return Plan{}, Argf("the content is larger than %d bytes", MaxFileContent)
	}
	if !isText(content) {
		return Plan{}, Argf("the content is binary (a NUL byte or no valid UTF-8); only text files are written")
	}
	input, err := json.Marshal(writeInput{Content: content})
	if err != nil {
		return Plan{}, err
	}
	sentence := fmt.Sprintf("Write the given content to %s in place: an existing file keeps its inode, owner, group and "+
		"permissions, a new file gets the owner and group of its folder.", path)
	if system {
		sentence += systemSentence
	}
	var state fileState
	return Plan{
		Sentence: sentence,
		Commands: []Command{{Argv: []string{stepFileWrite, path}, Builtin: true, Secret: input, SecretLabel: contentLabel}},
		// A write in place can stop halfway; the inverse writes the previous content over any such state.
		PartialSteps: true,
		Redact:       [][]byte{content},
		Capture: func(context.Context) (json.RawMessage, error) {
			st, err := captureFile(path)
			if err != nil {
				return nil, err
			}
			state = st
			return json.Marshal(st)
		},
		Seal: func(context.Context) (json.RawMessage, error) {
			after, err := resultOf(path)
			if err != nil {
				return nil, err
			}
			state.After = &after
			return json.Marshal(state)
		},
		Check: func(context.Context) error { return checkWritten(state, content) },
	}, nil
}

// captureFile reads the file before the change. A file that is not text up to MaxFileContent is refused,
// because neither the journal nor the file browser keeps it.
func captureFile(path string) (fileState, error) {
	dir, name, err := openParentDir(path)
	if err != nil {
		return fileState{}, err
	}
	defer unix.Close(dir)
	fd, meta, err := openFileAt(dir, name, path, unix.O_RDONLY)
	if errors.Is(err, errNoEntry) {
		return fileState{Path: path}, nil
	}
	if err != nil {
		return fileState{}, err
	}
	defer unix.Close(fd)
	data, err := readFD(fd)
	if errors.Is(err, errTooLarge) {
		return fileState{}, Refusef("%s is larger than %d bytes and is not written here", path, MaxFileContent)
	}
	if err != nil {
		return fileState{}, fmt.Errorf("read %s: %w", path, err)
	}
	if !isText(data) {
		return fileState{}, Refusef("%s holds binary content and is not written here", path)
	}
	return fileState{Path: path, Existed: true, Content: data, Meta: &meta}, nil
}

// resultOf reads the file as it is now.
func resultOf(path string) (fileResult, error) {
	dir, name, err := openParentDir(path)
	if err != nil {
		return fileResult{}, err
	}
	defer unix.Close(dir)
	fd, meta, err := openFileAt(dir, name, path, unix.O_RDONLY)
	if errors.Is(err, errNoEntry) {
		return fileResult{}, nil
	}
	if err != nil {
		return fileResult{}, err
	}
	defer unix.Close(fd)
	data, err := readFD(fd)
	if err != nil {
		return fileResult{}, fmt.Errorf("read %s: %w", path, err)
	}
	return fileResult{Exists: true, Meta: &meta, SHA256: sha256Hex(data)}, nil
}

func checkWritten(state fileState, content []byte) error {
	a := state.After
	switch {
	case a == nil || !a.Exists || a.Meta == nil:
		return fmt.Errorf("%s is missing after the change", state.Path)
	case a.SHA256 != sha256Hex(content):
		return fmt.Errorf("%s does not hold the new content after the change", state.Path)
	}
	now := *a.Meta
	if state.Existed {
		was := *state.Meta
		if now.Dev != was.Dev || now.Ino != was.Ino {
			return fmt.Errorf("%s was replaced instead of written in place", state.Path)
		}
		if now.UID != was.UID || now.GID != was.GID || now.Mode != was.Mode {
			return fmt.Errorf("owner, group or permissions of %s changed from %d:%d %04o to %d:%d %04o",
				state.Path, was.UID, was.GID, was.Mode, now.UID, now.GID, now.Mode)
		}
		return nil
	}
	parent, err := statFolder(filepath.Dir(state.Path))
	if err != nil {
		return err
	}
	if now.UID != parent.UID || now.GID != parent.GID || now.Mode != newFileMode(parent.Mode) {
		return fmt.Errorf("%s has %d:%d %04o, not the owner and group of its folder with mode %04o",
			state.Path, now.UID, now.GID, now.Mode, newFileMode(parent.Mode))
	}
	return nil
}

func inverseFileWrite(prev json.RawMessage) (Plan, error) {
	var st fileState
	if err := decodeState(prev, &st); err != nil {
		return Plan{}, err
	}
	if err := checkFileState(st); err != nil {
		return Plan{}, err
	}
	input, err := json.Marshal(st)
	if err != nil {
		return Plan{}, err
	}
	cmd := Command{Argv: []string{stepFileRestore, st.Path}, Builtin: true, Secret: input, SecretLabel: recordedStateLabel}
	sentence := fmt.Sprintf("Remove %s, which the change created, if it is unchanged since.", st.Path)
	if st.Existed {
		cmd.SecretLabel = previousContentLabel
		sentence = fmt.Sprintf("Write the previous content back to %s in place and restore its owner, group and permissions, "+
			"if it is unchanged since the change.", st.Path)
	}
	return Plan{Sentence: sentence, Commands: []Command{cmd}, Redact: [][]byte{st.Content}}, nil
}

func checkFileState(st fileState) error {
	bad := errors.New("the recorded state of the file is not usable")
	if checkFilePath(st.Path) != nil || len(st.Content) > MaxFileContent {
		return bad
	}
	if st.Existed != (st.Meta != nil) || (!st.Existed && len(st.Content) > 0) {
		return bad
	}
	if m := st.Meta; m != nil && (m.UID < 0 || m.GID < 0 || m.Mode > 0o7777) {
		return bad
	}
	if a := st.After; a != nil && a.Exists && (a.Meta == nil || len(a.SHA256) != sha256.Size*2) {
		return bad
	}
	return nil
}

// folderCreateState is the journaled state of file.create-folder: the folder did not exist.
type folderCreateState struct {
	Path string `json:"path"`
}

func planCreateFolder(args []string, _ []byte) (Plan, error) {
	path, system, err := fileTarget(ActionFileCreateFolder, args, "a folder there can change how the system or its services behave")
	if err != nil {
		return Plan{}, err
	}
	sentence := fmt.Sprintf("Create the folder %s with the owner and group of the folder it lies in.", path)
	if system {
		sentence += systemSentence
	}
	return Plan{
		Sentence: sentence,
		Commands: []Command{{Argv: []string{stepFolderCreate, path}, Builtin: true}},
		Capture: func(context.Context) (json.RawMessage, error) {
			dir, name, err := openParentDir(path)
			if err != nil {
				return nil, err
			}
			defer unix.Close(dir)
			var st unix.Stat_t
			switch err := unix.Fstatat(dir, name, &st, unix.AT_SYMLINK_NOFOLLOW); {
			case err == nil:
				return nil, Refusef("%s exists already", path)
			case !errors.Is(err, unix.ENOENT):
				return nil, fmt.Errorf("read %s: %w", path, err)
			}
			return json.Marshal(folderCreateState{Path: path})
		},
		Check: func(context.Context) error {
			now, err := statFolder(path)
			if err != nil {
				return err
			}
			parent, err := statFolder(filepath.Dir(path))
			if err != nil {
				return err
			}
			if now.UID != parent.UID || now.GID != parent.GID || now.Mode != newFolderMode(parent.Mode) {
				return fmt.Errorf("%s has %d:%d %04o, not the owner and group of its folder with mode %04o",
					path, now.UID, now.GID, now.Mode, newFolderMode(parent.Mode))
			}
			return nil
		},
	}, nil
}

func inverseCreateFolder(prev json.RawMessage) (Plan, error) {
	var st folderCreateState
	if err := decodeState(prev, &st); err != nil {
		return Plan{}, err
	}
	if checkFilePath(st.Path) != nil {
		return Plan{}, errors.New("the recorded state of the folder is not usable")
	}
	return Plan{
		Sentence: fmt.Sprintf("Remove the folder %s again if it is still empty.", st.Path),
		Commands: []Command{{Argv: []string{stepFolderRemove, st.Path}, Builtin: true}},
	}, nil
}
