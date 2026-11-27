---
title: Container
keywords: [Container, Docker, Compose, Start, Stopp, Neustart, Engine, fremdes Projekt]
---
# Container

Die Liste zeigt jeden Container des Hosts, gruppiert als eigener Stack, fremdes Projekt oder einzelne Container. Ein fremdes Projekt ist ein Compose-Projekt, das kein Stack dieses Hosts besitzt. Eine Gruppe mit der Markierung Konflikt trägt den Namen eines Stacks, liegt aber in einem anderen Ordner. Jede Zeile zeigt Name, Zustand, Image, Laufzeit und Neustarts. Der Zustand ist ok, unhealthy, restarting, stopped oder starting. Die Liste liest die Container-Engine alle paar Sekunden neu.

- {enter} öffnet die Details des markierten Containers oder klappt die markierte Gruppe ein.
- {left} und {right} klappen Gruppen ein und aus, {search} filtert die Liste.
- {containerlogs} öffnet das Log des markierten Containers.
- {stackstart}, {stackstop} und {stackrestart} starten, stoppen oder starten den markierten Container neu. Eine Frage nennt seinen Zustand und was folgt, Abbrechen ist die Standardantwort. Ohne Root zeigt die Frage den Befehl, der über sudo läuft. Das Ergebnis zeigt den Zustand danach. Ein Container, der nicht rechtzeitig bereit ist, gilt als fehlgeschlagen.
- {new} übernimmt das fremde Projekt der markierten Zeile, siehe die Seite zum Übernehmen eines Projekts.
- {reload} liest die Liste sofort neu, {area} wechselt den Bereich.

Bei mehreren Engines bildet jede Engine einen eigenen Abschnitt. Eine Engine ohne Deploy-Dienst ist nur lesbar: Ihre Container werden nur angezeigt, und keine Taste wirkt auf sie. Ohne Root werden solche Engines gar nicht aufgelistet. Lässt sich die Engine nicht lesen, nennt der Bildschirm den Grund und behält die letzte Liste.
