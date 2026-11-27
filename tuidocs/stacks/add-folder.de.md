---
title: Stack-Ordner hinzufügen
keywords: [hinzufügen, Ordner, prüfen, klonen, übernehmen, lokal, Git, Engine, Hostschlüssel, abgelehnt]
---
# Stack-Ordner hinzufügen

Das Formular fragt nach dem absoluten Pfad eines Ordners. Der Ordner wird nur geprüft, vor der letzten Frage ändert sich nichts. Was die Prüfung findet, bestimmt die folgenden Fragen, und jede listet, was sie gefunden hat und was Ja ändert.

- Fehlender oder leerer Ordner: Ein Repository hineinklonen. Weiter fragt nach der Adresse und zeigt dann den Hostschlüssel.
- Klon mit Origin-Remote: Ihn mit seiner Adresse übernehmen. Weiter zeigt den Hostschlüssel und prüft den Deploy-Schlüssel.
- Repository ohne Remote: Es als lokalen Ordner übernehmen.
- Dateien ohne Repository: Ein Git-Repository mit einem ersten Commit der aufgeführten Dateien anlegen. Vorher wird der Ordner noch einmal angesehen, und es ändert sich nichts, wenn seine Liste dann abweicht.
- Ein Ordner, der sich nicht übernehmen lässt oder keine Datei für einen ersten Commit hat, sagt nur den Grund. Schließe ihn mit {no}.

Tasten des Formulars:

- {save} prüft den Ordner. {back} verwirft das Formular, nach einer Frage.
- {next} und {prev} wechseln zwischen den Feldern.

Tasten einer Frage:

- {yes} nimmt die unten aufgeführten Änderungen vor. Die letzte Frage zeigt den Befehl, den es ausführt, und ohne root fragt sie zuerst nach dem sudo-Passwort.
- {no} bricht ab. {enter} nimmt die markierte Antwort, das ist Abbrechen.
- {withremote} nimmt einen Ordner mit Dateien zusammen mit einem Remote auf und fragt nach dessen Adresse.
- {choose} wählt unklare Dateien für den ersten Commit.
- {up} und {down} blättern in der Frage.

Bei einer zweiten Engine kommt zuerst das Formular Engine. {left} und {right} gehen durch die Engines, {enter} öffnet die Liste, und {save} übernimmt die Wahl. Für jede andere Engine als system wird der Ordner neu geprüft.

Der Hostschlüssel wird mit seinem Fingerabdruck gezeigt: Vergleiche ihn vor {yes} mit der Git-Plattform. Hat der Ordner keinen Deploy-Schlüssel, öffnet sich stattdessen die Schlüsselansicht, und nichts wird hinzugefügt.
