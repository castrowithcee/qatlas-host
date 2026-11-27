---
title: Prüfung eines Stacks
keywords: [Prüfung, Gesundheit, HTTP, Container, Zeitlimit, Images vergleichen]
---
# Prüfung eines Stacks

Dieses Formular legt fest, wann der Stack nach einem Deploy als gesund gilt. Das Speichern ändert nur die Prüfung und lässt Update-Regeln, Backup und Compose-Dateien, wie sie sind. {save} prüft die Werte und öffnet eine Frage. Ihr {yes} erzeugt im Stack-Ordner einen Commit mit den neuen Einstellungen und pusht ihn, wenn der Ordner einen Remote hat. Die Prüfung wirkt ab dem nächsten Deploy, der Vergleich ab der nächsten Übernahme. {no} oder {enter} bricht ab.

- {next} und {prev} wechseln zwischen den Feldern. {enter} wählt in einem Auswahlfeld an oder aus.
- Container-Prüfung: An heißt, der Stack gilt nur als gesund, solange alle seine Container laufen.
- HTTP-Prüfung: An heißt, die Adresse darunter muss wie erwartet antworten. Aus lässt die drei Felder darunter unbeachtet.
- Adresse (URL): eine absolute http- oder https-URL.
- Erwarteter Status: der HTTP-Status, mit dem die Adresse antworten muss, 100 bis 599. Standard 200.
- Zeitlimit: wie lange auf die Antwort gewartet wird, von 1s bis 10m, zum Beispiel 10s.
- Images bei Übernahme vergleichen: An heißt, eine Übernahme von Versionen in diesen Stack wird verweigert, wenn seine Images nicht die der Quelle sind, nach Fingerabdruck. Standard aus.
- {back} verwirft das Formular und fragt zuerst, wenn sich etwas geändert hat.
