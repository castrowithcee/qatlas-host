---
title: Check of a stack
order: 5
keywords: [check, health, http, containers, timeout, compare images]
screens: ["title:title.checkform"]
---
# Check of a stack

This form sets when the stack counts as healthy after a deployment. Saving changes only the check and leaves update rules, backup and compose files as they are. {save} checks the values and opens a question. Its {yes} creates a commit with the new settings in the stack folder and pushes it where the folder has a remote. The check acts from the next deployment, the comparison from the next promotion. {no} or {enter} cancels.

- {next} and {prev} move between the fields. {enter} chooses on or off in a choice field.
- Container check: on means the stack counts as healthy only while all its containers run.
- HTTP check: on means the address below must answer as expected. Off ignores the three fields below it.
- Address (URL): an absolute http or https URL.
- Expected status: the HTTP status the address must answer, 100 to 599. 200 by default.
- Timeout: how long to wait for the answer, from 1s to 10m, for example 10s.
- Compare images on promotion: on means taking over versions into this stack is refused unless its images are the ones the source runs, by fingerprint. Off by default.
- {back} discards the form and asks first when something changed.
