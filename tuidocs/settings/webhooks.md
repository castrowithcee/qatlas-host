---
title: Webhooks
order: 5
keywords: [webhooks, receiver, address, listen, proxy, set up]
screens: ["settings/webhooks", "title:title.hook", "title:title.hooklisten"]
---
# Webhooks

This tab shows the webhook receiver: whether it is not set up, running or not running, the address it listens on and the number of hooks. The receiver lets a call of a secret address make qhost check a stack at once. A hook itself is added to a stack in the stack detail, never here.

- {new} sets the receiver up, only while there is none. It listens on a default address that only this host reaches. The question Set up the webhook receiver creates a system user, its configuration and a service, and starts it. Nothing changes for the stacks. {yes} sets it up, {no} cancels.
- {edit} opens Address of the webhook receiver with one field, written host:port with a port from 1025 to 65535. 127.0.0.1 reaches only this host. {save} checks it, {back} discards it. The same address as now is refused.
- Saving asks to let the receiver listen there. It restarts on the new address and the hooks stay. {yes} confirms.

An address that is not local gets a warning. The receiver speaks plain HTTP, so a reverse proxy should provide HTTPS. {reload} reads the state again.
