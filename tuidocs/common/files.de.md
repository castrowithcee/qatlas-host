---
title: Neu, umbenennen und kopieren
keywords: [Datei, Ordner, neu, anlegen, umbenennen, kopieren, Name, Browser]
---
# Neu, umbenennen und kopieren

Diese Formulare ändern einen Eintrag des Ordners, den der Dateibrowser zeigt. Jedes hat ein Namensfeld. Neu fragt zusätzlich, was angelegt wird, eine leere Datei oder ein Ordner.

- {next} und {prev} wechseln zwischen den Feldern.
- {save} prüft den Namen und fragt nach der Änderung.
- {back} verwirft das Formular.

Der Name muss ein Name in diesem Ordner sein: nicht leer, nicht . oder .., ohne Schrägstrich und Steuerzeichen und noch nicht vergeben. Nichts Vorhandenes wird ersetzt. Ein Pfad, den der Ort nur liest, wird abgelehnt. Ein neuer Eintrag braucht einen zuvor gelesenen Ordner, lies ihn also mit {reload} neu, wenn das Formular es sagt.

Nach {save} zeigt die Frage den Satz und den genauen sudo-Befehl, und {yes} führt ihn aus. Die Kopie liegt im selben Ordner und erhält Besitzer und Gruppe dieses Ordners. Die Kopie eines Ordners über die Größe hinaus, die das Journal behält, lässt sich nicht rückgängig machen, und die Frage ist rot.
