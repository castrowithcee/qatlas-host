---
title: Wartungsfenster
keywords: [Wartungsfenster, Tage, Beginn, Dauer, Zeitzone, Host-Fenster, eigenes Fenster]
---
# Wartungsfenster

Dieser Reiter zeigt für jeden Deploy-Dienst Tage, Beginn, Dauer und Zeitzone des geltenden Fensters, ob es jetzt offen ist, die nächste Öffnung und die letzte Arbeit. Eine Beschriftung nennt die Herkunft des Fensters: das Host-Fenster, dem jeder Dienst ohne eigenes folgt, oder ein eigenes Fenster.

- {edit} öffnet Host-Fenster ändern. {new} öffnet Eigenes Fenster der gewählten Instanz. Beide haben die Felder Tage (daily oder Wochentage wie mon,thu), Beginn (HH:MM), Dauer (1m bis 12h) und Zeitzone (local oder ein IANA-Name). {next} und {prev} wechseln zwischen den Feldern, {save} prüft das Formular, {back} verwirft es. Nur geänderte Werte werden gesendet. Ein Host-Fenster-Formular ohne Änderung wird abgewiesen.
- Speichern fragt nach der Bestätigung. Eine Änderung des Host-Fensters erreicht binnen Sekunden jeden Dienst ohne eigenes Fenster. Ein eigenes Fenster ändert nur seine Instanz, die dem Host-Fenster erst nach dem Zurücksetzen wieder folgt. {yes} bestätigt, {no} bricht ab.
- {delete} fragt, ob die Instanz wieder dem Host-Fenster folgen soll. Das entfernt das eigene Fenster und lässt das Host-Fenster unberührt. Eine Instanz ohne eigenes Fenster bekommt nur den Hinweis darauf.
- {instance} wählt die Instanz für das eigene Fenster, wo mehrere Dienste existieren. {reload} liest die Fenster neu.
