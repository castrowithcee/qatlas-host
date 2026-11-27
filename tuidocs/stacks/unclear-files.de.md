---
title: Unklare Dateien wählen
keywords: [unklare Dateien, erster Commit, Secrets, übernehmen, wählen]
---
# Unklare Dateien wählen

Ein Ordner mit Dateien bekommt einen ersten Commit der Dateien, die seine Stacks brauchen. Unklare Dateien, die ein Stack beschreibbar einbindet oder als Secrets-Datei nennt, bleiben draußen, solange du sie nicht wählst. Daten oder ein Secret können darin liegen. Das Formular listet jede mit ihrem Grund.

Das Speichern plant den Ordner mit deiner Wahl neu, und die Fragen zeigen dann die neue Liste. Eine gewählte Datei mit Secret-Inhalt wird vor jedem Push weiter geprüft.

- {toggle} wählt die Datei unter dem Cursor oder hebt die Wahl auf. Gewählte Dateien kommen in den ersten Commit.
- {up} und {down} wechseln zwischen den Dateien.
- {save} übernimmt die Wahl.
- {back} verwirft das Formular, nach einer Frage.
