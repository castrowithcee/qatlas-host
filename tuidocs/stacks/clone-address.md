---
title: Address of the repository
order: 3
keywords: [address, url, ssh, clone, host key, repository]
screens: [title:title.folderurl]
---
# Address of the repository

The form asks for the SSH address of the repository, as user@host:path or ssh://[user@]host[:port]/path. It comes from adding a folder that is missing or empty, or a folder of files that joins a remote with {withremote}.

The address may not be empty or hold blanks. Only the service knows which addresses it accepts, and it gives the reason when it refuses one. Saving the form reads the host key the server presents and stores nothing. The question about trusting the host key follows.

- {save} takes the address and reads the host key.
- {back} discards the form, after a question.
- {next} and {prev} move between the fields.
