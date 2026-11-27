---
title: Nicht committete Änderungen
keywords: [Änderungen, Unterschied, nicht committet, committen, verwerfen, geändert, neu, gelöscht]
---
# Nicht committete Änderungen

Die Ansicht listet die Dateien eines Stack-Ordners, die vom letzten Commit abweichen, jede als geändert, neu oder gelöscht markiert, mit ihrem Stack. Ein Commit nimmt alle Dateien der Liste. secrets.env, Datenordner und neue Dateien unter einem Bind-Mount gehören dem Host und sind nie Teil davon. Updates lassen einen Ordner aus, bis seine Änderungen committet oder verworfen sind.

- {enter} zeigt den Unterschied der markierten Datei. Ohne root wird er nach einer Frage über sudo gelesen, weil er Werte enthalten kann, die nur root lesen darf. Nichts ändert sich.
- {delete} verwirft die Änderung der markierten Datei. Zuvor kommt eine Frage.
- {commit} committet alle Dateien der Liste, nach einer Frage. Eine gekürzte Liste lässt sich hier nicht committen.
- {search} filtert die Liste. {up} und {down} wählen eine Datei.
- {back} verlässt die Ansicht. {reload} liest die Liste neu.

Im Unterschied:

- {up}, {down}, {pageup} und {pagedown} blättern. Der Unterschied ist nach 256 KiB gekürzt.
- {search} durchsucht den Unterschied. {log.next} und {log.prev} springen zwischen Treffern.
- {log.wrap} bricht lange Zeilen um. {text.numbers} zeigt Zeilennummern.
- {back} führt zur Liste zurück.

In der Frage zum Unterschied liest {yes} ihn, und {no} bricht ab.
