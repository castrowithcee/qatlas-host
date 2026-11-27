---
title: Back to the state before
order: 3
keywords: [rollback, return, update run, data, backup, restore, pin]
screens: [title:title.rollback]
---
# Back to the state before

This dialog sets a stack back to the image versions it ran before an update run. It lists the versions that change and what happens to the data. Afterwards the stack is pinned, so no update and no maintenance window moves it on. Release it with unpin in Stacks.

Choose what happens to the data:

- {rollbackdata} stops the stack, backs up its current data if it has backup rules, restores the backup of that update and starts the old versions. Everything written since the update is lost. It is offered only when the run made a backup.
- {rollbackkeep} starts the old versions and keeps the data as it is. The old version may not understand data that the update migrated.
- {no} cancels.

A second question follows with the sentence for your choice. It works like the other questions: {yes} runs the return and {no} cancels. Without root it shows the exact sudo command. A failed return names the run whose log tells why.
