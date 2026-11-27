---
title: Gruppenformulare
keywords: [neue Gruppe, Mitglieder, Gruppe umbenennen]
---
# Gruppenformulare

Jedes Formular führt nach {save} zu einer Rückfrage mit den Befehlen. {back} verwirft das Formular, {next} und {prev} wechseln zwischen den Feldern.

## Neue Gruppe

Gib einen Namen aus Kleinbuchstaben, Ziffern, _ und - ein, höchstens 32. Der Name muss frei sein. Das System gibt der Gruppe eine reguläre Gruppennummer.

## Mitglieder

Hinzufügen listet die Benutzer außerhalb der Gruppe, Entfernen die Mitglieder. {toggle} wählt einen Benutzer, auch mehrere auf einmal. Eine Rückfrage deckt alle Änderungen ab. Wessen primäre Gruppe dies ist, steht in keiner der Listen. Bei sudo gibt das Hinzufügen auch Zugang zu qhost, und das Entfernen nimmt beides. Du kannst dich nicht selbst aus sudo nehmen, und ein Administrator mit Zugang zu qhost muss bleiben.

## Umbenennen

Gib den neuen Namen ein. Konfigurationen wie sudoers sprechen eine Gruppe über ihren Namen an und behalten den alten, sie hören also auf zu wirken, bis auch sie geändert werden. Die Rückfrage ist deshalb rot.
