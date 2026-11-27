---
title: Compose files of a stack
order: 4
keywords: [compose, files, merge order]
screens: ["title:title.compose", "title:title.composefiles"]
---
# Compose files of a stack

This form sets the compose files of the stack. Compose merges them in the order of the list, and a later file overrides an earlier one. An empty list uses the files Compose finds itself. Saving changes only the compose files and leaves update rules, backup and check as they are. {save} checks the entries and opens a question. Its {yes} creates a commit with the new settings in the stack folder and pushes it where the folder has a remote. The files act from the next deployment. {no} or {enter} cancels.

- {enter} on the field opens the editor of the list. An entry is a file name relative to the stack folder, for example compose.yaml. It must not start with a dash, and a stack has at most 16 files.
- {new} adds a file, {edit} changes the one under the cursor and {delete} removes it.
- {moveup} and {movedown} move the entry, because the order decides which file wins.
- {back} closes the editor and writes the entries to the field.
- In the form of an entry, {save} applies it and {back} discards it.
- {back} in the main form discards it and asks first when something changed.
