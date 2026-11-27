---
title: System
order: 7
keywords: [system, host, users, groups, folders, permissions, journal]
screens: [system]
---
# System

System manages the host itself: its users and groups, the permissions of folders and the files below them. It reads the account files and the file system directly, so it works while the deploy service is not set up.

The tabs are Users, Groups, Folders, Proposals, Changes and Log. On the tab line, {left} and {right} step through them, and {down} enters the page.

- A change is never made at once. A form or key first leads to a question that shows the exact commands. {yes} runs them with sudo, {no} drops the change.
- Every change goes into a journal. The Changes tab lists it and takes a change back where that is possible.
- Proposals of other administrators wait on the Proposals tab until you approve or reject them.

While the lists are read, the area shows a message. If reading fails, the reason stands there, and {reload} reads again.
