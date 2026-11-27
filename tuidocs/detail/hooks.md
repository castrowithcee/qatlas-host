---
title: Webhooks of a stack
order: 11
keywords: [webhook, hook, address, receiver, trigger]
screens: ["title:title.hookadd", "title:title.hookremove", "title:title.hooksecret"]
---
# Webhooks of a stack

A webhook is a secret address. A POST call of it, without content, makes qhost check the stack. {hookadd} and {hookremove} work once a webhook receiver is set up under Settings, Webhooks. A hook acts only with the update trigger Polling or Webhook only. The panel warns while the trigger is Maintenance window.

Add a hook:

- {hookadd} asks whether to give the stack a webhook. {yes} creates it, {no} or {enter} cancels.
- A dialog then shows the new address once. It does not appear again, so copy it first.
- {no} or {enter} closes the dialog.

Remove a hook:

- {hookremove} opens the list of the hooks of the stack, and choosing one opens its question.
- {yes} removes it. Its address stops working within ten seconds, and this cannot be undone.
- {no} or {enter} cancels.
- A stack without hooks gets a message instead.
