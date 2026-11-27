---
title: Einstellungen eines Stacks entfernen
keywords: [Einstellungen, entfernen, Deploy only]
---
# Einstellungen eines Stacks entfernen

Diese Frage will wissen, ob alle Einstellungen des Stacks entfernt werden sollen: Update-Regeln, Backup, Compose-Dateien und Prüfung. {yes} erzeugt einen Commit, der die Einstellungsdatei im Stack-Ordner löscht, und pusht ihn, wenn der Ordner einen Remote hat. Danach wird der Stack nur noch deployt und nie aktualisiert. Von hier aus lässt sich das nicht rückgängig machen.

- {yes} entfernt die Einstellungen.
- {no} bricht ab und ändert nichts. {enter} tut dasselbe, weil Abbrechen die vorgewählte Antwort ist.

Ein Stack ohne Einstellungen bekommt stattdessen eine Meldung. Er wird schon nur deployt, und es wird nichts entfernt.
