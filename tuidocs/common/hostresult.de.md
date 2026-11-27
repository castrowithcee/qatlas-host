---
title: Ergebnis eines sudo-Laufs
keywords: [sudo, Ergebnis, Journal, rückgängig, abgelehnt, zurückgestellt, fehlgeschlagen, Prüfung]
---
# Ergebnis eines sudo-Laufs

Dieser Dialog meldet, was ein mit sudo ausgeführter Befehl getan hat. Seine erste Zeile sagt eins von vier Dingen:

- Erledigt: Die Änderung wurde gemacht. Ein Hinweis nennt den Journaleintrag, und System > Änderungen nimmt ihn zurück. Eine Änderung, die sich nicht rückgängig machen lässt, sagt das stattdessen.
- Nicht geändert: Eine Regel des Hosts hat sie abgelehnt, bevor etwas geschah.
- Fehlgeschlagen, vorheriger Zustand zurückgestellt: Die Änderung oder ihre Prüfung ist fehlgeschlagen, und der frühere Zustand ist wieder da.
- Fehlgeschlagen: Die Änderung ist nicht gelungen. Prüfe, was lief, bevor du es erneut versuchst.

Weitere Zeilen können folgen: der Grund, ob die Prüfung bestanden wurde, eine Audit-Warnung oder der Hinweis, dass dieser Sitzung die Gruppe qhost noch fehlt und die Oberfläche neu gestartet werden muss.

Gab der Lauf keine brauchbare Antwort, erklärt der Dialog den Grund in einfachen Worten: sudo fehlt, die Passwortabfrage wurde abgebrochen oder abgelehnt, der Befehl endete mit einem Exit-Code, oder seine Antwort ließ sich nicht lesen. Eine abgebrochene Abfrage heißt, dass nichts geändert wurde.

{no} und {enter} schließen den Dialog. Die Listen dahinter werden neu gelesen.
