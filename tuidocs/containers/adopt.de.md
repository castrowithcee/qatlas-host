---
title: Fremdes Projekt übernehmen
keywords: [übernehmen, fremdes Projekt, eigene Quelle, Git-Repository, Umzug, Stack-Ordner]
---
# Fremdes Projekt übernehmen

Das Übernehmen macht ein laufendes Compose-Projekt zu einem Stack-Ordner dieses Hosts. Es geht von den Labels der laufenden Container aus, die das Arbeitsverzeichnis und die Compose- und Env-Dateien nennen. Vor der letzten Frage ändert sich nichts.

Könnte eine Quelle derselben Engine das Projekt aufnehmen, fragt zuerst ein Formular, wie es übernommen wird:

- Eine eigene Quelle macht den Ordner zu einem eigenen Stack-Ordner. Ein Ordner ohne Repository bekommt ein neues eigenes Git-Repository mit einem ersten Commit seiner Dateien, und die nächsten Fragen fragen nach einem Remote, falls du eines willst. Ein Ordner, der schon ein Repository ist, wird unverändert übernommen.
- In eine bestehende Quelle verschoben gibt dem Projekt einen Ordner in einer Quelle, die es schon gibt, siehe die Seite zum Umzug. Ein Ordner mit eigenem Repository kann danach nicht mehr in eine bestehende Quelle umziehen, wenn er eine eigene Quelle geworden ist.

{enter} öffnet die Auswahl, {save} geht weiter, {back} verwirft das Formular. Ohne Auswahl läuft der Ablauf als eigene Quelle weiter.

Die nächsten Seiten sind die Fragen des Hinzufügens eines Stack-Ordners: was im Ordner gefunden wurde, welche Stacks er enthält und wofür jeder taugt, und die Änderungen nach Ja. Sie fragen womöglich nach dem Hostschlüssel und einem Deploy-Schlüssel oder bieten {withremote} und {choose} an. Warnungen nennen, was ein Deploy anders an Compose übergäbe als das, was jetzt läuft: einen anderen Projektnamen, andere Compose-Dateien oder Env-Dateien. Der erste Deploy legte dann neue Container an, statt die laufenden zu übernehmen.

- {yes} nimmt die Änderungen vor, {no} oder {enter} bricht ab, {up} und {down} scrollen.

Ein Projekt, das sich nicht übernehmen lässt, bekommt eine Seite mit dem Grund und ohne Weg weiter. Gründe: Die Labels nennen kein oder ein relatives Arbeitsverzeichnis, eine Datei liegt außerhalb des Ordners, der Ordner enthält nicht genau einen Stack, oder seine Compose-Datei liegt nicht direkt im Ordner. Schließe sie mit {no}, {enter} oder {back}.
