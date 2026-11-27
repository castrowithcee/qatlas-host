---
title: New, rename and copy
order: 9
keywords: [file, folder, new, create, rename, copy, name, browser]
screens: [title:title.filenew, title:title.filerename, title:title.filecopy]
---
# New, rename and copy

These forms change one entry of the folder the file browser shows. Each has one name field. New also asks what to create, an empty file or a folder.

- {next} and {prev} move between the fields.
- {save} checks the name and asks for the change.
- {back} discards the form.

The name must be one name in this folder: not empty, not . or .., without a slash or control character, and not already taken. Nothing that exists is replaced. A path the place only reads is refused. A new entry needs the folder to be read first, so read it again with {reload} if the form says so.

After {save} the question shows the sentence and the exact sudo command, and {yes} runs it. The copy lies in the same folder and gets the owner and group of that folder. A copy of a folder beyond the size the journal keeps cannot be undone, and the question is red.
