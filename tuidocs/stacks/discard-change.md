---
title: Discard a change
order: 12
keywords: [discard, delete, restore, uncommitted, file]
screens: [title:stackchanges.titlediscard]
---
# Discard a change

The question is about the marked file of the changes. What happens depends on its kind:

- Modified: the file goes back to the last commit. The edits are lost and cannot be brought back.
- New: the file is deleted. It is in no commit, so this cannot be undone.
- Deleted: the file comes back as the last commit has it.

Without root the question shows the sudo command that Yes runs. Afterwards the list is read again.

- {yes} does it. {no} cancels, and {enter} takes the marked answer, which is cancel.
