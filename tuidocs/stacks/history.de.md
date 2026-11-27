---
title: Verlauf eines Stack-Ordners
keywords: [Verlauf, Commits, Autor, Unterschied, git]
---
# Verlauf eines Stack-Ordners

Die Ansicht listet die letzten Commits eines Stack-Ordners, die neuesten zuerst entlang des ersten Elternteils, mit Datum, Autor und Betreff. Autor, Betreff und Pfade sind Text anderer Menschen und werden nur angezeigt. Der Verlauf kommt vom Deploy-Dienst, git wird im Ordner selbst nie aufgerufen. Eine gekürzte Liste sagt das.

- {enter} zeigt die Dateien des markierten Commits, jede als neu, geändert oder gelöscht markiert.
- {showcommit} zeigt den Unterschied des Commits zu seinem ersten Elternteil, aus der Liste oder aus seinen Dateien. Ohne root wird er nach einer Frage über sudo gelesen, weil er Werte enthalten kann, die nur root lesen darf. Nichts ändert sich.
- {search} filtert die Liste. {up} und {down} wählen einen Commit oder eine Datei.
- {back} geht einen Schritt zurück: vom Unterschied zu den Dateien, von den Dateien zur Liste, von der Liste zur Ordnerzeile oder zum Dateibrowser. {reload} liest den Verlauf neu.

Im Unterschied:

- {up}, {down}, {pageup} und {pagedown} blättern. Der Unterschied ist nach 256 KiB gekürzt.
- {search} durchsucht den Unterschied. {log.next} und {log.prev} springen zwischen Treffern.
- {log.wrap} bricht lange Zeilen um. {text.numbers} zeigt Zeilennummern.

In der Frage zum Unterschied liest {yes} ihn, und {no} bricht ab.
