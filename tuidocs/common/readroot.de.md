---
title: Als root lesen
keywords: [sudo, root, lesen, Passwort, Berechtigung]
---
# Als root lesen

Nur root darf die Datei oder den Ordner lesen, die die Frage nennt. Die Frage zeigt den genauen Befehl, der sie liest, denn die Oberfläche führt keinen Befehl aus, den du nicht gesehen hast.

- {yes} führt den Befehl aus. sudo fragt dann auf dem Terminal selbst nach deinem Passwort. Die Oberfläche sieht das Passwort nie.
- {no} bricht ab, und es wird nichts gelesen.

Lesen ändert nichts am Host. Ein so gelesener Ordner wird nach einer Änderung nicht erneut gelesen, weil das das Passwort ein zweites Mal verlangen würde. Die Liste sagt das, und {reload} liest sie neu.
