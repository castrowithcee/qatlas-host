---
title: Deploy-Dienst einrichten
keywords: [Deploy-Dienst, Einrichten, Reparieren, läuft nicht, Kanal abgelehnt, rootless, System-Engine, Docker, root]
---
# Deploy-Dienst einrichten

Solange kein Deploy-Dienst eingerichtet ist, zeigen die Bereiche, die ihn brauchen, diesen Hinweis statt ihres Inhalts. Der Deploy-Dienst rollt deine Stacks aus und hält sie aktuell, läuft als eigener Systembenutzer getrennt vom Host-Dienst und hat Zugriff auf Docker. Der Hinweis zeigt auch die Kommandozeile für das Einrichten.

{setupdeploy} öffnet den Dialog. Im Bereich Stacks wird er auch auf einem Host angeboten, der schon einen Dienst hat, um den einer rootless Engine hinzuzufügen.

1. Wähle den Deploy-Dienst. Die System-Engine bekommt einen Dienst mit eigenem Systembenutzer. Eine rootless Engine bekommt einen Dienst, der als der Benutzer läuft, dem die Engine gehört.
2. Gib bei einer rootless Engine den Benutzer an. Die Benutzer mit Engine werden vorgeschlagen, ein anderer Name lässt sich tippen. Schritte zeigen geht weiter.
3. Die letzte Frage listet die Schritte, die als root laufen, und die Warnungen, die du zuerst lesen sollst. Vor deiner Antwort Ja läuft nichts.

Tasten:

- {yes} richtet die System-Engine ein oder bestätigt die letzte Frage.
- {rootless} wählt die rootless Engine.
- {no} bricht ab. {enter} nimmt die markierte Antwort.
- {up} und {down} wählen einen vorgeschlagenen Benutzer. {next} vervollständigt ihn.

Ohne root fragt das Einrichten vor dem Start nach dem sudo-Passwort. Ist der Dienst schon eingerichtet, sagt der Dialog nur das.

## Ein Dienst, der nicht antwortet

Die Oberfläche startet auch, wenn ein eingerichteter Deploy-Dienst nicht antwortet. Dieselben Bereiche zeigen dann die Ursache statt der Einrichtung: Der Dienst läuft nicht, oder sein Kanal hat die Verbindung abgelehnt, weil Besitzer oder Modus des Kanalordners oder der Benutzer, als der der Dienst läuft, nicht dem entspricht, was der Host erwartet. Einstellungen und System bleiben bedienbar.

{setupdeploy} repariert den Deploy-Dienst der System-Engine. Der Dialog zeigt, was nicht stimmt, als die genauen Schritte, die als root laufen, und vor deiner Antwort Ja läuft nichts. Ohne root fragt er zuerst nach dem sudo-Passwort. Hat die Reparatur gewirkt, verschwindet der Hinweis beim nächsten Lesen.

Findet die Einrichtung keinen Schritt zum Reparieren, sagt der Hinweis das und bietet keine Reparatur an. Das Dienstlog nennt dann den Grund: System, Log zeigt es, und die Kommandozeile zeigt es mit journalctl -u qhost-deploy. Eine rootless Instanz, die nicht antwortet, wird im Bereich genannt und hier nicht repariert.
