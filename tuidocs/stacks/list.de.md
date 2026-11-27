---
title: Stack-Liste
keywords: [Liste, Ordner, Stack, Abrufen, Lösen, Hinzufügen, Deploy-Schlüssel, Remote]
---
# Stack-Liste

Jeder Stack-Ordner ist eine Zeile mit seiner Art (Klon oder lokal) und seinem letzten Abruf, die Stacks des Ordners stehen darunter. Die Details des markierten Ordners zeigen seine Adresse und seinen letzten Abruf. Ohne Ordner weist der Bereich aufs Hinzufügen hin.

Auf einer Ordnerzeile:

- {new} fügt einen Stack-Ordner hinzu. {enter}, {left} und {right} klappen den Ordner zu und auf.
- {fetch} ruft das Remote jetzt ab. Nur bei einem Klon.
- {edit} öffnet den Deploy-Schlüssel des Ordners. Nur bei einem Klon.
- {remote} fügt einem lokalen Ordner ein Remote hinzu.
- {secure} macht das .git eines Ordners nur für root beschreibbar, wenn die Zeile sagt, dass dort kein Git läuft. Ohne root läuft sudo qhost source secure.
- {delete} löst den Ordner.
- {files} öffnet die Dateien des Ordners. {changes} öffnet seine Änderungen, wo er welche hat.

Auf einer Stack-Zeile:

- {enter} öffnet den Stack. {toggle} markiert ihn für eine Aktion.
- {update}, {deploy} und die weiteren Buchstabentasten der Fußzeile führen eine Aktion auf den markierten Stacks aus, sonst auf dem gewählten.

Auf jeder Zeile:

- {search} filtert die Liste. {reload} liest sie neu.
- {setupdeploy} richtet einen Deploy-Dienst ein, angezeigt wo einer hinzukommen kann.
- {area} wechselt den Bereich.

In der Dateiansicht eines Ordners, geöffnet mit {files}:

- {enter} öffnet einen Ordner oder eine Datei. {search} filtert die Einträge. Ein Filter, der mit einem Stern beginnt, sucht unterhalb des gezeigten Ordners.
- {back} löscht den Filter, geht einen Ordner hoch und führt an der Wurzel zur Liste zurück.
- {new}, {rename}, {copy} und {delete} ändern Dateien, {edit} bearbeitet die markierte. Der Ordner .git und die Einstellungen in .qatlas/qhost werden hier nur gelesen.
- {changes} öffnet die nicht committeten Änderungen, wo es welche gibt. {reload} liest den Ordner neu.
