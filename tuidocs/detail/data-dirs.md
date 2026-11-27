---
title: Create data directories
order: 7
keywords: [data directories, owner, mode, missing folders]
screens: ["title:title.datadirs"]
---
# Create data directories

The stack waits until its data directories exist. This form lists the missing ones below the stack folder and asks for the owner and mode they get. {save} checks both values and opens a question. Its {yes} creates only the listed directories. Existing ones stay as they are. Then a run of the stack starts. Without root the question shows the command that runs through sudo. {no} or {enter} cancels.

- {next} and {prev} move between the fields.
- Owner: UID:GID of the user the container runs as, for example 1000:1000. It is filled in when the listed directories agree on one.
- Mode: octal permission bits, for example 0750. Setuid, setgid and sticky are not allowed.
- {back} discards the form and asks first when something changed.

If the creation fails, the form stays open with the reason, so you can correct a value and save again.
