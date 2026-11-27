---
title: Read as root
order: 4
keywords: [sudo, root, read, password, permission]
screens: [title:title.readroot]
---
# Read as root

Only root may read the file or folder named in the question. The question shows the exact command that would read it, because the interface never runs a command you have not seen.

- {yes} runs the command. sudo then asks for your password on the terminal itself. The interface never sees the password.
- {no} cancels and nothing is read.

Reading changes nothing on the host. A folder read this way is not read again after a change, because that would ask for the password a second time. The list says so, and {reload} reads it again.
