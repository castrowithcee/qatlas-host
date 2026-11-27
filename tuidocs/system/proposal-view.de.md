---
title: Vorschlag einer Änderung
keywords: [Vorschlag, freigeben, Startpasswort, freie Befehle, ausführen, Ergebnis]
---
# Vorschlag einer Änderung

## Vorschlag

Das Blatt zeigt, wer ihn vorgeschlagen hat und in welchem Zustand er ist, die Änderung als Satz, eine rote Warnung, wo sie root, ein Systemkonto, das Docker-Recht oder einen Systempfad berührt, und die genauen Befehle, wie von Hand eingegeben. {up} und {down} blättern.

- {yes} gibt einen offenen Vorschlag frei, der die Prüfung besteht. Das führt nur zur Rückfrage mit den Befehlen, und vorher wird nichts angewendet.
- {no} schließt das Blatt. Das tun auch {back} und {enter}.

## Startpasswort

Ein Vorschlag für einen neuen Benutzer enthält nie das Passwort. Das Formular fragt es zweimal ab, bei der Eingabe verborgen. {save} führt zur Rückfrage, {back} verwirft das Formular.

## Freie Befehle

Bei einem Vorschlag freier Befehle zeigt das Blatt den Grund und die Befehle wörtlich, und {yes} heißt ausführen. Die Rückfrage nennt die Anzahl der Befehle. {yes} führt sie als root mit sudo nacheinander aus. Der Lauf stoppt beim ersten Befehl, der scheitert, und nichts lässt sich rückgängig machen. {no} verwirft ihn.

## Ergebnis der Befehle

Das Blatt listet jeden Befehl mit Exit-Code und Ausgabe und kennzeichnet die, die nicht liefen. {no}, {back} und {enter} schließen es. {up} und {down} blättern.
