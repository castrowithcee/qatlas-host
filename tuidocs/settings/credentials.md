---
title: Credentials
order: 2
keywords: [credentials, registry, login, token, github, instance]
screens: ["settings/credentials", "title:title.cred"]
---
# Credentials

This tab lists the stored credentials by name and storage time. Without any, qhost works anonymously. A credential is either a registry login or the GitHub token that fetches changelogs. Its value is never shown again.

- {up} and {down} choose an entry.
- {new} opens the form Set a credential: Credential kind, then Registry host and Registry user for a registry login, and the hidden Token. {next} and {prev} move between the fields, {left} and {right} choose the kind, {save} checks the form and asks for confirmation, {back} discards it. A credential of the same name is replaced.
- {delete} asks to delete the chosen credential. It cannot be recovered, and qhost then works anonymously for that service. {yes} deletes, {no} cancels.
- {instance} chooses the deploy service whose credentials the tab shows. It appears only where several services exist. Without a default one, choose first, because the keys act on the instance shown.
- {reload} reads the list again.
