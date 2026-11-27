---
title: Set the deploy key
order: 8
keywords: [deploy key, store, generate, replace, private key, key file]
screens: [title:title.key, title:title.keystore]
---
# Set the deploy key

The first form asks whether to store an existing key or let the service generate a new one. Generating asks for confirmation. Storing opens a second form, which takes the OpenSSH private key without passphrase either pasted or from a key file, not both. The key may not exceed 16 KiB and is never shown. Afterwards the view shows the public part.

If the folder has a key already, a question asks whether to replace it. The old key stops working at once and the git platform refuses this host until the new public key is entered there. This cannot be undone.

The same dialog tells you when a folder you wanted to add has no deploy key yet.

In the forms:

- {left} and {right} step through the choices, {enter} opens the list.
- In the key field {enter} starts a new line.
- {save} takes the form. {back} discards it, after a question.
- {next} and {prev} move between the fields.

In a question:

- {yes} does it. Without root it asks for the sudo password first.
- {no} cancels. {enter} takes the marked answer, which is cancel.
