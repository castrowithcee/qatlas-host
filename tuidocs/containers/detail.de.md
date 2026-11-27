---
title: Container-Details
keywords: [Details, Ports, Mounts, Netzwerke, Labels, Compose-Dateien, Env-Dateien]
---
# Container-Details

Die Details zeigen einen Container: seine Gruppe, den Zustand (mit dem Zustand der Engine und seinem Health-Check), Image, Laufzeit und Neustarts. Darunter stehen die Abschnitte Ports, Mounts, Netzwerke, Labels und Umgebungsvariablen, die nur Namen und nie Werte nennen. Ein Container eines fremden Projekts zeigt außerdem seinen Ordner und die Abschnitte Compose-Dateien und Env-Dateien, wie die Labels des Projekts sie nennen. Ein Abschnitt ohne Einträge sagt keine. Der Bildschirm liest alle paar Sekunden neu.

- {up}, {down}, {pageup} und {pagedown} scrollen die Seite.
- {containerlogs} öffnet das Log dieses Containers.
- {stackstart}, {stackstop} und {stackrestart} starten, stoppen oder starten ihn neu, nach einer Frage wie in der Liste. Die Details zeigen danach den neuen Zustand.
- {new} übernimmt das fremde Projekt dieses Containers, siehe die Seite zum Übernehmen eines Projekts.
- {reload} liest die Details sofort neu.
- {back} führt zurück zur Liste.
