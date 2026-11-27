---
title: Deploy-Dienst einrichten
keywords: [Deploy-Dienst, Einrichten, rootless, System-Engine, Docker, root]
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
