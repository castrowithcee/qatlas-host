---
title: Projekt in eine Quelle verschieben
keywords: [Umzug, Vorschau, Quelle, Sperren, Warnungen, Rückweg]
---
# Projekt in eine Quelle verschieben

Das verschiebt ein fremdes Projekt in eine bestehende Quelle, gewählt im Formular zum Übernehmen. Vorschau und Umzug laufen als Befehl über sudo oder direkt als Root, und die letzte Frage zeigt den Befehl. Die Vorschau ändert nichts.

Die Vorschau zeigt den Ordner, die Quelle, den Zielordner, das Projekt und seine Engine. Sie listet die Dateien des ersten Commits, was ausgelassen wird, Sperren und Warnungen. Eine Sperre heißt, dass der Umzug so nicht geht, mit dem, was dagegen zu tun ist, etwa eine Quelle in einem anderen Dateisystem. Bei einer Sperre schließt die Seite nur mit {no}, {enter} oder {back}, und es wurde nichts geändert.

Ohne Sperren fragt die Frage, ob das Projekt verschoben wird. Antwortest du {yes}, wird das Projekt heruntergefahren, sein Ordner in die Quelle umbenannt, committet (und gepusht, wenn die Quelle ein Remote hat) und dort deployt. Daten werden nie kopiert oder gelöscht. Eine Warnung wie Ausfallzeit wird gezeigt, hält den Umzug aber nicht auf. {no} bricht ab, {up} und {down} scrollen.

Das Ergebnis nennt, wo der Ordner jetzt liegt. Scheitert ein Schritt, wird alles zurückgenommen, und das Ergebnis nennt Schritt und Grund. Wird auch der Rückweg nicht fertig, sagt das Ergebnis es, listet seine Schritte und was von Hand zu tun bleibt. Die Containerliste wird in jedem Fall neu gelesen.
