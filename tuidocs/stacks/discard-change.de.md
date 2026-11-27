---
title: Änderung verwerfen
keywords: [verwerfen, löschen, zurückholen, nicht committet, Datei]
---
# Änderung verwerfen

Die Frage betrifft die markierte Datei der Änderungen. Was geschieht, hängt von ihrer Art ab:

- Geändert: Die Datei geht auf den letzten Commit zurück. Die Änderungen sind verloren und lassen sich nicht zurückholen.
- Neu: Die Datei wird gelöscht. Sie ist in keinem Commit, das lässt sich daher nicht rückgängig machen.
- Gelöscht: Die Datei kommt so zurück, wie der letzte Commit sie hat.

Ohne root zeigt die Frage den sudo-Befehl, den Ja ausführt. Danach wird die Liste neu gelesen.

- {yes} führt es aus. {no} bricht ab, und {enter} nimmt die markierte Antwort, das ist Abbrechen.
