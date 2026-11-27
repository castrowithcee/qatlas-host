---
title: Result of a sudo run
order: 6
keywords: [sudo, result, journal, undo, refused, restored, failed, check]
screens: [title:title.hostresult]
---
# Result of a sudo run

This dialog reports what a command run with sudo did. Its first line says one of four things:

- Done: the change was made. A note names the journal entry, and System > Changes takes it back. A change that cannot be undone says so instead.
- Not changed: a rule of the host refused it before anything was done.
- Failed, previous state restored: the change or its check failed and the earlier state is back.
- Failed: the change did not succeed. Check what ran before you try again.

Further lines may follow: the reason, whether the check passed, an audit warning, or a hint that this session does not have the group qhost yet and needs a restart of the interface.

When the run gave no usable answer, the dialog explains why in plain words: sudo is missing, the password prompt was cancelled or denied, the command ended with an exit code, or its answer could not be read. A cancelled prompt means nothing was changed.

{no} and {enter} close the dialog. The lists behind it are read again.
