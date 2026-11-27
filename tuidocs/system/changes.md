---
title: Changes
order: 10
keywords: [changes, journal, undo, history]
screens: [system/changes]
---
# Changes

The list shows the latest changes of the host from its journal, newest first: time, who made it, the change in a sentence, the result and whether an undo is possible. The journal can only be read by root, so the page first asks you to press {reload}, which reads it through sudo after a question. Below the list the marked entry shows its undo as a sentence and the exact commands.

- {up} and {down} mark an entry, {search} filters the list.
- {enter} asks to undo the marked change. The question shows the commands, {yes} runs them with sudo and {no} drops the undo.
- {reload} reads the journal again.

An undo is itself in the journal and cannot be undone. For an entry without undo, {enter} explains why: it was undone already, it did not stand, or it is final, as a deleted home. After a change from this area the page says the journal is older than the host.
