---
title: Editor questions
order: 11
keywords: [editor, edit, check, invalid, traces, backup files, swap]
screens: [title:title.edittraces, title:title.editinvalid]
---
# Editor questions

Editing a file in the file browser hands the terminal to an external editor on a copy of the text. Two questions can come up.

## Unknown editor

qhost does not know the editor, and it may write backup or swap files of the text that qhost cannot remove.

- {yes} opens the editor anyway.
- {no} cancels and nothing is opened.

## Check failed

After you close the editor, the text is checked against its format. The question names the file, the format and, where known, the line. Nothing has been written.

- {yes} opens the editor again to correct the text.
- {no} discards the change.

After a passing check the change goes on as a question with the exact sudo command.
