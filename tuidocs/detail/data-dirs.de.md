---
title: Datenordner anlegen
keywords: [Datenordner, Besitzer, Modus, fehlende Ordner]
---
# Datenordner anlegen

Der Stack wartet, bis seine Datenordner existieren. Dieses Formular listet die fehlenden unterhalb des Stack-Ordners und fragt nach Besitzer und Modus, die sie bekommen. {save} prüft beide Werte und öffnet eine Frage. Ihr {yes} legt nur die aufgelisteten Ordner an. Vorhandene bleiben, wie sie sind. Danach startet ein Lauf des Stacks. Ohne Root zeigt die Frage den Befehl, der über sudo läuft. {no} oder {enter} bricht ab.

- {next} und {prev} wechseln zwischen den Feldern.
- Besitzer: UID:GID des Benutzers, als der der Container läuft, zum Beispiel 1000:1000. Das Feld ist vorbelegt, wenn die aufgelisteten Ordner sich auf einen einigen.
- Modus: oktale Zugriffsrechte, zum Beispiel 0750. Setuid, Setgid und Sticky sind nicht erlaubt.
- {back} verwirft das Formular und fragt zuerst, wenn sich etwas geändert hat.

Schlägt das Anlegen fehl, bleibt das Formular mit dem Grund offen, damit du einen Wert korrigieren und erneut speichern kannst.
