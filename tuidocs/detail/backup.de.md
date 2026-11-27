---
title: Backup eines Stacks
keywords: [Backup, Aufbewahrung, Volumes, Datenordner, Befehl]
---
# Backup eines Stacks

Dieses Formular legt fest, was ein Backup des Stacks sichert und wie lange Backups bleiben. Ein Backup braucht mindestens einen Datenordner, ein Volume oder einen Befehl. Das Speichern ändert nur das Backup und lässt Update-Regeln, Prüfung und Compose-Dateien, wie sie sind. {save} prüft die Werte und öffnet eine Frage. Ihr {yes} erzeugt im Stack-Ordner einen Commit mit den neuen Einstellungen und pusht ihn, wenn der Ordner einen Remote hat. Das Backup wirkt beim nächsten Update. {no} oder {enter} bricht ab.

- {next} und {prev} wechseln zwischen den Feldern.
- Datenordner: Ordner unterhalb des Stack-Ordners, die gesichert werden. Eine Liste, {enter} im Feld öffnet ihren Editor.
- Volumes: benannte Volumes der Compose-Dateien. {toggle} hakt eines ab. Ein (!) sagt, es ist gespeichert, steht aber nicht mehr in den Compose-Dateien.
- Befehl: Programm und Argumente, auf dem Host ohne Shell ausgeführt. Der erste Eintrag ist das Programm. Eine Liste.
- Aufbewahren (Anzahl): so viele neueste Backups bleiben, mindestens 1. Leer behält 7, wenn kein Alter gesetzt ist.
- Höchstalter (Tage): ältere Backups werden entfernt, mindestens 1. Leer setzt keine Grenze.
- {back} verwirft das Formular und fragt zuerst, wenn sich etwas geändert hat.

In einem Editor fügt {new} einen Eintrag hinzu, {edit} ändert den unter dem Cursor, {delete} entfernt ihn und {back} schließt den Editor. Beim Befehl verschieben {moveup} und {movedown} den Eintrag. {save} übernimmt einen Eintrag und {back} verwirft ihn.
