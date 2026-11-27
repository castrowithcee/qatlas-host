---
title: Rechte eines Ordners
keywords: [Rechte, Besitzer, Gruppe, Modus, ACL, ändern]
---
# Rechte eines Ordners

Die Seite zeigt, was für den markierten Eintrag gelesen wurde: den echten Pfad hinter einem Link, die Art, was der Besitzer, die Gruppe und alle anderen dürfen, den Modus, einen Hinweis zu ACLs und zu Sonderbits. Ein Systempfad ist als solcher gekennzeichnet.

- {edit} öffnet das Formular, das einen Ordner ändert. Eine Datei lässt sich hier nicht ändern, ebenso nichts in einem .git-Ordner.
- {reload} liest die Rechte neu.
- {back} kehrt zum Browser zurück.

## Formular

Besitzer und Gruppe werden aus den Konten des Hosts gewählt. Besitzer darf, Gruppe darf und Alle anderen dürfen nehmen je nichts, lesen oder lesen und schreiben. Steht Alles darunter auf Ja, ändert das auch jede Datei und jeden Ordner darin, und die Rückfrage zeigt zuerst ihre Anzahl.

{left} und {right} gehen durch eine Auswahl, {next} und {prev} wechseln zwischen den Feldern, {save} führt zur Rückfrage mit den Befehlen und {back} verwirft das Formular. Ein Systempfad fragt in Rot.
