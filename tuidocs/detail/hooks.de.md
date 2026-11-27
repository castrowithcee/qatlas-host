---
title: Webhooks eines Stacks
keywords: [Webhook, Hook, Adresse, Empfänger, Auslöser]
---
# Webhooks eines Stacks

Ein Webhook ist eine geheime Adresse. Ein POST-Aufruf von ihr ohne Inhalt lässt qhost den Stack prüfen. {hookadd} und {hookremove} wirken, sobald unter Einstellungen, Webhooks ein Webhook-Empfänger eingerichtet ist. Ein Hook wirkt nur mit dem Update-Auslöser Abfrage oder Nur Webhook. Das Feld warnt, solange der Auslöser Wartungsfenster ist.

Einen Hook anlegen:

- {hookadd} fragt, ob der Stack einen Webhook bekommen soll. {yes} legt ihn an, {no} oder {enter} bricht ab.
- Ein Dialog zeigt danach die neue Adresse einmal. Sie erscheint nicht wieder, kopiere sie also zuerst.
- {no} oder {enter} schließt den Dialog.

Einen Hook entfernen:

- {hookremove} öffnet die Liste der Hooks des Stacks, und die Wahl eines Hooks öffnet seine Frage.
- {yes} entfernt ihn. Seine Adresse wirkt nach spätestens zehn Sekunden nicht mehr, und das lässt sich nicht rückgängig machen.
- {no} oder {enter} bricht ab.
- Ein Stack ohne Hooks bekommt stattdessen eine Meldung.
