---
title: Stack-Details
keywords: [Stack-Details, Versionen, Backups, Geheimnisse, Webhooks, Freigaben, Update-Regeln]
---
# Stack-Details

Die Details zeigen den Stack, den du in Stacks mit {enter} gewählt hast: Modus, Zustand und Befunde, dann je ein Feld für Image-Versionen, Container, Changelog, Geheimnisse, Webhooks, Freigaben, Backups, Letzte Läufe und Funktionen. Markierungen für einen gestoppten Stack, fehlende Datenordner, einen Ausfall, eine Reparatur, eine Beobachtung und übersprungene Versionen stehen über den Feldern, solange sie zutreffen.

Blättere mit {up}, {down}, {pageup}, {pagedown}, {first} und {last}. {reload} liest alles neu. {back} führt zurück zu Stacks.

Diese Tasten öffnen ein Formular, eine Liste oder eine Frage. Vor ihrer Antwort ändert sich nichts:

- {edit} setzt die Update-Regeln.
- {backupform} setzt das Backup.
- {composeform} setzt die Compose-Dateien.
- {checkform} setzt die Prüfung.
- {removesettings} entfernt alle Einstellungen des Stacks.
- {datadirs} legt die fehlenden Datenordner an. Es wirkt nur, solange das Feld welche nennt.
- {new} setzt ein Geheimnis und {delete} löscht eines.
- {restore} spielt ein älteres Backup ein.
- {rollback} geht auf den Stand vor dem letzten Update zurück. Es braucht einen erfolgreichen Update-Lauf.
- {promote} übernimmt die Versionen eines anderen Stacks.
- {hookadd} und {hookremove} verwalten Webhooks. Sie wirken, sobald ein Webhook-Empfänger eingerichtet ist.
- {grantadd} und {grantremove} verwalten Freigaben.
- {containerlogs} öffnet das Log der Container und {files} die Dateien des Stack-Ordners.
