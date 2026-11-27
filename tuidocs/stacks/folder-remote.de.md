---
title: Remote hinzufügen
keywords: [Remote, Push, Vergleich, lokaler Ordner, Historie, Secrets]
---
# Remote hinzufügen

Ein lokaler Stack-Ordner kann gegen ein Remote laufen. Das Formular fragt nach der Adresse des Repositorys. Der Hostschlüssel und der Deploy-Schlüssel des Ordners werden wie beim Hinzufügen eines Klons geprüft, und danach wird der Ordner mit dem Repository verglichen, bevor sich etwas ändert. Der Vergleich listet den Branch, die abweichenden Pfade und den öffentlichen Deploy-Schlüssel, der Schreibrecht auf das Repository braucht.

Die Frage hängt davon ab, was das Repository enthält:

- Leer: Ja pusht die Historie des Ordners dorthin, danach läuft der Ordner gegen das Remote.
- Dieselben Dateien: Ja lässt den Ordner ihm folgen. Die lokale Historie bleibt erhalten, gepusht wird nichts.
- Andere Dateien: setzen und pushen fügt einen Commit mit den Dateien des Hosts auf die Historie des Remotes und pusht ihn. Die Historie des Remotes wird nie überschrieben.
- Geheimnisartige Dateien in den zu pushenden Commits: die Frage sagt es nur. Nichts wird gepusht, und du entfernst die Dateien selbst aus der Historie.
- Ein Ordner mit Dateien, dessen Repository keinen Commit hat, kann nicht beitreten: Übernimm den Ordner zuerst ohne Adresse.

Hat der Ordner keinen Deploy-Schlüssel, öffnet sich stattdessen die Schlüsselansicht, und nichts ändert sich.

Tasten:

- {save} nimmt im Formular die Adresse. {back} verwirft es, nach einer Frage.
- {yes} beantwortet die Frage. {no} bricht ab, und {enter} nimmt die markierte Antwort, das ist Abbrechen.
- {up} und {down} blättern in der Frage.
