---
title: File preview
order: 8
keywords: [file, preview, search, wrap, line numbers, secrets, edit]
screens: ["system/folders/preview"]
---
# File preview

The preview shows a text file read only, coloured by its format. A binary or very large file, a link and anything that is no regular file show a note instead of the text. A file that only root can read is read with sudo after a question.

- {up}, {down}, {pageup}, {pagedown}, {first} and {last} scroll.
- {search} starts a search, {enter} keeps it, {back} clears it. {log.next} and {log.prev} jump between matches.
- {log.wrap} wraps long lines.
- {text.numbers} switches the line numbers.
- {text.reveal} uncovers values the preview hides, such as secrets, and hides them again.
- {edit} edits the file in your editor.
- {reload} reads the file again.
- {back} closes the preview and returns to the folder.
