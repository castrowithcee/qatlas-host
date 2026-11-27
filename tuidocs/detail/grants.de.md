---
title: Freigaben eines Stacks
keywords: [Freigabe, entziehen, sudo, Benutzer, Ablauf, Aktion]
---
# Freigaben eines Stacks

Eine Freigabe erlaubt einem Benutzer und jedem Agenten, der unter seinem Konto arbeitet, eine Aktion auf diesem Stack ohne sudo. Nur Benutzer mit Zugang zu qhost können eine Freigabe haben. Ohne einen solchen Benutzer sagt eine Meldung das.

Eine Aktion freigeben:

- {grantadd} öffnet ein Formular. {next} und {prev} wechseln zwischen den Feldern.
- Benutzer: ein Benutzer mit Zugang zu qhost.
- Aktion: Stack neu starten, Container neu starten, Updates prüfen oder Stack aktualisieren.
- Letzter Tag: JJJJ-MM-TT, gültig bis zum Ende dieses Tages (UTC). Leer heißt ohne Ablauf. Dieselbe Aktion erneut freizugeben ersetzt den Ablauf.
- {save} prüft den Tag und öffnet eine Frage, die den sudo-Befehl zeigt, ob der Stack Backup und Prüfung eingerichtet hat und bei einem Update, dass er in die Quelle committet und pusht. {yes} gibt frei, {no} oder {enter} bricht ab.
- {back} verwirft das Formular und fragt zuerst, wenn sich etwas geändert hat.

Eine Freigabe entziehen:

- {grantremove} öffnet die Liste der Freigaben, und die Wahl einer Freigabe öffnet ihre Frage.
- {yes} entzieht sie. Der Benutzer und jeder Agent brauchen dafür wieder sudo. {no} oder {enter} bricht ab.

Das Feld Freigaben markiert eine Freigabe als gültig oder nennt den Grund, warum nicht: abgelaufen, der Stack fehlt oder das Konto hat sich geändert.
