---
title: Webhooks
keywords: [Webhooks, Empfänger, Adresse, lauschen, Proxy, einrichten]
---
# Webhooks

Dieser Reiter zeigt den Webhook-Empfänger: ob er nicht eingerichtet ist, läuft oder nicht läuft, auf welcher Adresse er lauscht und die Zahl der Hooks. Der Empfänger lässt den Aufruf einer geheimen Adresse qhost sofort einen Stack prüfen. Ein Hook selbst wird in den Stack-Details zu einem Stack angelegt, nie hier.

- {new} richtet den Empfänger ein, nur solange es keinen gibt. Er lauscht auf einer Standardadresse, die nur dieser Host erreicht. Die Frage Den Webhook-Empfänger einrichten legt einen Systembenutzer, seine Konfiguration und einen Dienst an und startet ihn. An den Stacks ändert sich nichts. {yes} richtet ihn ein, {no} bricht ab.
- {edit} öffnet Adresse des Webhook-Empfängers mit einem Feld, geschrieben host:port mit einem Port von 1025 bis 65535. 127.0.0.1 erreicht nur diesen Host. {save} prüft sie, {back} verwirft sie. Die Adresse, die schon gilt, wird abgewiesen.
- Speichern fragt, ob der Empfänger dort lauschen soll. Er startet auf der neuen Adresse neu, und die Hooks bleiben. {yes} bestätigt.

Eine Adresse, die nicht lokal ist, bekommt eine Warnung. Der Empfänger spricht unverschlüsseltes HTTP, deshalb sollte ein Reverse Proxy HTTPS bieten. {reload} liest den Zustand neu.
