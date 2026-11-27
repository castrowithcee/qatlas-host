---
title: Protokoll
keywords: [Protokoll, Aktionsprotokoll, Audit, Kette der Hashes, Dienst]
---
# Protokoll

Die Seite zeigt das Aktionsprotokoll des Host-Dienstes, das neueste zuerst: Zeit, Benutzer, Aktion, Ergebnis und die zugehörige Änderung. Über der Liste sagt die Prüfung der Kette der Hashes, ob das Protokoll intakt ist oder wo es bricht. Administratoren in der Gruppe qhost lesen es ohne sudo. Es wird gelesen, wenn die Seite sich öffnet, nach einer Änderung von hier und bei {reload}, nie nach der Uhr.

- {up} und {down} markieren einen Eintrag, {search} filtert die Liste. {back} leert den Filter.
- {instance} wechselt zwischen dem Host-Dienst und den Deploy-Diensten. Es wird angeboten, wo es mehr als einen gibt. Ein Deploy-Dienst zeigt stattdessen Funktion, Stack und Herkunft.
- {savelog} speichert die gezeigten Zeilen als Datei.
- {reload} liest das Protokoll neu.

Läuft der Host-Dienst nicht oder erreicht ihn die Oberfläche nicht, sagt die Seite das statt der Liste. Nach einer Änderung weist die Seite darauf hin, dass das Protokoll älter ist als der Host.
