---
title: Permissions of a folder
order: 9
keywords: [permissions, rights, owner, group, mode, acl, change]
screens: ["system/folders/rights", "title:title.changefolder"]
---
# Permissions of a folder

The page shows what was read for the marked entry: the real path behind a link, the type, what the owner, the group and everyone else may do, the mode, a note on ACLs and on special bits. A system path is marked as such.

- {edit} opens the form to change a folder. A file cannot be changed here, and nothing inside a .git folder.
- {reload} reads the permissions again.
- {back} returns to the browser.

## Change form

Owner and group are chosen from the accounts of the host. Owner may, Group may and Everyone else may each take nothing, read, or read and write. Everything below it set to yes changes every file and folder inside too, and the question shows their number first.

{left} and {right} step through a choice, {next} and {prev} move between fields, {save} leads to the question with the commands and {back} discards the form. A system path asks in red.
