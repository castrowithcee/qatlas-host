---
title: secrets.env saved
order: 10
keywords: [secrets, deploy, saved, env, later]
screens: [title:title.secretssaved]
---
# secrets.env saved

You saved the secrets.env of a stack folder. The new values take effect only when the stack is deployed next. This question offers to deploy it now. Deploying starts a run that creates the containers again with the new values.

- {yes} deploys the stack now, like the deploy action of the Stacks list. Without root a question with the exact sudo command follows.
- {no} deploys later. The values stay saved.

Where no stack uses the folder, a message in the message line says so instead.
