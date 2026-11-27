---
title: Users
order: 1
keywords: [users, accounts, admin, docker, login, ssh]
screens: [system/users]
---
# Users

The list shows the local accounts of the host with UID, whether the account is an administrator with access to qhost, Docker access, login state and further groups. Regular users come first, system accounts follow in their own group. Below the list the marked account shows home, primary group, access, login state and SSH keys. Login state and SSH keys are shown to root only.

- {up} and {down} mark an account, {search} filters the list by name.
- {new} opens the form for a new user.
- {enter} or {edit} opens the choice of a change for the marked user.
- {delete} opens the form that deletes the marked user.
- {reload} reads the accounts again.

Every change ends in a question with the exact commands. While a change runs, further changes wait.
