---
title: Änderungen committen
keywords: [committen, Push, abgelehnt, Secrets, geschützter Branch, Läufe]
---
# Änderungen committen

Die Frage nennt die Zahl der Dateien und den Ordner und listet die Dateien. qhost prüft, dass jeder Stack der Dateien noch lädt und kein Secret mitgeht, und committet die Dateien, wie sie jetzt sind. Bei einem Klon pusht es den Commit auch. Der normale Lauf rollt danach ihre Stacks aus. Ohne root zeigt die Frage den sudo-Befehl, den Ja ausführt.

- {yes} committet. {no} bricht ab, und {enter} nimmt die markierte Antwort, das ist Abbrechen.
- {up} und {down} blättern in der Frage.

Ist der Commit gemacht, nennt ihn ein Hinweis und sagt, dass der Lauf der Stacks jetzt beginnt. {showlog} öffnet den Bereich Läufe, {no} schließt den Hinweis.

Wird der Commit abgelehnt, sagt ein Hinweis den Grund und dass nichts committet wurde, und die Liste wird neu gelesen. Die Gründe:

- Der Commit würde Secret-artige Dateien pushen. Halte geheime Werte in secrets.env, die den Host nie verlässt.
- Das Remote hat den Push abgelehnt, wie es ein geschützter Branch tut. Ändere das bei der Git-Plattform.
- Der Ordner ist weitergegangen, oder die Liste passt nicht mehr. Prüfe die neue Liste und committe erneut.
- Ein Stack der Dateien lädt nicht mehr. Korrigiere zuerst die Datei.
- Ein Lauf eines Stacks der Dateien ist im Gang. Committe erneut, wenn er beendet ist.

{no} oder {enter} schließt den Hinweis.
