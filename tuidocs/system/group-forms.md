---
title: Group forms
order: 6
keywords: [new group, members, rename group]
screens: ["title:title.newgroup", "title:title.members", "title:title.renamegroup"]
---
# Group forms

Each form leads to one question with the commands after {save}. {back} discards the form, {next} and {prev} move between the fields.

## New group

Enter a name of lower-case letters, digits, _ and -, at most 32. The name must be free. The system gives the group a regular group number.

## Members

Add lists the users outside the group, Remove the members. {toggle} ticks a user, several at once are fine. One question covers all changes. A user whose primary group this is appears in neither list. For sudo, adding also gives access to qhost and removing takes both. You cannot take yourself out of sudo, and an administrator with access to qhost must remain.

## Rename

Enter the new name. Configurations such as sudoers address a group by its name and keep the old one, so they stop working until they are changed too. The question is red for that reason.
