---
title: Einen Vorschlag lesen
keywords: [Vorschlag, Inhalt, Wirkung, freigeben, ablehnen, Hinweis, Grund]
---
# Einen Vorschlag lesen

Dieser Bildschirm zeigt einen Vorschlag: wer ihn eingereicht hat, wann, wann er abläuft und seinen Zustand. Gibt es einen Hinweis, einen Fehlschlag oder den Grund einer Ablehnung, folgt er danach. Darunter listet Vorgeschlagen den Inhalt vollständig, genau wie eingereicht. Bei einem offenen Vorschlag zeigt Wirkung bei Anwendung jetzt, was die Freigabe am Host ändern würde. Die Wirkung wird nur gelesen, nie angewendet. Lies jeden Wert, besonders einen Backup-Befehl, der später auf dem Host läuft.

Ohne Root wird die Wirkung eines Stack-Ordners erst bei der Freigabe berechnet, und das sudo-Passwort wird dann gefragt. Ist der Inhalt nicht lesbar oder kann diese Version die Art nicht zeigen, sagt der Bildschirm das, und der Vorschlag lässt sich nicht freigeben.

- {approve} gibt einen offenen Vorschlag frei. Vor deiner Bestätigung wird nichts angewendet.
- {reject} lehnt einen offenen Vorschlag ab und fragt nach einem optionalen Grund.
- {edit} setzt den Ablauf einer Dauerfreigabe, bevor du sie freigibst.
- {up} und {down} scrollen.
- {reload} liest den Vorschlag neu.
- {back} führt zurück zur Liste.

Beide Tasten werden mit einem Hinweis abgewiesen, wenn der Vorschlag nicht mehr offen ist oder diese Version ihn nicht freigeben kann.
