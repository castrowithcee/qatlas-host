---
title: Geheime Teile eines Kanals
keywords: [Kanal, Geheimnis, Passwort, Token, Schlüssel, Benachrichtigung]
---
# Geheime Teile eines Kanals

Ein Vorschlag für einen Benachrichtigungskanal enthält nie dessen Geheimnisse. Dieses Formular fragt sie bei der Freigabe ab: das Passwort, den Token oder den Schlüssel der Kanalart. Ein SMTP-Kanal ohne Benutzer braucht keines, dann folgt die Freigabefrage sofort. Die Werte werden geprüft, einmal zum Anlegen des Kanals genutzt und nie am Vorschlag gehalten.

- {save} übernimmt die Werte und stellt die Freigabefrage.
- {next} und {prev} wechseln zwischen den Feldern.
- {back} bricht ab und gibt nichts frei.

In der Freigabefrage markiert {yes} den Vorschlag als freigegeben und legt den Kanal an. {no} bricht ab.
