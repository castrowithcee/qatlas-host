---
title: Änderungen
keywords: [Änderungen, Journal, rückgängig, Verlauf]
---
# Änderungen

Die Liste zeigt die letzten Änderungen am Host aus seinem Journal, die neueste zuerst: Zeit, von wem, die Änderung als Satz, das Ergebnis und ob ein Rückgängig möglich ist. Das Journal kann nur root lesen, deshalb bittet die Seite zuerst um {reload}, das es nach einer Rückfrage über sudo liest. Unter der Liste zeigt der markierte Eintrag sein Rückgängig als Satz und mit den genauen Befehlen.

- {up} und {down} markieren einen Eintrag, {search} filtert die Liste.
- {enter} fragt, ob die markierte Änderung rückgängig gemacht werden soll. Die Rückfrage zeigt die Befehle, {yes} führt sie mit sudo aus und {no} verwirft das Rückgängigmachen.
- {reload} liest das Journal neu.

Ein Rückgängigmachen steht selbst im Journal und lässt sich nicht rückgängig machen. Bei einem Eintrag ohne Rückgängig erklärt {enter} den Grund: Er wurde schon rückgängig gemacht, er hat nicht gewirkt, oder er ist endgültig, etwa ein gelöschtes Home. Nach einer Änderung aus diesem Bereich weist die Seite darauf hin, dass das Journal älter ist als der Host.
