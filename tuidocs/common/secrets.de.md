---
title: secrets.env gespeichert
keywords: [Secrets, Deploy, gespeichert, env, später]
---
# secrets.env gespeichert

Du hast die secrets.env eines Stack-Ordners gespeichert. Die neuen Werte wirken erst, wenn der Stack das nächste Mal ausgerollt wird. Diese Frage bietet an, ihn jetzt auszurollen. Das Ausrollen startet einen Lauf, der die Container mit den neuen Werten neu erstellt.

- {yes} rollt den Stack jetzt aus, wie die Deploy-Aktion der Stacks-Liste. Ohne root folgt eine Frage mit dem genauen sudo-Befehl.
- {no} rollt später aus. Die Werte bleiben gespeichert.

Nutzt kein Stack den Ordner, sagt das stattdessen eine Meldung in der Meldezeile.
