---
title: Grants of a stack
order: 12
keywords: [grant, withdraw, sudo, user, expiry, action]
screens: ["title:title.grant", "title:title.grantremove"]
---
# Grants of a stack

A grant lets a user, and every agent working under the account of that user, do one action on this stack without sudo. Only users with access to qhost can hold a grant. Without such a user a message says so.

Grant an action:

- {grantadd} opens a form. {next} and {prev} move between the fields.
- User: a user with access to qhost.
- Action: restart the stack, restart a container, check for updates or update the stack.
- Last day: YYYY-MM-DD, valid until the end of that day (UTC). Empty means no expiry. Granting the same action again replaces the expiry.
- {save} checks the day and opens a question that shows the sudo command, whether the stack has a backup and a check set up and, for an update, that it commits and pushes to the source. {yes} grants, {no} or {enter} cancels.
- {back} discards the form and asks first when something changed.

Withdraw a grant:

- {grantremove} opens the list of the grants, and choosing one opens its question.
- {yes} withdraws it. The user and every agent need sudo for this again. {no} or {enter} cancels.

The Grants panel marks a grant as valid or says why it is not: expired, the stack is gone or the account changed.
