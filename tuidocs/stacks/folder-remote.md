---
title: Add a remote
order: 5
keywords: [remote, push, compare, local folder, history, secrets]
screens: [title:title.remote]
---
# Add a remote

A local stack folder can run against a remote. The form asks for the address of the repository. The host key and the deploy key of the folder are checked as when you add a clone, and the folder is then compared with the repository before anything changes. The comparison lists the branch, the paths that differ and the public deploy key, which needs write access to the repository.

The question depends on what the repository holds:

- Empty: Yes pushes the history of the folder to it, and the folder then runs against the remote.
- Same files: Yes makes the folder follow it. The local history is kept and nothing is pushed.
- Different files: put and push adds one commit with the files of the host on top of the remote history and pushes it. The remote history is never overwritten.
- Secret-like files in the commits to push: the question only tells. Nothing is pushed, and you remove the files from the history yourself.
- A folder of files whose repository has no commit cannot join it: take the folder over without an address first.

Without a deploy key the key view opens instead and nothing changes.

Keys:

- {save} in the form takes the address. {back} discards it, after a question.
- {yes} answers the question. {no} cancels, and {enter} takes the marked answer, which is cancel.
- {up} and {down} scroll the question.
