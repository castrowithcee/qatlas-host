---
title: Uncommitted changes
order: 10
keywords: [changes, diff, uncommitted, commit, discard, modified, new, deleted]
screens: [stacks/changes, title:title.diff]
---
# Uncommitted changes

The view lists the files of a stack folder that differ from its last commit, each marked modified, new or deleted, with its stack. A commit takes all files of the list. secrets.env, data folders and new files below a bind mount belong to the host and are never part of it. Updates leave a folder out until its changes are committed or discarded.

- {enter} shows the diff of the marked file. Without root the diff is read through sudo after a question, because it can hold values only root may read. Nothing changes.
- {delete} discards the change of the marked file. A question comes first.
- {commit} commits all files of the list, after a question. A list that is cut cannot be committed here.
- {search} filters the list. {up} and {down} choose a file.
- {back} leaves the view. {reload} reads the list again.

In the diff:

- {up}, {down}, {pageup} and {pagedown} scroll. The diff is cut after 256 KiB.
- {search} searches the diff. {log.next} and {log.prev} jump between matches.
- {log.wrap} wraps long lines. {text.numbers} shows line numbers.
- {back} returns to the list.

In the question about the diff, {yes} reads it and {no} cancels.
