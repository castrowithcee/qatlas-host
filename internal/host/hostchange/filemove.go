package hostchange

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// The tree actions rename, copy and remove a file, a link or a folder with everything below it. Their steps are in
// filetree.go.
const (
	// ActionFileRename renames an entry within one file system. Its arguments are the source, the target and
	// optionally optionSystemPath.
	ActionFileRename = "file.rename"
	// ActionFileCopy copies an entry with everything below it. Its arguments are the source, the target and
	// optionally optionPermanent and optionSystemPath.
	ActionFileCopy = "file.copy"
	// ActionFileRemove removes an entry with everything below it. Its arguments are the path and optionally
	// optionPermanent and optionSystemPath.
	ActionFileRemove = "file.remove"
)

// optionPermanent lets a copy or a removal go beyond the limit of an undo; the change then cannot be undone. It is
// an argument rather than a property of the tree, because the plan, which says whether a change can be undone,
// reads nothing from the host.
const optionPermanent = "permanent"

const permanentRemoval = "removed with permanent: the journal keeps nothing of what was removed"

func registerTreeActions(r *Registry) error {
	for _, d := range []Definition{
		{ID: ActionFileRename, Plan: planRename, Inverse: inverseRename},
		{ID: ActionFileCopy, Plan: planCopy, Inverse: inverseCopy},
		{ID: ActionFileRemove, Plan: planRemove, Inverse: inverseRemove},
	} {
		if err := r.Register(d); err != nil {
			return err
		}
	}
	return nil
}

// treeArgs are the arguments of a tree action.
type treeArgs struct {
	paths          []string
	permanent, ack bool
}

// parseTreeArgs takes the paths, then the options in any order, each at most once.
func parseTreeArgs(action, usage string, args []string, paths int, permanent bool) (treeArgs, error) {
	if len(args) < paths {
		return treeArgs{}, Argf("%s needs %s", action, usage)
	}
	a := treeArgs{paths: args[:paths]}
	for _, o := range args[paths:] {
		switch {
		case o == optionPermanent && permanent && !a.permanent:
			a.permanent = true
		case o == optionSystemPath && !a.ack:
			a.ack = true
		default:
			return treeArgs{}, Argf("unknown or repeated option %q; %s needs %s", o, action, usage)
		}
	}
	return a, nil
}

// checkTreePath checks one path of a tree action; write says that the action writes, moves or removes there.
func checkTreePath(path string, write bool) error {
	if err := checkCleanPath(path); err != nil {
		return err
	}
	if reason := lockedReason(path); write && reason != "" {
		return Refusef("%s is locked: %s", path, reason)
	}
	if isPseudoFS(path) {
		return Refusef("%s lies in a virtual file system; the file actions do not work there", path)
	}
	return nil
}

func refuseSystemRoot(path, done string) error {
	if isSystemRoot(path) {
		return Refusef("%s is a system folder; it is not %s as a whole", path, done)
	}
	return nil
}

func refuseNested(src, dst string) error {
	if lexicallyUnder(src, dst) || lexicallyUnder(dst, src) {
		return Argf("the source and the target must not be the same or lie in one another")
	}
	return nil
}

// checkMovePaths is the check of the paths of a rename, before the plan and again in the step.
func checkMovePaths(src, dst string) error {
	for _, p := range []string{src, dst} {
		if err := checkTreePath(p, true); err != nil {
			return err
		}
		if err := refuseSystemRoot(p, "renamed"); err != nil {
			return err
		}
	}
	return refuseNested(src, dst)
}

// checkCopyPaths is the check of the paths of a copy. Copying out of a locked folder only reads it.
func checkCopyPaths(src, dst string) error {
	if err := checkTreePath(src, false); err != nil {
		return err
	}
	if err := checkTreePath(dst, true); err != nil {
		return err
	}
	if err := refuseSystemRoot(dst, "replaced"); err != nil {
		return err
	}
	return refuseNested(src, dst)
}

func checkRemovePath(path string) error {
	if err := checkTreePath(path, true); err != nil {
		return err
	}
	return refuseSystemRoot(path, "removed")
}

// needsAck refuses a system path without its acknowledgement, naming the consequence.
func needsAck(path string, ack bool, consequence string) error {
	if IsSystemPath(path) && !ack {
		return Refusef("%s is a system path: %s; it needs the acknowledgement of the system path", path, consequence)
	}
	return nil
}

// fileRenameState is the journaled state of file.rename: the entry that was renamed.
type fileRenameState struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Dev    uint64 `json:"dev"`
	Ino    uint64 `json:"ino"`
	// The stamp is set by Seal, after the rename, which changes the change time of the entry.
	stamp
}

func planRename(args []string, _ []byte) (Plan, error) {
	a, err := parseTreeArgs(ActionFileRename, "a source, a target and optionally "+optionSystemPath, args, 2, false)
	if err != nil {
		return Plan{}, err
	}
	src, dst := a.paths[0], a.paths[1]
	if err := checkMovePaths(src, dst); err != nil {
		return Plan{}, err
	}
	for _, p := range a.paths {
		if err := needsAck(p, a.ack, "renaming there can stop services or keep the host from starting"); err != nil {
			return Plan{}, err
		}
	}
	sentence := fmt.Sprintf("Rename %s to %s on the same file system, without copying; a link is renamed as a link, "+
		"and nothing that exists at %s is replaced.", src, dst, dst)
	if IsSystemPath(src) || IsSystemPath(dst) {
		sentence += systemSentence
	}
	var state fileRenameState
	return Plan{
		Sentence: sentence,
		Seal: func(context.Context) (json.RawMessage, error) {
			ddir, dname, err := openParentDir(dst)
			if err != nil {
				return nil, err
			}
			defer unix.Close(ddir)
			if n, err := statxAt(ddir, dname); err == nil && n.dev == state.Dev && n.ino == state.Ino {
				state.stamp = n.born
			}
			return json.Marshal(state)
		},
		Commands: []Command{{Argv: []string{stepFileRename, src, dst}, Builtin: true}},
		Capture: func(context.Context) (json.RawMessage, error) {
			st, err := captureRename(src, dst)
			if err != nil {
				return nil, err
			}
			state = st
			return json.Marshal(st)
		},
		Check: func(context.Context) error { return checkRenamed(state) },
	}, nil
}

func captureRename(src, dst string) (fileRenameState, error) {
	sdir, sname, err := openParentDir(src)
	if err != nil {
		return fileRenameState{}, err
	}
	defer unix.Close(sdir)
	ddir, dname, err := openParentDir(dst)
	if err != nil {
		return fileRenameState{}, err
	}
	defer unix.Close(ddir)
	n, err := statxAt(sdir, sname)
	if errors.Is(err, errNoEntry) {
		return fileRenameState{}, Refusef("%s does not exist", src)
	}
	if err != nil {
		return fileRenameState{}, fmt.Errorf("read %s: %w", src, err)
	}
	if err := refuseExisting(ddir, dname, dst); err != nil {
		return fileRenameState{}, err
	}
	sp, err := statxAt(sdir, "")
	if err != nil {
		return fileRenameState{}, err
	}
	dp, err := statxAt(ddir, "")
	if err != nil {
		return fileRenameState{}, err
	}
	if !sameMount(sp, dp) {
		return fileRenameState{}, Refusef("%s and %s lie on different file systems or mounts; a rename does not copy and "+
			"remove, copy it instead", src, filepath.Dir(dst))
	}
	if n.kind == kindDir && !sameMount(n, sp) {
		return fileRenameState{}, Refusef("%s is a mount point; it is not renamed", src)
	}
	if err := checkRenameTree(sdir, sname, src, dst); err != nil {
		return fileRenameState{}, err
	}
	return fileRenameState{Source: src, Target: dst, Dev: n.dev, Ino: n.ino}, nil
}

func refuseExisting(dir int, name, path string) error {
	switch _, err := statxAt(dir, name); {
	case err == nil:
		return Refusef("%s exists already; nothing is replaced", path)
	case !errors.Is(err, errNoEntry):
		return fmt.Errorf("read %s: %w", path, err)
	}
	return nil
}

// checkRenamed confirms that the renamed entry is at the target and the source is free.
func checkRenamed(st fileRenameState) error {
	ddir, dname, err := openParentDir(st.Target)
	if err != nil {
		return err
	}
	defer unix.Close(ddir)
	n, err := statxAt(ddir, dname)
	if err != nil || n.dev != st.Dev || n.ino != st.Ino {
		return fmt.Errorf("%s is not the renamed entry after the change", st.Target)
	}
	sdir, sname, err := openParentDir(st.Source)
	if err != nil {
		return err
	}
	defer unix.Close(sdir)
	if _, err := statxAt(sdir, sname); !errors.Is(err, errNoEntry) {
		return fmt.Errorf("%s still exists after the change", st.Source)
	}
	return nil
}

func inverseRename(prev json.RawMessage) (Plan, error) {
	var st fileRenameState
	if err := decodeState(prev, &st); err != nil {
		return Plan{}, err
	}
	if checkMovePaths(st.Source, st.Target) != nil {
		return Plan{}, errors.New("the recorded state of the rename is not usable")
	}
	input, err := json.Marshal(st)
	if err != nil {
		return Plan{}, err
	}
	back := fileRenameState{Source: st.Target, Target: st.Source, Dev: st.Dev, Ino: st.Ino}
	return Plan{
		Sentence: fmt.Sprintf("Rename %s back to %s, if it is still the entry the change renamed and %s is free.",
			st.Target, st.Source, st.Source),
		Commands: []Command{{Argv: []string{stepFileRename, st.Target, st.Source}, Builtin: true, Secret: input,
			SecretLabel: recordedStateLabel}},
		Check: func(context.Context) error { return checkRenamed(back) },
	}, nil
}

// copyState is the journaled state of file.copy. Copy is set once the copy ran: an undo removes only that tree.
type copyState struct {
	Source string        `json:"source"`
	Target string        `json:"target"`
	Copy   *treeManifest `json:"copy,omitempty"`
}

func planCopy(args []string, _ []byte) (Plan, error) {
	a, err := parseTreeArgs(ActionFileCopy, fmt.Sprintf("a source, a target and optionally %s and %s",
		optionPermanent, optionSystemPath), args, 2, true)
	if err != nil {
		return Plan{}, err
	}
	src, dst := a.paths[0], a.paths[1]
	if err := checkCopyPaths(src, dst); err != nil {
		return Plan{}, err
	}
	if err := needsAck(src, a.ack, "a copy can carry keys or passwords out of the system folders to the owner of the target"); err != nil {
		return Plan{}, err
	}
	if err := needsAck(dst, a.ack, "a copy there can change how the system or its services behave"); err != nil {
		return Plan{}, err
	}
	sentence := fmt.Sprintf("Copy %s to %s, a folder with everything below it and links as links; the copy gets the owner "+
		"and group of %s and the permissions of the source without setuid, setgid and sticky bit.", src, dst, filepath.Dir(dst))
	if IsSystemPath(src) || IsSystemPath(dst) {
		sentence += systemSentence
	}
	var state copyState
	var top node
	plan := Plan{
		Commands: []Command{{Argv: []string{stepFileCopy, src, dst}, Builtin: true}},
		Capture: func(context.Context) (json.RawMessage, error) {
			st, n, err := captureCopy(src, dst, a.permanent)
			if err != nil {
				return nil, err
			}
			state, top = st, n
			return json.Marshal(st)
		},
		Check: func(context.Context) error { return checkCopied(dst, top) },
	}
	if a.permanent {
		plan.NotReversible = "copied with permanent: the journal keeps no record of the copy"
		sentence += " The copy is permanent and cannot be undone."
	} else {
		sentence += fmt.Sprintf(" Undo removes the copy while it is unchanged; beyond %d entries or 10 MiB the copy needs %s "+
			"and cannot be undone.", MaxFolderEntries, optionPermanent)
		plan.Seal = func(context.Context) (json.RawMessage, error) {
			s, err := scanTree(dst, scanOpts{limit: MaxTreeEntries, lockBases: []string{dst}, hash: true})
			if err != nil {
				return nil, err
			}
			m := manifestOf(dst, s)
			state.Copy = &m
			return json.Marshal(state)
		}
	}
	plan.Sentence = sentence
	return plan, nil
}

func captureCopy(src, dst string, permanent bool) (copyState, node, error) {
	ddir, dname, err := openParentDir(dst)
	if err != nil {
		return copyState{}, node{}, err
	}
	defer unix.Close(ddir)
	if err := refuseExisting(ddir, dname, dst); err != nil {
		return copyState{}, node{}, err
	}
	s, err := measureForChange(src, "copy", "copied", permanent, []string{dst})
	if err != nil {
		return copyState{}, node{}, err
	}
	return copyState{Source: src, Target: dst}, s.entries[0].node, nil
}

// checkCopied confirms the copy: of the kind of the source, with the owner and group of its folder and the mode
// of a copy.
func checkCopied(dst string, top node) error {
	dir, name, err := openParentDir(dst)
	if err != nil {
		return err
	}
	defer unix.Close(dir)
	parent, err := statxAt(dir, "")
	if err != nil {
		return err
	}
	n, err := statxAt(dir, name)
	if err != nil {
		return fmt.Errorf("%s is missing after the copy", dst)
	}
	if n.kind != top.kind || n.uid != parent.uid || n.gid != parent.gid || (n.kind != kindLink && n.mode != copyMode(top.mode)) {
		return fmt.Errorf("%s has %d:%d %04o, not the owner and group of its folder with mode %04o",
			dst, n.uid, n.gid, n.mode, copyMode(top.mode))
	}
	return nil
}

func inverseCopy(prev json.RawMessage) (Plan, error) {
	var st copyState
	if err := decodeState(prev, &st); err != nil {
		return Plan{}, err
	}
	if st.Copy == nil {
		return Plan{}, errors.New("the copy was not recorded after it ran; it is not removed")
	}
	if checkCleanPath(st.Source) != nil || checkManifest(*st.Copy) != nil || st.Copy.Path != st.Target {
		return Plan{}, errors.New("the recorded state of the copy is not usable")
	}
	input, err := json.Marshal(st.Copy)
	if err != nil {
		return Plan{}, err
	}
	return Plan{
		Sentence: fmt.Sprintf("Remove the copy %s (%d entries) if it is unchanged since the copy.", st.Target, len(st.Copy.Entries)),
		Commands: []Command{{Argv: []string{stepFileRemoveCopy, st.Target}, Builtin: true, Secret: input,
			SecretLabel: recordedStateLabel}},
	}, nil
}

// removeState is the journaled state of file.remove: the whole tree, content included, or nothing with
// optionPermanent.
type removeState struct {
	Path      string      `json:"path"`
	Permanent bool        `json:"permanent,omitempty"`
	Entries   []treeEntry `json:"entries,omitempty"`
}

func planRemove(args []string, _ []byte) (Plan, error) {
	a, err := parseTreeArgs(ActionFileRemove, fmt.Sprintf("a path and optionally %s and %s", optionPermanent, optionSystemPath),
		args, 1, true)
	if err != nil {
		return Plan{}, err
	}
	path := a.paths[0]
	if err := checkRemovePath(path); err != nil {
		return Plan{}, err
	}
	if err := needsAck(path, a.ack, "removing there can stop services or keep the host from starting"); err != nil {
		return Plan{}, err
	}
	sentence := fmt.Sprintf("Remove %s, a folder with everything below it; a link is removed as a link, never followed.", path)
	if IsSystemPath(path) {
		sentence += systemSentence
	}
	argv := []string{stepFileRemove, path}
	plan := Plan{
		Capture: func(context.Context) (json.RawMessage, error) {
			st, err := captureRemove(path, a.permanent)
			if err != nil {
				return nil, err
			}
			return json.Marshal(st)
		},
		Check: func(context.Context) error {
			dir, name, err := openParentDir(path)
			if err != nil {
				return err
			}
			defer unix.Close(dir)
			if _, err := statxAt(dir, name); !errors.Is(err, errNoEntry) {
				return fmt.Errorf("%s still exists after the removal", path)
			}
			return nil
		},
	}
	if a.permanent {
		argv = append(argv, optionPermanent)
		plan.NotReversible = permanentRemoval
		sentence += " This is final and cannot be undone: the journal keeps nothing of it."
	} else {
		sentence += fmt.Sprintf(" The journal keeps everything removed, so that undo can put it back; this is possible up to "+
			"%d entries and 10 MiB, beyond that the removal needs %s and is final.", MaxFolderEntries, optionPermanent)
		// A removal can stop halfway; the inverse puts the whole recorded tree back at the then free path, or finds it
		// untouched.
		plan.PartialSteps = true
	}
	plan.Sentence = sentence
	plan.Commands = []Command{{Argv: argv, Builtin: true}}
	return plan, nil
}

func captureRemove(path string, permanent bool) (removeState, error) {
	s, err := measureForChange(path, "remove", "removed", permanent, []string{path})
	if err != nil {
		return removeState{}, err
	}
	if top := s.entries[0].node; top.kind == kindDir && !sameMount(top, s.parent) {
		return removeState{}, Refusef("%s is a mount point; it is not removed", path)
	}
	if permanent {
		return removeState{Path: path, Permanent: true}, nil
	}
	s, err = scanTree(path, scanOpts{limit: MaxFolderEntries, maxBytes: MaxUndoBytes, lockBases: []string{path}, content: true})
	if errors.Is(err, ErrTooManyEntries) {
		return removeState{}, Refusef("%s grew beyond %d entries while it was read; add %s to remove it without an undo",
			path, MaxFolderEntries, optionPermanent)
	}
	if err != nil {
		return removeState{}, err
	}
	st := removeState{Path: path, Entries: make([]treeEntry, len(s.entries))}
	for i, e := range s.entries {
		st.Entries[i] = treeEntry{Rel: e.rel, Kind: e.kind, UID: e.uid, GID: e.gid, Mode: e.mode, MTime: e.mtime.Nano(),
			Content: e.content, Link: e.link}
	}
	return st, nil
}

func inverseRemove(prev json.RawMessage) (Plan, error) {
	var st removeState
	if err := decodeState(prev, &st); err != nil {
		return Plan{}, err
	}
	if st.Permanent {
		return Plan{}, errors.New(permanentRemoval)
	}
	if err := checkRemoveState(st); err != nil {
		return Plan{}, err
	}
	input, err := json.Marshal(st)
	if err != nil {
		return Plan{}, err
	}
	var redact [][]byte
	for _, e := range st.Entries {
		if len(e.Content) > 0 {
			redact = append(redact, e.Content)
		}
	}
	return Plan{
		Sentence: fmt.Sprintf("Put %s back as it was before the removal (%d entries), with content, links, owner, group and "+
			"permissions, if %s is still free.", st.Path, len(st.Entries), st.Path),
		Commands: []Command{{Argv: []string{stepFileRestoreTree, st.Path}, Builtin: true, Secret: input,
			SecretLabel: previousContentLabel}},
		Redact: redact,
	}, nil
}
