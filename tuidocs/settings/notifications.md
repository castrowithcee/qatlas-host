---
title: Notifications
order: 4
keywords: [notifications, channels, webhook, slack, telegram, smtp, events, test, instance]
screens: ["settings/notifications", "title:title.notify", "title:title.notifydetail"]
---
# Notifications

This tab lists the notification channels with name, type, events and target. A channel reports events of a deploy service, such as a failed update or a rollback.

- {up} and {down} choose a channel.
- {new} adds a channel in two steps. The first form asks for Name, Type (webhook, slack, telegram or smtp) and Events. {left} and {right} choose the type, {toggle} ticks an event, and none ticked means all. {save} moves on to Settings of the channel, which asks for what the type needs: an address and key, a bot token and chat, or mail server, sender and recipients. The secret parts are hidden and never shown again. {next} and {prev} move between the fields, {save} checks the form, {back} discards it.
- The last {save} asks to add the channel. {yes} adds it, {no} cancels.
- {test} asks to send one short test message through the chosen channel, without retries. {yes} sends it.
- {delete} asks to remove the chosen channel. Its stored secrets are deleted with it. {yes} removes it.
- {instance} chooses the deploy service whose channels the tab shows, where several exist. {reload} reads the list again.
