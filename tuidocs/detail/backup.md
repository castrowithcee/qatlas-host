---
title: Backup of a stack
order: 3
keywords: [backup, retention, volumes, data directories, command]
screens: ["title:title.backup", "title:title.backuppaths", "title:title.backupcommand"]
---
# Backup of a stack

This form sets what is saved in a backup of the stack and how long backups are kept. A backup needs at least one data directory, volume or command. Saving changes only the backup and leaves update rules, check and compose files as they are. {save} checks the values and opens a question. Its {yes} creates a commit with the new settings in the stack folder and pushes it where the folder has a remote. The backup acts on the next update. {no} or {enter} cancels.

- {next} and {prev} move between the fields.
- Data directories: folders below the stack folder that are saved. A list, {enter} on the field opens its editor.
- Volumes: named volumes of the compose files. {toggle} ticks one. A mark (!) says it is saved but no longer in the compose files.
- Command: program and arguments, run on the host without a shell. The first entry is the program. A list.
- Keep (number): the newest backups kept, at least 1. Empty keeps 7 unless an age is set.
- Maximum age (days): older backups are removed, at least 1. Empty sets no limit.
- {back} discards the form and asks first when something changed.

In an editor, {new} adds an entry, {edit} changes the one under the cursor, {delete} removes it and {back} closes the editor. In the command, {moveup} and {movedown} move the entry. {save} applies an entry and {back} discards it.
