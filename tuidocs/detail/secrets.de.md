---
title: Geheimnisse eines Stacks
keywords: [Geheimnisse, setzen, löschen, Wert, Name]
---
# Geheimnisse eines Stacks

{new} öffnet ein Formular für ein Geheimnis. {delete} öffnet die Liste der Geheimnisnamen des Stacks, und die Wahl eines Namens öffnet seine Frage. Das Feld Geheimnisse listet nur Namen. Ein Wert wird nie wieder gezeigt.

Ein Geheimnis setzen:

- {next} und {prev} wechseln zwischen Name und Wert.
- Name: Buchstaben, Ziffern, _ . und -, beginnend mit Buchstabe oder _, bis zu 128 Zeichen. Ein Geheimnis gleichen Namens wird ersetzt.
- Wert: beim Tippen verborgen, 4 bis 8192 Zeichen.
- {save} prüft beides und öffnet eine Frage. Ihr {yes} lässt den Dienst den Wert in der Secrets-Datei des Stacks speichern. {no} oder {enter} bricht ab.
- {back} verwirft das Formular und fragt zuerst, wenn sich etwas geändert hat.

Ein Geheimnis löschen: Die Frage nennt das Geheimnis. {yes} löscht es, und sein Wert ist nicht wiederherstellbar. {no} oder {enter} bricht ab. Ein Stack ohne Geheimnisse bekommt stattdessen eine Meldung.

Beide Änderungen gelten ab dem nächsten Deploy.
