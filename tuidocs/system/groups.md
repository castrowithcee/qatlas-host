---
title: Groups
order: 5
keywords: [groups, members, sudo, docker, rename, delete]
screens: [system/groups]
---
# Groups

The list shows the local groups with GID and members. Regular groups come first, system groups follow in their own group.

- {up} and {down} mark a group, {search} filters the list by name.
- {new} opens the form for a new group.
- {enter} or {edit} opens the members of the marked group.
- {rename} opens the form that renames it.
- {delete} asks to delete it. The question comes with the commands, and {yes} runs them with sudo.
- {reload} reads the groups again.

Some groups are protected: the group that carries administrator rights and access to qhost is neither renamed nor deleted. A group that is the primary group of a user cannot be deleted either, and the message names the users. Members of sudo gain access to qhost, and members of docker use Docker, which is practically root.
