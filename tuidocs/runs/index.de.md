---
title: Läufe
keywords: [Läufe, Verlauf, Update, Rollback, Filter, Log]
---
# Läufe

Die Liste zeigt die Läufe des Deploy-Dienstes, die neuesten zuerst: den Stack, wann der Lauf begann, sein Ergebnis, was ihn ausgelöst hat, wie lange er dauerte, seine Lauf-ID und das Tag. Ein Lauf, der noch läuft, steht als läuft in der Liste.

- {enter} öffnet das Log des Laufs unter dem Cursor.
- {filter} beschränkt die Liste auf den unter Stacks markierten Stack. Noch einmal gedrückt zeigt sie wieder alle Stacks. Ohne markierten Stack erscheint nur eine Warnung.
- {rollback} setzt den Stack auf den Stand vor dem Lauf zurück. Das geht nur bei einem erfolgreichen Update- oder Promote-Lauf und öffnet eine Frage zu den Daten des Stacks. Bei jedem anderen Lauf erscheint nur eine Warnung.
- {servicelog} öffnet das Log des Deploy-Dienstes.
- {search} filtert die Liste nach deiner Eingabe. {enter} behält den Filter, {back} hebt ihn auf.
- {up}, {down}, {pageup}, {pagedown}, {first} und {last} bewegen den Cursor.
- {reload} liest die Läufe neu.
- {area} wechselt in einen anderen Bereich.
