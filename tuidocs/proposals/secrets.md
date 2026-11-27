---
title: Secret parts of a channel
order: 5
keywords: [channel, secret, password, token, key, notification]
screens: ["title:title.propsecrets"]
---
# Secret parts of a channel

A proposal for a notification channel never holds its secrets. This form asks for them while you approve: the password, token or key of the channel type. An SMTP channel without a user needs none, and the approval question follows at once. The values are checked, used once to add the channel and never kept on the proposal.

- {save} takes the values and asks the approval question.
- {next} and {prev} move between the fields.
- {back} cancels and approves nothing.

In the approval question, {yes} marks the proposal as approved and adds the channel. {no} cancels.
