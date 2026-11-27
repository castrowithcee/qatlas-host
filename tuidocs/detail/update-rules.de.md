---
title: Update-Regeln eines Stacks
keywords: [Update-Regeln, Modus, Obergrenze, Auslöser, Tag-Filter, Changelog, Beobachtung]
---
# Update-Regeln eines Stacks

Dieses Formular legt fest, wie der Stack aktualisiert wird. Das Speichern ändert nur die Update-Regeln und lässt Backup, Prüfung und Compose-Dateien, wie sie sind. {save} prüft die Werte und öffnet eine Frage. Ihr {yes} erzeugt im Stack-Ordner einen Commit mit den neuen Regeln und pusht ihn, wenn der Ordner einen Remote hat. Die Regeln wirken ab dem nächsten Update. {no} oder {enter} bricht ab.

- {next} und {prev} wechseln zwischen den Feldern. {enter} wählt in einem Auswahlfeld einen Wert.
- Modus: automatisch aktualisiert im Wartungsfenster, manuell nur auf Anforderung.
- Obergrenze: der größte Versionssprung, der von selbst erfolgt, patch, minor oder major.
- Major-Sprung: stepwise geht eine Major-Version nach der anderen, direct springt zur neuesten.
- Beobachtung (Minuten): Ein Ausfall nach einem Update setzt es zurück. 0 ist aus, höchstens 1440.
- Auslöser: Wartungsfenster, Abfrage oder Nur Webhook. Der Abstand von 30 bis 86400 Sekunden gilt nur bei Abfrage.
- Übernahme-Quelle: der Stack, dessen Versionen standardmäßig übernommen werden. Leer für keine.
- Tag-Filter und Changelog-Repositories sind Listen. {enter} im Feld öffnet ihren Editor.
- {back} verwirft das Formular und fragt zuerst, wenn sich etwas geändert hat.

Im Editor fügt {new} einen Eintrag hinzu, {edit} ändert den unter dem Cursor, {delete} entfernt ihn und {back} schließt den Editor und schreibt die Einträge ins Feld. Ein Eintrag nennt eine Variable des Stacks und einen regulären Ausdruck oder ein GitHub-Repository als owner/name. {save} übernimmt den Eintrag und {back} verwirft ihn. Gespeichert wird erst mit dem Formular selbst.
