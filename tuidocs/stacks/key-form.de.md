---
title: Deploy-Schlüssel setzen
keywords: [Deploy-Schlüssel, hinterlegen, erzeugen, ersetzen, privater Schlüssel, Schlüsseldatei]
---
# Deploy-Schlüssel setzen

Das erste Formular fragt, ob du einen vorhandenen Schlüssel hinterlegst oder den Dienst einen neuen erzeugen lässt. Das Erzeugen fragt nach einer Bestätigung. Das Hinterlegen öffnet ein zweites Formular, das den OpenSSH-Privatschlüssel ohne Passphrase entweder eingefügt oder aus einer Schlüsseldatei nimmt, nicht beides. Der Schlüssel darf 16 KiB nicht überschreiten und wird nie angezeigt. Danach zeigt die Ansicht den öffentlichen Teil.

Hat der Ordner schon einen Schlüssel, fragt eine Frage, ob er ersetzt werden soll. Der alte Schlüssel wirkt sofort nicht mehr, und die Git-Plattform weist diesen Host ab, bis der neue öffentliche Schlüssel dort eingetragen ist. Das lässt sich nicht rückgängig machen.

Derselbe Dialog sagt dir auch, dass ein Ordner, den du hinzufügen wolltest, noch keinen Deploy-Schlüssel hat.

In den Formularen:

- {left} und {right} gehen durch die Auswahl, {enter} öffnet die Liste.
- Im Schlüsselfeld beginnt {enter} eine neue Zeile.
- {save} nimmt das Formular. {back} verwirft es, nach einer Frage.
- {next} und {prev} wechseln zwischen den Feldern.

In einer Frage:

- {yes} führt es aus. Ohne root fragt es zuerst nach dem sudo-Passwort.
- {no} bricht ab. {enter} nimmt die markierte Antwort, das ist Abbrechen.
