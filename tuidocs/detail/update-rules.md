---
title: Update rules of a stack
order: 2
keywords: [update rules, mode, ceiling, trigger, tag filters, changelog, observation]
screens: ["title:title.updaterules", "title:title.tags", "title:title.changelog"]
---
# Update rules of a stack

This form sets how the stack is updated. Saving changes only the update rules and leaves backup, check and compose files as they are. {save} checks the values and opens a question. Its {yes} creates a commit with the new rules in the stack folder and pushes it where the folder has a remote. The rules act from the next update. {no} or {enter} cancels.

- {next} and {prev} move between the fields. {enter} chooses a value in a choice field.
- Mode: automatic updates in the maintenance window, manual only on request.
- Ceiling: the largest version step taken by itself, patch, minor or major.
- Major jump: stepwise goes one major version at a time, direct jumps to the newest.
- Observation (minutes): a failure after an update rolls it back. 0 is off, at most 1440.
- Trigger: Maintenance window, Polling or Webhook only. The interval of 30 to 86400 seconds counts with Polling only.
- Promote source: the stack whose versions are taken over by default. Empty for none.
- Tag filters and Changelog repositories are lists. {enter} on the field opens its editor.
- {back} discards the form and asks first when something changed.

In the editor, {new} adds an entry, {edit} changes the one under the cursor, {delete} removes it and {back} closes the editor and writes the entries to the field. An entry names a variable of the stack and a regular expression, or a GitHub repository as owner/name. {save} applies the entry and {back} discards it. Nothing is stored before the form itself is saved.
