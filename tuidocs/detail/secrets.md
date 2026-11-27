---
title: Secrets of a stack
order: 8
keywords: [secrets, set, delete, value, name]
screens: ["title:title.secret", "title:title.secretdelete"]
---
# Secrets of a stack

{new} opens a form for one secret. {delete} opens the list of the secret names of the stack, and choosing one opens its question. The Secrets panel lists only names. A value is never shown again.

Set a secret:

- {next} and {prev} move between Name and Value.
- Name: letters, digits, _ . and -, starting with a letter or _, up to 128 characters. A secret of that name is replaced.
- Value: hidden while typed, 4 to 8192 characters.
- {save} checks both and opens a question. Its {yes} lets the service store the value in the secrets file of the stack. {no} or {enter} cancels.
- {back} discards the form and asks first when something changed.

Delete a secret: the question names the secret. {yes} deletes it, and its value cannot be recovered. {no} or {enter} cancels. A stack without secrets gets a message instead.

Both changes apply from the next deployment.
