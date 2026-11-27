---
title: A file of a stack folder
order: 13
keywords: [file, preview, edit, secrets, reveal, search, wrap, line numbers]
screens: [stacks/preview]
---
# A file of a stack folder

The file view shows the opened file as text. Values in secrets.env are masked until you reveal them. The folder .git and the settings in .qatlas/qhost are only read here.

- {up}, {down}, {pageup}, {pagedown}, {first} and {last} scroll.
- {search} searches the file. {log.next} and {log.prev} jump between matches.
- {log.wrap} wraps long lines. {text.numbers} shows line numbers.
- {text.reveal} reveals or masks secret values.
- {edit} edits the file. Saving writes it as root and keeps a journal entry. Nothing is committed: the change shows up in the uncommitted changes of the folder.
- {back} closes the file and returns to the folder view.

A data folder of a running container may be overwritten by it, so stop the stack first when in doubt.
