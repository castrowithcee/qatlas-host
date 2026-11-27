---
title: Gruppen
keywords: [Gruppen, Mitglieder, sudo, docker, umbenennen, löschen]
---
# Gruppen

Die Liste zeigt die lokalen Gruppen mit GID und Mitgliedern. Reguläre Gruppen stehen vorn, Systemgruppen folgen in einer eigenen Gruppe.

- {up} und {down} markieren eine Gruppe, {search} filtert die Liste nach dem Namen.
- {new} öffnet das Formular für eine neue Gruppe.
- {enter} oder {edit} öffnet die Mitglieder der markierten Gruppe.
- {rename} öffnet das Formular, das sie umbenennt.
- {delete} fragt, ob sie gelöscht werden soll. Die Rückfrage kommt mit den Befehlen, und {yes} führt sie mit sudo aus.
- {reload} liest die Gruppen neu.

Einige Gruppen sind geschützt: Die Gruppe, an der Administratorrechte und der Zugang zu qhost hängen, wird weder umbenannt noch gelöscht. Auch eine Gruppe, die für einen Benutzer die primäre ist, lässt sich nicht löschen, und die Meldung nennt die Benutzer. Mitglieder von sudo erhalten Zugang zu qhost, und Mitglieder von docker nutzen Docker, und das ist praktisch root.
