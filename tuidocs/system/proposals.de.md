---
title: Vorschläge
keywords: [Vorschläge, freigeben, ablehnen, Hoständerung, freie Befehle]
---
# Vorschläge

Administratoren und Agenten können eine Änderung am Host vorschlagen. Die Liste zeigt jeden Vorschlag mit Art, Absender, Zeitpunkt, Ablauf und Zustand. Nur offene Vorschläge lassen sich entscheiden.

- {up} und {down} markieren einen Vorschlag, {search} filtert die Liste. {back} leert den Filter.
- {enter} zeigt den Vorschlag mit seiner Änderung und den Befehlen.
- {approve} führt zur Freigabe eines offenen Vorschlags.
- {reject} fragt, ob ein offener Vorschlag abgelehnt werden soll. Die Rückfrage kommt zuerst, {yes} lehnt mit sudo ab und {no} verwirft sie. Auf dem Host ändert sich nichts.
- {reload} liest die Vorschläge neu.

Ein Vorschlag freier Befehle enthält Befehle, die nichts prüft. Er lässt sich nur ausführen, solange Freie Vorschläge unter Einstellungen, Allgemein eingeschaltet ist.

Hat sich der Host oder ein Vorschlag geändert, seit die Liste gelesen wurde, bittet die Seite um {reload}.
