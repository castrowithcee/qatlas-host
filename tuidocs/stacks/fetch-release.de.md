---
title: Ordner abrufen oder lösen
keywords: [abrufen, lösen, entfernen, Stack-Ordner, Poll]
---
# Ordner abrufen oder lösen

Zwei Fragen zum gewählten Stack-Ordner teilen sich diese Seite.

Abrufen aktualisiert den Remote-Zweig vom Remote. Die Arbeitskopie bleibt unverändert. Das Ergebnis sagt, ob der Ordner aktuell ist oder das Remote neuere Commits hat. Ein lokaler Ordner hat kein Remote, {fetch} sagt dir dann nur das.

Lösen hält qhost davon ab, aus dem Ordner auszurollen. Nichts wird gelöscht und kein Container gestoppt. Die Rechte, der Eintrag im systemd-Drop-in und der Deploy-Schlüssel bleiben.

- {yes} führt es aus. Ohne root fragt es zuerst nach dem sudo-Passwort.
- {no} bricht ab. {enter} nimmt die markierte Antwort, das ist Abbrechen.
