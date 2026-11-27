---
title: Delete a user
order: 4
keywords: [delete user, home, mail spool]
screens: ["title:title.deleteuser"]
---
# Delete a user

Locking is the usual way when someone leaves. The form asks what happens to the home of the marked user.

- Keep the home and its files: the default. Only this delete can be undone later from the Changes tab. A user with running processes is not deleted.
- Delete the home and the mail spool: the home and all its files are removed. This cannot be undone. The question names what else the home touches, such as a system path or the home of another user.

{left} and {right} step through the choice. {save} leads to the red question with the commands, where {yes} runs it with sudo and {no} drops it. {back} discards the form.

You cannot delete yourself.
