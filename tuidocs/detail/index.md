---
title: Stack detail
order: 3
keywords: [stack detail, versions, backups, secrets, webhooks, grants, update rules]
screens: [detail]
---
# Stack detail

The detail shows the stack you chose in Stacks with {enter}: its mode, state and findings, then one panel each for Image versions, Containers, Changelog, Secrets, Webhooks, Grants, Backups, Last runs and Functions. Marks for a stopped stack, missing data directories, a failure, a repair, an observation and skipped versions appear above the panels while they apply.

Scroll with {up}, {down}, {pageup}, {pagedown}, {first} and {last}. {reload} reads everything again. {back} returns to Stacks.

These keys open a form, a list or a question. Nothing changes before its answer:

- {edit} sets the update rules.
- {backupform} sets the backup.
- {composeform} sets the compose files.
- {checkform} sets the check.
- {removesettings} removes all settings of the stack.
- {datadirs} creates the missing data directories. It works only while the panel lists some.
- {new} sets a secret and {delete} deletes one.
- {restore} restores an older backup.
- {rollback} goes back to the state before the last update. It needs a successful update run.
- {promote} takes over the versions of another stack.
- {hookadd} and {hookremove} manage webhooks. They work once a webhook receiver is set up.
- {grantadd} and {grantremove} manage grants.
- {containerlogs} opens the log of the containers and {files} the files of the stack folder.
