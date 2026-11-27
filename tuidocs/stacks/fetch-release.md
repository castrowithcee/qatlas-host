---
title: Fetch or release a folder
order: 6
keywords: [fetch, release, remove, stack folder, poll]
screens: [title:title.folders]
---
# Fetch or release a folder

Two questions about the marked stack folder share this page.

Fetch updates the remote-tracking branch from the remote. The working copy stays as it is. The result says whether the folder is up to date or the remote has newer commits. A local folder has no remote, so {fetch} only tells you that.

Release stops qhost from deploying out of the folder. Nothing is deleted and no container is stopped. The permissions, the systemd drop-in entry and the deploy key stay.

- {yes} does it. Without root it asks for the sudo password first.
- {no} cancels. {enter} takes the marked answer, which is cancel.
