---
title: Änderung mit sudo
keywords: [sudo, Änderung, Plan, Befehl, Bestätigung, Journal, rückgängig, endgültig]
---
# Änderung mit sudo

Diese Frage kommt vor jeder Änderung am Host, etwa an Benutzern, Gruppen, Dateien und Ordnern in System. Sie hat drei Teile: einen Satz in einfachen Worten, was sich ändert, eine Zeile dazu, was folgt, und darunter die genauen Befehle, jeder hinter einem Dollarzeichen. Die Befehle werden gezeigt, damit du prüfen kannst, was mit root-Rechten läuft. Nichts läuft ohne deine Bestätigung.

Die Zeile dazu, was folgt, sagt, ob die Änderung ins Journal geht und sich rückgängig machen lässt oder nicht. Ein Risiko für root, ein Systemkonto oder eine Systemgruppe macht die Frage rot, ebenso ein Löschen.

- {yes} übergibt das Terminal an sudo, das selbst nach deinem Passwort fragt. Der Befehl prüft dann jede Regel erneut und bleibt die maßgebliche Stelle.
- {no} bricht ab. Alles, was die Änderung gebraucht hätte, etwa ein getipptes Passwort, wird gelöscht.

Mehrere Befehle laufen nacheinander. Sind es mehr, als hineinpassen, sagt die Frage, wie viele noch folgen. Eine lange Frage füllt den Arbeitsbereich statt eines Dialogs: Blättere mit {up}, {down}, {pageup}, {pagedown}, {first} und {last}.

Einen Benutzer zu sperren ist so eine Änderung. Die Änderung eines Benutzers in System > Benutzer bietet Sperren an: Danach ist kein Login mehr möglich, weder mit Passwort noch mit SSH-Schlüssel. Laufende Sitzungen bleiben offen, bis sie enden, und du kannst dich nicht selbst sperren.

Das Ergebnis folgt in einem eigenen Dialog.
