---
title: Benachrichtigungen
keywords: [Benachrichtigungen, Kanäle, Webhook, Slack, Telegram, SMTP, Ereignisse, Test, Instanz]
---
# Benachrichtigungen

Dieser Reiter listet die Benachrichtigungskanäle mit Name, Typ, Ereignissen und Ziel. Ein Kanal meldet Ereignisse eines Deploy-Dienstes, etwa ein fehlgeschlagenes Update oder einen Rollback.

- {up} und {down} wählen einen Kanal.
- {new} legt einen Kanal in zwei Schritten an. Das erste Formular fragt Name, Typ (webhook, slack, telegram oder smtp) und Ereignisse ab. {left} und {right} wählen den Typ, {toggle} hakt ein Ereignis an, und keines angehakt heißt alle. {save} führt weiter zu Einstellungen des Kanals, das fragt, was der Typ braucht: Adresse und Schlüssel, Bot-Token und Chat oder Mailserver, Absender und Empfänger. Die geheimen Teile sind verborgen und werden nie wieder angezeigt. {next} und {prev} wechseln zwischen den Feldern, {save} prüft das Formular, {back} verwirft es.
- Das letzte {save} fragt, ob der Kanal angelegt wird. {yes} legt ihn an, {no} bricht ab.
- {test} fragt, ob eine kurze Testnachricht über den gewählten Kanal gesendet wird, ohne Wiederholung. {yes} sendet sie.
- {delete} fragt, ob der gewählte Kanal entfernt wird. Seine gespeicherten Geheimnisse werden mit gelöscht. {yes} entfernt ihn.
- {instance} wählt den Deploy-Dienst, dessen Kanäle der Reiter zeigt, wo mehrere existieren. {reload} liest die Liste neu.
