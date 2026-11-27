---
title: Einen Vorschlag freigeben
keywords: [freigeben, anwenden, bestätigen, sudo, Host-Schlüssel, Engine]
---
# Einen Vorschlag freigeben

Diese Frage will wissen, ob du den Vorschlag freigibst und die darunter gezeigten Änderungen anwendest, etwa die Einstellungen, das Wartungsfenster, den Kanal, die Freigabe oder den Stack-Ordner mit seinen Rechte- und Dienständerungen. Direkt vor dem Anwenden werden der Vorschlag und seine Wirkung neu gelesen. Weicht eines von beiden vom Gezeigten ab, wird nichts angewendet, und die Ansicht zeigt den Vorschlag im jetzigen Stand. Ja markiert den Vorschlag als freigegeben, nimmt die Änderungen vor und hält das Ergebnis am Vorschlag fest. Ohne Root wird der Befehl gezeigt, der über sudo läuft, und das sudo-Passwort wird gefragt.

Die Freigabe eines Stack-Ordners kann zuvor mehrere Schritte haben: eine Frage zum Fortfahren nach der Prüfung, die Engine, wenn der Host mehr als eine hat, und ob du dem Host-Schlüssel des Servers traust. Jeder Schritt nennt, was folgt, und lässt sich abbrechen.

- {yes} bestätigt den Schritt.
- {no} bricht ab und ändert nichts.
- {up} und {down} scrollen die Details.

Hat sich der Stand während deiner Entscheidung geändert, sagt eine Meldung, dass nichts angewendet wurde.
