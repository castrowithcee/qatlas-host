---
title: New user
order: 2
keywords: [new user, create, initial password, ssh key]
screens: ["title:title.newuser"]
---
# New user

The form creates a local account. The user must change the initial password at the first login.

- Name: lower-case letters, digits, _ and -, at most 32. The name must be free and cannot be root.
- Initial password and Repeat: hidden while typed, never shown and never part of a command line. Both must match.
- Admin: administrator with sudo and access to qhost.
- Docker: access to Docker, which is practically root.
- Further groups: {toggle} ticks a group, several at once are fine.
- SSH keys: optional, one public key per line in authorized_keys format, not shown afterwards.

{next} and {prev} move between the fields, {left} and {right} step through a choice. {save} checks the entries and leads to the question with the commands. A wrong entry stays in the form with its reason. {back} discards the form.
