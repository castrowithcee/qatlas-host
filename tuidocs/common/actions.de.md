---
title: Stack-Aktionen
keywords: [Update, Deploy, Pause, Fortsetzen, Festhalten, Prüfen, Start, Stopp, Neustart, Bestätigung, Vorschau, Ergebnis]
---
# Stack-Aktionen

Jede Aktion auf Stacks fragt zuerst. Die Frage nennt den Stack, oder die Zahl der Stacks mit ihren Namen, und sagt in einem Satz, was danach geschieht. Nichts erreicht den Dienst, bevor du antwortest. Dieselben Fragen erscheinen für Start, Stopp und Neustart in Container, wo sie einen Container nennen.

- {yes} führt die Aktion aus.
- {no} bricht ab. {enter} bricht ebenfalls ab, ein versehentlicher Tastendruck bewirkt also nichts.

Ohne root folgt eine zweite Frage. Sie zeigt den genauen sudo-Befehl jedes Stacks, und die Aktionen laufen dann nacheinander.

## Update-Vorschau

Update öffnet zuerst eine längere Ansicht. Sie listet je Stack die Versionen, die sich ändern, und die Release-Notes. Die Antwort {yes} erscheint, sobald jeder Changelog gelesen ist oder gescheitert ist. Blättere mit {up}, {down}, {pageup}, {pagedown}, {first} und {last}.

## Ergebnis

Das Ergebnis listet jeden Stack als ok, fehlgeschlagen oder nicht ausgeführt, mit einer Zeile dazu, was geschah. Start, Stopp und Neustart enden beim ersten fehlgeschlagenen Stack und markieren die übrigen als nicht ausgeführt. Stacks, die scheiterten, bleiben angehakt.

- {no} schließt das Ergebnis.
- {showlog} öffnet den Lauf in Läufe. Es wird nur angeboten, wo ein Lauf gestartet wurde.

Fortsetzen hebt nur eine Pause oder eine angehaltene Reparatur auf und startet nichts. Registries prüfen fragt die Registries und ändert nichts an den Stacks.
