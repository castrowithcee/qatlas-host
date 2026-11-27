---
title: Container- und Dienstlog
keywords: [Log, Ausgabe, Container, Dienst, Stufe, Diagnose, Folgen, Zeitraum, Speichern]
---
# Container- und Dienstlog

Diese Ansicht zeigt Zeilen, die mit der Zeit eintreffen. In Container zeigt sie die Ausgabe eines Containers. In den Stack-Details führt sie die Ausgabe aller Container des Stacks zusammen. In Läufe zeigt sie das Log des Deploy-Dienstes selbst, mit der Stufe jedes Eintrags.

- {up}, {down}, {pageup}, {pagedown}, {first} und {last} blättern.
- {log.follow} folgt der neuesten Zeile.
- {search} durchsucht die Zeilen, {log.next} und {log.prev} springen zwischen Treffern.
- {log.filter} wählt den Zeitraum der Zeilen.
- {log.wrap} bricht lange Zeilen um, {log.time} zeigt die Zeitspalte.
- {savelog} speichert die Zeilen der Ansicht als neue Datei und nennt den Ort.
- {reload} liest neu.
- {back} schließt das Log.

## Deploy-Dienst

- {loglevel} schaltet die niedrigste gezeigte Stufe weiter: alle Stufen, info, warn, error.
- {diagnose} sammelt ein Diagnosepaket und speichert es als Datei. Das Paket enthält Hostnamen und Pfade, prüfe es daher, bevor du es weitergibst. Ein zweiter Druck während des Laufs sagt nur, dass er noch läuft.
