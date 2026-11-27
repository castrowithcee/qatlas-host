---
title: Fragen zum Editor
keywords: [Editor, bearbeiten, Prüfung, ungültig, Spuren, Sicherungsdateien, Auslagerung]
---
# Fragen zum Editor

Eine Datei im Dateibrowser zu bearbeiten übergibt das Terminal an einen externen Editor auf einer Kopie des Textes. Zwei Fragen können dabei auftauchen.

## Unbekannter Editor

qhost kennt den Editor nicht, und er kann Sicherungs- oder Auslagerungsdateien des Textes schreiben, die qhost nicht entfernen kann.

- {yes} öffnet den Editor trotzdem.
- {no} bricht ab, und nichts wird geöffnet.

## Prüfung fehlgeschlagen

Nachdem du den Editor geschlossen hast, wird der Text gegen sein Format geprüft. Die Frage nennt die Datei, das Format und, wo bekannt, die Zeile. Es wurde nichts geschrieben.

- {yes} öffnet den Editor erneut, um den Text zu korrigieren.
- {no} verwirft die Änderung.

Besteht die Prüfung, geht die Änderung als Frage mit dem genauen sudo-Befehl weiter.
