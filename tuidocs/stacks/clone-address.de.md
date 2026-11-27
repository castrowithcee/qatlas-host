---
title: Adresse des Repositorys
keywords: [Adresse, URL, SSH, klonen, Hostschlüssel, Repository]
---
# Adresse des Repositorys

Das Formular fragt nach der SSH-Adresse des Repositorys, als user@host:pfad oder ssh://[user@]host[:port]/pfad. Es kommt beim Hinzufügen eines fehlenden oder leeren Ordners, oder eines Ordners mit Dateien, der mit {withremote} einem Remote beitritt.

Die Adresse darf nicht leer sein und keine Leerzeichen enthalten. Nur der Dienst weiß, welche Adressen er annimmt, und er nennt den Grund, wenn er eine ablehnt. Das Speichern des Formulars liest den Hostschlüssel, den der Server vorlegt, und speichert nichts. Danach folgt die Frage, ob dem Hostschlüssel zu trauen ist.

- {save} nimmt die Adresse und liest den Hostschlüssel.
- {back} verwirft das Formular, nach einer Frage.
- {next} und {prev} wechseln zwischen den Feldern.
