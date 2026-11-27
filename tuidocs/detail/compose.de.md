---
title: Compose-Dateien eines Stacks
keywords: [Compose, Dateien, Reihenfolge]
---
# Compose-Dateien eines Stacks

Dieses Formular legt die Compose-Dateien des Stacks fest. Compose führt sie in der Reihenfolge der Liste zusammen, und eine spätere Datei überschreibt eine frühere. Eine leere Liste nutzt die Dateien, die Compose selbst findet. Das Speichern ändert nur die Compose-Dateien und lässt Update-Regeln, Backup und Prüfung, wie sie sind. {save} prüft die Einträge und öffnet eine Frage. Ihr {yes} erzeugt im Stack-Ordner einen Commit mit den neuen Einstellungen und pusht ihn, wenn der Ordner einen Remote hat. Die Dateien wirken ab dem nächsten Deploy. {no} oder {enter} bricht ab.

- {enter} im Feld öffnet den Editor der Liste. Ein Eintrag ist ein Dateiname relativ zum Stack-Ordner, zum Beispiel compose.yaml. Er darf nicht mit einem Bindestrich beginnen, und ein Stack hat höchstens 16 Dateien.
- {new} fügt eine Datei hinzu, {edit} ändert die unter dem Cursor und {delete} entfernt sie.
- {moveup} und {movedown} verschieben den Eintrag, denn die Reihenfolge entscheidet, welche Datei gewinnt.
- {back} schließt den Editor und schreibt die Einträge ins Feld.
- Im Formular eines Eintrags übernimmt {save} ihn und {back} verwirft ihn.
- {back} im Hauptformular verwirft es und fragt zuerst, wenn sich etwas geändert hat.
