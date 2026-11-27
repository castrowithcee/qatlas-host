---
title: Zurück auf den Stand davor
keywords: [Zurücksetzen, Update-Lauf, Daten, Backup, Wiederherstellen, Festhalten]
---
# Zurück auf den Stand davor

Dieser Dialog setzt einen Stack auf die Image-Versionen zurück, die er vor einem Update-Lauf betrieb. Er listet die Versionen, die sich ändern, und was mit den Daten geschieht. Danach ist der Stack festgehalten, kein Update und kein Wartungsfenster bewegt ihn weiter. Gib ihn mit unpin in Stacks wieder frei.

Wähle, was mit den Daten geschieht:

- {rollbackdata} stoppt den Stack, sichert seine aktuellen Daten, wenn er Backup-Regeln hat, spielt das Backup dieses Updates ein und startet die alten Versionen. Alles seit dem Update Geschriebene geht verloren. Es wird nur angeboten, wenn der Lauf ein Backup gemacht hat.
- {rollbackkeep} startet die alten Versionen und behält die Daten, wie sie sind. Die alte Version versteht vom Update migrierte Daten womöglich nicht.
- {no} bricht ab.

Es folgt eine zweite Frage mit dem Satz zu deiner Wahl. Sie funktioniert wie die anderen Fragen: {yes} führt das Zurücksetzen aus und {no} bricht ab. Ohne root zeigt sie den genauen sudo-Befehl. Ein gescheitertes Zurücksetzen nennt den Lauf, dessen Log den Grund zeigt.
