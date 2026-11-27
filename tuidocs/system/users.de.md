---
title: Benutzer
keywords: [Benutzer, Konten, Admin, Docker, Login, SSH]
---
# Benutzer

Die Liste zeigt die lokalen Konten des Hosts mit UID, ob das Konto ein Administrator mit Zugang zu qhost ist, Docker-Zugang, Login-Zustand und weiteren Gruppen. Reguläre Benutzer stehen vorn, Systemkonten folgen in einer eigenen Gruppe. Unter der Liste zeigt das markierte Konto Home, primäre Gruppe, Zugang, Login-Zustand und SSH-Schlüssel. Login-Zustand und SSH-Schlüssel zeigt nur root.

- {up} und {down} markieren ein Konto, {search} filtert die Liste nach dem Namen.
- {new} öffnet das Formular für einen neuen Benutzer.
- {enter} oder {edit} öffnet die Auswahl einer Änderung für den markierten Benutzer.
- {delete} öffnet das Formular, das den markierten Benutzer löscht.
- {reload} liest die Konten neu.

Jede Änderung endet in einer Rückfrage mit den genauen Befehlen. Solange eine Änderung läuft, warten weitere.
