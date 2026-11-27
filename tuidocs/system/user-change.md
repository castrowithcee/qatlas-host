---
title: Change a user
order: 3
keywords: [change user, admin, docker, lock, password, groups, ssh key]
screens: ["system/users/pick", "title:title.changeuser", "title:title.addgroups", "title:title.removegroups", "title:title.usersecret", "title:title.addkey", "title:title.removekey"]
---
# Change a user

The choice lists what can be changed for the marked user. {up} and {down} choose, {search} filters, {enter} picks, {back} closes the choice.

- Make or take administrator: administrator with sudo and access to qhost. You cannot take it from yourself, and the last administrator stays one.
- Give or take access to Docker, or to qhost for an administrator without it.
- Lock or unlock: a locked user cannot log in with a password or an SSH key. Running sessions stay open.
- Add to groups, Remove from groups, Set a new password, Add an SSH key and Remove an SSH key open a form. The question with the commands follows.

In the forms, {toggle} ticks a group, {left} and {right} step through a choice, {next} and {prev} move between fields, {save} leads to the question and {back} discards the form.

- New password: typed twice and hidden. The choice Change at login makes the user pick a new one at the next login.
- Add an SSH key: paste exactly one public key.
- Remove an SSH key: choose the key by its fingerprint.
