---
title: History of a stack folder
order: 14
keywords: [history, log, commits, author, diff, git]
screens: [stacks/history, title:title.show]
---
# History of a stack folder

The view lists the latest commits of a stack folder, newest first along the first parent, with date, author and subject. Author, subject and paths are text of other people and are only shown. The history comes from the deploy service, so git is never called in the folder itself. A list that is cut says so.

- {enter} shows the files of the marked commit, each marked new, modified or deleted.
- {showcommit} shows the diff of the commit against its first parent, from the list or from its files. Without root the diff is read through sudo after a question, because it can hold values only root may read. Nothing changes.
- {search} filters the list. {up} and {down} choose a commit or a file.
- {back} goes one step back: from the diff to the files, from the files to the list, from the list to the folder row or the file browser. {reload} reads the history again.

In the diff:

- {up}, {down}, {pageup} and {pagedown} scroll. The diff is cut after 256 KiB.
- {search} searches the diff. {log.next} and {log.prev} jump between matches.
- {log.wrap} wraps long lines. {text.numbers} shows line numbers.

In the question about the diff, {yes} reads it and {no} cancels.
