---
title: Change with sudo
order: 5
keywords: [sudo, change, plan, command, confirm, journal, undo, irreversible]
screens: [title:title.hostchange]
---
# Change with sudo

This question comes before every change of the host, such as users, groups, files and folders in System. It has three parts: a sentence in plain words on what changes, a line on what follows, and below them the exact commands, each behind a dollar sign. The commands are shown so you can check what runs with root rights. Nothing runs without your confirmation.

The line on what follows says whether the change goes into the journal and can be undone, or cannot be undone. A risk to root, a system account or a system group makes the question red, and so does a delete.

- {yes} hands the terminal to sudo, which asks for your password itself. The command then checks every rule again and stays the authority.
- {no} cancels. Anything the change would have used, such as a typed password, is wiped.

Several commands run one after another. If there are more than fit, the question says how many more follow. A long question fills the workspace instead of a dialog: scroll it with {up}, {down}, {pageup}, {pagedown}, {first} and {last}.

Locking a user is such a change. The change of a user in System > Users offers Lock: after it no login is possible any more, with a password or an SSH key. Running sessions stay open until they end, and you cannot lock yourself.

The result follows in its own dialog.
